// provider.go: InstallerProvider — the per-OS strategy object. Each
// provider owns everything OS-specific: answer media, boot overrides,
// and what to undo once the installed system exists. Quicktest the
// orchestrator never embeds per-OS logic.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
	"github.com/simia-tech/crypt"
)

// provider adapts a just-downloaded VM (conf + image) so one launch
// produces an installed, reachable system.
type provider interface {
	id() string
	// prepare writes seed media and rewrites the conf for the
	// automated boot (kernel overrides, seed, port forwards, headless
	// display, serial+monitor sockets for observability).
	prepare(r *runCtx) error
	// dropConfKeys are the conf keys to remove before the installed
	// system's first boot — whatever would re-trigger the installer.
	dropConfKeys() []string
	// postInstallFiles are vmDir files to delete at reconfigure —
	// media the launcher auto-attaches on every boot (unattended.iso)
	// that must not outlive the install.
	postInstallFiles() []string
	// needsSecondBoot reports whether the run must stop the install
	// boot and relaunch (ISO installers) or boots straight into the
	// final system (pre-imaged disks).
	needsSecondBoot() bool
	// completionMode says how await-install learns the installer is
	// done: "poweroff" waits for the qemu exit the answer file
	// arranges; "serial:<regex>" watches the captured serial log for
	// a finished marker and the run kills the guest itself.
	completionMode() string
	// interactive reports whether the run ends at launch — the user
	// completes a graphical installer by hand (macOS and friends).
	interactive() bool
	// sshUser is the login the seed arranges.
	sshUser(p *Profile) string
}

func pickProvider(p *Profile) (provider, error) {
	switch p.Automation.Provider {
	case "ubuntu-autoinstall":
		return &ubuntuAutoinstall{}, nil
	case "ubuntu-desktop-autoinstall":
		return &ubuntuDesktopAutoinstall{}, nil
	case "nocloud-image":
		return &noCloudImage{}, nil
	case "debian-preseed":
		return &debianPreseed{}, nil
	case "windows-autounattend":
		return &windowsAutounattend{}, nil
	case "anaconda-kickstart":
		return &anacondaKickstart{}, nil
	case "opensuse-autoyast":
		return &opensuseAutoyast{}, nil
	case "alpine-unattended":
		return &alpineUnattended{}, nil
	case "arch-pacstrap":
		return &archPacstrap{}, nil
	case "nixos-serial":
		return &nixosSerial{}, nil
	case "proxmox-autoinstall":
		return &proxmoxAutoInstall{}, nil
	case "openbsd-autoinstall":
		return &openbsdAutoinstall{}, nil
	case "freebsd-shell":
		return &freebsdShell{}, nil
	case "void-xbps":
		return &voidXbps{}, nil
	case "gentoo-stage3":
		return &gentooStage3{}, nil
	case "flatcar-ignition":
		return &flatcarIgnition{}, nil
	case "coreos-ignition":
		return &coreosIgnition{}, nil
	case "ubiquity-preseed":
		return &ubiquityPreseed{}, nil
	case "slackware-live":
		return &slackwareLive{}, nil
	case "artix-basestrap":
		return &artixBasestrap{}, nil
	case "calculate-clinstall":
		return &calculateClinstall{}, nil
	case "guix-serial":
		return &guixSerial{}, nil
	case "netbsd-sets":
		return &netbsdSets{}, nil
	case "visual":
		return &visualProvider{}, nil
	case "manual":
		return nil, fmt.Errorf(
			"%s %s has no automation provider — catalog image_type gives none; "+
				"set automation.provider to install manually-assisted", p.System.OS, p.System.Release)
	default:
		return nil, fmt.Errorf("unknown provider %q", p.Automation.Provider)
	}
}

// extractCandidates tries each {dir, file, file...} ISO layout until
// one extracts — boot-file names differ across distros (flatcar's
// kernel is vmlinuz-a, gentoo's is gentoo.igz, void ships an
// initramfs…). First entry to succeed wins.
func extractCandidates(iso, vmDir string, cands [][]string) (map[string]string, error) {
	var last error
	for _, c := range cands {
		if len(c) < 2 {
			continue
		}
		if got, err := extractBootfiles(iso, c[0], vmDir, c[1:]); err == nil {
			return got, nil
		} else {
			last = err
		}
	}
	return nil, last
}

// yamlQ quotes a scalar for inline YAML. user-data is generated text —
// the values come from a validated profile, but quoting defends
// against colons/spaces all the same.
func yamlQ(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// pwHash resolves the profile password to a SHA512-crypt hash —
// empty password yields a locked-but-key-only account hash of a
// random secret so nothing is guessable.
func pwHash(p *Profile) (string, error) {
	pw := p.Identity.Password
	if pw == "" {
		pw = qelib.CryptSalt(24)
	}
	return crypt.Crypt(pw, "$6$"+qelib.CryptSalt(8))
}

// authKeysYAML renders the run key + profile keys as a YAML list.
func authKeysYAML(p *Profile, k *keypair) string {
	keys := append([]string{k.pub}, p.Identity.SSHAuthorizedKeys...)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "      - %s\n", strings.TrimSpace(key))
	}
	return b.String()
}

// baseMetaData is the NoCloud meta-data every provider emits. It is
// JSON rather than YAML: cloud-init accepts both (JSON ⊂ YAML) but
// cirros-init feeds meta-data to json2fstree and rejects anything
// else. The public-keys map is EC2-style metadata that installs the
// run key for the guest's default user (CirrOS: cirros).
func baseMetaData(p *Profile, r *runCtx) string {
	md, _ := json.Marshal(map[string]any{
		"instance-id":    r.runID,
		"local-hostname": p.Identity.Hostname,
		"public-keys":    map[string]string{"quickprovision": strings.TrimSpace(r.kp.pub)},
	})
	return string(md) + "\n"
}

// commonConfEdits: the headless/observable launch every provider
// wants — no display, HMP+QMP+serial sockets for automation, the
// profile's resources and the SSH forward.
func commonConfEdits(r *runCtx) map[string]string {
	m := map[string]string{
		"display":       "none",
		"monitor":       "socket",
		"serial":        "socket",
		"ram":           r.p.Machine.RAM,
		"cpu_cores":     fmt.Sprint(r.p.Machine.CPUs),
		"port_forwards": fmt.Sprintf("%d:22", r.sshPort),
		"seed":          filepath.Join(r.vmDir, "seed.iso"),
	}
	if r.p.Machine.Boot != "" {
		m["boot"] = r.p.Machine.Boot
	}
	return m
}

// --- ubuntu-server autoinstall ------------------------------------------

// ubuntuAutoinstall drives the subiquity installer: casper kernel +
// initrd extracted from the ISO, an autoinstall answer file on the
// cidata seed, and `shutdown: poweroff` so the end of the install is
// an observable qemu exit rather than a serial-regex guess.
type ubuntuAutoinstall struct{}

func (u *ubuntuAutoinstall) id() string                 { return "ubuntu-autoinstall" }
func (u *ubuntuAutoinstall) needsSecondBoot() bool      { return true }
func (u *ubuntuAutoinstall) interactive() bool          { return false }
func (u *ubuntuAutoinstall) completionMode() string     { return "poweroff" }
func (u *ubuntuAutoinstall) postInstallFiles() []string { return nil }

func (u *ubuntuAutoinstall) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso", "seed"}
}

func (u *ubuntuAutoinstall) sshUser(p *Profile) string { return p.Identity.Username }

