// tools/quickget-vm/conf_gen.go: guest-OS profile selection + .conf generation (mkconf)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// guestFor: the OS -> guest_os/IMAGE_TYPE case table.
func guestFor(osName, release string) (guest, imgType string) {
	switch {
	case osName == "batocera":
		return "batocera", "img"
	case osName == "custom":
		return osName, "iso"
	case osName == "dragonflybsd":
		return "dragonflybsd", "iso"
	case osName == "easyos" || osName == "openfyde":
		return "linux", "img"
	case osName == "freebsd" || osName == "ghostbsd":
		return "freebsd", "iso"
	case osName == "haiku" || osName == "freedos" || osName == "kolibrios" ||
		osName == "netbsd" || osName == "openbsd" || osName == "reactos" ||
		osName == "templeos":
		return osName, "iso"
	case osName == "macos":
		return "macos", "img"
	case osName == "openindiana":
		return "solaris", "iso"
	case strings.HasPrefix(osName, "ubuntu"):
		if !strings.Contains(release, "daily") {
			num := strings.ReplaceAll(release, ".", "")
			n := 0
			fmt.Sscanf(num, "%d", &n)
			if n > 0 && n < 1604 {
				return "linux_old", "iso"
			}
		}
		return "linux", "iso"
	case osName == "windows":
		return "windows", "iso"
	case osName == "windows-server":
		return "windows-server", "iso"
	default:
		return "linux", "iso"
	}
}

// osTweaks: the per-OS conf append block (config.sh case ${OS}).
func osTweaks(osName, release, edition string) []string {
	var t []string
	switch osName {
	case "alma", "centos-stream", "endless", "garuda", "gentoo", "kali", "nixos", "omarchy", "oraclelinux", "popos", "rockylinux":
		t = append(t, `disk_size="32G"`)
	case "archlinux":
		t = append(t, `secureboot="on"`, `tpm="on"`, `disk_size="32G"`)
	case "openindiana":
		t = append(t, `boot="legacy"`, `disk_size="32G"`)
	case "batocera":
		t = append(t, `disk_size="8G"`)
	case "bazzite":
		t = append(t, `disk_size="64G"`)
	case "qubes":
		t = append(t, `disk_size="128G"`)
	case "dragonflybsd", "haiku", "openbsd", "netbsd", "slackware", "slax", "tinycore":
		t = append(t, `boot="legacy"`)
	case "debian":
		if strings.Contains(release, "12") || strings.Contains(release, "13") {
			t = append(t, `tpm="on"`)
		}
		t = append(t, `secureboot="on"`)
	case "deepin":
		t = append(t, `disk_size="64G"`, `ram="4G"`)
	case "freedos":
		t = append(t, `boot="legacy"`, `disk_size="4G"`, `ram="256M"`)
	case "kolibrios":
		t = append(t, `boot="legacy"`, `disk_size="2G"`, `ram="128M"`)
	case "templeos":
		t = append(t, `boot="legacy"`, `disk_size="2G"`, `ram="512M"`)
	case "slint":
		t = append(t, `disk_size="50G"`)
	case "slitaz":
		t = append(t, `boot="legacy"`, `disk_size="4G"`, `ram="512M"`)
	case "ubuntu-server":
		t = append(t, `disk_size="10G"`, `ram="4G"`)
		if strings.Contains(release, "22.04") {
			t = append(t, `tpm="on"`)
		}
	case "vanillaos":
		t = append(t, `disk_size="64G"`)
	case "zorin":
		if edition == "education64" || edition == "edulite64" {
			t = append(t, `disk_size="32G"`)
		}
	case "reactos":
		t = append(t, `boot="legacy"`, `disk_size="12G"`, `ram="2048M"`)
	case "macos":
		t = append(t, `disk_size="128G"`, `macos_release="`+release+`"`)
		if release == "monterey" {
			t = append(t, "cpu_cores=2")
		}
	case "elementary":
		if release == "8.1" {
			t = append(t, `display="spice"`)
		}
	case "proxmox-ve":
		t = append(t, `disk_size="20G"`, `ram="4G"`)
	}
	if osName == "ubuntu" && strings.Contains(release, "daily") {
		t = append(t, `disk_size="32G"`)
	}
	if strings.HasPrefix(osName, "windows") {
		t = append(t, `disk_size="64G"`)
	}
	if osName == "windows" && release == "11" || osName == "windows-server" && release == "2022" {
		t = append(t, `tpm="on"`, `secureboot="off"`)
	}
	return t
}

var goArch = runtime.GOARCH

// hostArch: Bash NORMALISED_HOST_ARCH parity — quickget names
// (amd64/arm64), not QEMU's (x86_64/aarch64).
func hostArch() string {
	if goArch == "arm64" {
		return "arm64"
	}
	return "amd64"
}

