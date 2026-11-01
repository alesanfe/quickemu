package qespec

// e2e_provision_test.go — end-to-end coverage of the quickprovision
// pipeline over the principal VM families.
//
// Two layers:
//
//  TestProvisionPlanMatrix — ALWAYS ON and fully offline: every matrix
//  entry renders a profile, runs `quickprovision plan` and asserts the
//  resolved provider plus the step sequence the run will execute.
//  This validates the e2e contract without launching anything.
//
//  TestProvisionRunE2E — INERT unless explicitly enabled. It downloads
//  multi-GB install media, needs /dev/kvm (TCG installs are impractically
//  slow) and runs for tens of minutes per OS. Gate it on the Ubuntu/KVM
//  host:
//
//      QESPEC_PROVISION_E2E=1                     master switch
//      QESPEC_PROVISION_ONLY=ubuntu-server,nixos  optional subset
//
//  Left prepared but never run in CI/WSL — every test skips itself
//  when the gates are absent.

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// provisionCase describes one VM family to install end-to-end. The
// profile is generated (not the shipped testdata) so the matrix also
// exercises inferProvider — os/release/edition are the only inputs a
// user would give.
type provisionCase struct {
	name        string // test + profile name
	os          string
	release     string
	edition     string
	username    string   // guests with a fixed built-in user (cirros)
	wantProvide string   // provider inferProvider must resolve
	wantSteps   []string // steps that MUST appear (subset, in order)
	interactive bool     // visual launches end at launch — no ssh/verify
	needsFw     string   // "", "efi" — windows needs efi+swtpm
	minRAM      string   // guest memory floor for the installer
	minDisk     string   // installer needs real headroom
	installMin  int      // install timeout, minutes
	sshMin      int      // ssh-wait timeout, minutes
	notes       string   // documented risk, kept with the case
}

