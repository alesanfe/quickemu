// ops.go: the remaining vm.sh surface — export_vm, import_vm,
// autostart_create, shortcut_create/delete_shortcut, backup_*,
// trash_*, vfio_scan, snapshot_info, delete_disk.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// runtimeExts: artifacts excluded from exports/delete-plans.
var runtimeExts = []string{".pid", ".sock", ".socket", ".ports", ".spice", ".monitor", ".lock", ".runtime.json"}

func isRuntimeArtifact(name string) bool {
	for _, e := range runtimeExts {
		if strings.HasSuffix(name, e) || strings.Contains(name, ".lock") {
			return true
		}
	}
	return false
}

// exportVm: tar.gz of <name>.conf + <name>/ (runtime artifacts
// excluded) + quickvm-manifest.json + quickvm-checksums.sha256 —
// byte-layout parity with export_vm. The conf is the real file the
// user passed (QE_VMCONF), not <dir>.conf: a conf may live inside or
// beside the VM dir (Bash resolves it after its pushd into VMPATH).
func exportVm(dir, out string) int {
	name := filepath.Base(dir)
	parent := filepath.Dir(dir)
	conf := filepath.Join(parent, name+".conf")
	if qc := os.Getenv("QE_VMCONF"); qc != "" {
		if abs, err := filepath.Abs(qc); err == nil {
			qc = abs
		}
		if _, err := os.Stat(qc); err == nil {
			conf = qc
			parent = filepath.Dir(conf)
			name = strings.TrimSuffix(filepath.Base(conf), filepath.Ext(conf))
		}
	}
	if _, err := os.Stat(conf); err != nil {
		die("NO_CONF", "no "+conf)
	}
	// Member list: conf + every non-runtime file under the VM dir
	// (<parent>/<name>/ — Bash only archives the convention dir named
	// after the VM, present or not).
	var members []string
	members = append(members, name+".conf")
	vdir := filepath.Join(parent, name)
	if st, err := os.Stat(vdir); err == nil && st.IsDir() {
		filepath.Walk(vdir, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(parent, p)
			if !isRuntimeArtifact(fi.Name()) {
				members = append(members, filepath.ToSlash(rel))
			}
			return nil
		})
	}
	sort.Strings(members[1:])
	if out == "" {
		out = name + ".quickvm.tar.gz"
	}
	f, err := os.Create(out)
	if err != nil {
		die("EXPORT_FAILED", err.Error())
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	// Checksum file accumulated while streaming members.
	var sums strings.Builder
	writeFile := func(arcName, diskPath string) error {
		b, err := os.ReadFile(diskPath)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		sums.WriteString(fmt.Sprintf("%x  %s\n", sum, arcName))
		tw.WriteHeader(&tar.Header{Name: arcName, Mode: 0644, Size: int64(len(b)), ModTime: time.Now()})
		_, err = tw.Write(b)
		return err
	}
	for _, m := range members {
		if err := writeFile(m, filepath.Join(parent, filepath.FromSlash(m))); err != nil {
			die("EXPORT_FAILED", err.Error())
		}
	}
	mb, _ := json.Marshal(map[string]any{
		"formatVersion": 1, "quickemuVersion": vmVersion(),
		"createdAt": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"vm":        name, "checksums": "quickvm-checksums.sha256"})
	manifest := string(mb) + "\n"
	tw.WriteHeader(&tar.Header{Name: "quickvm-manifest.json", Mode: 0644, Size: int64(len(manifest))})
	tw.Write([]byte(manifest))
	sumb := sums.String()
	tw.WriteHeader(&tar.Header{Name: "quickvm-checksums.sha256", Mode: 0644, Size: int64(len(sumb))})
	tw.Write([]byte(sumb))
	infof(" - Exported %s to %s\n", name, out)
	okMap(map[string]any{"export": out})
	return 0
}

func vmVersion() string { return "5.0.0" }