func mkconf(osName, release, edition, imageFile, isoFile, dir, arch string, cloudInit ...bool) int {
	if dir == "" {
		dir = osName + "-" + release
		if edition != "" {
			dir += "-" + edition
		}
	}
	// Values below land inside key="value" conf lines: a quote or
	// newline would corrupt the file or inject extra settings.
	for _, v := range []string{dir, osName, release, edition, imageFile, isoFile, arch} {
		if strings.ContainsAny(v, "\"\n\r") {
			fmt.Printf("invalid VM configuration value %q\n", v)
			return 1
		}
	}
	guest, imgType := guestFor(osName, release)

	// Disk-image downloads override the media type: the file becomes the
	// VM disk itself rather than installer media.
	diskLine := `disk_img="` + dir + `/disk.qcow2"`
	typeLine := imgType + `="` + dir + `/` + imageFile + `"`
	diskFormat := "qcow2"
	switch strings.ToLower(imageFile) {
	case "":
		typeLine = ""
	default:
		switch {
		case strings.HasSuffix(strings.ToLower(imageFile), ".qcow2"):
			diskFormat = "qcow2"
			diskLine = `disk_img="` + dir + `/` + imageFile + `"`
			typeLine = ""
		case strings.HasSuffix(strings.ToLower(imageFile), ".vmdk"):
			diskFormat = "vmdk"
			diskLine = `disk_img="` + dir + `/` + imageFile + `"`
			typeLine = ""
		case strings.HasSuffix(strings.ToLower(imageFile), ".raw"), strings.HasSuffix(strings.ToLower(imageFile), ".img"):
			diskFormat = "raw"
			diskLine = `disk_img="` + dir + `/` + imageFile + `"`
			typeLine = ""
		case strings.HasSuffix(strings.ToLower(imageFile), ".vdi"):
			// Bash emits a dead `disk=` line for .vdi; disk_img is the
			// functional equivalent the launcher actually consumes.
			diskFormat = "vdi"
			diskLine = `disk_img="` + dir + `/` + imageFile + `"`
			typeLine = ""
		}
	}

	confFile := dir + ".conf"
	if _, err := os.Stat(confFile); err == nil {
		fmt.Println(confFile + " already exists")
		return 0
	}
	var b strings.Builder
	// resolve_quickemu parity: sibling/dev-tree quickemu wins, PATH is
	// checked inside FindTool, QG_BINDIR is the packaging override.
	qe := ""
	if p, err := qelib.FindTool("quickemu"); err == nil {
		qe = p
	}
	if qe == "" {
		// ./quickemu then $QG_BINDIR — the resolve_quickemu fallbacks
		// for running from a source tree or a custom install dir.
		if st, err := os.Stat("./quickemu"); err == nil && !st.IsDir() {
			qe, _ = filepath.Abs("./quickemu")
		}
	}
	if qe == "" {
		if bd := os.Getenv("QG_BINDIR"); bd != "" {
			if st, err := os.Stat(filepath.Join(bd, "quickemu")); err == nil && !st.IsDir() {
				qe = filepath.Join(bd, "quickemu")
			}
		}
	}
	if qe == "" {
		qe = "quickemu"
	}
	b.WriteString("#!" + qe + " --vm\n")
	b.WriteString("guest_os=\"" + guest + "\"\n")
	if arch != "" && arch != hostArch() {
		if arch == "arm64" {
			b.WriteString("arch=\"aarch64\"\n")
		} else {
			b.WriteString("arch=\"x86_64\"\n")
		}
	}
	b.WriteString(diskLine + "\n")
	if typeLine == "" {
		b.WriteString("disk_format=\"" + diskFormat + "\"\n")
	} else {
		b.WriteString(typeLine + "\n")
	}
	// Bash appends fixed_iso right after the base conf block (before the
	// per-OS tweaks land).
	if isoFile != "" {
		b.WriteString("fixed_iso=\"" + dir + "/" + isoFile + "\"\n")
	}
	for _, l := range osTweaks(osName, release, edition) {
		b.WriteString(l + "\n")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Println(err)
		return 1
	}
	if err := os.WriteFile(confFile, []byte(b.String()), 0755); err != nil {
		fmt.Println(err)
		return 1
	}
	fmt.Println(" - Setting " + confFile + " executable")
	// gen_seed_iso parity: --cloud-init appends seed="..." to the conf
	// and builds user-data/meta-data/seed.iso inside the VM dir.
	if len(cloudInit) > 0 && cloudInit[0] {
		if rc := genSeedISO(dir, confFile); rc != 0 {
			return rc
		}
	}
	fmt.Println("Making " + confFile)
	fmt.Println("\nTo start your " + osName + " virtual machine run:")
	if osName == "slint" {
		fmt.Println("    quickemu --vm " + confFile + "\nTo start Slint with braille support run:")
		fmt.Println("    quickemu --vm --braille --display sdl " + confFile)
	} else {
		fmt.Println("    quickemu --vm " + confFile)
	}
	return 0
}
