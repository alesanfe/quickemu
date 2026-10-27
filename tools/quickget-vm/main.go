// quickget-vm: make_vm_config parity — generates the VM .conf next to
// a downloaded image, with the same guest_os/image-type mapping, disk
// overrides and per-OS tweaks as lib/quickget/vm/config.sh.
//
// usage: quickget-vm mkconf --os <os> --release <r> --edition <e>
//
//	--image <file> [--iso <file>] [--dir <vmdir>]
//	[--arch arm64|x86_64] [--unattended] [--cloud-init]
package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func hostArch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "x86_64"
}

func mkconf(osName, release, edition, imageFile, isoFile, dir, arch string, cloudInit ...bool) int {
	if dir == "" {
		dir = osName + "-" + release
		if edition != "" {
			dir += "-" + edition
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
	qe, _ := exec.LookPath("quickemu")
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
	for _, l := range osTweaks(osName, release, edition) {
		b.WriteString(l + "\n")
	}
	if isoFile != "" {
		b.WriteString("fixed_iso=\"" + dir + "/" + isoFile + "\"\n")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(confFile, []byte(b.String()), 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Making " + confFile)
	fmt.Println("\nTo start your " + osName + " virtual machine run:")
	fmt.Println("    quickemu --vm " + confFile)
	return 0
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickget-vm <mkconf|fetch> ...")
		os.Exit(2)
	}
	if os.Args[1] == "fetch" {
		var url, dest, sha string
		for i := 2; i < len(os.Args); i++ {
			v := ""
			if i+1 < len(os.Args) {
				v = os.Args[i+1]
			}
			switch os.Args[i] {
			case "--url":
				url = v
			case "--dest":
				dest = v
			case "--sha256":
				sha = v
			}
			i++
		}
		if url == "" || dest == "" {
			fmt.Fprintln(os.Stderr, "fetch: --url and --dest are required")
			os.Exit(2)
		}
		os.Exit(fetch(url, dest, sha))
	}
	if os.Args[1] == "get" {
		var osName, release, edition, dir, arch string
		pos := []string{}
		for i := 2; i < len(os.Args); i++ {
			if strings.HasPrefix(os.Args[i], "--") && i+1 < len(os.Args) {
				switch os.Args[i] {
				case "--dir":
					dir = os.Args[i+1]
				case "--arch":
					arch = os.Args[i+1]
				case "--cloud-init":
					cloudInitFlag = true
					i--
				case "--no-conf":
					noConf = true
					i--
				case "--unattended":
					unattendedFlag = true
					i--
				}
				i++
			} else {
				pos = append(pos, os.Args[i])
			}
		}
		if len(pos) > 0 {
			osName = pos[0]
		}
		if len(pos) > 1 {
			release = pos[1]
		}
		if len(pos) > 2 {
			edition = pos[2]
		}
		if osName == "" {
			fmt.Fprintln(os.Stderr, "get: <os> [release] [edition] required")
			os.Exit(2)
		}
		os.Exit(getOS(osName, release, edition, dir, arch))
	}
	if os.Args[1] == "customconf" {
		// create_config parity: quickget-vm customconf <vm-path> <input>
		// [--unattended|--disable-unattended]
		var vmPath, input, flag string
		pos := []string{}
		for _, a := range os.Args[2:] {
			if strings.HasPrefix(a, "--") {
				flag = a
			} else {
				pos = append(pos, a)
			}
		}
		if len(pos) > 0 {
			vmPath = pos[0]
		}
		if len(pos) > 1 {
			input = pos[1]
		}
		os.Exit(customConf(vmPath, input, flag))
	}
	if os.Args[1] != "mkconf" {
		fmt.Fprintln(os.Stderr, "usage: quickget-vm <get|mkconf|fetch|customconf> ...")
		os.Exit(2)
	}
	var osName, release, edition, image, iso, dir, arch string
	cloudInit := false
	for i := 2; i < len(os.Args); i++ {
		v := ""
		if i+1 < len(os.Args) {
			v = os.Args[i+1]
		}
		switch os.Args[i] {
		case "--os":
			osName = v
		case "--release":
			release = v
		case "--edition":
			edition = v
		case "--image":
			image = v
		case "--iso":
			iso = v
		case "--dir":
			dir = v
		case "--arch":
			arch = v
		case "--cloud-init":
			cloudInit = true
			i--
		default:
			continue
		}
		i++
	}
	if osName == "" || image == "" {
		fmt.Fprintln(os.Stderr, "mkconf: --os and --image are required")
		os.Exit(2)
	}
	_ = filepath.Separator
	os.Exit(mkconf(osName, release, edition, image, iso, dir, arch, cloudInit))
}

// fetch: web_get/check_hash parity — download url -> dest via a .part
// staging file (atomic rename), resume a partial download with the
// Range header, and verify sha256 when provided. Uses net/http so no
// curl/zsync dependency is required.
func fetch(url, dest, wantSHA string) int {
	part := dest + ".part"
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "download failed: HTTP %d\n", resp.StatusCode)
		return 1
	}
	// Server ignored Range -> restart the staging file.
	if have > 0 && resp.StatusCode != http.StatusPartialContent {
		have = 0
	}
	flag := os.O_CREATE | os.O_WRONLY
	if have > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(part, flag, 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	f.Close()
	if wantSHA != "" {
		b, err := os.ReadFile(part)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(b))
		if !strings.EqualFold(sum, wantSHA) {
			os.Remove(part)
			fmt.Fprintf(os.Stderr, "checksum mismatch: got %s want %s\n", sum, wantSHA)
			return 1
		}
	}
	if err := os.Rename(part, dest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(" - Downloaded " + dest)
	return 0
}

// getOS: the quickget pipeline — resolve url+hash via the catalog
// tool, fetch the checksum file, fetch+verify the image, then mkconf.
// quickget-catalog is our own Go binary; zsync delegates to the
// external zsync binary only for .zsync sources (Bash zsync_get).
var cloudInitFlag, noConf, unattendedFlag bool

func getOS(osName, release, edition, dir, arch string) int {
	cat, err := exec.LookPath("quickget-catalog")
	if err != nil {
		fmt.Fprintln(os.Stderr, "get: quickget-catalog not on PATH")
		return 1
	}
	args := []string{"url", osName}
	if release != "" {
		args = append(args, release)
	}
	if edition != "" {
		args = append(args, edition)
	}
	if arch != "" {
		// url takes [arch] as the 4th positional arg.
		for len(args) < 4 {
			args = append(args, "")
		}
		args = append(args, arch)
	}
	out, err := exec.Command(cat, args...).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "get: url resolution failed for %s %s %s\n", osName, release, edition)
		return 1
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) == 0 {
		fmt.Fprintln(os.Stderr, "get: empty url")
		return 1
	}
	url := parts[0]
	hashURL := ""
	if len(parts) > 1 {
		hashURL = parts[1]
	}
	if dir == "" {
		dir = osName + "-" + release
		if edition != "" {
			dir += "-" + edition
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	image := filepath.Join(dir, filepath.Base(url))
	// checksum verification: fetch the hash file and grep the line for
	// this image's basename (check_hash parity — SHA256SUMS format).
	sha := ""
	if hashURL != "" {
		hf := filepath.Join(dir, ".hashsums")
		if fetch(hashURL, hf, "") == 0 {
			if b, err := os.ReadFile(hf); err == nil {
				base := filepath.Base(url)
				for _, l := range strings.Split(string(b), "\n") {
					f := strings.Fields(l)
					if len(f) == 2 && strings.HasSuffix(f[1], base) {
						sha = f[0]
						break
					}
				}
			}
			os.Remove(hf)
		}
	}
	// zsync sources prefer the external zsync binary (delta download);
	// otherwise plain fetch handles .zsync URLs as a normal download.
	if strings.HasSuffix(url, ".zsync") {
		if z, err := exec.LookPath("zsync"); err == nil {
			cmd := exec.Command(z, "-o", image, url)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if cmd.Run() != nil {
				return 1
			}
		} else if fetch(url, image, sha) != 0 {
			return 1
		}
	} else if fetch(url, image, sha) != 0 {
		return 1
	}
	if noConf {
		return 0
	}
	// image_normalize: decompress/convert the artifact per-OS and pick
	// up the final image name inside the VM dir.
	imageName := imageNormalize(osName, filepath.Base(image), dir)
	// windows/windows-server: fetch virtio-win.iso + build the
	// unattended install media for retail releases 10/11 (get.sh flow).
	fixedISO := ""
	if osName == "windows" || osName == "windows-server" {
		virtio := filepath.Join(dir, "virtio-win.iso")
		if _, err := os.Stat(virtio); err != nil {
			fmt.Println("Downloading VirtIO drivers...")
			fetch("https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso", virtio, "")
		}
		fixedISO = "virtio-win.iso"
		if release == "10" || release == "11" {
			os.Remove(filepath.Join(dir, "unattended.iso"))
			unattendedWindows(dir, "w"+release, "")
		}
	}
	rc := mkconf(osName, release, edition, imageName, fixedISO, dir, arch, cloudInitFlag)
	if rc != 0 {
		return rc
	}
	// --unattended: ubuntu-server subiquity seed (config.sh flow).
	if unattendedFlag {
		if osName == "ubuntu-server" {
			confFile := dir + ".conf"
			return genUbuntuAutoinstall(dir, filepath.Join(dir, imageName), confFile)
		}
		fmt.Fprintln(os.Stderr, "WARNING! --unattended is only supported for ubuntu-server; ignoring.")
	}
	return 0
}

// genSeedISO: gen_seed_iso parity — write user-data + meta-data and
// build a NoCloud seed.iso via the first available tool
// (cloud-localds > genisoimage > xorrisofs > mkisofs), then append
// seed="..." to the conf.
func genSeedISO(vmPath, confFile string) int {
	userName := os.Getenv("USER")
	if userName == "" {
		userName = "quickemu"
	}
	sshKey := ""
	for _, k := range []string{"id_ed25519.pub", "id_rsa.pub"} {
		if b, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".ssh", k)); err == nil {
			sshKey = strings.TrimSpace(string(b))
			break
		}
	}
	userData := filepath.Join(vmPath, "user-data")
	var ud strings.Builder
	ud.WriteString("#cloud-config\nhostname: " + vmPath + "\nmanage_etc_hosts: true\nusers:\n  - name: " + userName +
		"\n    sudo: ALL=(ALL) NOPASSWD:ALL\n    groups: sudo\n    shell: /bin/bash\n    lock_passwd: false\n    plain_text_passwd: quickemu\n")
	if sshKey != "" {
		ud.WriteString("    ssh_authorized_keys:\n      - " + sshKey + "\n")
	}
	ud.WriteString("ssh_pwauth: true\npackage_update: true\n")
	if err := os.WriteFile(userData, []byte(ud.String()), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	metaData := filepath.Join(vmPath, "meta-data")
	if _, err := os.Stat(metaData); err != nil {
		os.WriteFile(metaData, []byte("instance-id: "+vmPath+"\nlocal-hostname: "+vmPath+"\n"), 0644)
	}
	seed := filepath.Join(vmPath, "seed.iso")
	type tool struct {
		name string
		args []string
	}
	candidates := []tool{
		{"cloud-localds", []string{seed, userData, metaData}},
		{"genisoimage", []string{"-output", seed, "-volid", "cidata", "-joliet", "-rock", userData, metaData}},
		{"xorrisofs", []string{"-output", seed, "-volid", "cidata", "-joliet", "-rock", userData, metaData}},
		{"mkisofs", []string{"-output", seed, "-volid", "cidata", "-joliet", "-rock", userData, metaData}},
	}
	built := false
	for _, t := range candidates {
		if bin, err := exec.LookPath(t.name); err == nil {
			if exec.Command(bin, t.args...).Run() == nil {
				built = true
				break
			}
		}
	}
	if !built {
		fmt.Fprintln(os.Stderr, "ERROR! No tool to build seed.iso (need cloud-localds, genisoimage, xorrisofs or mkisofs).")
		return 1
	}
	if _, err := os.Stat(seed); err == nil {
		f, _ := os.OpenFile(confFile, os.O_APPEND|os.O_WRONLY, 0644)
		fmt.Fprintf(f, "seed=\"%s\"\n", seed)
		f.Close()
		fmt.Println(" - NoCloud seed.iso created (user: " + userName + ", password: quickemu)")
	}
	return 0
}
