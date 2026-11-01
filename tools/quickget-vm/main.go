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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// HTTP clients come from qelib -- same transport policy everywhere:
// probes die in 30s, downloads bound dial+header but stream
// unbounded, redirect scheme-guard + cap (curl parity).
var (
	httpProbe = qelib.ProbeClient
	httpDL    = qelib.DLClient
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: quickget-vm <mkconf|fetch|get|customconf|chunkcheck|seed|overlay|checkbase|rebase|flatten|cache> ...")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "seed":
		os.Exit(seedSubcommand(os.Args[2:]))
	case "overlay":
		os.Exit(cmdOverlay(os.Args[2:]))
	case "checkbase":
		os.Exit(cmdCheckBase(os.Args[2:]))
	case "rebase":
		os.Exit(cmdRebase(os.Args[2:]))
	case "flatten":
		os.Exit(cmdFlatten(os.Args[2:]))
	case "cache":
		os.Exit(cmdCache(os.Args[2:]))
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
			a := os.Args[i]
			if !strings.HasPrefix(a, "--") {
				pos = append(pos, a)
				continue
			}
			switch a {
			case "--dir", "--arch":
				if i+1 < len(os.Args) {
					i++
					if a == "--dir" {
						dir = os.Args[i]
					} else {
						arch = os.Args[i]
					}
				}
			case "--cloud-init":
				cloudInitFlag = true
			case "--no-conf":
				noConf = true
			case "--unattended":
				unattendedFlag = true
			case "--cache":
				cacheFlag = true
			case "--offline":
				offlineFlag, cacheFlag = true, true
			case "--overlay":
				overlayFlag = true
			default:
				ok, err := seedOpts.parseSeedFlag(os.Args, &i)
				if err != nil {
					fmt.Println("get: " + err.Error())
					os.Exit(2)
				}
				if !ok {
					fmt.Println("get: unsupported argument " + a)
					os.Exit(2)
				}
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
		fmt.Println("usage: quickget-vm <get|mkconf|fetch|customconf|chunkcheck|seed|overlay|checkbase|rebase|flatten|cache> ...")
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
		case "--overlay":
			overlayFlag = true
			i--
		default:
			if _, err := seedOpts.parseSeedFlag(os.Args, &i); err != nil {
				fmt.Println("mkconf: " + err.Error())
				os.Exit(2)
			}
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

// getOS: the quickget pipeline — resolve url+hash via the catalog
// tool, fetch the checksum file, fetch+verify the image, then mkconf.
// quickget-catalog is our own Go binary; zsync delegates to the
// external zsync binary only for .zsync sources (Bash zsync_get).
var cloudInitFlag, noConf, unattendedFlag bool

// overlayFlag routes downloaded qcow2s through the shared base cache
// (QE-042); cacheFlag routes downloads through the secure dl cache and
// offlineFlag forbids network fetches entirely (QE-043).
var overlayFlag, cacheFlag, offlineFlag bool

// fetchImage downloads url to dest honouring the --cache/--offline
// flags: cached hits are copied out of the read-mostly store, offline
// misses fail rather than touching the network.
func fetchImage(url, dest, sha string) int {
	if !cacheFlag && !offlineFlag {
		return fetch(url, dest, sha)
	}
	p, err := cachedFetch(url, sha, offlineFlag)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	if p != dest {
		if err := copyFile(p, dest); err != nil {
			fmt.Println(err)
			return 1
		}
	}
	return 0
}

func getOS(osName, release, edition, dir, arch string) int {
	// Custom pipelines that bypass create_vm in Bash (cli/main.sh):
	// macos recovery flow, ubuntu SHA256SUMS flow, windows Mido flow.
	switch {
	case osName == "macos":
		return getMacos(release, dir, arch)
	case osName == "ubuntu-server":
		return getUbuntu(osName, release, dir, arch)
	case strings.Contains(osName, "ubuntu") && osName != "ubuntu-cloud":
		// Every *buntu desktop flavour lives on cdimage/releases —
		// ubuntu-cloud is a qcow2 cloud image resolved by the catalog
		// scraper, not a cdimage ISO.
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
		if fetchImage(hashURL, hf, "") == 0 {
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
	// web_get echo parity: "Downloading <pretty> <rel> <ed>" + "- URL:".
	// zsync_get prints its own "… from <url>" form instead.
	pretty := prettyNameFor(osName)
	if strings.HasSuffix(url, ".zsync") {
		fmt.Printf("Downloading %s %s %s from %s\n", pretty, release, edition, url)
	} else {
		fmt.Printf("Downloading %s %s %s\n", pretty, release, edition)
		fmt.Println("- URL: " + url)
	}
	// zsync sources prefer the external zsync binary (delta download);
	// the fallback fetches the underlying image URL (.zsync stripped),
	// never the control file itself.
	if strings.HasSuffix(url, ".zsync") && !offlineFlag {
		if z, err := exec.LookPath("zsync"); err == nil {
			cmd := execCommand(z, "-o", image, url)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if cmd.Run() != nil {
				return 1
			}
		} else {
			base := strings.TrimSuffix(url, ".zsync")
			image = filepath.Join(dir, filepath.Base(base))
			if fetchImage(base, image, sha) != 0 {
				return 1
			}
		}
	} else if fetchImage(url, image, sha) != 0 {
		return 1
	}
	if noConf {
		return 0
	}
	// image_normalize: decompress/convert the artifact per-OS and pick
	// up the final image name inside the VM dir.
	imageName := imageNormalize(osName, filepath.Base(image), dir)
	// --overlay: a downloaded qcow2 becomes a shared read-only base;
	// the VM's disk is a thin overlay on top of it (QE-042).
	if overlayFlag && qcowMagic(filepath.Join(dir, imageName)) {
		dst := filepath.Join(dir, "disk.qcow2")
		base, err := makeOverlay(filepath.Join(dir, imageName), dst,
			baseImageName(osName, release, arch))
		if err != nil {
			fmt.Println("overlay: " + err.Error())
			return 1
		}
		fmt.Println(" - Base image cached read-only at " + base)
		imageName = "disk.qcow2"
	} else if overlayFlag {
		fmt.Println("WARNING! --overlay requires a qcow2 download; using the image directly.")
	}
	// windows/windows-server: fetch virtio-win.iso + build the
	// unattended install media for retail releases 10/11 (get.sh flow).
	fixedISO := ""
	if osName == "windows" || osName == "windows-server" {
		virtio := filepath.Join(dir, "virtio-win.iso")
		if _, err := os.Stat(virtio); err != nil {
			fmt.Println("Downloading VirtIO drivers...")
			if fetch("https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso", virtio, "") != 0 {
				return 1
			}
		}
		fixedISO = "virtio-win.iso"
		if release == "10" || release == "11" {
			os.Remove(filepath.Join(dir, "unattended.iso"))
			if unattendedWindows(dir, "w"+release, "") != 0 {
				return 1
			}
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
