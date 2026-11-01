// profile.go: MachineProfile — the declarative input a run resolves
// against the catalog. TOML, matching the .conf → TOML migration; the
// cloud-init user-data it produces is YAML but that is output, not
// input.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Profile is the apiVersion-1 machine automation profile. Field names
// mirror the MachineProfile kind in the design doc; only the subset
// the engine actually consumes is modelled — unknown keys are ignored
// so profiles can grow ahead of the engine.
type Profile struct {
	sourcePath string // file the profile was loaded from (resume needs it)
	Profile    struct {
		Name        string `toml:"name"`
		Description string `toml:"description"`
	} `toml:"profile"`
	System struct {
		OS      string `toml:"os"`
		Release string `toml:"release"`
		Edition string `toml:"edition"`
		Arch    string `toml:"arch"`
	} `toml:"system"`
	Machine struct {
		CPUs     int    `toml:"cpus"`
		RAM      string `toml:"ram"`
		DiskSize string `toml:"disk_size"`
		SSHPort  int    `toml:"ssh_port"`
		Boot     string `toml:"boot"` // efi|legacy|secure — empty keeps the conf default
	} `toml:"machine"`
	Identity struct {
		Hostname          string   `toml:"hostname"`
		Username          string   `toml:"username"`
		Password          string   `toml:"password"`
		SSHKeyFile        string   `toml:"ssh_key_file"`
		SSHAuthorizedKeys []string `toml:"ssh_authorized_keys"`
		AllowPwAuth       bool     `toml:"allow_pw_auth"`
	} `toml:"identity"`
	Locale struct {
		Lang     string `toml:"lang"`
		Keyboard string `toml:"keyboard"`
		Timezone string `toml:"timezone"`
	} `toml:"locale"`
	Automation struct {
		Provider           string `toml:"provider"`
		InstallTimeoutMin  int    `toml:"install_timeout_min"`
		SSHTimeoutMin      int    `toml:"ssh_timeout_min"`
		AllowVisualInstall bool   `toml:"allow_visual_install"`
	} `toml:"automation"`
	Provision struct {
		Packages []string `toml:"packages"`
		Services []string `toml:"services"`
		Files    []struct {
			Source      string `toml:"source"`
			Destination string `toml:"destination"`
			Mode        string `toml:"mode"`
			Owner       string `toml:"owner"`
		} `toml:"files"`
		Commands []struct {
			Name string `toml:"name"`
			Run  string `toml:"run"`
			As   string `toml:"as"`
		} `toml:"commands"`
	} `toml:"provision"`
	Verify []struct {
		Name   string `toml:"name"`
		Run    string `toml:"run"`
		Expect string `toml:"expect"`
	} `toml:"verify"`
	Finalize struct {
		Shutdown bool   `toml:"shutdown"`
		Snapshot string `toml:"snapshot"`
	} `toml:"finalize"`
}

// loadProfile reads + validates a TOML profile, applying defaults for
// anything omitted. Validation only rejects what would make a run
// nonsensical — capability checks (does this OS support the provider)
// happen at plan time when the catalog is consulted.
func loadProfile(path string) (*Profile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := toml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if abs, err := filepath.Abs(path); err == nil {
		p.sourcePath = abs
	}
	if p.System.OS == "" {
		return nil, fmt.Errorf("system.os is required")
	}
	if p.System.Release == "" {
		return nil, fmt.Errorf("system.release is required")
	}
	if p.System.Arch == "" {
		p.System.Arch = "amd64"
	}
	if p.Profile.Name == "" {
		p.Profile.Name = p.System.OS + "-" + p.System.Release
	}
	if p.Identity.Hostname == "" {
		p.Identity.Hostname = safeHostname(p.Profile.Name)
	}
	if p.Identity.Username == "" {
		p.Identity.Username = "quickemu"
	}
	if p.Machine.CPUs <= 0 {
		p.Machine.CPUs = 2
	}
	if p.Machine.RAM == "" {
		p.Machine.RAM = "4G"
	}
	if p.Automation.Provider == "" || p.Automation.Provider == "auto" {
		p.Automation.Provider = inferProvider(&p)
	}
	if p.Automation.InstallTimeoutMin <= 0 {
		p.Automation.InstallTimeoutMin = 45
	}
	if p.Automation.SSHTimeoutMin <= 0 {
		p.Automation.SSHTimeoutMin = 15
	}
	return &p, nil
}