func (u *ubuntuAutoinstall) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a server ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractBootfiles(iso, "casper", r.vmDir, []string{"vmlinuz", "initrd"})
	if err != nil {
		return err
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	p := r.p
	user := p.Identity.Username
	var sb strings.Builder
	sb.WriteString("#cloud-config\nautoinstall:\n  version: 1\n  identity:\n")
	fmt.Fprintf(&sb, "    hostname: %s\n    username: %s\n    password: %s\n",
		p.Identity.Hostname, user, yamlQ(hash))
	if p.Locale.Lang != "" {
		fmt.Fprintf(&sb, "  locale: %s\n", p.Locale.Lang)
	}
	if p.Locale.Keyboard != "" {
		fmt.Fprintf(&sb, "  keyboard:\n    layout: %s\n", p.Locale.Keyboard)
	}
	if p.Locale.Timezone != "" {
		fmt.Fprintf(&sb, "  timezone: %s\n", p.Locale.Timezone)
	}
	sb.WriteString("  ssh:\n    install-server: true\n")
	fmt.Fprintf(&sb, "    allow-pw: %v\n", p.Identity.AllowPwAuth || p.Identity.Password != "")
	sb.WriteString("    authorized-keys:\n")
	sb.WriteString(authKeysYAML(p, r.kp))
	sb.WriteString("  storage:\n    layout:\n      name: direct\n")
	sb.WriteString("  late-commands:\n")
	fmt.Fprintf(&sb, "    - 'echo \"%s ALL=(ALL) NOPASSWD:ALL\" > /target/etc/sudoers.d/%s'\n", user, user)
	fmt.Fprintf(&sb, "    - 'chmod 440 /target/etc/sudoers.d/%s'\n", user)
	sb.WriteString("  shutdown: poweroff\n")
	if _, err := buildSeed(r.vmDir, sb.String(), baseMetaData(p, r), ""); err != nil {
		return err
	}
	edits := commonConfEdits(r)
	edits["kernel"] = got["vmlinuz"]
	edits["initrd"] = got["initrd"]
	edits["boot_args"] = "autoinstall ds=nocloud;s=/cdrom/ console=ttyS0"
	return confSet(r.confPath, edits)
}

// --- generic NoCloud disk image ------------------------------------------

// noCloudImage covers pre-imaged systems (CirrOS today, cloud images
// when the catalog gains them): nothing to install — the seed carries
// identity + SSH keys and the first boot IS the final system.
type noCloudImage struct{}

func (n *noCloudImage) id() string                 { return "nocloud-image" }
func (n *noCloudImage) needsSecondBoot() bool      { return false }
func (n *noCloudImage) interactive() bool          { return false }
func (n *noCloudImage) completionMode() string     { return "poweroff" }
func (n *noCloudImage) postInstallFiles() []string { return nil }
func (n *noCloudImage) dropConfKeys() []string {
	return []string{"seed"}
}

func (n *noCloudImage) sshUser(p *Profile) string {
	// Guests with a fixed built-in user must override via profile;
	// cirros ignores user-data users entirely.
	return p.Identity.Username
}

func (n *noCloudImage) prepare(r *runCtx) error {
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	p := r.p
	user := p.Identity.Username
	var sb strings.Builder
	sb.WriteString("#cloud-config\n")
	fmt.Fprintf(&sb, "hostname: %s\nfqdn: %s\npreserve_hostname: false\n",
		p.Identity.Hostname, p.Identity.Hostname)
	fmt.Fprintf(&sb, "ssh_pwauth: %v\n", p.Identity.AllowPwAuth)
	if p.Locale.Timezone != "" {
		fmt.Fprintf(&sb, "timezone: %s\n", p.Locale.Timezone)
	}
	if p.Locale.Lang != "" {
		fmt.Fprintf(&sb, "locale: %s\n", p.Locale.Lang)
	}
	sb.WriteString("users:\n")
	fmt.Fprintf(&sb, "  - name: %s\n    groups: [sudo]\n    shell: /bin/sh\n", user)
	sb.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n    lock_passwd: false\n")
	fmt.Fprintf(&sb, "    passwd: %s\n", yamlQ(hash))
	sb.WriteString("    ssh_authorized_keys:\n")
	for _, k := range append([]string{r.kp.pub}, p.Identity.SSHAuthorizedKeys...) {
		fmt.Fprintf(&sb, "      - %s\n", strings.TrimSpace(k))
	}
	// sudo is not guaranteed on the base image (Alpine ships doas);
	// every provision/verify step goes through it, so ask cloud-init
	// to install it during first boot on whichever PM the guest has.
	sb.WriteString("packages: [sudo]\n")
	if _, err := buildSeed(r.vmDir, sb.String(), baseMetaData(p, r), ""); err != nil {
		return err
	}
	return confSet(r.confPath, commonConfEdits(r))
}

// --- ubuntu desktop (ubuntu-desktop-bootstrap) ----------------------

// ubuntuDesktopAutoinstall reuses the subiquity machinery — same
// casper kernel/initrd extraction and cidata seed — but the desktop
// installer's schema is narrower (no late-commands, no shutdown key)
// and it reboots instead of powering off. Completion is therefore a
// serial marker: curtin's "Installation finished" reaches ttyS0 via
// console=ttyS0 + journald forwarding, then the run kills the guest
// itself (the conf still points at the installer kernel — a guest
// reboot would re-enter the installer and wipe again).
type ubuntuDesktopAutoinstall struct{}

func (u *ubuntuDesktopAutoinstall) id() string            { return "ubuntu-desktop-autoinstall" }
func (u *ubuntuDesktopAutoinstall) needsSecondBoot() bool { return true }
func (u *ubuntuDesktopAutoinstall) interactive() bool     { return false }
func (u *ubuntuDesktopAutoinstall) completionMode() string {
	return `serial:Installation (finished|complete)|curtin.*[Ff]inish|finish:.*curtin`
}
func (u *ubuntuDesktopAutoinstall) postInstallFiles() []string { return nil }

func (u *ubuntuDesktopAutoinstall) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso", "seed"}
}

func (u *ubuntuDesktopAutoinstall) sshUser(p *Profile) string { return p.Identity.Username }

func (u *ubuntuDesktopAutoinstall) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a desktop ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractBootfiles(iso, "casper", r.vmDir, []string{"vmlinuz", "initrd"})
	if err != nil {
		return err
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	ud := renderDesktopAutoinstall(r.p, hash, authKeysYAML(r.p, r.kp))
	if _, err := buildSeed(r.vmDir, ud, baseMetaData(r.p, r), ""); err != nil {
		return err
	}
	edits := commonConfEdits(r)
	edits["kernel"] = got["vmlinuz"]
	edits["initrd"] = got["initrd"]
	// journald forwarding guarantees the install-finished line lands
	// on ttyS0 even when the desktop installer skips kernel logging.
	edits["boot_args"] = "autoinstall ds=nocloud;s=/cdrom/ console=ttyS0 " +
		"systemd.journald.forward_to_console=1"
	return confSet(r.confPath, edits)
}

// --- Windows / Windows Server autounattend --------------------------

// windowsAutounattend: the unattended.iso the launcher auto-attaches
// (cdrom index=2) plus the virtio-win driver CD. The answer file ends
// with `shutdown /s` after sshd is installed, so the qemu exit is the
// install-complete signal and the second boot serves OpenSSH.
type windowsAutounattend struct{}

func (w *windowsAutounattend) id() string                 { return "windows-autounattend" }
func (w *windowsAutounattend) needsSecondBoot() bool      { return true }
func (w *windowsAutounattend) interactive() bool          { return false }
func (w *windowsAutounattend) completionMode() string     { return "poweroff" }
func (w *windowsAutounattend) postInstallFiles() []string { return []string{"unattended.iso"} }
func (w *windowsAutounattend) dropConfKeys() []string {
	return []string{"iso", "fixed_iso"}
}
func (w *windowsAutounattend) sshUser(p *Profile) string { return p.Identity.Username }

