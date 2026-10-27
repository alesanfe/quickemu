// image.go: post-download image handling — migrates
// lib/quickget/image/{generate,normalize,windows}.sh:
//
//	iso_extract_bootfiles, gen_ubuntu_autoinstall, image_normalize,
//	is_unattended_windows_iso, unattended_windows.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// finalURL: web_redirect parity — follow redirects and return the
// resolved URL (used to name URL-sourced images).
func finalURL(url string) string {
	c := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	resp, err := c.Head(url)
	if err != nil {
		return url
	}
	defer resp.Body.Close()
	return resp.Request.URL.String()
}

// isoExtractBootfiles: extract PREFIX/vmlinuz + PREFIX/initrd from an
// ISO using the first available tool (bsdtar > 7z > isoinfo), then
// normalise the extracted names to OUT root.
func isoExtractBootfiles(iso, out, prefix string) bool {
	lower := strings.ToLower(prefix)
	upper := strings.ToUpper(prefix)
	done := false
	if b, err := exec.LookPath("bsdtar"); err == nil {
		done = exec.Command(b, "-C", out, "-xf", iso,
			prefix+"/vmlinuz", prefix+"/initrd").Run() == nil
	} else if z, err := exec.LookPath("7z"); err == nil {
		done = exec.Command(z, "e", "-o"+out, iso,
			prefix+"/vmlinuz", prefix+"/initrd").Run() == nil
	} else if i, err := exec.LookPath("isoinfo"); err == nil {
		for _, pair := range [][2]string{
			{"/" + upper + "/VMLINUZ.;1", out + "/vmlinuz"},
			{"/" + upper + "/INITRD.;1", out + "/initrd"},
		} {
			data, err := exec.Command(i, "-i", iso, "-x", pair[0]).Output()
			if err == nil && len(data) > 0 {
				os.WriteFile(pair[1], data, 0644)
				done = true
			}
		}
	}
	_ = lower
	if !done {
		fmt.Fprintln(os.Stderr, "ERROR! Need bsdtar, 7z or isoinfo to extract kernel from the ISO.")
		return false
	}
	// 7z/bsdtar may place files at OUT root or under PREFIX.
	if st, err := os.Stat(out + "/vmlinuz"); err != nil || st.Size() == 0 {
		if st, err := os.Stat(filepath.Join(out, prefix, "vmlinuz")); err == nil && st.Size() > 0 {
			os.Rename(filepath.Join(out, prefix, "vmlinuz"), out+"/vmlinuz")
			os.Rename(filepath.Join(out, prefix, "initrd"), out+"/initrd")
		}
	}
	vok := fileNonEmpty(out + "/vmlinuz")
	iok := fileNonEmpty(out + "/initrd")
	return vok && iok
}

func fileNonEmpty(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Size() > 0
}

