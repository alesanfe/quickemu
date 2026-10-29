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
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
	"golang.org/x/crypto/blake2b"
)

// HTTP clients come from qelib -- same transport policy everywhere:
// probes die in 30s, downloads bound dial+header but stream
// unbounded, redirect scheme-guard + cap (curl parity).
var (
	httpProbe = qelib.ProbeClient
	httpDL    = qelib.DLClient
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

func hostArch() string {
	if goArch == "arm64" {
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
	for _, l := range osTweaks(osName, release, edition) {
		b.WriteString(l + "\n")
	}
	if isoFile != "" {
		b.WriteString("fixed_iso=\"" + dir + "/" + isoFile + "\"\n")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Println(err)
		return 1
	}
	if err := os.WriteFile(confFile, []byte(b.String()), 0755); err != nil {
		fmt.Println(err)
		return 1
	}
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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: quickget-vm <mkconf|fetch> ...")
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
			fmt.Println("fetch: --url and --dest are required")
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
			fmt.Println("get: <os> [release] [edition] required")
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
	if os.Args[1] == "chunkcheck" {
		// chunkcheck.py parity (the libexec helper the Bash backend
		// called): verify <vmdir>/RecoveryImage.dmg against its CNKL
		// chunklist. QuickGUI's verifyImage reaches this command.
		if len(os.Args) < 3 {
			fmt.Println("usage: quickget-vm chunkcheck <vmdir>")
			os.Exit(2)
		}
		dir := os.Args[2]
		// QuickGUI passes the iso= path (…/RecoveryImage.dmg); the
		// original helper expected the containing vmdir.
		if st, err := os.Stat(dir); err == nil && !st.IsDir() {
			dir = filepath.Dir(dir)
		}
		err := checkChunklist(
			filepath.Join(dir, "RecoveryImage.dmg"),
			filepath.Join(dir, "RecoveryImage.chunklist"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "chunkcheck: "+err.Error())
			os.Exit(1)
		}
		fmt.Println("Verification passed")
		os.Exit(0)
	}
	if os.Args[1] != "mkconf" {
		fmt.Println("usage: quickget-vm <get|mkconf|fetch|customconf|chunkcheck> ...")
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
		fmt.Println("mkconf: --os and --image are required")
		os.Exit(2)
	}
	_ = filepath.Separator
	os.Exit(mkconf(osName, release, edition, image, iso, dir, arch, cloudInit))
}

// fetch: web_get/check_hash parity — download url -> dest via a .part
// staging file (atomic rename), resume a partial download with the
// Range header, bounded retries (curl --retry 3), http(s)-only scheme
// restriction, and sha256 verify when provided. Uses net/http so no
// curl/zsync dependency is required.
var fetch = func(url, dest, wantSHA string) int {
	if u, err := neturl.Parse(url); err != nil ||
		(u.Scheme != "http" && u.Scheme != "https") {
		fmt.Printf("ERROR! unsupported URL scheme: %s\n", url)
		return 1
	}
	part := dest + ".part"
	for attempt := 1; attempt <= 3; attempt++ {
		if fetchOnce(url, part, wantSHA) == 0 {
			break
		}
		if attempt == 3 {
			return 1
		}
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	if err := os.Rename(part, dest); err != nil {
		fmt.Println(err)
		return 1
	}
	fmt.Println(" - Downloaded " + dest)
	return 0
}

func fetchOnce(url, part, wantSHA string) int {
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := httpDL.Do(req)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	defer resp.Body.Close()
	// 416 with a staged file means the local .part is already at/past
	// the remote size — treat as complete and let the SHA verify below
	// decide correctness (curl parity: a finished .part resumes to done).
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && have > 0 {
		return fetchVerify(part, wantSHA)
	}
	if resp.StatusCode >= 400 {
		fmt.Printf("download failed: HTTP %d\n", resp.StatusCode)
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
		fmt.Println(err)
		return 1
	}
	// curl --speed-limit 1024 --speed-time 60 parity: if the body
	// stalls for 60s the connection is dropped — io.Copy returns an
	// error and the outer retry resumes from the .part.
	var lastRead atomic.Int64
	lastRead.Store(time.Now().UnixNano())
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(5 * time.Second):
				if time.Since(time.Unix(0, lastRead.Load())) > 60*time.Second {
					resp.Body.Close()
					return
				}
			}
		}
	}()
	_, err = io.Copy(f, &stallWatchReader{r: resp.Body, last: &lastRead})
	close(done)
	if err != nil {
		f.Close()
		fmt.Println(err)
		return 1
	}
	f.Close()
	return fetchVerify(part, wantSHA)
}