// inferProvider picks the install strategy for the OS when the
// profile does not pin one: the mainstream unattended-install paths
// first (edition-aware — only Fedora's Server ISO runs anaconda, the
// desktop spins are live images), then the catalog image_type —
// pre-imaged disks boot straight into a seeded system, everything
// else falls back to the "manual" error telling the user to pick a
// provider by hand.
func inferProvider(p *Profile) string {
	switch p.System.OS {
	case "ubuntu-server":
		return "ubuntu-autoinstall"
	case "ubuntu", "ubuntudde", "edubuntu":
		// Flavours on the shared flutter/subiquity installer take the
		// same autoinstall path; calamares flavours (kubuntu, lubuntu,
		// unity) do not and stay visual.
		return "ubuntu-desktop-autoinstall"
	case "ubuntu-budgie", "ubuntustudio", "xubuntu", "ubuntu-mate",
		"ubuntukylin":
		// These flavours switched installer mid-stream: ubiquity up to
		// 23.04, the flutter/subiquity installer from 23.10.
		if releaseAtLeast(p.System.Release, 23, 10) {
			return "ubuntu-desktop-autoinstall"
		}
		return "ubiquity-preseed"
	case "linuxmint", "zorin", "bodhi", "ubuntucinnamon", "mythbuntu",
		"linuxlite", "backbox", "hamonikr":
		// Still on ubiquity — Cinnamon held back from the flutter
		// migration; mint, zorin, bodhi and the Ubuntu/Mint-derived
		// respins never shipped the new one.
		return "ubiquity-preseed"
	case "debian":
		// Only the installer ISOs run d-i; the DE editions are
		// calamares live images.
		switch p.System.Edition {
		case "", "netinst", "standard":
			return "debian-preseed"
		}
	case "kali", "debian-edu", "parrotsec", "canaima", "bosslinux",
		"openmediavault", "yunohost", "turnkey", "turnkeylinux":
		// All ship a Debian-Installer ISO. OMV/Yunohost's own media
		// bakes their preseed in — ours installs the Debian base and
		// the appliance layer belongs to the provision stage.
		return "debian-preseed"
	case "devuan":
		// Only the netinstall ISO runs d-i; the desktop-live the
		// catalog defaults to is refractainstaller.
		if p.System.Edition == "netinst" {
			return "debian-preseed"
		}
	case "trisquel":
		// Only the netinst ISO runs d-i; the DE editions are live.
		if p.System.Edition == "netinst" {
			return "debian-preseed"
		}
	case "windows", "windows-server":
		return "windows-autounattend"
	case "macos":
		return "visual"
	case "fedora":
		// Spins/Workstation resolve to live media — only installer
		// ISOs run anaconda unattended: the Server DVD or the
		// Everything netinst.
		if p.System.Edition == "Server" || p.System.Edition == "Everything" {
			return "anaconda-kickstart"
		}
	case "qubes":
		// The Qubes installer is anaconda with documented kickstart
		// support.
		return "anaconda-kickstart"
	case "mageia":
		// Only the classic DVD runs drakx auto_inst; the Live media
		// has its own installer.
		if p.System.Edition == "DVD" {
			return "mageia-autoinst"
		}
	case "alma", "centos", "centos-stream", "rockylinux", "oraclelinux",
		"tencentos", "anolis", "eurolinux", "opencloudos", "openeuler",
		"springdale", "nethserver", "ovirt-node", "koozali", "ultramarine",
		"nobara", "risios", "securityonion", "freepbx", "elastix",
		"issabel", "fedoracore":
		// The whole RHEL-family tree runs anaconda — kickstart is
		// distro-agnostic, only the ISO layout is shared. Ultramarine's
		// resolver prefers the -anaconda ISO variant over readymade.
		// Fedora Core 1-6 predates the family but runs the same
		// installer; Security Onion ships an Oracle-Linux anaconda.
		return "anaconda-kickstart"
	case "opensuse", "opensuse-slowroll":
		// MicroOS needs combustion, a different mechanism.
		if p.System.Release != "microos" {
			return "opensuse-autoyast"
		}
	case "opensuse-microos":
		// OpenStack-Cloud qcow2 — the only MicroOS variant with
		// cloud-init (NoCloud datasource included). The kvm-and-xen
		// combustion path was implemented but firstboot.target does
		// not fire on the current image (verified via fw_cfg and
		// config-drive boots: straight to login, no dev user).
		return "nocloud-image"
	case "proxmox-ve", "proxmox-backup-server", "proxmox-mail-gateway",
		"proxmox-datacenter-manager":
		return "proxmox-autoinstall"
	case "openbsd":
		return "openbsd-autoinstall"
	case "freebsd", "hardenedbsd":
		// HardenedBSD ships the stock bsdinstall menus — the same
		// serial navigation applies.
		return "freebsd-shell"
	case "void":
		return "void-xbps"
	case "gentoo":
		return "gentoo-stage3"
	case "flatcar":
		return "flatcar-ignition"
	case "fedora-coreos":
		// qcow2 + ignition via -fw_cfg — no install boot at all.
		// (fedoracore is legacy Fedora Core 1-6 — anaconda, handled
		// with the RHEL family above.)
		return "coreos-ignition"
	case "nixos":
		// Live root shell — serial-driven nixos-install.
		return "nixos-serial"
	case "alpine":
		// The unattended ssh_key URL param needs mkinitfs ≥ ~3.19.
		if alpineReleaseOK(p.System.Release) {
			return "alpine-unattended"
		}
	case "archlinux", "parabola":
		// No answer file exists — serial-driven pacstrap instead.
		// Parabola's ISO is an archiso twin whose pacstrap pulls its
		// own libre repos.
		return "arch-pacstrap"
	case "slackware":
		// Root shell on the install media + installpkg — serial.
		return "slackware-live"
	case "artixlinux":
		// basestrap is pacstrap's artix twin — serial.
		return "artix-basestrap"
	case "calculate-linux":
		return "calculate-clinstall"
	case "guix":
		// Same class as NixOS: declarative `guix system init` driven
		// over the serial root shell — kernel/initrd live under
		// /gnu/store paths parsed out of the ISO's grub.cfg.
		return "guix-serial"
	case "netbsd":
		// Serial sysinst on the boot-com companion ISO → shell →
		// manual set extraction (the documented headless path).
		return "netbsd-sets"
	case "vyos":
		// `install image` is a fixed question sequence — the live
		// config (dhcp+ssh+key) is copied into the installed image.
		return "vyos-live"
	case "openwrt":
		// Raw image, already-installed — serial console gives a root
		// shell; first-boot config (dropbear+dhcp) is driven there.
		return "openwrt-serial"
	case "mikrotik-chr":
		// CHR ships serial console — the first-login dialog is fixed;
		// ssh key + dhcp + identity land via a fetched .rsc.
		return "routeros-chr"
	}
	// Pre-imaged disks only get nocloud when they actually boot
	// cloud-init: the *-cloud entries plus a small whitelist. Other
	// raw/qcow2 images (fedora-coreos Ignition, dietpi, openwrt,
	// appliances…) would silently produce an unreachable VM.
	os := p.System.OS
	if strings.HasSuffix(os, "-cloud") || os == "amazonlinux" || os == "cirros" {
		return "nocloud-image"
	}
	// Everything else in the catalog is still useful: obtain the
	// image, build the conf and launch it for a visual install —
	// honest automation for calamares/live distros and self-booting
	// appliances alike. "manual" stays selectable for a hard stop.
	return "visual"
}