// customConf: create_config parity — make a VM dir from a local image
// file or URL, move it in, detect the guest OS by filename, build
// unattended Windows media when applicable, then write the .conf.
func customConf(vmPath, input, flag string) int {
	if vmPath == "" || input == "" {
		fmt.Fprintln(os.Stderr, "customconf: <vm-path> <input> required")
		return 2
	}
	if err := os.Mkdir(vmPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! Could not create directory: %s. Please verify that it does not already exist\n", vmPath)
		return 1
	}
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		input = finalURL(input)
		if strings.HasSuffix(input, ".iso") || strings.HasSuffix(input, ".img") {
			if fetch(input, filepath.Join(vmPath, filepath.Base(input)), "") != 0 {
				return 1
			}
			input = filepath.Base(input)
		} else {
			fmt.Fprintln(os.Stderr, "ERROR! Only ISO,IMG and QCOW2 file types are supported for --create-config")
			return 1
		}
	} else {
		if _, err := os.Stat(input); err != nil {
			fmt.Fprintln(os.Stderr, "ERROR! The input must be a valid URL or path to an ISO, IMG, or QCOW2 file.")
			return 1
		}
		fmt.Println("Moving image to VM dir")
		base := filepath.Base(input)
		if err := os.Rename(input, filepath.Join(vmPath, base)); err != nil {
			// cross-device: copy+remove fallback
			if cp := copyFile(input, filepath.Join(vmPath, base)); cp != nil {
				fmt.Fprintln(os.Stderr, cp)
				return 1
			}
			os.Remove(input)
		}
		input = base
	}
	low := strings.ToLower(input)
	switch {
	case strings.HasSuffix(low, ".iso"), strings.HasSuffix(low, ".img"),
		strings.HasSuffix(low, ".qcow2"), strings.HasSuffix(low, ".vmdk"),
		strings.HasSuffix(low, ".raw"), strings.HasSuffix(low, ".vdi"):
	default:
		fmt.Fprintln(os.Stderr, "ERROR! Only ISO,IMG and QCOW2 file types are supported for --create-config")
		return 1
	}
	fmt.Println("Creating custom VM config for " + input + ".")
	customOS := "linux"
	fixedISO := ""
	switch {
	case strings.Contains(low, "freebsd"):
		customOS = "freebsd"
	case strings.Contains(low, "kolibrios"):
		customOS = "kolibrios"
	case strings.Contains(low, "reactos"):
		customOS = "reactos"
	case strings.Contains(low, "windows-server"), strings.Contains(low, "eval_oemret_x"), strings.Contains(low, "eval_x"):
		customOS = "windows-server"
	case strings.Contains(low, "windows"), strings.Contains(low, "win"):
		customOS = "windows"
		if flag != "--disable-unattended" && (flag == "--unattended" || isUnattendedWindowsISO(input)) {
			fmt.Println("Creating unattended Windows installation files. To disable, pass --disable-unattended")
			fmt.Println()
			fmt.Println("Downloading VirtIO drivers...")
			fetch("https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso",
				filepath.Join(vmPath, "virtio-win.iso"), "")
			fixedISO = "virtio-win.iso"
			os.Remove(filepath.Join(vmPath, "unattended.iso"))
			drv := "w11"
			if strings.Contains(low, "win") && strings.Contains(low, "10") {
				drv = "w10"
			}
			unattendedWindows(vmPath, drv, "")
		}
	}
	fmt.Printf("Selecting OS: %s. If this is incorrect, please modify the config file to include the correct OS.\n\n", customOS)
	return mkconf(customOS, "", "", input, fixedISO, vmPath, "")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// genUbuntuAutoinstall: subiquity unattended install — extract casper
