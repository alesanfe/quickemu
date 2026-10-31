// provider.go: InstallerProvider — the per-OS strategy object. Each
// provider owns everything OS-specific: answer media, boot overrides,
// and what to undo once the installed system exists. Quicktest the
// orchestrator never embeds per-OS logic.
package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

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
	// needsSecondBoot reports whether the run must stop the install
	// boot and relaunch (ISO installers) or boots straight into the
	// final system (pre-imaged disks).
	needsSecondBoot() bool
	// sshUser is the login the seed arranges.
	sshUser(p *Profile) string
}

func pickProvider(p *Profile) (provider, error) {
	switch p.Automation.Provider {
	case "ubuntu-autoinstall":
		return &ubuntuAutoinstall{}, nil
	case "nocloud-image":
		return &noCloudImage{}, nil
	case "manual":
		return nil, fmt.Errorf(
			"%s %s has no automation provider — catalog image_type gives none; "+
				"set automation.provider to install manually-assisted", p.System.OS, p.System.Release)
	default:
		return nil, fmt.Errorf("unknown provider %q", p.Automation.Provider)
	}
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
		"public-keys":    map[string]string{"quicktest": strings.TrimSpace(r.kp.pub)},
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

func (u *ubuntuAutoinstall) id() string { return "ubuntu-autoinstall" }

func (u *ubuntuAutoinstall) needsSecondBoot() bool { return true }

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

func (n *noCloudImage) id() string            { return "nocloud-image" }
func (n *noCloudImage) needsSecondBoot() bool { return false }
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