// stallWatchReader pings lastRead on every chunk so the watchdog in
// fetchOnce can drop a stalled connection.
type stallWatchReader struct {
	r    io.Reader
	last *atomic.Int64
}

func (s *stallWatchReader) Read(b []byte) (int, error) {
	n, err := s.r.Read(b)
	if n > 0 {
		s.last.Store(time.Now().UnixNano())
	}
	return n, err
}

// fetchVerify: check_hash parity — verify the staged .part (streaming;
// multi-GB images never enter memory) and drop it on mismatch. Called
// after a completed body and on the 416 already-complete path. The
// expected digest carries an optional "algo:" prefix (b2sum/blake2
// included); bare digests pick the algorithm by hex length.
func fetchVerify(part, wantSHA string) int {
	if wantSHA == "" {
		return 0
	}
	want, h := hashFor(wantSHA)
	if h == nil {
		fmt.Printf("WARNING! Can't guess hash algorithm, not checking %s hash.\n", part)
		return 0
	}
	f, err := os.Open(part)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		fmt.Println(err)
		return 1
	}
	sum := fmt.Sprintf("%x", h.Sum(nil))
	if !strings.EqualFold(sum, want) {
		os.Remove(part)
		fmt.Printf("checksum mismatch: got %s want %s\n", sum, want)
		return 1
	}
	return 0
}

// hashFor resolves the digest string to (bare-hex, hasher): explicit
// "algo:hex" prefixes win over the length guess — 32 md5, 40 sha1
// (trisquel), 64 sha256, 128 sha512.
func hashFor(digest string) (string, hash.Hash) {
	if i := strings.Index(digest, ":"); i >= 0 {
		algo := strings.ToLower(digest[:i])
		digest = digest[i+1:]
		switch algo {
		case "md5":
			return digest, md5.New()
		case "sha1":
			return digest, sha1.New()
		case "sha256":
			return digest, sha256.New()
		case "sha512":
			return digest, sha512.New()
		case "b2sum", "blake2", "blake2b":
			b, err := blake2b.New512(nil)
			if err != nil {
				return "", nil
			}
			return digest, b
		}
		return "", nil // unknown prefix: warn+skip like Bash
	}
	switch len(digest) {
	case 32:
		return digest, md5.New()
	case 40:
		return digest, sha1.New()
	case 64:
		return digest, sha256.New()
	case 128:
		return digest, sha512.New()
	}
	return "", nil
}

// getOS: the quickget pipeline — resolve url+hash via the catalog
// tool, fetch the checksum file, fetch+verify the image, then mkconf.
// quickget-catalog is our own Go binary; zsync delegates to the
// external zsync binary only for .zsync sources (Bash zsync_get).
var cloudInitFlag, noConf, unattendedFlag bool