// importVm: safe extract of a .quickvm.tar.gz into the cwd —
// anti-bomb limits, path whitelist, manifest/checksum verification.
func importVm(in, destDir string) int {
	st, err := os.Stat(in)
	if err != nil || st.IsDir() {
		fmt.Fprintln(os.Stderr, "ERROR! '--import' needs an existing .tar.gz file.")
		return 1
	}
	f, err := os.Open(in)
	if err != nil {
		die("IMPORT_FAILED", err.Error())
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! Not a gzip tar.")
		return 1
	}
	// Pass 1: enumerate + validate members (same limits as Bash).
	tr := tar.NewReader(gz)
	var members []string
	base := ""
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		members = append(members, h.Name)
		if len(members) > 100000 {
			die("UNSAFE_ARCHIVE", fmt.Sprintf("Archive has >100000 members."))
		}
		if strings.HasPrefix(h.Name, "/") || h.Name == ".." ||
			strings.HasPrefix(h.Name, "../") || strings.Contains(h.Name, "/..") {
			die("UNSAFE_ARCHIVE", "Unsafe path in archive: '"+h.Name+"'")
		}
		if strings.Count(h.Name, "/") > 16 {
			die("UNSAFE_ARCHIVE", "Path too deep in archive: '"+h.Name+"'")
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
			die("UNSAFE_ARCHIVE", "Archive contains links or special files; refusing import.")
		}
		if base == "" && strings.HasSuffix(h.Name, ".conf") {
			base = strings.TrimSuffix(filepath.Base(h.Name), ".conf")
		}
	}
	f.Close()
	if base == "" {
		fmt.Fprintln(os.Stderr, "ERROR! No .conf found inside "+in+".")
		return 1
	}
	if base == "." || base == ".." || strings.ContainsAny(base, "/") || strings.HasPrefix(base, "-") {
		fmt.Fprintln(os.Stderr, "ERROR! Unsafe VM name '"+base+"' in archive.")
		return 1
	}
	if _, err := os.Stat(filepath.Join(destDir, base+".conf")); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", base)
		return 1
	}
	if _, err := os.Stat(filepath.Join(destDir, base)); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", base)
		return 1
	}
	// Pass 2: extract to a private staging dir.
	f2, _ := os.Open(in)
	defer f2.Close()
	gz2, _ := gzip.NewReader(f2)
	tr2 := tar.NewReader(gz2)
	tmp, err := os.MkdirTemp("", "quickemu-import.")
	if err != nil {
		die("IMPORT_FAILED", err.Error())
	}
	defer os.RemoveAll(tmp)
	for {
		h, err := tr2.Next()
		if err != nil {
			break
		}
		dest := filepath.Join(tmp, h.Name)
		if h.FileInfo().IsDir() {
			os.MkdirAll(dest, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(dest), 0755)
		w, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, h.FileInfo().Mode())
		if err != nil {
			fmt.Fprintln(os.Stderr, "ERROR! Failed to extract "+in+".")
			return 1
		}
		io.Copy(w, tr2)
		w.Close()
	}
	// Manifest + checksum verification.
	mf := filepath.Join(tmp, "quickvm-manifest.json")
	if b, err := os.ReadFile(mf); err == nil {
		var m struct {
			FormatVersion int    `json:"formatVersion"`
			VM            string `json:"vm"`
		}
		json.Unmarshal(b, &m)
		if m.FormatVersion > 1 {
			die("UNSAFE_ARCHIVE", fmt.Sprintf("Unsupported manifest formatVersion '%d'.", m.FormatVersion))
		}
		if m.VM != "" && m.VM != base {
			die("IMPORT_CONFLICT", fmt.Sprintf("Manifest vm '%s' does not match '%s'.", m.VM, base))
		}
		sf := filepath.Join(tmp, "quickvm-checksums.sha256")
		sb, err := os.ReadFile(sf)
		if err != nil {
			die("UNSAFE_ARCHIVE", "Manifest present but checksums missing.")
		}
		for _, l := range strings.Split(string(sb), "\n") {
			flds := strings.Fields(l)
			if len(flds) != 2 {
				continue
			}
			mb, err := os.ReadFile(filepath.Join(tmp, filepath.FromSlash(flds[1])))
			if err != nil || fmt.Sprintf("%x", sha256.Sum256(mb)) != flds[0] {
				die("CHECKSUM_MISMATCH", "Archive checksum verification failed.")
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, " - WARNING! %s has no manifest; integrity is not verified.\n", in)
	}
	// Install: move staged entries into destDir.
	ents, _ := os.ReadDir(tmp)
	for _, e := range ents {
		src := filepath.Join(tmp, e.Name())
		dst := filepath.Join(destDir, e.Name())
		if err := os.Rename(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! Failed to install %s.\n", e.Name())
			return 1
		}
	}
	infof(" - Imported %s from %s\n", base, in)
	fmt.Fprintf(os.Stderr, " - WARNING! %s.conf is executed as code on launch; review it before running the VM.\n", base)
	okMap(map[string]any{"import": base})
	return 0
}

