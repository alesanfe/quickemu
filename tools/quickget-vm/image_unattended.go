// image_unattended — unattended & generated-image builders (split of image.go).
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	iso9660fs "github.com/diskfs/go-diskfs/filesystem/iso9660"
	"github.com/simia-tech/crypt"
)

// customConf: create_config parity — make a VM dir from a local image
// file or URL, move it in, detect the guest OS by filename, build
// unattended Windows media when applicable, then write the .conf.
func customConf(vmPath, input, flag string) int {
	if vmPath == "" || input == "" {
		fmt.Println("customconf: <vm-path> <input> required")
		return 2
	}
	if err := os.Mkdir(vmPath, 0755); err != nil {
		fmt.Printf("ERROR! Could not create directory: %s. Please verify that it does not already exist\n", vmPath)
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
			fmt.Println("ERROR! Only ISO,IMG and QCOW2 file types are supported for --create-config")
			return 1
		}
	} else {
		// create_config parity: extension decides before the move —
		// unsupported types are rejected without touching the file.
		low := strings.ToLower(input)
		switch {
		case strings.HasSuffix(low, ".iso"), strings.HasSuffix(low, ".img"),
			strings.HasSuffix(low, ".qcow2"), strings.HasSuffix(low, ".vmdk"),
			strings.HasSuffix(low, ".raw"), strings.HasSuffix(low, ".vdi"):
		default:
			if _, err := os.Stat(input); err != nil {
				fmt.Println("ERROR! The input must be a valid URL or path to an ISO, IMG, or QCOW2 file.")
				return 1
			}
			fmt.Println("ERROR! Only ISO,IMG and QCOW2 file types are supported for --create-config")
			return 1
		}
		if _, err := os.Stat(input); err != nil {
			fmt.Println("ERROR! The input must be a valid URL or path to an ISO, IMG, or QCOW2 file.")
			return 1
		}
		fmt.Println("Moving image to VM dir")
		base := filepath.Base(input)
		if err := qelib.MoveFile(input, filepath.Join(vmPath, base)); err != nil {
			fmt.Println(err)
			return 1
		}
		input = base
	}
	low := strings.ToLower(input)
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
			// fixed_iso references virtio-win.iso in the generated
			// conf — a failed download must abort, not leave a conf
			// pointing at a missing ISO.
			if fetch("https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso",
				filepath.Join(vmPath, "virtio-win.iso"), "") != 0 {
				return 1
			}
			fixedISO = "virtio-win.iso"
			os.Remove(filepath.Join(vmPath, "unattended.iso"))
			drv := "w11"
			// *win*10* glob: "win" must precede "10" (Bash case order).
			if i := strings.Index(low, "win"); i >= 0 && strings.Contains(low[i+3:], "10") {
				drv = "w10"
			}
			unattendedWindows(vmPath, drv, "")
		}
	}
	fmt.Printf("Selecting OS: %s. If this is incorrect, please modify the config file to include the correct OS.\n\n", customOS)
	return mkconf(customOS, "", "", input, fixedISO, vmPath, "")
}

// genUbuntuAutoinstall: subiquity unattended install — extract casper
// kernel+initrd, write the autoinstall user-data, build the NoCloud
// seed, and append kernel/initrd/boot_args to the conf.
// cryptSalt: random SHA512-crypt salt chars — simia-tech/crypt expects
// "$6$<salt>" as the setting string.
func cryptSalt(n int) string { return qelib.CryptSalt(n) }