// kernel+initrd, write the autoinstall user-data, build the NoCloud
// seed, and append kernel/initrd/boot_args to the conf.
func genUbuntuAutoinstall(vmPath, iso, confFile string) int {
	if !isoExtractBootfiles(iso, vmPath, "casper") {
		fmt.Fprintln(os.Stderr, "ERROR! Could not extract casper/vmlinuz+initrd from "+iso)
		return 1
	}
	out, err := exec.Command("openssl", "passwd", "-6", "quickemu").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! openssl is required to generate the install password hash.")
		return 1
	}
	userName := os.Getenv("USER")
	if userName == "" {
		userName = "quickemu"
	}
	pwHash := strings.TrimSpace(string(out))
	userData := "#cloud-config\nautoinstall:\n  version: 1\n  identity:\n" +
		"    hostname: " + vmPath + "\n    username: " + userName +
		"\n    password: \"" + pwHash + "\"\n  ssh:\n    install-server: true\n" +
		"    allow-pw: true\n  storage:\n    layout:\n      name: direct\n" +
		"  late-commands:\n    - echo \"" + userName + " ALL=(ALL) NOPASSWD:ALL\" > /target/etc/sudoers.d/" + userName + "\n"
	if err := os.WriteFile(filepath.Join(vmPath, "user-data"), []byte(userData), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if rc := genSeedISO(vmPath, confFile); rc != 0 {
		return rc
	}
	f, err := os.OpenFile(confFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer f.Close()
	fmt.Fprintf(f, "kernel=\"%s\"\ninitrd=\"%s\"\n", vmPath+"/vmlinuz", vmPath+"/initrd")
	fmt.Fprintf(f, "boot_args=\"autoinstall ds=nocloud;s=/cdrom/\"\n")
	fmt.Println(" - Unattended install configured (user: " + userName + ", password: quickemu)")
	return 0
}

// imageNormalize: per-OS post-download fixups (decompress, convert to
// qcow2, resize). Returns the final image filename inside vmPath.
func imageNormalize(osName, iso, vmPath string) string {
	run := func(name string, args ...string) error {
		bin, err := exec.LookPath(name)
		if err != nil {
			return err
		}
		return exec.Command(bin, args...).Run()
	}
	strip := func(sfx string) string {
		os.Remove(filepath.Join(vmPath, iso))
		return strings.TrimSuffix(iso, sfx)
	}
	_ = strip
	switch osName {
	case "batocera":
		if strings.HasSuffix(iso, ".gz") {
			run("gzip", "-d", filepath.Join(vmPath, iso))
			iso = strings.TrimSuffix(iso, ".gz")
		}
		// Grow the raw image so Batocera can expand its partition.
		run("qemu-img", "resize", "-f", "raw", filepath.Join(vmPath, iso), "128G")
	case "dragonflybsd":
		if strings.HasSuffix(iso, ".bz2") {
			run("bzip2", "-d", filepath.Join(vmPath, iso))
			iso = strings.TrimSuffix(iso, ".bz2")
		}
	case "easyos":
		if strings.HasSuffix(iso, ".img") {
			run("qemu-img", "convert", "-f", "raw", "-O", "qcow2",
				filepath.Join(vmPath, iso), filepath.Join(vmPath, "disk.qcow2"))
			iso = strings.TrimSuffix(iso, ".img")
		}
	case "openfyde":
		if strings.HasSuffix(iso, ".xz") {
			run("xz", "-d", filepath.Join(vmPath, iso))
			iso = strings.TrimSuffix(iso, ".xz")
			run("qemu-img", "convert", "-f", "raw", "-O", "qcow2",
				filepath.Join(vmPath, iso), filepath.Join(vmPath, "disk.qcow2"))
			iso = "disk.qcow2"
		}
	case "redoxos":
		if strings.HasSuffix(iso, ".zst") {
			if _, err := exec.LookPath("zstd"); err != nil {
				fmt.Fprintln(os.Stderr, "ERROR! 'zstd' needs installing. Unable to extract file.")
			} else if run("zstd", "-dq", filepath.Join(vmPath, iso)) == nil {
				iso = strip(".zst")
			}
		}
	case "freedos", "reactos":
		if strings.HasSuffix(iso, ".zip") {
			run("unzip", "-qo", filepath.Join(vmPath, iso), "-d", vmPath)
			iso = findISO(vmPath, iso)
		}
	case "kolibrios":
		if strings.HasSuffix(iso, ".7z") {
			if _, err := exec.LookPath("7z"); err != nil {
				fmt.Fprintln(os.Stderr, "ERROR! '7zip' needs installing. Unable to extract file.")
			} else {
				run("7z", "e", filepath.Join(vmPath, iso), "-o"+vmPath)
				iso = findISO(vmPath, iso)
			}
		}
	}
	return iso
}

// findISO: after an archive unpack, pick up the extracted .iso name
// (Bash: ISO=$(ls -1 $VM_PATH/*.iso | basename)).
func findISO(vmPath, old string) string {
	os.Remove(filepath.Join(vmPath, old))
	m, _ := filepath.Glob(filepath.Join(vmPath, "*.iso"))
	if len(m) > 0 {
		return filepath.Base(m[0])
	}
	return old
}

// isUnattendedWindowsISO: retail Win10/11 ISO filename detection
// (Win10_22H2_..._x64.iso style).
var winISORx = regexp.MustCompile(`Win(10|11)_([0-9]{2}H(1|2)|[0-9]{4})_[^.]*?(x64|x32)(v[0-9])?\.iso`)

func isUnattendedWindowsISO(name string) bool {
	return winISORx.MatchString(name)
}

var winCultures = map[string][2]string{
	"Arabic": {"ar-SA", "0401"}, "Brazilian Portuguese": {"pt-BR", "0416"},
	"Bulgarian": {"bg-BG", "0402"}, "Chinese (Simplified)": {"zh-CN", "0804"},
	"Chinese (Traditional)": {"zh-TW", "0404"}, "Croatian": {"hr-HR", "041a"},
	"Czech": {"cs-CZ", "0405"}, "Danish": {"da-DK", "0406"},
	"Dutch": {"nl-NL", "0413"}, "English International": {"en-GB", "0809"},
	"Estonian": {"et-EE", "0425"}, "Finnish": {"fi-FI", "040b"},
	"French": {"fr-FR", "040c"}, "French Canadian": {"fr-CA", "0c0c"},
	"German": {"de-DE", "0407"}, "Greek": {"el-GR", "0408"},
	"Hebrew": {"he-IL", "040d"}, "Hungarian": {"hu-HU", "040e"},
	"Italian": {"it-IT", "0410"}, "Japanese": {"ja-JP", "0411"},
	"Korean": {"ko-KR", "0412"}, "Latvian": {"lv-LV", "0426"},
	"Lithuanian": {"lt-LT", "0427"}, "Norwegian": {"nb-NO", "0414"},
	"Polish": {"pl-PL", "0415"}, "Portuguese": {"pt-PT", "0816"},
	"Romanian": {"ro-RO", "0418"}, "Russian": {"ru-RU", "0419"},
	"Serbian Latin": {"sr-Latn-RS", "081a"}, "Slovak": {"sk-SK", "041b"},
	"Slovenian": {"sl-SI", "0424"}, "Spanish": {"es-ES", "0c0a"},
	"Spanish (Mexico)": {"es-MX", "080a"}, "Swedish": {"sv-SE", "041d"},
	"Thai": {"th-TH", "041e"}, "Turkish": {"tr-TR", "041f"},
	"Ukrainian": {"uk-UA", "0422"},
}

// unattendedWindows: write autounattend.xml (culture + driver dir
// substitutions), download the SPICE guest tools, and build
// unattended.iso via mkisofs.
func unattendedWindows(vmPath, driverDir, i18n string) int {
	dir := filepath.Join(vmPath, "unattended")
	os.MkdirAll(dir, 0755)
	if driverDir == "" {
		driverDir = "w10"
	}
	qxldod := ""
	if driverDir == "w10" {
		qxldod = `        <PathAndCredentials wcm:action="add" wcm:keyValue="6">
          <Path>E:\qxldod\w10\amd64</Path>
        </PathAndCredentials>`
	}
	cult := [2]string{"en-US", "0409"}
	if c, ok := winCultures[i18n]; ok {
		cult = c
	}
	xml := strings.ReplaceAll(unattendXML, "${CULTURE}", cult[0])
	xml = strings.ReplaceAll(xml, "${LANGID}", cult[1])
	xml = strings.ReplaceAll(xml, "${DRIVER_DIR}", driverDir)
	xml = strings.ReplaceAll(xml, "${QXLDOD_PATH}", qxldod)
	if err := os.WriteFile(filepath.Join(dir, "autounattend.xml"), []byte(xml), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Downloading Spice drivers...")
	fetch("https://www.spice-space.org/download/windows/spice-webdavd/spice-webdavd-x64-latest.msi",
		filepath.Join(dir, "spice-webdavd-x64-latest.msi"), "")
	fetch("https://www.spice-space.org/download/windows/vdagent/vdagent-win-0.10.0/spice-vdagent-x64-0.10.0.msi",
		filepath.Join(dir, "spice-vdagent-x64-0.10.0.msi"), "")
	fmt.Println("Making unattended.iso")
	iso := filepath.Join(vmPath, "unattended.iso")
	for _, tool := range []string{"mkisofs", "genisoimage", "xorrisofs"} {
		if b, err := exec.LookPath(tool); err == nil {
			if exec.Command(b, "-quiet", "-J", "-o", iso, dir+"/").Run() == nil {
				return 0
			}
		}
	}
	fmt.Fprintln(os.Stderr, "ERROR! Need mkisofs/genisoimage/xorrisofs to build unattended.iso")
	return 1
}