func getOS(osName, release, edition, dir, arch string) int {
	// Custom pipelines that bypass create_vm in Bash (cli/main.sh):
	// macos recovery flow, ubuntu SHA256SUMS flow, windows Mido flow.
	switch {
	case osName == "macos":
		return getMacos(release, dir, arch)
	case osName == "ubuntu-server":
		return getUbuntu(osName, release, dir, arch)
	case strings.Contains(osName, "ubuntu"):
		return getUbuntu(osName, release, dir, arch)
	case osName == "windows" || osName == "windows-server":
		// For windows, the positional 'edition' is the I18N language.
		return getWindows(osName, release, edition, dir, arch)
	}
	cat, err := qelib.FindTool("quickget-catalog")
	if err != nil {
		fmt.Println("get: quickget-catalog not on PATH")
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
	out, err := execCommand(cat, args...).Output()
	if err != nil {
		fmt.Printf("get: url resolution failed for %s %s %s\n", osName, release, edition)
		return 1
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) == 0 {
		fmt.Println("get: empty url")
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
		fmt.Println(err)
		return 1
	}
	image := filepath.Join(dir, filepath.Base(url))
	// checksum verification: the catalog's second field is either a
	// checksum-file URL (fetched then greps the image basename, the
	// SHA256SUMS format) or the expected digest itself.
	sha := ""
	if hashURL != "" && !strings.HasPrefix(hashURL, "http") {
		sha = hashURL
		hashURL = ""
	}
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
	// the fallback fetches the underlying image URL (.zsync stripped),
	// never the control file itself.
	if strings.HasSuffix(url, ".zsync") {
		if z, err := exec.LookPath("zsync"); err == nil {
			cmd := execCommand(z, "-o", image, url)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if cmd.Run() != nil {
				return 1
			}
		} else {
			base := strings.TrimSuffix(url, ".zsync")
			image = filepath.Join(dir, filepath.Base(base))
			if fetch(base, image, sha) != 0 {
				return 1
			}
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
		fmt.Println("WARNING! --unattended is only supported for ubuntu-server; ignoring.")
	}
	return 0
}

// safeUsername: $USER is interpolated into cloud-init YAML and a sudoers
// echo — keep POSIX login-name chars only; a value that sanitizes to
// nothing or an illegal first char falls back to "quickemu".
func safeUsername(u string) string {
	var b strings.Builder
	for _, r := range u {
		if r == '_' || r == '-' || (r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" || len(s) > 32 || s[0] == '-' || (s[0] >= '0' && s[0] <= '9') {
		return "quickemu"
	}
	return s
}

// safeHostname: the VM dir becomes a cloud-init hostname — basename,
// lowercased, non [a-z0-9-] folded to '-'.
func safeHostname(p string) string {
	var b strings.Builder
	for _, r := range filepath.Base(p) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		default:
			b.WriteRune('-')
		}
	}
	if s := strings.Trim(b.String(), "-"); s != "" {
		return s
	}
	return "vm"
}

// genSeedISO: gen_seed_iso parity — write user-data + meta-data and
// build a NoCloud seed.iso in pure Go (writeISO9660/diskfs — replaces
// the cloud-localds > genisoimage > xorrisofs > mkisofs chain), then
// append seed="..." to the conf.
func genSeedISO(vmPath, confFile string) int {
	userName := safeUsername(os.Getenv("USER"))
	hostName := safeHostname(vmPath)
	sshKey := ""
	for _, k := range []string{"id_ed25519.pub", "id_rsa.pub"} {
		if b, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".ssh", k)); err == nil {
			sshKey = strings.TrimSpace(string(b))
			break
		}
	}
	userData := filepath.Join(vmPath, "user-data")
	var ud strings.Builder
	ud.WriteString("#cloud-config\nhostname: " + hostName + "\nmanage_etc_hosts: true\nusers:\n  - name: " + userName +
		"\n    sudo: ALL=(ALL) NOPASSWD:ALL\n    groups: sudo\n    shell: /bin/bash\n    lock_passwd: false\n    plain_text_passwd: quickemu\n")
	if sshKey != "" {
		ud.WriteString("    ssh_authorized_keys:\n      - " + sshKey + "\n")
	}
	ud.WriteString("ssh_pwauth: true\npackage_update: true\n")
	if err := os.WriteFile(userData, []byte(ud.String()), 0644); err != nil {
		fmt.Println(err)
		return 1
	}
	metaData := filepath.Join(vmPath, "meta-data")
	if _, err := os.Stat(metaData); err != nil {
		if err := os.WriteFile(metaData, []byte("instance-id: "+hostName+"\nlocal-hostname: "+hostName+"\n"), 0644); err != nil {
			fmt.Println(err)
			return 1
		}
	}
	seed := filepath.Join(vmPath, "seed.iso")
	// cloud-localds parity: a cidata seed ISO is just an ISO9660 image
	// (volid=cidata, Rock Ridge + Joliet) holding user-data/meta-data.
	if err := writeISO9660(seed, "cidata", map[string]string{
		"/user-data": userData, "/meta-data": metaData,
	}); err != nil {
		fmt.Println("ERROR! Could not build seed.iso: " + err.Error())
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