var provisionMatrix = []provisionCase{
	{
		name: "ubuntu-server", os: "ubuntu-server", release: "24.04",
		wantProvide: "ubuntu-autoinstall",
		wantSteps:   []string{"acquire-image", "await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "subiquity autoinstall via nocloud seed on extracted casper kernel",
	},
	{
		name: "ubuntu-desktop", os: "ubuntu", release: "24.04",
		wantProvide: "ubuntu-desktop-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		minRAM:      "6G", minDisk: "30G",
		installMin: 90, sshMin: 30,
		notes: "flutter installer ends with a serial marker, not poweroff",
	},
	{
		name: "debian-netinst", os: "debian", release: "latest", edition: "netinst",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "d-i preseed over slirp; netinst pulls packages from mirrors",
	},
	{
		name: "debian-cloud", os: "debian-cloud", release: "latest", edition: "genericcloud",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"acquire-image", "prepare", "launch", "wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "genericcloud qcow2 — no install boot, cloud-init only",
	},
	{
		name: "ubuntu-cloud", os: "ubuntu-cloud", release: "24.04",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "kvm qcow2 — cloud-init datasource seed",
	},
	{
		name: "fedora-server", os: "fedora", release: "44", edition: "Server",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "inst.ks fetched over slirp on extracted pxeboot kernel",
	},
	{
		name: "fedora-cloud", os: "fedora-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "GenericCloud qcow2 — cloud-init",
	},
	{
		name: "rockylinux", os: "rockylinux", release: "latest", edition: "minimal",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "anaconda; minimal edition still runs lorax pxeboot kernel",
	},
	{
		name: "alma-cloud", os: "alma-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "genericcloud qcow2 — cloud-init",
	},
	{
		name: "alpine", os: "alpine", release: "v3.22", edition: "virt",
		wantProvide: "alpine-unattended",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  45, sshMin: 20,
		minRAM: "1G", minDisk: "8G",
		notes: "unattended answers via kernel params; needs mkinitfs ≥3.19",
	},
	{
		name: "alpine-cloud", os: "alpine-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  20, sshMin: 15,
		notes: "alpine cloud image — cloud-init present",
	},
	{
		name: "kali", os: "kali", release: "current", edition: "netinst",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "30G",
		notes: "kali is d-i — rolling mirror keys in the preseed",
	},
	{
		name: "opensuse", os: "opensuse", release: "15.6",
		wantProvide: "opensuse-autoyast",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30, minDisk: "30G",
		notes: "autoyast= on cmdline; tumbleweed DVD is multi-GB",
	},
	{
		name: "archlinux", os: "archlinux", release: "latest",
		wantProvide: "arch-pacstrap",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "serial-driven pacstrap — archiso root shell",
	},
	{
		name: "nixos", os: "nixos", release: "latest", edition: "minimal",
		wantProvide: "nixos-serial",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 20, minDisk: "24G",
		notes: "serial-driven nixos-install with rendered configuration.nix",
	},
	{
		name: "guix", os: "guix", release: "latest",
		wantProvide: "guix-serial",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30, minDisk: "24G",
		notes: "grub.cfg-parsed store paths; serial getty layout unverified on real ISO",
	},
	{
		name: "freebsd", os: "freebsd", release: "latest", edition: "disc1",
		wantProvide: "freebsd-shell",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "bsdinstall menu nav via serial escapes — menu order is a risk",
	},
	{
		name: "openbsd", os: "openbsd", release: "latest",
		wantProvide: "openbsd-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "graphics=off drops kernel to com0; (A)utoinstall answer path",
	},
	{
		name: "netbsd", os: "netbsd", release: "latest",
		wantProvide: "netbsd-sets",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20, minRAM: "2G",
		notes: "boot-com companion ISO; sysinst lettered menu nav is a risk",
	},
	{
		name: "void", os: "void", release: "latest", edition: "glibc",
		wantProvide: "void-xbps",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "serial login root/voidlinux → xbps-install into /mnt",
	},
	{
		name: "gentoo", os: "gentoo", release: "latest", edition: "minimal",
		wantProvide: "gentoo-stage3",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 20, minDisk: "24G",
		notes: "stage3+getbinpkg — kernel/initrd name guesses are a risk",
	},
	{
		name: "flatcar", os: "flatcar", release: "latest",
		wantProvide: "flatcar-ignition",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "flatcar-install -i ignition.json over serial as core",
	},
	{
		name: "fedora-coreos", os: "fedora-coreos", release: "latest",
		wantProvide: "coreos-ignition",
		wantSteps:   []string{"launch", "wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "qcow2 + fw_cfg ignition — no install boot at all",
	},
	{
		name: "linuxmint", os: "linuxmint", release: "22.1", edition: "cinnamon",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "casper+automatic-ubiquity; initrd name variants are a risk",
	},
	{
		name: "windows", os: "windows", release: "11", needsFw: "efi",
		wantProvide: "windows-autounattend",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 45, minRAM: "6G", minDisk: "64G",
		notes: "autounattend.iso on a second cdrom; TPM2+EFI required",
	},
	{
		name: "windows-server", os: "windows-server", release: "2022", needsFw: "efi",
		wantProvide: "windows-autounattend",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 45, minRAM: "4G", minDisk: "64G",
		notes: "same unattend path; ssh comes from provisioned openssh",
	},
	{
		name: "proxmox-ve", os: "proxmox-ve", release: "latest", needsFw: "efi",
		wantProvide: "proxmox-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minRAM: "4G", minDisk: "32G",
		notes: "answer.toml on the fetched ISO via qemu-img embedding; webhook completion",
	},
	{
		name: "macos", os: "macos", release: "sonoma",
		wantProvide: "visual", interactive: true,
		wantSteps:  []string{"acquire-image", "prepare", "launch"},
		installMin: 10,
		notes:      "recoveryOS is interactive — run ends at launch by design",
	},

	// --- remaining cloud-init images (nocloud-image) -------------------
	{
		name: "centos-stream-cloud", os: "centos-stream-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "genericcloud qcow2 — cloud-init",
	},
	{
		name: "rocky-cloud", os: "rocky-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "genericcloud qcow2 — cloud-init",
	},
	{
		name: "opensuse-cloud", os: "opensuse-cloud", release: "tumbleweed",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "JeOS qcow2 ships cloud-init",
	},
	{
		name: "arch-cloud", os: "arch-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "arch cloud image ships cloud-init",
	},
	{
		name: "amazonlinux", os: "amazonlinux", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "AL2023 kvm image — cloud-init",
	},
	{
		name: "cirros", os: "cirros", release: "latest", username: "cirros",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  15, sshMin: 10, minRAM: "512M", minDisk: "1G",
		notes: "tiny (~20MB) — the cheapest full-pipeline e2e; cirros ssh user",
	},

	// --- remaining d-i family ------------------------------------------
	{
		name: "debian-edu", os: "debian-edu", release: "latest",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "edu metapackage question is preseeded",
	},
	{
		name: "trisquel", os: "trisquel", release: "12.0", edition: "netinst",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "netinst ISO runs d-i; trisquel mirror keys — DE editions are live",
	},

	// --- remaining RHEL-family anaconda (kickstart) ---------------------
	{
		name: "alma", os: "alma", release: "latest", edition: "dvd",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "dvd edition → cdrom kickstart source",
	},
	{
		name: "centos-stream", os: "centos-stream", release: "latest", edition: "dvd1",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "dvd1 → cdrom source",
	},
	{
		name: "centos", os: "centos", release: "8.5.2111",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "legacy release — vault mirrors may need %pre tweaks",
	},
	{
		name: "oraclelinux", os: "oraclelinux", release: "latest",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "anaconda via yum.oracle.com ISO",
	},
	{
		name: "tencentos", os: "tencentos", release: "latest", edition: "minimal",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "tencent anaconda; mirrors on Tencent CDN",
	},
	{
		name: "anolis", os: "anolis", release: "latest", edition: "minimal",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "openanolis anaconda fork",
	},
	{
		name: "eurolinux", os: "eurolinux", release: "latest", edition: "minimal",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "RHEL clone — anaconda",
	},
	{
		name: "opencloudos", os: "opencloudos", release: "latest", edition: "minimal",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "Tencent/Intel community RHEL derivative",
	},
	{
		name: "openeuler", os: "openeuler", release: "latest", edition: "netinst",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30,
		notes: "openEuler's anaconda fork — inst.ks support unverified on real ISO",
	},
	{
		name: "springdale", os: "springdale", release: "latest",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "Princeton RHEL rebuild",
	},
	{
		name: "nethserver", os: "nethserver", release: "latest",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30, minDisk: "25G",
		notes: "appliance anaconda; post-install is a specialized image",
	},
	{
		name: "ovirt-node", os: "ovirt-node", release: "latest", edition: "el9",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30,
		notes: "oVirt node appliance — product ks extensions are a risk",
	},
	{
		name: "koozali", os: "koozali", release: "latest", edition: "standard",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30, minDisk: "25G",
		notes: "SME Server — CentOS anaconda with product hooks",
	},
	{
		name: "ultramarine", os: "ultramarine", release: "latest",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "scrape prefers -anaconda ISOs; readymade variants skipped",
	},

	// --- remaining autoyast ---------------------------------------------
	{
		name: "opensuse-slowroll", os: "opensuse-slowroll", release: "latest",
		wantProvide: "opensuse-autoyast",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30, minDisk: "30G",
		notes: "slowroll DVD — same autoyast mechanism as tumbleweed",
	},

	// --- remaining serial-driven installers -----------------------------
	{
		name: "parabola", os: "parabola", release: "latest",
		wantProvide: "arch-pacstrap",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "archiso twin — pacstrap pulls libre repos",
	},
	{
		name: "slackware", os: "slackware", release: "latest",
		wantProvide: "slackware-live",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 20, minRAM: "2G",
		notes: "serial root shell → installpkg all series; hugesmp.s kernel arg",
	},
	{
		name: "artixlinux", os: "artixlinux", release: "latest", edition: "base-openrc",
		wantProvide: "artix-basestrap",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "basestrap over serial; ISO boot-file layout is the risk",
	},
	{
		name: "calculate-linux", os: "calculate-linux", release: "latest",
		wantProvide: "calculate-clinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 20, minDisk: "25G",
		notes: "cl-install -d/--disk documented non-interactive",
	},

	// --- remaining ubiquity ---------------------------------------------
	{
		name: "zorin", os: "zorin", release: "17", edition: "core64",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30, minDisk: "30G",
		notes: "zorin's ubiquity fork — same automatic-ubiquity path",
	},
	{
		name: "ubuntucinnamon", os: "ubuntucinnamon", release: "latest",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "cinnamon kept ubiquity past the flutter migration",
	},
	{
		name: "xubuntu-legacy", os: "xubuntu", release: "22.04.5",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "release-gated: ≤23.04 still ships ubiquity",
	},
	{
		name: "ubuntu-mate-legacy", os: "ubuntu-mate", release: "22.04.5",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "release-gated: ≤23.04 still ships ubiquity",
	},
	{
		name: "bodhi", os: "bodhi", release: "latest", edition: "standard",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "Moksha on Ubuntu LTS — ubiquity",
	},

	// --- remaining proxmox appliances -----------------------------------
	{
		name: "proxmox-backup-server", os: "proxmox-backup-server", release: "latest",
		needsFw:     "efi",
		wantProvide: "proxmox-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minRAM: "4G", minDisk: "32G",
		notes: "same answer.toml + webhook flow as pve",
	},
	{
		name: "proxmox-mail-gateway", os: "proxmox-mail-gateway", release: "latest",
		needsFw:     "efi",
		wantProvide: "proxmox-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minRAM: "4G", minDisk: "32G",
		notes: "same answer.toml + webhook flow as pve",
	},
	{
		name: "proxmox-datacenter-manager", os: "proxmox-datacenter-manager",
		release:     "latest",
		needsFw:     "efi",
		wantProvide: "proxmox-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minRAM: "4G", minDisk: "32G",
		notes: "alpha-stage appliance — answer.toml schema may drift",
	},

	// --- ubiquity family: Ubuntu/Mint-derived respins that never
	// shipped the flutter installer ---
	{
		name: "linuxlite", os: "linuxlite", release: "7.6",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "Ubuntu LTS base on ubiquity",
	},
	{
		name: "backbox", os: "backbox", release: "9",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "Ubuntu-based pentest respin on ubiquity",
	},
	{
		name: "mythbuntu", os: "mythbuntu", release: "16.04",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "frozen at 16.04 — old ubiquity, still preseedable",
	},
	{
		name: "hamonikr", os: "hamonikr", release: "7.0", edition: "lts",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "Linux Mint-based Korean respin — mint's ubiquity",
	},
	{
		name: "ubuntukylin-legacy", os: "ubuntukylin", release: "20.04.6",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "release-gate legacy side — all catalog releases are ≤20.04",
	},
	{
		name: "ubuntukylin-current", os: "ubuntukylin", release: "24.04",
		wantProvide: "ubuntu-desktop-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minRAM: "6G",
		notes: "release-gate modern side — kylin migrated with xubuntu/mate",
	},

	// --- anaconda family: Fedora/CentOS-derived ISOs ---
	{
		name: "nobara", os: "nobara", release: "44", edition: "Official",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minRAM: "6G", minDisk: "30G",
		notes: "gaming-fedora anaconda ISO",
	},
	{
		name: "risios", os: "risios", release: "latest",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minRAM: "6G", minDisk: "30G",
		notes: "Fedora remix with anaconda",
	},
	{
		name: "securityonion", os: "securityonion", release: "latest",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 45, minRAM: "8G", minDisk: "100G",
		notes: "Oracle-Linux anaconda base — SO services belong to provision",
	},
	{
		name: "freepbx", os: "freepbx", release: "17",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "Sangoma distro is CentOS-anaconda",
	},
	{
		name: "elastix", os: "elastix", release: "latest",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "CentOS7-era anaconda — oldest supported ks syntax",
	},
	{
		name: "issabel", os: "issabel", release: "latest",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "CentOS7-era anaconda (Elastix fork)",
	},
	{
		name: "fedoracore", os: "fedoracore", release: "6",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  45, sshMin: 30,
		notes: "legacy Fedora Core — anaconda predates RHEL5, pxeboot " +
			"layout present but oldest ks dialect",
	},

	// --- debian-installer family ---
	{
		name: "parrotsec", os: "parrotsec", release: "7.4", edition: "security",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "Parrot's installer is d-i derived",
	},
	{
		name: "canaima", os: "canaima", release: "9.0", edition: "gnome",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "Venezuelan Debian derivative — d-i",
	},
	{
		name: "bosslinux", os: "bosslinux", release: "8.0",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "CDAC's Debian derivative — d-i",
	},
	{
		name: "openmediavault", os: "openmediavault", release: "8.3.1",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "d-i ISO whose baked preseed we override — OMV packages " +
			"belong to the provision stage",
	},
	{
		name: "yunohost", os: "yunohost", release: "latest",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "same shape as OMV — Debian base + yunohost layer via provision",
	},
	{
		name: "turnkeylinux", os: "turnkeylinux", release: "18.1", edition: "core",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "di-live frontend — preseed applies but the live-installer " +
			"path is the risk",
	},
	{
		name: "turnkey", os: "turnkey", release: "latest", edition: "core",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "upstream twin of turnkeylinux",
	},

	// --- freebsd family ---
	{
		name: "hardenedbsd", os: "hardenedbsd", release: "14-stable",
		wantProvide: "freebsd-shell",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "stock bsdinstall menus on the hardened fork",
	},

	// --- network-OS serial drivers ---
	{
		name: "vyos", os: "vyos", release: "latest",
		wantProvide: "vyos-live",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 15,
		notes: "install-image dialog driven over serial; prompt order " +
			"varies between rolling builds",
	},
	{
		name: "openwrt", os: "openwrt", release: "latest",
		wantProvide: "openwrt-serial",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  20, sshMin: 15,
		notes: "raw image — x86 build enables ttyS0; first-boot script " +
			"does uci/dropbear then powers off",
	},
	{
		name: "mikrotik-chr", os: "mikrotik-chr", release: "latest",
		wantProvide: "routeros-chr",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  20, sshMin: 15,
		notes: "first-login dialog (license/password) varies across " +
			"RouterOS releases",
	},
}

// ---------- always-on, offline: the matrix must resolve ----------

func TestProvisionPlanMatrix(t *testing.T) {
	bin := Bin(t, "quickprovision")
	for _, tc := range provisionMatrix {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			prof := writeProvisionProfile(t, tc)
			r := RunArgv(t, bin, nil, "plan", "--profile", prof)
			env := r.Envelope(t)
			if ok, _ := env["success"].(bool); !ok {
				t.Fatalf("plan failed: %s", r.Stdout)
			}
			data, _ := env["data"].(map[string]any)
			if got := data["provider"]; got != tc.wantProvide {
				t.Fatalf("provider=%v want %s", got, tc.wantProvide)
			}
			steps, _ := data["steps"].([]any)
			var ids []string
			for _, s := range steps {
				ids = append(ids, fmt.Sprint(s))
			}
			assertStepSubset(t, ids, tc.wantSteps)
			if tc.interactive && len(ids) != 4 {
				t.Fatalf("visual pipeline should be 4 steps, got %v", ids)
			}
		})
	}
}