// autostartCreate: systemd user unit (autostart_create).
// cloneVm: clone_vm parity — sibling <name>.conf + new dir with the
// non-runtime files and a qcow2 backed by the source disk (default)
// or a full qemu-img convert copy (--full). Rollback on any failure.
func cloneVm(dir, name string, full bool) int {
	parent := filepath.Dir(dir)
	newDir := filepath.Join(parent, name)
	conf := vmConfPath(dir)
	newConf := filepath.Join(parent, name+".conf")
	vmname := filepath.Base(dir)
	if conf != "" {
		vmname = strings.TrimSuffix(filepath.Base(conf), ".conf")
	}
	if _, err := os.Stat(newDir); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", name)
		return 1
	}
	if _, err := os.Stat(newConf); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", name)
		return 1
	}
	if err := os.MkdirAll(newDir, 0755); err != nil {
		die("CLONE_FAILED", err.Error())
	}
	rollback := func() {
		os.RemoveAll(newDir)
		os.Remove(newConf)
		fmt.Fprintf(os.Stderr, " - Clone aborted; removed partial %s.\n", name)
	}
	// Copy the config, rewriting references to the old VM directory.
	if conf != "" {
		b, err := os.ReadFile(conf)
		if err != nil {
			rollback()
			die("CLONE_FAILED", err.Error())
		}
		b = []byte(strings.ReplaceAll(string(b), vmname+"/", name+"/"))
		if err := os.WriteFile(newConf, b, 0644); err != nil {
			rollback()
			die("CLONE_FAILED", err.Error())
		}
	}
	disk := confDisk(dir)
	skip := func(base string) bool {
		if base == filepath.Base(disk) {
			return true
		}
		for _, suf := range []string{".pid", ".sock", ".socket", ".ports", ".log", ".sh",
			".spice", ".monitor", ".runtime.json", ".events.jsonl", ".paused",
			".lock", ".lock.shared", ".qmp"} {
			if strings.HasSuffix(base, suf) {
				return true
			}
		}
		return base == "ops" || base == "backups"
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		// Skip the clone artifacts themselves when the new dir/conf
		// lands inside the source dir, plus dotfiles (Bash's `*` glob
		// never matches them).
		if e.Name() == name || e.Name() == name+".conf" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if skip(e.Name()) {
			continue
		}
		src := filepath.Join(dir, e.Name())
		dst := filepath.Join(newDir, e.Name())
		var err error
		if e.IsDir() {
			err = cpDir(src, dst)
		} else {
			info, _ := e.Info()
			err = cpFile(src, dst, info.Mode())
		}
		if err != nil {
			rollback()
			die("CLONE_FAILED", err.Error())
		}
	}
	if _, err := os.Stat(disk); err == nil {
		qi, qerr := exec.LookPath("qemu-img")
		if qerr != nil {
			rollback()
			die("NO_QEMU_IMG", "qemu-img not found")
		}
		dst, _ := filepath.Abs(filepath.Join(newDir, filepath.Base(disk)))
		srcAbs, _ := filepath.Abs(disk)
		fmt_ := confGet(dir, "disk_format")
		if fmt_ == "" {
			fmt_ = "qcow2"
		}
		var cerr error
		if full {
			cerr = exec.Command(qi, "convert", "-O", fmt_, srcAbs, dst).Run()
		} else {
			cerr = exec.Command(qi, "create", "-f", "qcow2", "-b", srcAbs, "-F", fmt_, dst).Run()
		}
		if cerr != nil {
			rollback()
			die("CLONE_FAILED", cerr.Error())
		}
		if full {
			infof(" - Cloned %s to %s (full disk copy).\n", vmname, name)
		} else {
			infof(" - Cloned %s to %s (shared backing disk).\n", vmname, name)
		}
	} else {
		infof(" - Cloned %s to %s (config only; no disk image found).\n", vmname, name)
	}
	okMap(map[string]any{"clone": newDir})
	return 0
}