func genUbuntuAutoinstall(vmPath, iso, confFile string) int {
	if !isoExtractBootfiles(iso, vmPath, "casper") {
		fmt.Println("ERROR! Could not extract casper/vmlinuz+initrd from " + iso)
		return 1
	}
	// SHA512-crypt of "quickemu" — pure-Go equivalent of
	// `openssl passwd -6 quickemu` (removes the openssl dependency).
	pwHash, err := crypt.Crypt("quickemu", "$6$"+cryptSalt(8))
	if err != nil {
		fmt.Println("ERROR! could not generate the install password hash.")
		return 1
	}
	userName := safeUsername(os.Getenv("USER"))
	userData := "#cloud-config\nautoinstall:\n  version: 1\n  identity:\n" +
		"    hostname: " + safeHostname(vmPath) + "\n    username: " + userName +
		"\n    password: \"" + pwHash + "\"\n  ssh:\n    install-server: true\n" +
		"    allow-pw: true\n  storage:\n    layout:\n      name: direct\n" +
		"  late-commands:\n    - echo \"" + userName + " ALL=(ALL) NOPASSWD:ALL\" > /target/etc/sudoers.d/" + userName + "\n"
	if err := os.WriteFile(filepath.Join(vmPath, "user-data"), []byte(userData), 0644); err != nil {
		fmt.Println(err)
		return 1
	}
	if rc := genSeedISO(vmPath, confFile); rc != 0 {
		return rc
	}
	f, err := os.OpenFile(confFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	defer f.Close()
	fmt.Fprintf(f, "kernel=\"%s\"\ninitrd=\"%s\"\n", vmPath+"/vmlinuz", vmPath+"/initrd")
	fmt.Fprintf(f, "boot_args=\"autoinstall ds=nocloud;s=/cdrom/\"\n")
	fmt.Println(" - Unattended install configured (user: " + userName + ", password: quickemu)")
	return 0
}

// imageNormalize: per-OS post-download fixups (decompress, convert to
// qcow2, resize). Compression is pure Go; only qemu-img and 7z stay
// external. Returns the final image filename inside vmPath.
func imageNormalize(osName, iso, vmPath string) string {
	switch osName {
	case "batocera":
		if strings.HasSuffix(iso, ".gz") {
			if decompressGZ(filepath.Join(vmPath, iso)) {
				iso = strings.TrimSuffix(iso, ".gz")
			}
		}
		// Grow the raw image so Batocera can expand its partition.
		run("qemu-img", "resize", "-f", "raw", filepath.Join(vmPath, iso), "128G")
	case "dragonflybsd", "harvey":
		if strings.HasSuffix(iso, ".bz2") {
			if decompressBZ2(filepath.Join(vmPath, iso)) {
				iso = strings.TrimSuffix(iso, ".bz2")
			}
		}
	case "easyos":
		if strings.HasSuffix(iso, ".img") {
			run("qemu-img", "convert", "-f", "raw", "-O", "qcow2",
				filepath.Join(vmPath, iso), filepath.Join(vmPath, "disk.qcow2"))
			iso = strings.TrimSuffix(iso, ".img")
		}
	case "openfyde":
		if strings.HasSuffix(iso, ".xz") {
			if decompressXZ(filepath.Join(vmPath, iso)) {
				iso = strings.TrimSuffix(iso, ".xz")
				run("qemu-img", "convert", "-f", "raw", "-O", "qcow2",
					filepath.Join(vmPath, iso), filepath.Join(vmPath, "disk.qcow2"))
				iso = "disk.qcow2"
			}
		}
	case "dietpi", "bastillebsd", "mobian":
		if strings.HasSuffix(iso, ".xz") {
			if decompressXZ(filepath.Join(vmPath, iso)) {
				iso = strings.TrimSuffix(iso, ".xz")
			}
		}
	case "9front":
		if strings.HasSuffix(iso, ".gz") {
			if decompressGZ(filepath.Join(vmPath, iso)) {
				iso = strings.TrimSuffix(iso, ".gz")
			}
		}
	case "nomadbsd":
		if strings.HasSuffix(iso, ".lzma") {
			if decompressLZMA(filepath.Join(vmPath, iso)) {
				iso = strings.TrimSuffix(iso, ".lzma")
			}
		}
	case "redoxos":
		if strings.HasSuffix(iso, ".zst") {
			if decompressZstd(filepath.Join(vmPath, iso)) {
				os.Remove(filepath.Join(vmPath, iso))
				iso = strings.TrimSuffix(iso, ".zst")
			} else {
				fmt.Println("ERROR! Unable to extract file.")
			}
		}
	case "freedos", "reactos":
		if strings.HasSuffix(iso, ".zip") {
			if extractZip(filepath.Join(vmPath, iso), vmPath) {
				iso = findISO(vmPath, iso)
			}
		}
	case "kolibrios":
		if strings.HasSuffix(iso, ".7z") {
			if extract7z(filepath.Join(vmPath, iso), vmPath) {
				iso = findISO(vmPath, iso)
			}
		}
	}
	return iso
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
		fmt.Println(err)
		return 1
	}
	fmt.Println("Downloading Spice drivers...")
	// autounattend.xml runs msiexec on these files — a failed fetch
	// must abort rather than ship an ISO missing promised drivers.
	if fetch("https://www.spice-space.org/download/windows/spice-webdavd/spice-webdavd-x64-latest.msi",
		filepath.Join(dir, "spice-webdavd-x64-latest.msi"), "") != 0 {
		return 1
	}
	if fetch("https://www.spice-space.org/download/windows/vdagent/vdagent-win-0.10.0/spice-vdagent-x64-0.10.0.msi",
		filepath.Join(dir, "spice-vdagent-x64-0.10.0.msi"), "") != 0 {
		return 1
	}
	fmt.Println("Making unattended.iso")
	iso := filepath.Join(vmPath, "unattended.iso")
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files["/"+filepath.ToSlash(rel)] = p
		return nil
	})
	if err == nil {
		err = writeISO9660(iso, "unattended", files)
	}
	if err != nil {
		fmt.Println("ERROR! Could not build unattended.iso: " + err.Error())
		return 1
	}
	return 0
}

// writeISO9660 builds an ISO9660 image with Rock Ridge + Joliet
// extensions in pure Go — replaces the mkisofs/genisoimage/xorrisofs/
// cloud-localds external chain (genisoimage -rock -joliet -volid).
// `files` maps ISO paths (leading /) to host source paths.
func writeISO9660(out, volid string, files map[string]string) error {
	// diskfs preallocates the backing file; size it to content plus
	// ISO9660 structural overhead (descriptors, path tables, padding).
	var total int64 = 1 << 20
	for _, src := range files {
		if st, err := os.Stat(src); err == nil {
			total += st.Size() + 8192
		}
	}
	d, err := diskfs.Create(out, total*2, diskfs.SectorSize(2048))
	if err != nil {
		return err
	}
	fs, err := d.CreateFilesystem(disk.FilesystemSpec{
		Partition: 0, FSType: filesystem.TypeISO9660})
	if err != nil {
		return err
	}
	// Deterministic order so builds are reproducible.
	names := make([]string, 0, len(files))
	for k := range files {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, name := range names {
		if dir := path.Dir(name); dir != "/" && dir != "." {
			if err := fs.Mkdir(dir); err != nil {
				return err
			}
		}
		fh, err := fs.OpenFile(name, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(files[name])
		if err == nil {
			_, err = fh.Write(b)
		}
		fh.Close()
		if err != nil {
			return err
		}
	}
	isofs, ok := fs.(*iso9660fs.FileSystem)
	if !ok {
		return fmt.Errorf("unexpected filesystem type %T", fs)
	}
	return isofs.Finalize(iso9660fs.FinalizeOptions{
		RockRidge: true, Joliet: true, VolumeIdentifier: volid})
}