// assertStepSubset requires want to appear in got in order.
func assertStepSubset(t *testing.T, got, want []string) {
	t.Helper()
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i != len(want) {
		t.Fatalf("steps %v missing ordered subset %v", got, want)
	}
}

// writeProvisionProfile renders the minimal profile a user would give —
// provider left unset so inference is exercised per case.
func writeProvisionProfile(t *testing.T, tc provisionCase) string {
	t.Helper()
	ram, disk := tc.minRAM, tc.minDisk
	if ram == "" {
		ram = "4G"
	}
	if disk == "" {
		disk = "20G"
	}
	install, ssh := tc.installMin, tc.sshMin
	if install == 0 {
		install = 90
	}
	if ssh == 0 {
		ssh = 30
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[profile]\nname = %q\n", tc.name)
	fmt.Fprintf(&b, "[system]\nos = %q\n", tc.os)
	if tc.release != "" {
		fmt.Fprintf(&b, "release = %q\n", tc.release)
	}
	if tc.edition != "" {
		fmt.Fprintf(&b, "edition = %q\n", tc.edition)
	}
	fmt.Fprintf(&b, "arch = \"amd64\"\n")
	fmt.Fprintf(&b, "[machine]\ncpus = 2\nram = %q\ndisk_size = %q\n", ram, disk)
	if tc.needsFw == "efi" {
		b.WriteString("boot = \"efi\"\n")
	}
	host := strings.ReplaceAll(tc.name, "_", "-")
	user := tc.username
	if user == "" {
		user = "dev"
	}
	fmt.Fprintf(&b, "[identity]\nhostname = %q\nusername = %q\n", host, user)
	if strings.HasPrefix(tc.os, "windows") {
		// unattend creates the user with a real password — an empty one
		// would leave a locked account the provision stage cannot log
		// into over WinRM/ssh.
		b.WriteString("password = \"Prov#2026x\"\n")
	}
	b.WriteString("[automation]\n")
	fmt.Fprintf(&b, "install_timeout_min = %d\nssh_timeout_min = %d\n", install, ssh)
	b.WriteString("[[verify]]\nname = \"ssh-alive\"\nrun = \"true\"\n")
	// uname -n over hostname — minimal images (arch-cloud) lack the
	// hostname binary entirely.
	fmt.Fprintf(&b, "[[verify]]\nname = \"hostname\"\nrun = \"uname -n\"\nexpect = %q\n",
		host)
	b.WriteString("[finalize]\nshutdown = true\nsnapshot = \"prepared\"\n")
	dir := t.TempDir()
	f := filepath.Join(dir, "profile.toml")
	if err := os.WriteFile(f, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// ---------- gated: real downloads + KVM boots — prepared, not run ----

func provisionE2EEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("QESPEC_PROVISION_E2E") != "1" {
		t.Skip("provision e2e inert — set QESPEC_PROVISION_E2E=1 on a KVM host")
	}
	// Writable check — /dev/kvm can exist but be root-only.
	if !KVMUsable() {
		t.Skip("/dev/kvm unusable — TCG installs are impractically slow")
	}
	// WAN probe: downloads are multi-GB; fail fast instead of hanging.
	c, err := net.DialTimeout("tcp", "deb.debian.org:443", 5*time.Second)
	if err != nil {
		t.Skip("no outbound network")
	}
	c.Close()
}

// provisionWorkDir picks a run dir with room for multi-GB ISOs and
// qcow2s — t.TempDir() can land on tmpfs and OOM the host.
func provisionWorkDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("QESPEC_PROVISION_DIR"); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("QESPEC_PROVISION_DIR %s: %v", d, err)
		}
		work, err := os.MkdirTemp(d, "prov-e2e-*")
		if err != nil {
			t.Fatal(err)
		}
		if os.Getenv("QESPEC_PROVISION_KEEP") != "1" {
			t.Cleanup(func() { os.RemoveAll(work) })
		}
		return work
	}
	return t.TempDir()
}