// launcherPath: the `quickemu` CLI the unit/shortcut should invoke.
// QE_LAUNCHER is set by quickemu-cli (its own argv0); otherwise prefer
// a `quickemu` on PATH, then our own binary as last resort.
func launcherPath() string {
	if l := os.Getenv("QE_LAUNCHER"); l != "" {
		return l
	}
	if l, err := exec.LookPath("quickemu"); err == nil {
		return l
	}
	self, _ := os.Executable()
	return self
}

func autostartCreate(name, confPath string) int {
	unitDir := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user")
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		unitDir = filepath.Join(x, "systemd", "user")
	}
	os.MkdirAll(unitDir, 0755)
	unit := filepath.Join(unitDir, "quickemu-"+name+".service")
	self, _ := filepath.EvalSymlinks(launcherPath())
	if self == "" {
		self = launcherPath()
	}
	abs, _ := filepath.Abs(confPath)
	unitBody := "[Unit]\nDescription=Quickemu VM " + name + "\nAfter=default.target\n\n" +
		"[Service]\nExecStart=" + self + " --vm " + abs + "\nRestart=on-failure\n\n" +
		"[Install]\nWantedBy=default.target\n"
	if err := os.WriteFile(unit, []byte(unitBody), 0644); err != nil {
		die("AUTOSTART_FAILED", err.Error())
	}
	if s, err := exec.LookPath("systemctl"); err == nil {
		exec.Command(s, "--user", "daemon-reload").Run()
		exec.Command(s, "--user", "enable", "quickemu-"+name+".service").Run()
	}
	info(" - Autostart unit written: " + unit)
	info("             Start now with: systemctl --user start quickemu-" + name + ".service")
	okMap(map[string]any{"unit": unit})
	return 0
}

// shortcutCreate/Delete: .desktop file management.
func shortcutCreate(name, conf, opts string) int {
	dir := filepath.Join(os.Getenv("HOME"), ".local", "share", "applications")
	os.MkdirAll(dir, 0755)
	file := filepath.Join(dir, name+".desktop")
	body := "[Desktop Entry]\nVersion=1.0\nType=Application\nTerminal=false\n" +
		"Exec=" + filepath.Base(launcherPath()) + " --vm " + conf + " " + opts + "\n" +
		"Path=" + filepath.Dir(conf) + "\nName=" + name + "\nIcon=qemu\n"
	if err := os.WriteFile(file, []byte(body), 0644); err != nil {
		die("SHORTCUT_FAILED", err.Error())
	}
	info(" - " + file + " created.")
	okMap(map[string]any{"shortcut": file})
	return 0
}

func shortcutDelete(name string) {
	file := filepath.Join(os.Getenv("HOME"), ".local", "share", "applications", name+".desktop")
	if _, err := os.Stat(file); err == nil {
		os.Remove(file)
		info(" - Deleted " + file)
	}
}

// backupDir: <vmdir>/backups — backups reuse the export format.
func backupDir(dir string) string {
	return filepath.Join(dir, "backups")
}

// backupVm: export into <vmdir>/backups/<utc>-<vm>.quickvm.tar.gz.
func backupVm(dir string) int {
	bdir := backupDir(dir)
	if err := os.MkdirAll(bdir, 0755); err != nil {
		die("BACKUP_FAILED", err.Error())
	}
	out := filepath.Join(bdir, time.Now().UTC().Format("20060102T150405Z")+"-"+filepath.Base(dir)+".quickvm.tar.gz")
	return exportVm(dir, out)
}

// backupList: enumerate *.quickvm.tar.gz in the backup dir.
func backupList(dir string) int {
	bdir := backupDir(dir)
	m, _ := filepath.Glob(filepath.Join(bdir, "*.quickvm.tar.gz"))
	if outputJSON {
		items := []map[string]any{}
		for _, f := range m {
			st, _ := os.Stat(f)
			var sz int64
			if st != nil {
				sz = st.Size()
			}
			items = append(items, map[string]any{"file": f, "size": sz})
		}
		okMap(map[string]any{"backups": items})
		return 0
	}
	info("Backups in " + bdir + ":")
	for _, f := range m {
		info(" - " + filepath.Base(f))
	}
	return 0
}