// alpineReleaseOK gates the unattended provider to releases whose
// mkinitfs supports the ssh_key boot param (≥ v3.19; "v3.XY" or
// "latest"-style releases pass, anything unparsable is refused).
func alpineReleaseOK(release string) bool {
	r := strings.TrimPrefix(release, "v")
	parts := strings.SplitN(r, ".", 3)
	if len(parts) < 2 {
		return release == "" || release == "latest"
	}
	minor, err := strconv.Atoi(parts[1])
	return parts[0] == "3" && err == nil && minor >= 19
}

// releaseAtLeast parses the leading "N.N[.P]" of a release tag and
// reports whether it is ≥ major.minor — used to gate flavours that
// swapped installers mid-history. Unknown tags ("latest", "") count
// as current, i.e. the newest installer.
func releaseAtLeast(release string, major, minor int) bool {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return true
	}
	a, errA := strconv.Atoi(parts[0])
	b, errB := strconv.Atoi(parts[1])
	if errA != nil || errB != nil {
		return true
	}
	return a > major || (a == major && b >= minor)
}

// catalogImageType resolves the entry's declared image_type from the
// shipped catalog — DataFile walks dev-tree and packaged layouts.
func catalogImageType(osName string) string {
	type entry struct {
		OS        string `json:"os"`
		ImageType string `json:"image_type"`
	}
	var cat struct {
		Entries []entry `json:"entries"`
	}
	b, err := os.ReadFile(dataFile("catalog", "data", "catalog.json"))
	if err != nil {
		return ""
	}
	if jsonUnmarshal(b, &cat) != nil {
		return ""
	}
	for _, e := range cat.Entries {
		if e.OS == osName {
			return e.ImageType
		}
	}
	return ""
}

// safeHostname mirrors quickget-vm's hostname scrubbing: hostnames
// must be DNS-safe or cloud-init/subiquity reject them.
func safeHostname(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '_', r == ' ', r == '.':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