func provisionOnly() map[string]bool {
	only := map[string]bool{}
	for _, s := range strings.Split(os.Getenv("QESPEC_PROVISION_ONLY"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			only[s] = true
		}
	}
	return only
}

func TestProvisionRunE2E(t *testing.T) {
	provisionE2EEnabled(t)
	bin := Bin(t, "quickprovision")
	only := provisionOnly()
	for _, tc := range provisionMatrix {
		tc := tc
		if len(only) > 0 && !only[tc.name] {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			work := provisionWorkDir(t)
			prof := writeProvisionProfile(t, tc)

			// 1. plan — the run must see the same provider the matrix asserts.
			r := RunArgv(t, bin, nil, "plan", "--profile", prof)
			data, _ := r.Envelope(t)["data"].(map[string]any)
			if data["provider"] != tc.wantProvide {
				t.Fatalf("provider=%v want %s", data["provider"], tc.wantProvide)
			}

			// 2. run — JSONL events on stdout; the runner is kill-safe
			// on timeout so a stuck install never orphans a QEMU.
			r = RunArgv(t, bin, []RunOpt{
				InDir(work),
				Timeout(time.Duration(tc.installMin+tc.sshMin+30) * time.Minute),
			}, "run", "--profile", prof, "--dir", work)
			evs := parseRunEvents(r.Stdout)

			// 3. fast-fail on an explicit failure before asserting the
			// step contract — a crashed/aborted run otherwise reports as
			// a confusing "missing step" or an index panic on empty evs.
			if len(evs) == 0 {
				t.Fatalf("no events (rc=%d) — tail: %s\nstderr: %s",
					r.Code, stdoutTail(r.Stdout, 8), stdoutTail(r.Stderr, 20))
			}
			for _, e := range evs {
				if e["event"] == "run.failed" {
					d := eventData(e)
					t.Fatalf("run.failed step=%v code=%v err=%v — tail: %s\nstderr: %s",
						d["step"], d["code"], d["error"], stdoutTail(r.Stdout, 8),
						stdoutTail(r.Stderr, 20))
				}
			}

			// 4. the step sequence must contain the ordered contract.
			var done []string
			for _, e := range evs {
				if e["event"] == "step.completed" {
					done = append(done, eventStep(e))
				}
			}
			assertStepSubset(t, done, tc.wantSteps)
			if tc.interactive {
				// visual launches complete the run at launch — no ssh.
				if last := evs[len(evs)-1]["event"]; last != "run.completed" {
					t.Fatalf("visual run ended on %v", last)
				}
				return
			}

			// 5. verify results all passed; run completed.
			for _, e := range evs {
				if e["event"] != "verify.result" {
					continue
				}
				if pass, _ := eventData(e)["pass"].(bool); !pass {
					t.Errorf("verify %v failed: %v", eventData(e)["name"], e)
				}
			}
			if last := evs[len(evs)-1]["event"]; last != "run.completed" {
				t.Fatalf("run ended on %v — tail: %s", last, stdoutTail(r.Stdout, 8))
			}

			// 6. the sealed snapshot must be visible to quickemu —
			// the .conf sits beside its vm dir in the standard layout.
			confs, _ := filepath.Glob(filepath.Join(work, "*.conf"))
			nested, _ := filepath.Glob(filepath.Join(work, "*", "*.conf"))
			confs = append(confs, nested...)
			if len(confs) == 0 {
				t.Fatalf("no VM conf under %s", work)
			}
			conf := confs[0]
			// Cleanup runs even when assertions above fail — honoring
			// QESPEC_PROVISION_KEEP for post-mortem inspection.
			t.Cleanup(func() {
				if os.Getenv("QESPEC_PROVISION_KEEP") == "1" {
					t.Logf("KEEP=1: VM left at %s", conf)
					return
				}
				Run(t, "quickemu", nil, "--vm", conf, "--delete-vm", "--trash")
			})
			Run(t, "quickemu", nil, "--vm", conf, "--snapshot-list").
				Contains(t, "prepared")
		})
	}
}