// backupPrune: keep the newest N backups.
func backupPrune(dir string, keep int) int {
	if keep <= 0 {
		keep = 5
	}
	bdir := backupDir(dir)
	m, _ := filepath.Glob(filepath.Join(bdir, "*.quickvm.tar.gz"))
	// newest-first by modtime
	sort.Slice(m, func(i, j int) bool {
		si, _ := os.Stat(m[i])
		sj, _ := os.Stat(m[j])
		if si == nil || sj == nil {
			return m[i] > m[j]
		}
		return si.ModTime().After(sj.ModTime())
	})
	for i, f := range m {
		if i >= keep {
			os.Remove(f)
			info(" - Pruned " + filepath.Base(f))
		}
	}
	ok("{}")
	return 0
}

// backupVerify: manifest + checksums must exist and validate.
func backupVerify(fpath string) int {
	f, err := os.Open(fpath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! Backup "+fpath+" not found.")
		return 1
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! Not a gzip tar.")
		return 1
	}
	tr := tar.NewReader(gz)
	var sums map[string]string
	hasManifest := false
	sums = map[string]string{}
	files := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(tr)
		files[h.Name] = b
		if h.Name == "quickvm-manifest.json" {
			hasManifest = true
		}
		if h.Name == "quickvm-checksums.sha256" {
			for _, l := range strings.Split(string(b), "\n") {
				flds := strings.Fields(l)
				if len(flds) == 2 {
					sums[flds[1]] = flds[0]
				}
			}
		}
	}
	if !hasManifest {
		fmt.Fprintln(os.Stderr, "ERROR! No quickvm-manifest.json (not a verified backup).")
		return 1
	}
	bad := false
	for name, want := range sums {
		b, ok := files[name]
		if !ok || fmt.Sprintf("%x", sha256.Sum256(b)) != want {
			bad = true
		}
	}
	if bad {
		fmt.Fprintln(os.Stderr, "ERROR! Backup "+fpath+" failed verification.")
		return 1
	}
	info(" - Backup " + fpath + " verified OK.")
	return 0
}

// backupRestore: verify then import into dest.
func backupRestore(fpath, dest string) int {
	if backupVerify(fpath) != 0 {
		return 1
	}
	return importVm(fpath, dest)
}

// trashRoot: XDG trash location for deleted VMs.
func trashRoot() string {
	return filepath.Join(xdgData(), "quickemu", "trash")
}

func trashList() int {
	root := trashRoot()
	ents, _ := filepath.Glob(filepath.Join(root, "*/"))
	if outputJSON {
		var items []map[string]any
		for _, s := range ents {
			meta, _ := os.ReadFile(filepath.Join(s, "trash-meta.json"))
			if len(meta) == 0 {
				meta = []byte("{}")
			}
			items = append(items, map[string]any{"slot": strings.TrimSuffix(s, "/"),
				"meta": json.RawMessage(meta)})
		}
		okMap(map[string]any{"items": items})
		return 0
	}
	info("Trash contents:")
	for _, s := range ents {
		meta := ""
		if b, err := os.ReadFile(filepath.Join(s, "trash-meta.json")); err == nil {
			var m struct {
				VM string `json:"vm"`
			}
			if json.Unmarshal(b, &m) == nil {
				meta = m.VM
			}
		}
		suffix := ""
		if meta != "" {
			suffix = " (" + meta + ")"
		}
		info(" - " + filepath.Base(strings.TrimSuffix(s, "/")) + suffix)
	}
	return 0
}

func trashEmpty() int {
	root := trashRoot()
	if _, err := os.Stat(root); err != nil {
		info(" - Trash is empty.")
		return 0
	}
	if err := os.RemoveAll(root); err != nil {
		die("TRASH_FAILED", err.Error())
	}
	info(" - Emptied trash (" + root + ")")
	return 0
}