func (w *windowsAutounattend) prepare(r *runCtx) error {
	if confGet(r.confPath, "iso") == "" {
		return fmt.Errorf("conf %s has no iso= — is this a Windows ISO VM?", r.confPath)
	}
	virtio := filepath.Join(r.vmDir, "virtio-win.iso")
	if _, err := os.Stat(virtio); err != nil {
		r.e.progress("prepare", "downloading virtio-win.iso")
		if err := fetchFile(
			"https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso",
			virtio); err != nil {
			return fmt.Errorf("virtio-win.iso: %w", err)
		}
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	xml := renderAutounattend(r.p, strings.Join(keys, "\n"))
	ans := filepath.Join(r.artDir, "autounattend.xml")
	if err := os.WriteFile(ans, []byte(xml), 0o644); err != nil {
		return err
	}
	// The launcher auto-attaches any unattended.iso in the VM dir as
	// cdrom index=2 — no conf key needed, and postInstallFiles drops
	// it after install.
	iso := filepath.Join(r.vmDir, "unattended.iso")
	if err := writeFilesISO(iso, "unattended",
		map[string]string{"/autounattend.xml": ans}); err != nil {
		return err
	}
	edits := commonConfEdits(r)
	edits["fixed_iso"] = virtio
	return confSet(r.confPath, edits)
}

// --- debian-installer preseed (debian, kali) -------------------------

// debianPreseed drives d-i: kernel/initrd pulled off the netinst ISO,
// a preseed.cfg served to the guest over slirp by an embedded HTTP
// server bound to loopback (guests reach it at 10.0.2.2), and
// debian-installer/exit/poweroff for the completion signal.
type debianPreseed struct {
	bootDirs []string // candidate ISO dirs for d-i kernel+initrd
}

func (d *debianPreseed) id() string                 { return "debian-preseed" }
func (d *debianPreseed) needsSecondBoot() bool      { return true }
func (d *debianPreseed) interactive() bool          { return false }
func (d *debianPreseed) completionMode() string     { return "poweroff" }
func (d *debianPreseed) postInstallFiles() []string { return nil }
func (d *debianPreseed) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (d *debianPreseed) sshUser(p *Profile) string { return p.Identity.Username }

func (d *debianPreseed) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a netinst ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	dirs := d.bootDirs
	if len(dirs) == 0 {
		dirs = []string{"install.amd", "install", ".install", "install/gtk"}
	}
	var got map[string]string
	var err error
	for _, dir := range dirs {
		got, err = extractBootfiles(iso, dir, r.vmDir,
			[]string{"vmlinuz", "initrd.gz"})
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("d-i boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	cfg := renderPreseed(r.p, hash, keys)
	if err := os.WriteFile(filepath.Join(r.artDir, "preseed.cfg"),
		[]byte(cfg), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.e.progress("prepare",
		fmt.Sprintf("preseed served at http://10.0.2.2:%d/preseed.cfg", port))
	edits := commonConfEdits(r)
	delete(edits, "seed") // preseed travels over HTTP, not a cidata ISO
	edits["kernel"] = got["vmlinuz"]
	edits["initrd"] = got["initrd.gz"]
	edits["boot_args"] = fmt.Sprintf(
		"auto=true priority=critical preseed/url=http://10.0.2.2:%d/preseed.cfg "+
			"interface=auto console=ttyS0 net.ifnames=0", port)
	return confSet(r.confPath, edits)
}

// --- Anaconda kickstart (RHEL family: fedora Server, alma, rocky,
// centos-stream, oraclelinux, tencentos) -------------------------------

// anacondaKickstart drives the Red-Hat installer: the pxeboot kernel
// + initrd pulled off the DVD ISO, a kickstart file served over slirp
// by the same embedded HTTP server the preseed path uses, and the
// kickstart `poweroff` directive as the completion signal. Works for
// any anaconda distro — the renderer is distro-agnostic (@core +
// openssh-server exist everywhere).
type anacondaKickstart struct{}

func (a *anacondaKickstart) id() string                 { return "anaconda-kickstart" }
func (a *anacondaKickstart) needsSecondBoot() bool      { return true }
func (a *anacondaKickstart) interactive() bool          { return false }
func (a *anacondaKickstart) completionMode() string     { return "poweroff" }
func (a *anacondaKickstart) postInstallFiles() []string { return nil }
func (a *anacondaKickstart) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (a *anacondaKickstart) sshUser(p *Profile) string { return p.Identity.Username }

func (a *anacondaKickstart) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a DVD/netinst ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractBootfiles(iso, "images/pxeboot", r.vmDir,
		[]string{"vmlinuz", "initrd.img"})
	if err != nil {
		return fmt.Errorf("anaconda boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	ks := renderKickstart(r.p, hash, keys)
	if err := os.WriteFile(filepath.Join(r.artDir, "ks.cfg"),
		[]byte(ks), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.e.progress("prepare",
		fmt.Sprintf("kickstart served at http://10.0.2.2:%d/ks.cfg", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = got["vmlinuz"]
	edits["initrd"] = got["initrd.img"]
	// inst.stage2=cdrom: the DVD carries LiveOS/squashfs so anaconda
	// finds it on the install media itself; `cdrom` in the ks file
	// then makes the same media the package source.
	edits["boot_args"] = fmt.Sprintf(
		"inst.ks=http://10.0.2.2:%d/ks.cfg inst.stage2=cdrom console=ttyS0", port)
	return confSet(r.confPath, edits)
}

// --- openSUSE AutoYaST (leap/tumbleweed/slowroll) --------------------
//
// opensuseAutoyast drives YaST's unattended mode: loader kernel +
// initrd pulled off the DVD ISO, an autoinst.xml served over slirp by
// the same embedded HTTP server the preseed/kickstart paths use, and
// the profile's <final_halt> flag as the poweroff completion signal.
// install=cdrom tells linuxrc to source packages from the attached
// media (DVD ISOs); net ISOs are not covered here.
type opensuseAutoyast struct{}

func (o *opensuseAutoyast) id() string                 { return "opensuse-autoyast" }
func (o *opensuseAutoyast) needsSecondBoot() bool      { return true }
func (o *opensuseAutoyast) interactive() bool          { return false }
func (o *opensuseAutoyast) completionMode() string     { return "poweroff" }
func (o *opensuseAutoyast) postInstallFiles() []string { return nil }
func (o *opensuseAutoyast) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (o *opensuseAutoyast) sshUser(p *Profile) string { return p.Identity.Username }

func (o *opensuseAutoyast) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this an openSUSE DVD VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractBootfiles(iso, "boot/x86_64/loader", r.vmDir,
		[]string{"linux", "initrd"})
	if err != nil {
		return fmt.Errorf("autoyast boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	xml := renderAutoyast(r.p, hash, keys)
	if err := os.WriteFile(filepath.Join(r.artDir, "autoinst.xml"),
		[]byte(xml), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.e.progress("prepare",
		fmt.Sprintf("autoyast served at http://10.0.2.2:%d/autoinst.xml", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = got["linux"]
	edits["initrd"] = got["initrd"]
	edits["boot_args"] = fmt.Sprintf(
		"autoyast=http://10.0.2.2:%d/autoinst.xml install=cdrom console=ttyS0", port)
	return confSet(r.confPath, edits)
}

// --- Alpine ISO unattended setup-alpine --------------------------------
//
// alpineUnattended uses the mkinitfs unattended feature built into the
// alpine ISO: booting the extracted lts kernel with `unattended` runs
// setup-alpine against defaults (sys mode on the first disk), fetches
// `ssh_key` into root's authorized_keys and powers the guest off.
// Requires Alpine ≥ ~3.19 for the ssh_key URL param; the profile's
// packages still arrive via apk in the provision phase.
type alpineUnattended struct{}

func (a *alpineUnattended) id() string                 { return "alpine-unattended" }
func (a *alpineUnattended) needsSecondBoot() bool      { return true }
func (a *alpineUnattended) interactive() bool          { return false }
func (a *alpineUnattended) completionMode() string     { return "poweroff" }
func (a *alpineUnattended) postInstallFiles() []string { return nil }
func (a *alpineUnattended) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (a *alpineUnattended) sshUser(p *Profile) string { return "root" }

func (a *alpineUnattended) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this an Alpine ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractBootfiles(iso, "boot", r.vmDir,
		[]string{"vmlinuz-lts", "initramfs-lts"})
	if err != nil {
		return fmt.Errorf("alpine boot files: %w", err)
	}
	var keys strings.Builder
	for _, k := range append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...) {
		keys.WriteString(strings.TrimSpace(k) + "\n")
	}
	if err := os.WriteFile(filepath.Join(r.artDir, "sshkey"),
		[]byte(keys.String()), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.e.progress("prepare",
		fmt.Sprintf("ssh key served at http://10.0.2.2:%d/sshkey", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = got["vmlinuz-lts"]
	edits["initrd"] = got["initramfs-lts"]
	// alpine_dev points initramfs at the attached ISO for apkovl +
	// modloop; `unattended` runs the default sys install then powers
	// off; ssh_key is fetched over slirp like the answer files.
	edits["boot_args"] = fmt.Sprintf(
		"console=ttyS0 modules=loop,squashfs,sd-mod,usb-storage "+
			"alpine_dev=cdrom:iso9660 unattended "+
			"ssh_key=http://10.0.2.2:%d/sshkey", port)
	return confSet(r.confPath, edits)
}

// --- Arch Linux serial-driven pacstrap ---------------------------------
//
// serialDriver is the opt-in capability for providers with no answer
// file at all: they steer the live install environment by typing
// commands on the serial console. awaitInstall hands them control
// instead of waiting for a completion signal.
type serialDriver interface {
	driveInstall(ctx context.Context, r *runCtx) error
}

// archPacstrap: the arch ISO drops to a root shell on console — there
// is no unattended boot param. We extract the kernel+initramfs, boot
// with console=ttyS0 so the shell lives on the serial socket, wait for
// the archiso prompt, type a fetch-and-run of the install script the
// embedded HTTP server is holding, and let the script's `poweroff`
// be the completion signal. pacstrap rather than archinstall: the
// latter's config schema moves between releases, base+linux+pacstrap
// is stable since forever.
type archPacstrap struct{}

func (a *archPacstrap) id() string                 { return "arch-pacstrap" }
func (a *archPacstrap) needsSecondBoot() bool      { return true }
func (a *archPacstrap) interactive() bool          { return false }
func (a *archPacstrap) completionMode() string     { return "poweroff" }
func (a *archPacstrap) postInstallFiles() []string { return nil }
func (a *archPacstrap) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (a *archPacstrap) sshUser(p *Profile) string { return p.Identity.Username }

func (a *archPacstrap) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this an Arch ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractBootfiles(iso, "arch/boot/x86_64", r.vmDir,
		[]string{"vmlinuz-linux", "initramfs-linux.img"})
	if err != nil {
		return fmt.Errorf("archiso boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	script := renderArchInstall(r.p, hash, keys)
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(script), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port // driveInstall builds the fetch URL from this
	r.e.progress("prepare",
		fmt.Sprintf("install script at http://10.0.2.2:%d/install.sh", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = got["vmlinuz-linux"]
	edits["initrd"] = got["initramfs-linux.img"]
	// archisobasedir+archisodevice let initramfs mount the attached
	// ISO as the live root; console=ttyS0 puts the root shell on the
	// serial socket where driveInstall can type.
	edits["boot_args"] = "console=ttyS0 archisobasedir=arch archisodevice=/dev/sr0"
	return confSet(r.confPath, edits)
}

// driveInstall → shared serial-script driver below.
func (a *archPacstrap) driveInstall(ctx context.Context, r *runCtx) error {
	return driveSerialScript(ctx, r, serialPrompt{
		loginHint: `archiso login:`, loginReply: "root\n", prompt: `root@archiso`})
}

// serialPrompt configures driveSerialScript's console dance.
type serialPrompt struct {
	loginHint  string // regex that identifies a getty login prompt
	loginReply string // bytes sent at login (may carry a password line)
	prompt     string // regex for the ready root shell
}

// driveSerialScript is the shared live-ISO steering for providers
// whose guests expose a root shell on the serial console: wait for
// the prompt (answering a bare login: if that's what shows), type the
// fetch-and-run of install.sh over slirp, then treat the script's
// poweroff as completion. Idempotent across resume — if the sent
// command already echoes in the log we go straight to waiting for
// poweroff instead of wiping a half-installed disk.
func driveSerialScript(ctx context.Context, r *runCtx, sp serialPrompt) error {
	timeout := time.Duration(r.p.Automation.InstallTimeoutMin) * time.Minute
	cmd := fmt.Sprintf(
		"curl -sf http://10.0.2.2:%d/install.sh | sh\n", r.artPort)
	log := filepath.Join(r.artDir, "serial.log")
	if b, err := os.ReadFile(log); err == nil &&
		strings.Contains(string(b), "install.sh | sh") {
		r.e.progress("await-install", "install already started — waiting for poweroff")
		return waitPIDGone(ctx, r.e, r.vmDir, r.vmName, timeout)
	}
	if err := r.waitSerialPattern(ctx, sp.loginHint+`|`+sp.prompt,
		15*time.Minute); err != nil {
		return err
	}
	if b, _ := os.ReadFile(log); strings.Contains(string(b), sp.loginHint) &&
		!strings.Contains(string(b), sp.prompt) && sp.loginReply != "" {
		if err := r.ser.write(sp.loginReply); err != nil {
			return fmt.Errorf("serial login: %w", err)
		}
		if err := r.waitSerialPattern(ctx, sp.prompt, 2*time.Minute); err != nil {
			return err
		}
	}
	// Let the shell settle past the motd, then send the command.
	time.Sleep(2 * time.Second)
	if err := r.ser.write(cmd); err != nil {
		return fmt.Errorf("serial send: %w", err)
	}
	r.e.progress("await-install", "install command sent — installer running")
	return waitPIDGone(ctx, r.e, r.vmDir, r.vmName, timeout)
}

// --- NixOS serial-driven nixos-install --------------------------------
//
// nixosSerial: the minimal ISO drops to a root shell on console, and
// the whole install is declarative — the run types fetch+run of a
// script that partitions, generates the hardware config, pulls our
// configuration.nix over slirp and nixos-installs. poweroff is the
// completion signal.
type nixosSerial struct{}

func (n *nixosSerial) id() string                 { return "nixos-serial" }
func (n *nixosSerial) needsSecondBoot() bool      { return true }
func (n *nixosSerial) interactive() bool          { return false }
func (n *nixosSerial) completionMode() string     { return "poweroff" }
func (n *nixosSerial) postInstallFiles() []string { return nil }
func (n *nixosSerial) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (n *nixosSerial) sshUser(p *Profile) string { return p.Identity.Username }

func (n *nixosSerial) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a NixOS ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractBootfiles(iso, "boot", r.vmDir,
		[]string{"bzImage", "initrd"})
	if err != nil {
		return fmt.Errorf("nixos boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(renderNixosInstall(r.p, port)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.artDir, "configuration.nix"),
		[]byte(renderNixosConfig(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	r.e.progress("prepare",
		fmt.Sprintf("install script at http://10.0.2.2:%d/install.sh", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = got["bzImage"]
	edits["initrd"] = got["initrd"]
	edits["boot_args"] = "console=ttyS0"
	return confSet(r.confPath, edits)
}

func (n *nixosSerial) driveInstall(ctx context.Context, r *runCtx) error {
	return driveSerialScript(ctx, r, serialPrompt{
		loginHint: `nixos login:`, loginReply: "root\n", prompt: `root@nixos`})
}

// --- Proxmox unattended (VE, PBS, PMG, PDM) ---------------------------
//
// proxmoxAutoInstall is the one provider that needs no kernel
// override: the installer itself scans attached media for a
// PROXMOX_ANSWER-labelled volume holding answer.toml. Completion is
// the post-installation-webhook POST to /done (or a guest poweroff
// when reboot-mode is honoured) — the "webhook" completion mode.
// sshUser is root: the answer file has no user section.
type proxmoxAutoInstall struct{}

func (p *proxmoxAutoInstall) id() string             { return "proxmox-autoinstall" }
func (p *proxmoxAutoInstall) needsSecondBoot() bool  { return true }
func (p *proxmoxAutoInstall) interactive() bool      { return false }
func (p *proxmoxAutoInstall) completionMode() string { return "webhook" }
func (p *proxmoxAutoInstall) postInstallFiles() []string {
	return []string{"proxmox-answer.iso"}
}
func (p *proxmoxAutoInstall) dropConfKeys() []string {
	return []string{"iso", "fixed_iso"}
}
func (p *proxmoxAutoInstall) sshUser(pr *Profile) string { return "root" }

func (p *proxmoxAutoInstall) prepare(r *runCtx) error {
	if confGet(r.confPath, "iso") == "" {
		return fmt.Errorf("conf %s has no iso= — is this a Proxmox ISO VM?", r.confPath)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	ans := filepath.Join(r.artDir, "answer.toml")
	if err := os.WriteFile(ans,
		[]byte(renderProxmoxAnswer(r.p, hash, keys, port)), 0o644); err != nil {
		return err
	}
	fb := filepath.Join(r.artDir, "firstboot.sh")
	if err := os.WriteFile(fb,
		[]byte(renderProxmoxFirstboot(r.p, keys)), 0o644); err != nil {
		return err
	}
	// PROXMOX_ANSWER label is what the installer scans for.
	ansISO := filepath.Join(r.vmDir, "proxmox-answer.iso")
	if err := writeFilesISO(ansISO, "PROXMOX_ANSWER", map[string]string{
		"/answer.toml": ans,
	}); err != nil {
		return err
	}
	r.e.progress("prepare",
		fmt.Sprintf("answer media + firstboot at http://10.0.2.2:%d", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["fixed_iso"] = ansISO
	return confSet(r.confPath, edits)
}

// --- OpenBSD native autoinstall ----------------------------------------
//
// openbsdAutoinstall rides OpenBSD's own mechanism: with no VGA the
// kernel picks com0, the ISO's autoboot countdown lands on the
// (I)nstall/(A)utoinstall prompt, "a" then the install.conf URL runs
// a fully unattended install fetched over slirp. graphics=off goes in
// through the launcher's extra_args escape hatch — no QEMU flag
// support needed. Completion is the installer's reboot message.
type openbsdAutoinstall struct{}

func (o *openbsdAutoinstall) id() string            { return "openbsd-autoinstall" }
func (o *openbsdAutoinstall) needsSecondBoot() bool { return true }
func (o *openbsdAutoinstall) interactive() bool     { return false }
func (o *openbsdAutoinstall) completionMode() string {
	return `serial:rebooting|install.*(done|complete)|CONGRATULATIONS`
}
func (o *openbsdAutoinstall) postInstallFiles() []string { return nil }
func (o *openbsdAutoinstall) dropConfKeys() []string {
	return []string{"iso", "fixed_iso", "extra_args"}
}
func (o *openbsdAutoinstall) sshUser(p *Profile) string { return p.Identity.Username }

func (o *openbsdAutoinstall) prepare(r *runCtx) error {
	if confGet(r.confPath, "iso") == "" {
		return fmt.Errorf("conf %s has no iso= — is this an OpenBSD ISO VM?", r.confPath)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	conf := renderOpenBSDConf(r.p, keys)
	if err := os.WriteFile(filepath.Join(r.artDir, "install.conf"),
		[]byte(conf), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("install.conf at http://10.0.2.2:%d/install.conf", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	// No VGA → com0: the documented way to get OpenBSD's installer on
	// serial. extra_args lands after -machine, so last-wins merges.
	edits["extra_args"] = "-machine graphics=off"
	return confSet(r.confPath, edits)
}

// driveInstall answers the ramdisk's two questions: autoinstall then
// the response-file URL. The install itself is fully unattended once
// install.conf is fetched — the serial marker catches its reboot.
func (o *openbsdAutoinstall) driveInstall(ctx context.Context, r *runCtx) error {
	if err := r.waitSerialPattern(ctx,
		`\(A\)utoinstall|\(I\)nstall`, 15*time.Minute); err != nil {
		return err
	}
	if err := r.ser.write("a\n"); err != nil {
		return fmt.Errorf("serial send autoinstall: %w", err)
	}
	if err := r.waitSerialPattern(ctx,
		`[Rr]esponse file|install\.conf|[Ll]ocation`, 3*time.Minute); err != nil {
		return err
	}
	if err := r.ser.write(fmt.Sprintf(
		"http://10.0.2.2:%d/install.conf\n", r.artPort)); err != nil {
		return fmt.Errorf("serial send url: %w", err)
	}
	r.e.progress("await-install", "install.conf fetched — unattended install running")
	return nil
}

// --- FreeBSD serial Live-CD install ------------------------------------
//
// freebsdShell navigates bsdinstall's start menu to the Live CD entry
// (arrow keys are escape sequences on serial), logs in as root, and
// runs the same fetch-and-run script pattern as arch/nixos — the
// script does gpart/newfs/tar extraction from the ISO's own dist sets.
type freebsdShell struct{}

func (f *freebsdShell) id() string                 { return "freebsd-shell" }
func (f *freebsdShell) needsSecondBoot() bool      { return true }
func (f *freebsdShell) interactive() bool          { return false }
func (f *freebsdShell) completionMode() string     { return "poweroff" }
func (f *freebsdShell) postInstallFiles() []string { return nil }
func (f *freebsdShell) dropConfKeys() []string {
	return []string{"iso", "fixed_iso"}
}
func (f *freebsdShell) sshUser(p *Profile) string { return p.Identity.Username }

func (f *freebsdShell) prepare(r *runCtx) error {
	if confGet(r.confPath, "iso") == "" {
		return fmt.Errorf("conf %s has no iso= — is this a FreeBSD ISO VM?", r.confPath)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(renderFreeBSDInstall(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("install script at http://10.0.2.2:%d/install.sh", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	// FreeBSD's loader reads /boot.config off the ISO — serial output
	// on the disc images is the default in current releases; if an
	// image ships vidconsole-only this provider will time out waiting.
	return confSet(r.confPath, edits)
}

// driveInstall: Welcome menu → Live CD (2×Down+Enter) → login root →
// fetch+run. The nav sequence goes out blind once the menu shows.
func (f *freebsdShell) driveInstall(ctx context.Context, r *runCtx) error {
	timeout := time.Duration(r.p.Automation.InstallTimeoutMin) * time.Minute
	log := filepath.Join(r.artDir, "serial.log")
	if b, err := os.ReadFile(log); err == nil &&
		strings.Contains(string(b), "install.sh | sh") {
		return waitPIDGone(ctx, r.e, r.vmDir, r.vmName, timeout)
	}
	if err := r.waitSerialPattern(ctx,
		`Welcome to FreeBSD|boot:`,
		15*time.Minute); err != nil {
		return err
	}
	// Menu order: Install, Shell, Live CD — down,down,enter. Extra
	// settle time so the dialog redraws before keys land.
	time.Sleep(3 * time.Second)
	if err := r.ser.write("\x1b[B\x1b[B\r"); err != nil {
		return fmt.Errorf("serial nav: %w", err)
	}
	if err := r.waitSerialPattern(ctx, `login:`, 3*time.Minute); err != nil {
		return err
	}
	if err := r.ser.write("root\n"); err != nil {
		return fmt.Errorf("serial login: %w", err)
	}
	if err := r.waitSerialPattern(ctx, `root@|#`, 2*time.Minute); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	cmd := fmt.Sprintf("fetch -qo - http://10.0.2.2:%d/install.sh | sh\n", r.artPort)
	if err := r.ser.write(cmd); err != nil {
		return fmt.Errorf("serial send: %w", err)
	}
	r.e.progress("await-install", "install command sent — freebsd install running")
	return waitPIDGone(ctx, r.e, r.vmDir, r.vmName, timeout)
}

// --- Void Linux serial xbps install ------------------------------------
//
// voidXbps: live ISO login root/voidlinux → root shell → script does
// sfdisk/mkfs/xbps-install -r /mnt base-system from the void mirror.
type voidXbps struct{}

func (v *voidXbps) id() string                 { return "void-xbps" }
func (v *voidXbps) needsSecondBoot() bool      { return true }
func (v *voidXbps) interactive() bool          { return false }
func (v *voidXbps) completionMode() string     { return "poweroff" }
func (v *voidXbps) postInstallFiles() []string { return nil }
func (v *voidXbps) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (v *voidXbps) sshUser(p *Profile) string { return p.Identity.Username }

func (v *voidXbps) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a Void ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractCandidates(iso, r.vmDir, [][]string{
		{"boot", "vmlinuz", "initrd"},
		{"boot", "vmlinuz", "initramfs"},
		{"boot", "vmlinux", "initrd"},
	})
	if err != nil {
		return fmt.Errorf("void boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	// musl editions pull from a different repo path.
	musl := strings.Contains(strings.ToLower(r.p.System.Edition), "musl")
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(renderVoidInstall(r.p, hash, keys, musl)), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("install script at http://10.0.2.2:%d/install.sh", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = firstOf(got, "vmlinuz", "vmlinux")
	edits["initrd"] = firstOf(got, "initrd", "initramfs")
	// The live image mounts its own rootfs — console=ttyS0 puts the
	// login prompt on serial for driveInstall.
	edits["boot_args"] = "console=ttyS0"
	return confSet(r.confPath, edits)
}

func (v *voidXbps) driveInstall(ctx context.Context, r *runCtx) error {
	// void-live login is root/voidlinux — two lines sent as one reply.
	return driveSerialScript(ctx, r, serialPrompt{
		loginHint:  `void-live login:|login:`,
		loginReply: "root\nvoidlinux\n",
		prompt:     `root@void-live|]#`})
}

// firstOf returns the extracted path for the first key present.
func firstOf(got map[string]string, keys ...string) string {
	for _, k := range keys {
		if got[k] != "" {
			return got[k]
		}
	}
	return ""
}

// --- Gentoo serial stage3 install --------------------------------------
//
// gentooStage3: minimal ISO drops to a root shell; the script does a
// textbook manual install — partition, fetch+untar stage3, chroot
// emerge of kernel-bin/grub/sshd, bootloader. Long but deterministic.
type gentooStage3 struct{}

func (g *gentooStage3) id() string                 { return "gentoo-stage3" }
func (g *gentooStage3) needsSecondBoot() bool      { return true }
func (g *gentooStage3) interactive() bool          { return false }
func (g *gentooStage3) completionMode() string     { return "poweroff" }
func (g *gentooStage3) postInstallFiles() []string { return nil }
func (g *gentooStage3) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (g *gentooStage3) sshUser(p *Profile) string { return p.Identity.Username }

func (g *gentooStage3) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a Gentoo ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractCandidates(iso, r.vmDir, [][]string{
		{"boot", "gentoo", "gentoo.igz"},
		{"boot", "kernel-gentoo", "initramfs-gentoo"},
		{"boot", "vmlinuz", "initrd"},
	})
	if err != nil {
		return fmt.Errorf("gentoo boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(renderGentooInstall(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("install script at http://10.0.2.2:%d/install.sh", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = firstOf(got, "gentoo", "kernel-gentoo", "vmlinuz")
	edits["initrd"] = firstOf(got, "gentoo.igz", "initramfs-gentoo", "initrd")
	edits["boot_args"] = "console=ttyS0 dokeymap"
	return confSet(r.confPath, edits)
}

func (g *gentooStage3) driveInstall(ctx context.Context, r *runCtx) error {
	// gentoo minimal autologins root — no login dance.
	return driveSerialScript(ctx, r, serialPrompt{
		loginHint: `login:`, loginReply: "",
		prompt: `livecd.*#|root@livecd`})
}

// --- Flatcar serial flatcar-install -------------------------------------
//
// flatcarIgnition: ISO live env autologins `core`; the typed command
// fetches ignition.json over slirp and flatcar-install dd's the image
// to disk with it embedded. Reboot is the completion marker.
type flatcarIgnition struct{}

func (f *flatcarIgnition) id() string            { return "flatcar-ignition" }
func (f *flatcarIgnition) needsSecondBoot() bool { return true }
func (f *flatcarIgnition) interactive() bool     { return false }
func (f *flatcarIgnition) completionMode() string {
	return `serial:reboot|Restarting|Powering`
}
func (f *flatcarIgnition) postInstallFiles() []string { return nil }
func (f *flatcarIgnition) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (f *flatcarIgnition) sshUser(p *Profile) string { return p.Identity.Username }

func (f *flatcarIgnition) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a Flatcar ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractCandidates(iso, r.vmDir, [][]string{
		{"flatcar", "vmlinuz-a", "initrd"},
		{"flatcar", "vmlinuz-a", "initrd.gz"},
		{"boot", "vmlinuz", "initrd"},
	})
	if err != nil {
		return fmt.Errorf("flatcar boot files: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	if err := os.WriteFile(filepath.Join(r.artDir, "ignition.json"),
		[]byte(renderFlatcarIgnition(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(renderFlatcarInstall(port)), 0o644); err != nil {
		return err
	}
	r.e.progress("prepare",
		fmt.Sprintf("ignition+script at http://10.0.2.2:%d/", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = firstOf(got, "vmlinuz-a", "vmlinuz")
	edits["initrd"] = firstOf(got, "initrd", "initrd.gz")
	edits["boot_args"] = "console=ttyS0"
	return confSet(r.confPath, edits)
}

func (f *flatcarIgnition) driveInstall(ctx context.Context, r *runCtx) error {
	// flatcar live autologins `core` — sudo is passwordless inside.
	return driveSerialScript(ctx, r, serialPrompt{
		loginHint: `login:`, loginReply: "",
		prompt: `core@|flatcar.*\$`})
}

// --- Fedora CoreOS fw_cfg ignition -------------------------------------
//
// coreosIgnition: the qcow2 image needs no install boot at all — an
// Ignition config passed via -fw_cfg on first boot creates the user,
// keys and hostname (the documented opt/com.coreos/config channel).
// extra_args is the launcher's escape hatch — zero launcher changes.
type coreosIgnition struct{}

func (c *coreosIgnition) id() string                 { return "coreos-ignition" }
func (c *coreosIgnition) needsSecondBoot() bool      { return false }
func (c *coreosIgnition) interactive() bool          { return false }
func (c *coreosIgnition) completionMode() string     { return "poweroff" }
func (c *coreosIgnition) postInstallFiles() []string { return nil }
func (c *coreosIgnition) dropConfKeys() []string     { return []string{"extra_args"} }
func (c *coreosIgnition) sshUser(p *Profile) string  { return p.Identity.Username }

func (c *coreosIgnition) prepare(r *runCtx) error {
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	ign := filepath.Join(r.vmDir, "ignition.json")
	if err := os.WriteFile(ign,
		[]byte(renderCoreOSIgnition(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["extra_args"] = fmt.Sprintf(
		"-fw_cfg name=opt/com.coreos/config,file=%s", ign)
	return confSet(r.confPath, edits)
}

// --- Ubiquity preseed (linuxmint, zorin, xubuntu, ubuntu-mate) -------
//
// ubiquityPreseed rides ubiquity's own unattended mode: casper boots
// the live session with `automatic-ubiquity`, which runs the installer
// with the answers of a d-i-style preseed fetched over slirp.
// `ubiquity/poweroff boolean true` makes it end in a shutdown — the
// same completion contract as the answer-file providers.
type ubiquityPreseed struct{}

func (u *ubiquityPreseed) id() string                 { return "ubiquity-preseed" }
func (u *ubiquityPreseed) needsSecondBoot() bool      { return true }
func (u *ubiquityPreseed) interactive() bool          { return false }
func (u *ubiquityPreseed) completionMode() string     { return "poweroff" }
func (u *ubiquityPreseed) postInstallFiles() []string { return nil }
func (u *ubiquityPreseed) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (u *ubiquityPreseed) sshUser(p *Profile) string { return p.Identity.Username }

func (u *ubiquityPreseed) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a ubiquity ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractCandidates(iso, r.vmDir, [][]string{
		{"casper", "vmlinuz", "initrd"},
		{"casper", "vmlinuz", "initrd.gz"},
		{"casper", "vmlinuz", "initrd.lz"},
	})
	if err != nil {
		return fmt.Errorf("casper boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	if err := os.WriteFile(filepath.Join(r.artDir, "preseed.cfg"),
		[]byte(renderUbiquityPreseed(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("preseed at http://10.0.2.2:%d/preseed.cfg", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = firstOf(got, "vmlinuz")
	edits["initrd"] = firstOf(got, "initrd", "initrd.gz", "initrd.lz")
	edits["boot_args"] = fmt.Sprintf(
		"boot=casper automatic-ubiquity console=ttyS0 "+
			"preseed/url=http://10.0.2.2:%d/preseed.cfg", port)
	return confSet(r.confPath, edits)
}

// --- Slackware serial install ----------------------------------------
//
// slackwareLive: the install media boots to a bare `slackware login:`
// (root, no password); the script partitions, installpkg's every
// series from the mounted CD (Slackware's recommended full install),
// writes lilo.conf/elilo.conf, and enables sshd+DHCP by hand — no
// dialog tools needed.
type slackwareLive struct{}

func (s *slackwareLive) id() string                 { return "slackware-live" }
func (s *slackwareLive) needsSecondBoot() bool      { return true }
func (s *slackwareLive) interactive() bool          { return false }
func (s *slackwareLive) completionMode() string     { return "poweroff" }
func (s *slackwareLive) postInstallFiles() []string { return nil }
func (s *slackwareLive) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (s *slackwareLive) sshUser(p *Profile) string { return p.Identity.Username }

func (s *slackwareLive) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a Slackware ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	// The ISO ships kernels/{hugesmp.s,huge.s,generic.s}/bzImage — the
	// immediate parent dir is what the prefix matcher sees, and
	// hugesmp.s (SMP, PATA+SATA builtin) is the right installer kernel.
	kern, err := extractBootfiles(iso, "hugesmp", r.vmDir,
		[]string{"bzimage"})
	if err != nil {
		return fmt.Errorf("slackware kernel: %w", err)
	}
	initrd, err := extractBootfiles(iso, "isolinux", r.vmDir,
		[]string{"initrd.img"})
	if err != nil {
		return fmt.Errorf("slackware initrd: %w", err)
	}
	got := map[string]string{"bzImage": kern["bzimage"], "initrd.img": initrd["initrd.img"]}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(renderSlackwareInstall(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("install script at http://10.0.2.2:%d/install.sh", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = got["bzImage"]
	edits["initrd"] = got["initrd.img"]
	// Mirrors the ISO's isolinux append line minus the initrd key the
	// launcher adds itself.
	edits["boot_args"] = "console=ttyS0 load_ramdisk=1 prompt_ramdisk=0 rw " +
		"printk.time=0 SLACK_KERNEL=hugesmp.s"
	return confSet(r.confPath, edits)
}

func (s *slackwareLive) driveInstall(ctx context.Context, r *runCtx) error {
	// slackware login: root (no password on the install media).
	return driveSerialScript(ctx, r, serialPrompt{
		loginHint:  `slackware login:|login:`,
		loginReply: "root\n",
		prompt:     `root@slackware|root@darkstar|/#`})
}

// --- Artix serial basestrap ------------------------------------------
//
// artixBasestrap is arch-pacstrap's cousin: artix ISOs (based on
// archiso-style media) boot to a root console; the script basestraps
// base + openrc from artix repos, then fstabgen + artix-chroot for
// user/grub/services.
type artixBasestrap struct{}

func (a *artixBasestrap) id() string                 { return "artix-basestrap" }
func (a *artixBasestrap) needsSecondBoot() bool      { return true }
func (a *artixBasestrap) interactive() bool          { return false }
func (a *artixBasestrap) completionMode() string     { return "poweroff" }
func (a *artixBasestrap) postInstallFiles() []string { return nil }
func (a *artixBasestrap) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (a *artixBasestrap) sshUser(p *Profile) string { return p.Identity.Username }

func (a *artixBasestrap) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this an Artix ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractCandidates(iso, r.vmDir, [][]string{
		{"boot", "vmlinuz-linux", "initramfs-linux.img"},
		{"artix", "boot", "x86_64", "vmlinuz", "artix.img"},
		{"boot", "x86_64", "vmlinuz", "initramfs"},
	})
	if err != nil {
		return fmt.Errorf("artix boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(renderArtixInstall(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("install script at http://10.0.2.2:%d/install.sh", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = firstOf(got, "vmlinuz-linux", "vmlinuz")
	edits["initrd"] = firstOf(got, "initramfs-linux.img", "artix.img", "initramfs")
	edits["boot_args"] = "console=ttyS0"
	return confSet(r.confPath, edits)
}

func (a *artixBasestrap) driveInstall(ctx context.Context, r *runCtx) error {
	// root/artix — on autologin media the second line lands harmlessly
	// as a failed command.
	return driveSerialScript(ctx, r, serialPrompt{
		loginHint:  `artixiso login:|login:`,
		loginReply: "root\nartix\n",
		prompt:     `root@artixiso|root@artix|]#`})
}

// --- Calculate Linux serial cl-install --------------------------------
//
// calculateClinstall: Calculate's live media ships cl-install, a
// genuinely scriptable installer — `cl-install -d /dev/vda` does the
// whole disk install non-interactively. The serial shell then does
// user/sshd by chroot.
type calculateClinstall struct{}

func (c *calculateClinstall) id() string                 { return "calculate-clinstall" }
func (c *calculateClinstall) needsSecondBoot() bool      { return true }
func (c *calculateClinstall) interactive() bool          { return false }
func (c *calculateClinstall) completionMode() string     { return "poweroff" }
func (c *calculateClinstall) postInstallFiles() []string { return nil }
func (c *calculateClinstall) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (c *calculateClinstall) sshUser(p *Profile) string { return p.Identity.Username }

func (c *calculateClinstall) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a Calculate ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	got, err := extractCandidates(iso, r.vmDir, [][]string{
		{"boot", "calculate", "initrd"},
		{"boot", "vmlinuz", "initrd"},
		{"boot", "kernel", "initramfs"},
	})
	if err != nil {
		return fmt.Errorf("calculate boot files: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(renderCalculateInstall(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("install script at http://10.0.2.2:%d/install.sh", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = firstOf(got, "calculate", "vmlinuz", "kernel")
	edits["initrd"] = firstOf(got, "initrd", "initramfs")
	edits["boot_args"] = "console=ttyS0"
	return confSet(r.confPath, edits)
}

func (c *calculateClinstall) driveInstall(ctx context.Context, r *runCtx) error {
	// calculate live: login root, empty password accepted by pressing
	// enter — "root\n\n" covers both no-password and password-free
	// getty variants.
	return driveSerialScript(ctx, r, serialPrompt{
		loginHint: `login:`, loginReply: "root\n",
		prompt: `root@.*#|]#`})
}

// --- Guix serial guix system init -------------------------------------
//
// guixSerial mirrors nixos-serial: the install image's shepherd spawns
// a root getty on the console; a fetched script partitions, cow-stores
// and guix system init's the rendered config.scm. Kernel and initrd
// sit under /gnu/store/<hash>-… — the paths are parsed out of the
// ISO's own grub.cfg so any release layout resolves, then extracted
// by their store dir.
type guixSerial struct{}

func (g *guixSerial) id() string                 { return "guix-serial" }
func (g *guixSerial) needsSecondBoot() bool      { return true }
func (g *guixSerial) interactive() bool          { return false }
func (g *guixSerial) completionMode() string     { return "poweroff" }
func (g *guixSerial) postInstallFiles() []string { return nil }
func (g *guixSerial) dropConfKeys() []string {
	return []string{"kernel", "initrd", "boot_args", "iso", "fixed_iso"}
}
func (g *guixSerial) sshUser(p *Profile) string { return p.Identity.Username }

// grubLinuxRe/grubInitrdRe match the first menuentry's kernel and
// initrd lines in an ISO grub.cfg: `linux <store path> <args…>`.
var (
	grubLinuxRe  = regexp.MustCompile(`(?m)^\s*linux\s+(\S+)\s*(.*)$`)
	grubInitrdRe = regexp.MustCompile(`(?m)^\s*initrd\s+(\S+)\s*$`)
)

func (g *guixSerial) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a Guix ISO VM?", r.confPath)
	}
	iso = resolveConfPath(r.confPath, iso)
	cfgFile, err := extractBootfiles(iso, "grub", r.vmDir, []string{"grub.cfg"})
	if err != nil {
		return fmt.Errorf("guix grub.cfg: %w", err)
	}
	cfg, err := os.ReadFile(cfgFile["grub.cfg"])
	if err != nil {
		return fmt.Errorf("guix grub.cfg: %w", err)
	}
	kPath, kArgs, iPath := "", "", ""
	if m := grubLinuxRe.FindStringSubmatch(string(cfg)); m != nil {
		kPath, kArgs = m[1], strings.TrimSpace(m[2])
	}
	if m := grubInitrdRe.FindStringSubmatch(string(cfg)); m != nil {
		iPath = m[1]
	}
	if kPath == "" || iPath == "" {
		return fmt.Errorf("guix grub.cfg: no linux/initrd entries found")
	}
	// dirMatch's contains-test finds the store dirs by basename — the
	// hash prefix is part of the name so the match is precise.
	kern, err := extractBootfiles(iso, path.Base(path.Dir(kPath)), r.vmDir,
		[]string{path.Base(kPath)})
	if err != nil {
		return fmt.Errorf("guix kernel: %w", err)
	}
	init, err := extractBootfiles(iso, path.Base(path.Dir(iPath)), r.vmDir,
		[]string{path.Base(iPath)})
	if err != nil {
		return fmt.Errorf("guix initrd: %w", err)
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	keysFile := filepath.Join(r.artDir, "keys.pub")
	if err := os.WriteFile(keysFile,
		[]byte(strings.TrimSpace(strings.Join(keys, "\n"))+"\n"), 0o644); err != nil {
		return err
	}
	for _, f := range [][2]string{
		{"install.sh", renderGuixInstall(r.artPort)},
		{"config-efi.scm", renderGuixConfig(r.p, hash, true)},
		{"config-bios.scm", renderGuixConfig(r.p, hash, false)},
	} {
		if err := os.WriteFile(filepath.Join(r.artDir, f[0]), []byte(f[1]), 0o644); err != nil {
			return err
		}
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("guix artifacts at http://10.0.2.2:%d/", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["kernel"] = kern[path.Base(kPath)]
	edits["initrd"] = init[path.Base(iPath)]
	// The ISO's own --root/--system/--load args boot the live system;
	// console=ttyS0 puts shepherd's getty on the serial socket.
	edits["boot_args"] = kArgs + " console=ttyS0"
	return confSet(r.confPath, edits)
}

func (g *guixSerial) driveInstall(ctx context.Context, r *runCtx) error {
	// The install image runs the guided newt installer on tty1 and a
	// getty on the serial console — root has no password there. If a
	// release puts the installer on serial instead, the prompt wait
	// times out loudly rather than typing into menus.
	return driveSerialScript(ctx, r, serialPrompt{
		loginHint: `guix login:|login:`, loginReply: "root\n",
		prompt: `root@guix|root@.*#|\]#`})
}

// --- NetBSD serial sysinst → shell → sets ----------------------------
//
// netbsdSets follows the documented headless path: the boot-com ISO
// is NetBSD's serial-console build of the same installer — fetched as
// a companion from the release's images dir (quickget's catalog ISO
// is the VGA build). Serial sysinst → Utility menu → /bin/sh gives a
// shell; the script partitions via GPT wedges, untars the sets off
// the CD, writes bootblocks/rc.conf and powers off.
type netbsdSets struct{}

func (n *netbsdSets) id() string                 { return "netbsd-sets" }
func (n *netbsdSets) needsSecondBoot() bool      { return true }
func (n *netbsdSets) interactive() bool          { return false }
func (n *netbsdSets) completionMode() string     { return "poweroff" }
func (n *netbsdSets) postInstallFiles() []string { return nil }
func (n *netbsdSets) dropConfKeys() []string {
	return []string{"iso", "fixed_iso"}
}
func (n *netbsdSets) sshUser(p *Profile) string { return p.Identity.Username }

func (n *netbsdSets) prepare(r *runCtx) error {
	iso := confGet(r.confPath, "iso")
	if iso == "" {
		return fmt.Errorf("conf %s has no iso= — is this a NetBSD ISO VM?", r.confPath)
	}
	// The serial build of the same release lives next to the catalog
	// ISO on the CDN — deterministic name per release.
	rel := r.p.System.Release
	com := filepath.Join(r.vmDir, "netbsd-boot-com.iso")
	if _, err := os.Stat(com); err != nil {
		url := fmt.Sprintf("https://cdn.netbsd.org/pub/NetBSD/NetBSD-%s/"+
			"images/NetBSD-%s-amd64-boot-com.iso", rel, rel)
		if err := fetchFile(url, com); err != nil {
			return fmt.Errorf("netbsd boot-com.iso: %w", err)
		}
	}
	hash, err := pwHash(r.p)
	if err != nil {
		return fmt.Errorf("password hash: %w", err)
	}
	keys := append([]string{r.kp.pub}, r.p.Identity.SSHAuthorizedKeys...)
	if err := os.WriteFile(filepath.Join(r.artDir, "install.sh"),
		[]byte(renderNetbsdInstall(r.p, hash, keys)), 0o644); err != nil {
		return err
	}
	port, srv, err := startSeedHTTPServer(r.artDir)
	if err != nil {
		return err
	}
	r.seedSrv = srv
	r.artPort = port
	r.e.progress("prepare",
		fmt.Sprintf("install script at http://10.0.2.2:%d/install.sh", port))
	edits := commonConfEdits(r)
	delete(edits, "seed")
	edits["iso"] = com // the serial build replaces the VGA media for this boot
	return confSet(r.confPath, edits)
}

func (n *netbsdSets) driveInstall(ctx context.Context, r *runCtx) error {
	timeout := time.Duration(r.p.Automation.InstallTimeoutMin) * time.Minute
	log := filepath.Join(r.artDir, "serial.log")
	if b, err := os.ReadFile(log); err == nil &&
		strings.Contains(string(b), "install.sh") {
		r.e.progress("await-install", "install already started — waiting for poweroff")
		return waitPIDGone(ctx, r.e, r.vmDir, r.vmName, timeout)
	}
	// boot-com boots straight into sysinst on com0 — main menu items
	// take single-letter shortcuts: 'e' = Utility menu, then
	// 'a' = Run /bin/sh. Letter layouts held steady across 9/10/11;
	// a menu reshuffle fails loudly at the next pattern wait.
	if err := r.waitSerialPattern(ctx,
		`Install NetBSD to hard disk|Utility menu|sysinst`, 15*time.Minute); err != nil {
		return err
	}
	time.Sleep(3 * time.Second)
	if err := r.ser.write("e"); err != nil {
		return fmt.Errorf("serial nav: %w", err)
	}
	if err := r.waitSerialPattern(ctx,
		`Utility menu|/bin/sh`, 2*time.Minute); err != nil {
		return err
	}
	time.Sleep(time.Second)
	if err := r.ser.write("a"); err != nil {
		return fmt.Errorf("serial nav: %w", err)
	}
	if err := r.waitSerialPattern(ctx, `#`, 2*time.Minute); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	// Bring slirp up statically (the installer env has no DHCP client
	// running yet): virtio-net is vioif0, e1000 would be wm0 — try
	// both, then ftp(1) pulls the script (NetBSD ftp does HTTP).
	cmd := fmt.Sprintf(
		"ifconfig vioif0 inet 10.0.2.15 netmask 255.255.255.0 up "+
			"|| ifconfig wm0 inet 10.0.2.15 netmask 255.255.255.0 up; "+
			"route add default 10.0.2.2 2>/dev/null; "+
			"ftp -o - http://10.0.2.2:%d/install.sh | sh\n", r.artPort)
	if err := r.ser.write(cmd); err != nil {
		return fmt.Errorf("serial send: %w", err)
	}
	r.e.progress("await-install", "install command sent — netbsd install running")
	return waitPIDGone(ctx, r.e, r.vmDir, r.vmName, timeout)
}

// --- visual / manual install ----------------------------------------

// visualProvider covers OSes with no unattended-install answer file
// (macOS above all): the run obtains the image, prepares the VM and
// launches it with a window — the user drives the installer. The run
// completes at launch; provisioning a manually-installed guest is a
// later phase.
type visualProvider struct{}

func (v *visualProvider) id() string                 { return "visual" }
func (v *visualProvider) needsSecondBoot() bool      { return false }
func (v *visualProvider) interactive() bool          { return true }
func (v *visualProvider) completionMode() string     { return "poweroff" }
func (v *visualProvider) postInstallFiles() []string { return nil }
func (v *visualProvider) dropConfKeys() []string     { return nil }
func (v *visualProvider) sshUser(p *Profile) string  { return p.Identity.Username }

func (v *visualProvider) prepare(r *runCtx) error {
	edits := map[string]string{
		"monitor":       "socket",
		"serial":        "socket",
		"ram":           r.p.Machine.RAM,
		"cpu_cores":     fmt.Sprint(r.p.Machine.CPUs),
		"port_forwards": fmt.Sprintf("%d:22", r.sshPort),
	}
	if r.p.Machine.Boot != "" {
		edits["boot"] = r.p.Machine.Boot
	}
	r.e.progress("prepare",
		"visual install — complete the setup in the VM window; "+
			"the run finishes once the VM is launched")
	return confSet(r.confPath, edits)
}