// parseRunEvents decodes the run's JSONL stream (one event per line).
// Records look like {"seq":n,"event":"step.completed","data":{…}}.
func parseRunEvents(out string) []map[string]any {
	var evs []map[string]any
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || !strings.HasPrefix(l, "{") {
			continue
		}
		var e map[string]any
		if json.Unmarshal([]byte(l), &e) == nil {
			evs = append(evs, e)
		}
	}
	return evs
}

// eventData digs the data map out of a parsed event record.
func eventData(e map[string]any) map[string]any {
	d, _ := e["data"].(map[string]any)
	return d
}

// eventStep is the step id of a step.* event ("" for anything else).
func eventStep(e map[string]any) string {
	s, _ := eventData(e)["step"].(string)
	return s
}

// stdoutTail returns the last n lines for failure diagnostics.
func stdoutTail(out string, n int) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// TestProvisionMatrixCatalog guards the matrix against typos: every
// os/release/edition a case claims must exist in the shipped catalog —
// a case for a nonexistent entry would still plan (inference never
// validates) and then fail only in the gated run nobody executes.
func TestProvisionMatrixCatalog(t *testing.T) {
	var cat struct {
		Entries []struct {
			OS       string   `json:"os"`
			Releases []string `json:"releases"`
			Editions []string `json:"editions"`
		} `json:"entries"`
	}
	b, err := os.ReadFile(filepath.Join(Root(), "catalog", "data", "catalog.json"))
	if err != nil {
		t.Skip("catalog not reachable")
	}
	if err := json.Unmarshal(b, &cat); err != nil {
		t.Fatal(err)
	}
	type entry struct {
		releases map[string]bool
		editions map[string]bool
	}
	idx := map[string]entry{}
	for _, e := range cat.Entries {
		r := map[string]bool{}
		for _, v := range e.Releases {
			r[v] = true
		}
		ed := map[string]bool{}
		for _, v := range e.Editions {
			ed[v] = true
		}
		idx[e.OS] = entry{releases: r, editions: ed}
	}
	for _, tc := range provisionMatrix {
		t.Run("catalog/"+tc.name, func(t *testing.T) {
			e, ok := idx[tc.os]
			if !ok {
				t.Fatalf("os %q not in catalog", tc.os)
			}
			// releases/editions may be scrape-resolved (empty in
			// catalog) — only assert when the entry pins them.
			if tc.release != "" && len(e.releases) > 0 && !e.releases[tc.release] &&
				tc.release != "latest" {
				t.Errorf("%s release %q not in catalog %v",
					tc.os, tc.release, e.releases)
			}
			if tc.edition != "" && len(e.editions) > 0 && !e.editions[tc.edition] {
				t.Errorf("%s edition %q not in catalog %v",
					tc.os, tc.edition, e.editions)
			}
		})
	}
}