func trashRestore(name string) int {
	root := trashRoot()
	m, _ := filepath.Glob(filepath.Join(root, "*-"+name))
	if len(m) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR! No trash entry for '%s'.\n", name)
		return 1
	}
	if len(m) > 1 {
		fmt.Fprintf(os.Stderr, "ERROR! Multiple trash entries for '%s'; remove extras first.\n", name)
		return 1
	}
	slot := m[0]
	srcDir := filepath.Join(slot, "vm", name)
	srcConf := filepath.Join(slot, "vm", name+".conf")
	confDir := "."
	if b, err := os.ReadFile(filepath.Join(slot, "trash-meta.json")); err == nil {
		var meta struct {
			OriginConfDir string `json:"originConfDir"`
		}
		if json.Unmarshal(b, &meta) == nil && meta.OriginConfDir != "" {
			confDir = meta.OriginConfDir
		}
	}
	if _, err := os.Stat(filepath.Join(confDir, name+".conf")); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists in %s.\n", name, confDir)
		return 1
	}
	if _, err := os.Stat(filepath.Join(confDir, name)); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists in %s.\n", name, confDir)
		return 1
	}
	if st, err := os.Stat(srcDir); err == nil && st.IsDir() {
		os.Rename(srcDir, filepath.Join(confDir, name))
	}
	if err := os.Rename(srcConf, filepath.Join(confDir, name+".conf")); err != nil {
		die("RESTORE_FAILED", err.Error())
	}
	os.RemoveAll(slot)
	info(" - Restored " + name + " from trash into " + confDir)
	okMap(map[string]any{"restored": name})
	return 0
}

// vfioScan: read-only IOMMU/GPU inventory (vfio_scan).
func vfioScan(gpuPassthrough string) int {
	iommu := "absent"
	groups := 0
	if g, err := filepath.Glob("/sys/kernel/iommu_groups/*/devices"); err == nil && len(g) > 0 {
		groups = len(g)
		iommu = "present"
	}
	devs := pciGPUDevices()
	if outputJSON {
		var dj []map[string]any
		for _, d := range devs {
			ready := "false"
			if d.Driver == "vfio-pci" {
				ready = "true"
			}
			dj = append(dj, map[string]any{"pci": d.PCI, "name": d.Name,
				"driver": d.Driver, "vfioReady": ready == "true"})
		}
		okMap(map[string]any{"iommu": iommu, "iommuGroups": groups,
			"configuredPassthrough": gpuPassthrough, "devices": dj})
		return 0
	}
	info("VFIO/IOMMU inventory:")
	infof(" - IOMMU groups: %d (%s)\n", groups, iommu)
	if iommu == "absent" {
		info("   -> Enable intel_iommu=on or amd_iommu=on in the kernel command line.")
	}
	for _, d := range devs {
		suffix := ""
		if d.Driver != "" {
			suffix = " [driver: " + d.Driver + "]"
		}
		infof(" - %s %s%s\n", d.PCI, d.Name, suffix)
		if d.Driver == "vfio-pci" {
			info("     -> bound to vfio-pci; passable via gpu_passthrough=\"" + d.PCI + "\"")
		}
	}
	if gpuPassthrough != "" {
		info(" - Configured gpu_passthrough: " + gpuPassthrough)
	}
	return 0
}

// snapshotInfo: qemu-img info on the disk.
func snapshotInfo(dir string) int {
	disk := confDisk(dir)
	if disk == "" {
		die("NO_DISK", "no disk_img in "+dir)
	}
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		die("NO_QEMU_IMG", "qemu-img not found")
	}
	cmd := exec.Command(qi, "info", disk)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return exitCode(cmd.Run())
}

// snapshotListJSON: qemu-img info --output=json snapshot table
// (schemas/v1/vm-snapshots.schema.json shape).
func snapshotListJSON(dir, vmName string) int {
	disk := confDisk(dir)
	snaps := "[]"
	if disk != "" {
		if qi, err := exec.LookPath("qemu-img"); err == nil {
			out, err := exec.Command(qi, "info", "--output=json", disk).Output()
			if err == nil {
				var j struct {
					Snapshots []map[string]interface{} `json:"snapshots"`
				}
				if json.Unmarshal(out, &j) == nil && j.Snapshots != nil {
					var norm []map[string]interface{}
					for _, s := range j.Snapshots {
						norm = append(norm, map[string]interface{}{
							"id":            s["id"],
							"name":          s["name"],
							"date":          num(s["date-sec"]) + num(s["date-nsec"])/1e9,
							"vm-state-size": num(s["vm-state-size"]),
						})
					}
					b, _ := json.Marshal(norm)
					snaps = string(b)
				}
			}
		}
	}
	okMap(map[string]any{"vm": vmName, "snapshots": json.RawMessage(snaps)})
	return 0
}

