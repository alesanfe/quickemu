// image.go: post-download image handling — migrates
// lib/quickget/image/{generate,normalize,windows}.sh:
//
//	iso_extract_bootfiles, gen_ubuntu_autoinstall, image_normalize,
//	is_unattended_windows_iso, unattended_windows.
package main

import (
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"crypto/rand"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	iso9660fs "github.com/diskfs/go-diskfs/filesystem/iso9660"
	"github.com/kdomanski/iso9660"
	"github.com/simia-tech/crypt"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// finalURL: web_redirect parity — follow redirects and return the
// resolved URL (used to name URL-sourced images).
var finalURL = func(url string) string {
	if u, err := qelib.FinalURL(url); err == nil && u != "" {
		return u
	}
	return url
}

// isoExtractPure: pure-Go ISO9660 walk — finds prefix/vmlinuz and
// prefix/initrd (Rock Ridge names, case-insensitive, ";N" version
// suffix stripped). Replaces the bsdtar/7z/isoinfo dependency for
// standard ISO9660 images; UDF-only images fall back to the tools.
func isoExtractPure(iso, out, prefix string) bool {
	f, err := os.Open(iso)
	if err != nil {
		return false
	}
	defer f.Close()
	img, err := iso9660.OpenImage(f)
	if err != nil {
		return false
	}
	root, err := img.RootDir()
	if err != nil {
		return false
	}
	want := map[string]string{
		"vmlinuz": out + "/vmlinuz",
		"initrd":  out + "/initrd",
	}
	var walk func(dir *iso9660.File) bool
	walk = func(dir *iso9660.File) bool {
		kids, err := dir.GetChildren()
		if err != nil {
			return false
		}
		for _, k := range kids {
			name := strings.ToLower(strings.SplitN(k.Name(), ";", 2)[0])
			if k.IsDir() {
				if name == prefix || prefix == "" {
					if walk(k) {
						return true
					}
				}
				continue
			}
			dst, ok := want[name]
			if !ok {
				continue
			}
			w, err := os.Create(dst)
			if err != nil {
				continue
			}
			_, err = io.Copy(w, k.Reader())
			w.Close()
			if err == nil {
				delete(want, name)
			}
		}
		return len(want) == 0
	}
	if !walk(root) {
		return false
	}
	return true
}

// isoExtractBootfiles: extract PREFIX/vmlinuz + PREFIX/initrd from an
// ISO — pure-Go ISO9660 first, bsdtar > 7z > isoinfo as fallback for
// UDF/hybrid images — then normalise the extracted names to OUT root.
func isoExtractBootfiles(iso, out, prefix string) bool {
	if isoExtractPure(iso, out, strings.ToLower(prefix)) {
		return true
	}
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
				if os.WriteFile(pair[1], data, 0644) == nil {
					done = true
				}
			}
		}
	}
	if !done {
		fmt.Println("ERROR! Could not read the ISO (not ISO9660) and no bsdtar, 7z or isoinfo fallback is available.")
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
		if _, err := os.Stat(input); err != nil {
			fmt.Println("ERROR! The input must be a valid URL or path to an ISO, IMG, or QCOW2 file.")
			return 1
		}
		fmt.Println("Moving image to VM dir")
		base := filepath.Base(input)
		if err := os.Rename(input, filepath.Join(vmPath, base)); err != nil {
			// cross-device: copy+remove fallback
			if cp := copyFile(input, filepath.Join(vmPath, base)); cp != nil {
				fmt.Println(cp)
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
		fmt.Println("ERROR! Only ISO,IMG and QCOW2 file types are supported for --create-config")
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
// cryptSalt: random SHA512-crypt salt chars — simia-tech/crypt expects
// "$6$<salt>" as the setting string.
func cryptSalt(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./"
	b := make([]byte, n)
	rb := make([]byte, n)
	if _, err := rand.Read(rb); err != nil {
		copy(rb, "quickemu")
	}
	for i := range b {
		b[i] = chars[int(rb[i])%len(chars)]
	}
	return string(b)
}

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
	userName := os.Getenv("USER")
	if userName == "" {
		userName = "quickemu"
	}
	userData := "#cloud-config\nautoinstall:\n  version: 1\n  identity:\n" +
		"    hostname: " + vmPath + "\n    username: " + userName +
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
	case "dragonflybsd":
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

// extract7z: pure-Go 7z extraction (bodgit/sevenzip) — mirrors
// `7z e -o<dir>` flat extraction, no external binary needed.
func extract7z(arc, dir string) bool {
	r, err := sevenzip.OpenReader(arc)
	if err != nil {
		fmt.Println("ERROR! could not read 7z archive: " + err.Error())
		return false
	}
	defer r.Close()
	ok := false
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		out, err := os.Create(filepath.Join(dir, filepath.Base(f.Name)))
		if err == nil {
			io.Copy(out, rc)
			out.Close()
			ok = true
		}
		rc.Close()
	}
	return ok
}

func run(name string, args ...string) error {
	bin, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	return exec.Command(bin, args...).Run()
}

// decompressGZ/XZ/Zstd/BZ2 + extractZip: pure-Go decompressors so the
// pipeline does not depend on host gzip/xz/zstd/bzip2/unzip binaries.
func decompressGZ(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	w, err := os.Create(strings.TrimSuffix(p, ".gz"))
	if err != nil {
		return false
	}
	defer w.Close()
	if _, err := io.Copy(w, gr); err != nil {
		return false
	}
	os.Remove(p)
	return true
}

func decompressBZ2(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	w, err := os.Create(strings.TrimSuffix(p, ".bz2"))
	if err != nil {
		return false
	}
	defer w.Close()
	if _, err := io.Copy(w, bzip2.NewReader(f)); err != nil {
		return false
	}
	os.Remove(p)
	return true
}

func decompressXZ(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	xr, err := xz.NewReader(f)
	if err != nil {
		return false
	}
	w, err := os.Create(strings.TrimSuffix(p, ".xz"))
	if err != nil {
		return false
	}
	defer w.Close()
	if _, err := io.Copy(w, xr); err != nil {
		return false
	}
	os.Remove(p)
	return true
}

func decompressZstd(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		return false
	}
	w, err := os.Create(strings.TrimSuffix(p, ".zst"))
	if err != nil {
		return false
	}
	defer w.Close()
	if _, err := io.Copy(w, zr.IOReadCloser()); err != nil {
		return false
	}
	return true
}

func extractZip(zipPath, dest string) bool {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return false
	}
	defer zr.Close()
	for _, zf := range zr.File {
		d := filepath.Join(dest, zf.Name)
		if zf.FileInfo().IsDir() {
			os.MkdirAll(d, 0755)
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		os.MkdirAll(filepath.Dir(d), 0755)
		w, err := os.Create(d)
		if err == nil {
			io.Copy(w, rc)
			w.Close()
		}
		rc.Close()
	}
	return true
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
		fmt.Println(err)
		return 1
	}
	fmt.Println("Downloading Spice drivers...")
	fetch("https://www.spice-space.org/download/windows/spice-webdavd/spice-webdavd-x64-latest.msi",
		filepath.Join(dir, "spice-webdavd-x64-latest.msi"), "")
	fetch("https://www.spice-space.org/download/windows/vdagent/vdagent-win-0.10.0/spice-vdagent-x64-0.10.0.msi",
		filepath.Join(dir, "spice-vdagent-x64-0.10.0.msi"), "")
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