func num(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// vmRunningDir: vm_running parity — the VM's pidfile exists, the pid
// is alive AND the process belongs to the VM (comm matches the name,
// its 15-char truncation or qemu*; comm-unreadable hosts fall back to
// plain liveness, same as the Bash legacy path).
func vmRunningDir(dir, name string) bool {
	b, err := os.ReadFile(filepath.Join(dir, name+".pid"))
	if err != nil {
		return false
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid); err != nil || pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil || proc.Signal(syscall.Signal(0)) != nil {
		return false
	}
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return true // cannot verify identity — liveness decides
	}
	c := strings.TrimSpace(string(comm))
	short := name
	if len(short) > 15 {
		short = short[:15]
	}
	return c == name || c == short || strings.HasPrefix(c, "qemu")
}

// deleteDisk: remove the VM disk + EFI vars + shortcut when the VM is
// stopped (delete_disk parity).
func deleteDisk(dir, pidFile string) int {
	if pidFile != "" {
		if b, err := os.ReadFile(pidFile); err == nil && len(b) > 0 {
			die("VM_MUST_BE_STOPPED", "Stop the VM before deleting its disk.")
		}
	}
	name := filepath.Base(dir)
	if c := vmConfPath(dir); c != "" {
		name = strings.TrimSuffix(filepath.Base(c), ".conf")
	}
	info("Deleting " + name + " virtual hard disk")
	disk := confDisk(dir)
	if disk == "" {
		info(" - disk_img not found. Doing nothing.")
		return 0
	}
	os.Remove(disk)
	// Remove any EFI vars, but not for macOS (Bash removes them
	// unconditionally — macos uses OVMF_VARS-1920x1080.fd which the
	// glob also catches, same as Bash).
	for _, pat := range []string{"OVMF_VARS*.fd", name + "-vars.fd"} {
		for _, f := range glob(dir, pat) {
			os.Remove(f)
		}
	}
	info(" - Deleted " + disk)
	shortcutDelete(name)
	okMap(map[string]any{"deleted": disk})
	return 0
}

func glob(dir, pat string) []string {
	m, _ := filepath.Glob(filepath.Join(dir, pat))
	return m
}

// ---- external snapshots (snapshots.sh + snapshots_consolidate.sh) ----

type snapMeta struct {
	ID        string `json:"id"`
	Tag       string `json:"tag"`
	File      string `json:"file"`
	Parent    string `json:"parent"`
	Desc      string `json:"desc"`
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
}

func snapMetaFile(dir string) string { return filepath.Join(dir, "snapshots.json") }

func snapMetaRead(dir string) []snapMeta {
	b, err := os.ReadFile(snapMetaFile(dir))
	if err != nil {
		return nil
	}
	var arr []snapMeta
	json.Unmarshal(b, &arr)
	return arr
}

func snapMetaWrite(dir string, arr []snapMeta) {
	tmp := snapMetaFile(dir) + ".tmp"
	if arr == nil {
		arr = []snapMeta{}
	}
	b, _ := json.Marshal(arr)
	if os.WriteFile(tmp, append(b, '\n'), 0644) == nil {
		os.Rename(tmp, snapMetaFile(dir))
	}
}

// snapshotCreateExternal: qcow2 overlay backed by the current disk,
// repoint conf disk_img at the overlay, register metadata.
func snapshotCreateExternal(dir, tag, desc, confPath string) int {
	if tag == "" {
		fmt.Fprintln(os.Stderr, "ERROR! No snapshot tag.")
		return 1
	}
	disk := confDisk(dir)
	if _, err := os.Stat(disk); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", disk)
		return 1
	}
	for _, s := range snapMetaRead(dir) {
		if s.Tag == tag {
			die("SNAPSHOT_CONFLICT", "external snapshot '"+tag+"' already exists")
		}
	}
	name := filepath.Base(dir)
	if c := vmConfPath(dir); c != "" {
		name = strings.TrimSuffix(filepath.Base(c), ".conf")
	}
	overlay := filepath.Join(dir, name+"-"+tag+".qcow2")
	if _, err := os.Stat(overlay); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", overlay)
		return 1
	}
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		die("NO_QEMU_IMG", "qemu-img not found")
	}
	fmt_ := "qcow2"
	if f := confGet(dir, "disk_format"); f != "" {
		fmt_ = f
	}
	if exec.Command(qi, "create", "-f", "qcow2", "-b", disk, "-F", fmt_, overlay).Run() != nil {
		fmt.Fprintf(os.Stderr, "ERROR! qemu-img failed to create %s.\n", overlay)
		return 1
	}
	// Repoint the conf's disk_img at the overlay. Bash replaces the raw
	// conf value (often a relative path), not the resolved absolute one.
	if confPath != "" {
		if raw := confGet(dir, "disk_img"); raw != "" {
			if b, err := os.ReadFile(confPath); err == nil {
				old := `disk_img="` + raw + `"`
				if strings.Contains(string(b), old) {
					os.WriteFile(confPath, []byte(strings.Replace(string(b), old, `disk_img="`+overlay+`"`, 1)), 0644)
				}
			}
		}
	}
	arr := snapMetaRead(dir)
	arr = append(arr, snapMeta{
		ID:        fmt.Sprintf("snap-%s-%d", time.Now().UTC().Format("20060102150405"), os.Getpid()),
		Tag:       tag,
		File:      overlay,
		Parent:    disk,
		Desc:      desc,
		Type:      "external",
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	})
	snapMetaWrite(dir, arr)
	infof(" - External snapshot '%s' -> %s (parent %s)\n", tag, overlay, disk)
	okMap(map[string]any{"snapshot": tag, "file": overlay, "parent": disk})
	return 0
}

// snapshotTree: render the overlay chain from metadata.
func snapshotTree(dir string) int {
	arr := snapMetaRead(dir)
	disk := confDisk(dir)
	if outputJSON {
		b, _ := json.Marshal(arr)
		okMap(map[string]any{"active": disk, "snapshots": json.RawMessage(b)})
		return 0
	}
	name := filepath.Base(dir)
	if c := vmConfPath(dir); c != "" {
		name = strings.TrimSuffix(filepath.Base(c), ".conf")
	}
	infof("Snapshot chain for %s (active: %s):\n", name, disk)
	for _, s := range arr {
		infof(" - %s\n   %s <- %s\n", s.Tag, s.File, s.Parent)
	}
	if len(arr) == 0 {
		info(" - (no external snapshots)")
	}
	return 0
}

// snapshotVerify: every registered overlay must exist and its
// qemu-img backing must match the recorded parent.
func snapshotVerify(dir string) int {
	rc := 0
	for _, s := range snapMetaRead(dir) {
		if _, err := os.Stat(s.File); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! Snapshot overlay %s missing.\n", s.File)
			rc = 1
			continue
		}
		if s.Parent != "" {
			out, err := exec.Command("qemu-img", "info", s.File).Output()
			actual := ""
			if err == nil {
				for _, l := range strings.Split(string(out), "\n") {
					if strings.HasPrefix(l, "backing file: ") {
						actual = strings.TrimPrefix(l, "backing file: ")
					}
				}
			}
			if actual != s.Parent {
				fmt.Fprintf(os.Stderr, "ERROR! %s backs %s, expected %s.\n", s.File, actual, s.Parent)
				rc = 1
			}
		}
	}
	if rc == 0 {
		info(" - Snapshot chain verified.")
	}
	return rc
}

// snapshotConsolidate: commit a leaf overlay into its parent and
// remove the metadata node (snapshot_consolidate parity).
func snapshotConsolidate(dir, tag string) int {
	if tag == "" {
		fmt.Fprintln(os.Stderr, "ERROR! 'snapshot consolidate' needs a tag.")
		return 1
	}
	arr := snapMetaRead(dir)
	var node *snapMeta
	for i := range arr {
		if arr[i].Tag == tag {
			node = &arr[i]
		}
		if arr[i].Parent == tag {
			die("SNAPSHOT_CONFLICT", "'"+tag+"' has children — consolidate leaves only")
		}
	}
	if node == nil {
		die("SNAPSHOT_CONFLICT", "external snapshot '"+tag+"' not found")
	}
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		die("NO_QEMU_IMG", "qemu-img not found")
	}
	if exec.Command(qi, "commit", node.File).Run() != nil {
		die("CONSOLIDATE_FAILED", "qemu-img commit failed")
	}
	os.Remove(node.File)
	var rest []snapMeta
	for _, s := range arr {
		if s.Tag != tag {
			rest = append(rest, s)
		}
	}
	snapMetaWrite(dir, rest)
	infof(" - Consolidated '%s' into %s.\n", tag, node.Parent)
	okMap(map[string]any{"consolidated": tag, "into": node.Parent})
	return 0
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}
