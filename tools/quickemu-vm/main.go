// quickemu-vm: filesystem/state VM operations migrated from vm.sh.
// Covers backup, clone, rename, delete (trash), snapshot metadata, and
// move — the operations that don't need a running QEMU.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// die: qe_die parity — JSON mode emits the error envelope on stdout;
// human mode writes "ERROR! <msg>" to stderr. Always exits 1.
func die(code, msg string) {
	if !outputJSON {
		fmt.Fprintln(os.Stderr, "ERROR! "+msg)
		os.Exit(1)
	}
	qelib.Fail(opCmd, "quickemu-vm/"+vmVersion(), code, msg)
}

func ok(data string) {
	if !outputJSON {
		return
	}
	qelib.EmitOK(opCmd, "quickemu-vm/"+vmVersion(), json.RawMessage(data))
}

// okMap: typed variant of ok — data is marshalled by encoding/json
// instead of hand-built with fmt.Sprintf.
func okMap(data map[string]any) {
	if !outputJSON {
		return
	}
	qelib.EmitOK(opCmd, "quickemu-vm/"+vmVersion(), data)
}

// info/infof: qe_info parity — human progress lines print only in
// text mode; under --format json the envelope is the sole output.
func info(s string) {
	if !outputJSON {
		fmt.Println(s)
	}
}
func infof(f string, a ...any) {
	if !outputJSON {
		fmt.Printf(f, a...)
	}
}

// opCmd: the v1 "command" name for the running op (qe_emit parity —
// e.g. vm.delete-plan, vm.trash.list). Set in main() per dispatch.
var opCmd = ""

// Emission goes through qelib's canonical envelope
// {schemaVersion, command, success, data, warnings, errors, meta};
// qe_emit parity: nothing is printed unless OUTPUT_FORMAT=json — in
// text mode ops already printed their human lines.

func vmConfPath(dir string) string {
	// QE_VMCONF: the caller (quickemu-cli) knows exactly which conf the
	// user passed; a dir can contain several .conf files (clones).
	if c := os.Getenv("QE_VMCONF"); c != "" {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	ents, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
	if len(ents) > 0 {
		return ents[0]
	}
	return ""
}

// outputJSON toggles the v1 envelope vs human text (--format json).
var outputJSON bool

// deletePlanItems enumerates dir contents excluding runtime artifacts —
// mirrors delete_plan_items' find with the same exclusions and C-locale
// sort order.
func deletePlanItems(dir string) []string {
	excl := map[string]bool{
		".lock": true, ".lock.shared": true, ".runtime.json": true,
		".pid": true, ".sock": true, ".socket": true, ".ports": true,
		".paused": true, ".spice": true, ".monitor": true,
	}
	// Bash lists ${VM} and `find ${VMDIR}` output — paths relative to
	// its post-pushd cwd. Emit cwd-relative spellings for parity.
	cwd, _ := os.Getwd()
	rel := func(p string) string {
		if r, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
		return p
	}
	items := []string{}
	conf := vmConfPath(dir)
	if conf != "" {
		items = append(items, rel(conf))
	}
	filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return nil
		}
		// Ext() alone can't see compound suffixes (.runtime.json),
		// so exclude by suffix match against the artifact list.
		base := filepath.Base(p)
		for e := range excl {
			if e == filepath.Ext(p) || e == base || strings.HasSuffix(base, e) {
				return nil
			}
		}
		items = append(items, rel(p))
		return nil
	})
	// LC_ALL=C sort: bytewise; the conf first then sorted rest (Bash
	// lists ${VM} first, then the find output sorted).
	if len(items) > 1 {
		rest := items[1:]
		for i := 0; i < len(rest); i++ {
			for j := i + 1; j < len(rest); j++ {
				if rest[j] < rest[i] {
					rest[i], rest[j] = rest[j], rest[i]
				}
			}
		}
	}
	return items
}

// deletePlanID: sha256 over the canonical plan serialization —
// "vm=<name>\n" then "<path>|<size>|<mtime>|<type>" per item.
// validVMName: a VM name becomes a sibling dir plus <name>.conf —
// it must stay a single safe path component.
func validVMName(n string) bool {
	return n != "" && n != "." && n != ".." &&
		!strings.ContainsAny(n, `/\`) && !strings.HasPrefix(n, "-") &&
		strings.IndexFunc(n, unicode.IsControl) < 0
}

func deletePlanID(name string, items []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "vm=%s\n", name)
	for _, it := range items {
		line := it
		if st, err := os.Lstat(it); err == nil {
			kind := "file"
			switch {
			case st.IsDir():
				kind = "directory"
			case st.Mode()&os.ModeSymlink != 0:
				kind = "symbolic link"
			}
			line += fmt.Sprintf("|%d|%s|%s", st.Size(), st.ModTime().Format("2006-01-02 15:04:05.000000000 -0700"), kind)
		}
		b.WriteString(line + "\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("sha256:%x", sum)
}

// deletePlanDependents: qcow2s in dir whose backing_file resolves
// inside the same dir (deleting it would orphan the child).
func deletePlanDependents(dir string) []string {
	var out []string
	if _, err := exec.LookPath("qemu-img"); err != nil {
		return out
	}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !strings.HasSuffix(p, ".qcow2") || d.IsDir() {
			return nil
		}
		b, err := execCommand("qemu-img", "info", "--output=json", p).Output()
		if err != nil {
			return nil
		}
		var j struct {
			Backing string `json:"backing-filename"`
		}
		if json.Unmarshal(b, &j) != nil || j.Backing == "" {
			return nil
		}
		bf := j.Backing
		if strings.HasPrefix(bf, dir+"/") || strings.HasPrefix(bf, "./") || strings.HasPrefix(bf, filepath.Base(dir)+"/") {
			out = append(out, p+" -> "+bf)
		}
		return nil
	})
	return out
}

func main() { os.Exit(run()) }

func run() int {
	args := os.Args[1:]
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-vm <op> <vmdir> [args]")
		return 2
	}
	// Bash reads OUTPUT_FORMAT from the environment; --format=json is
	// the CLI-flag equivalent. Env wins only when the flag is absent.
	if os.Getenv("OUTPUT_FORMAT") == "json" {
		outputJSON = true
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--format" && i+1 < len(args) && args[i+1] == "json" {
			outputJSON = true
			args = append(args[:i], args[i+2:]...)
			i--
		} else if args[i] == "--format=json" {
			outputJSON = true
			args = append(args[:i], args[i+1:]...)
			i--
		}
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-vm <op> <vmdir> [args]")
		return 2
	}
	op, dir := args[0], args[1]
	// v1 envelope command name per op (qe_emit's command arg parity).
	opCmd = map[string]string{
		"delete-plan": "vm.delete-plan", "backup-list": "vm.backup.list",
		"trash-list": "vm.trash.list", "snapshot-list": "vm.snapshot.list",
		"snapshot-tree": "vm.snapshot.tree", "vfio-scan": "vm.vfio.scan",
		"stats": "vm.stats", "doctor": "vm.doctor", "plan": "vm.plan",
	}[op]
	if opCmd == "" {
		opCmd = "vm." + op
	}
	switch op {
	case "delete-plan":
		// delete_plan: enumerate what --delete-vm would remove (runtime
		// artifacts excluded), sha256 planId over canonical entries,
		// backing-chain dependents. Never deletes anything.
		name := filepath.Base(dir)
		conf := vmConfPath(dir)
		if conf != "" {
			name = strings.TrimSuffix(filepath.Base(conf), ".conf")
		}
		items := deletePlanItems(dir)
		planID := deletePlanID(name, items)
		dependents := deletePlanDependents(dir)
		if outputJSON {
			arr, _ := json.Marshal(items)
			d := map[string]any{"vm": name, "planId": planID,
				"delete": json.RawMessage(arr)}
			if len(dependents) > 0 {
				dep, _ := json.Marshal(dependents)
				d["dependents"] = json.RawMessage(dep)
			}
			okMap(d)
			return 0
		}
		fmt.Println("Delete plan for " + name + ":")
		if len(items) == 0 {
			fmt.Println(" - nothing to delete")
		}
		for _, it := range items {
			fmt.Println(" - " + it)
		}
		for _, d := range dependents {
			fmt.Println(" - dependent: " + d)
		}
		fmt.Printf("Plan id: %s (required by --delete-vm --plan-id)\n", planID)
	case "backup":
		// backup_vm: export into <vmdir>/backups/<utc>-<vm>.quickvm.tar.gz
		return backupVm(dir)
	case "backup-list":
		return backupList(dir)
	case "backup-verify":
		if len(args) < 3 {
			die("INVALID_ARGS", "backup-verify needs a file")
		}
		return backupVerify(args[2])
	case "backup-restore":
		f, dest := "", "."
		if len(args) > 2 {
			f = args[2]
		}
		if len(args) > 3 {
			dest = args[3]
		}
		if f == "" {
			die("INVALID_ARGS", "backup-restore needs a file")
		}
		return backupRestore(f, dest)
	case "backup-prune":
		keep := 5
		if len(args) > 2 {
			fmt.Sscanf(args[2], "%d", &keep)
		}
		return backupPrune(dir, keep)
	case "export":
		out := ""
		if len(args) > 2 {
			out = args[2]
		}
		return exportVm(dir, out)
	case "import":
		if len(args) < 3 {
			die("INVALID_ARGS", "import needs a .quickvm.tar.gz file")
		}
		return importVm(args[2], ".")
	case "autostart":
		name := filepath.Base(dir)
		conf := vmConfPath(dir)
		if conf != "" {
			name = strings.TrimSuffix(filepath.Base(conf), ".conf")
		}
		return autostartCreate(name, conf)
	case "shortcut":
		name := filepath.Base(dir)
		if conf := vmConfPath(dir); conf != "" {
			name = strings.TrimSuffix(filepath.Base(conf), ".conf")
		}
		opts := ""
		if len(args) > 2 {
			opts = strings.Join(args[2:], " ")
		}
		return shortcutCreate(name, vmConfPath(dir), opts)
	case "shortcut-delete":
		name := filepath.Base(dir)
		if conf := vmConfPath(dir); conf != "" {
			name = strings.TrimSuffix(filepath.Base(conf), ".conf")
		}
		shortcutDelete(name)
		info(" - Shortcut removed for " + name)
		okMap(map[string]any{"shortcutDeleted": name})
	case "trash-list":
		return trashList()
	case "trash-empty":
		return trashEmpty()
	case "trash-restore":
		name := filepath.Base(dir)
		if len(args) > 2 {
			name = args[2]
		}
		return trashRestore(name)
	case "vfio-scan":
		return vfioScan("")
	case "snapshot-info":
		return snapshotInfo(dir)
	case "snapshot-list-json":
		return snapshotListJSON(dir, filepath.Base(dir))
	case "delete-disk":
		pidFile := ""
		if len(args) > 2 {
			pidFile = args[2]
		}
		return deleteDisk(dir, pidFile)
	case "resize":
		// disk_resize: qemu-img resize, VM stopped, size arg required.
		if len(args) < 3 || args[2] == "" {
			fmt.Fprintln(os.Stderr, "ERROR! '--disk-resize' needs a size (e.g. 32G).")
			return 1
		}
		vmStoppedOrDie(dir, "resizing its disk")
		disk := confDisk(dir)
		if _, err := os.Stat(disk); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", disk)
			return 1
		}
		if err := execCommand("qemu-img", "resize", disk, args[2]).Run(); err != nil {
			fmt.Fprintln(os.Stderr, "ERROR! Failed to resize "+disk+".")
			return 1
		}
		info(" - Resized " + disk + " to " + args[2] + ".")
		info("             Remember to grow the partition and filesystem inside the guest.")
		okMap(map[string]any{"resized": disk, "size": args[2]})
	case "compact":
		// disk_compact: qemu-img convert to .compact then atomic swap.
		vmStoppedOrDie(dir, "compacting its disk")
		disk := confDisk(dir)
		if _, err := os.Stat(disk); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", disk)
			return 1
		}
		fmt_ := confGet(dir, "disk_format")
		if fmt_ == "" {
			fmt_ = "qcow2"
		}
		out := disk + ".compact"
		if err := execCommand("qemu-img", "convert", "-O", fmt_, disk, out).Run(); err != nil {
			os.Remove(out)
			fmt.Fprintln(os.Stderr, "ERROR! Failed to compact "+disk+".")
			return 1
		}
		if err := os.Rename(out, disk); err != nil {
			os.Remove(out)
			fmt.Fprintln(os.Stderr, "ERROR! Failed to replace "+disk+".")
			return 1
		}
		info(" - Compacted " + disk)
		okMap(map[string]any{"compacted": disk})
	case "clone":
		// clone_vm parity: sibling conf rewrite + linked backing disk
		// (default) or full disk copy (--full). Runtime artifacts,
		// snapshots metadata, ops and backups are not copied.
		name := ""
		full := false
		for _, a := range args[2:] {
			if a == "--full" {
				full = true
			} else {
				name = a
			}
		}
		if !validVMName(name) {
			die("INVALID_ARGS", "clone needs a valid new name")
		}
		return cloneVm(dir, name, full)
	case "delete":
		// delete_vm: confinement + plan-id guard, then conf+dir+shortcut
		// to trash (default) or rm -rf (--no-trash / --hard-delete).
		name := filepath.Base(dir)
		conf := dir + ".conf"
		planID, useTrash := "", false
		for _, a := range args[2:] {
			if strings.HasPrefix(a, "--plan-id=") {
				planID = strings.TrimPrefix(a, "--plan-id=")
			} else if a == "--trash" {
				useTrash = true
			}
		}
		if c := vmConfPath(dir); c != "" {
			conf = c
			name = strings.TrimSuffix(filepath.Base(c), ".conf")
		}
		// vm_running guard: refuse while the pidfile's process is alive
		// and owned by this VM (delete_vm parity).
		if vmRunningDir(dir, name) {
			die("VM_MUST_BE_STOPPED", "Stop "+name+" before deleting it.")
		}
		// Confinement: only delete the canonical sibling of the conf.
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			expected, _ := filepath.EvalSymlinks(filepath.Join(filepath.Dir(conf), name))
			resolved, _ := filepath.EvalSymlinks(dir)
			if resolved != "" && expected != "" && resolved != expected {
				die("PERMISSION_DENIED", "quickemu refuses to delete '"+resolved+"': not the VM directory "+expected+".")
			}
		}
		if planID != "" {
			actual := deletePlanID(name, deletePlanItems(dir))
			if actual != planID {
				code := "PLAN_MISMATCH"
				if actual == "" || actual == "sha256:empty" {
					code = "PLAN_EXPIRED"
				}
				die(code, fmt.Sprintf("delete plan is stale (expected %s, got %s); regenerate with --delete-plan", planID, actual))
			}
		}
		info("Deleting " + name + " completely")
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			if useTrash {
				slot := filepath.Join(trashRoot(), time.Now().UTC().Format("20060102T150405Z")+"-"+name)
				if err := os.MkdirAll(filepath.Join(slot, "vm"), 0755); err != nil {
					die("DELETE_FAILED", err.Error())
				}
				if err := moveAny(dir, filepath.Join(slot, "vm", filepath.Base(dir))); err != nil {
					die("DELETE_FAILED", "Could not move "+dir+" to trash.")
				}
				if err := moveAny(conf, filepath.Join(slot, "vm", filepath.Base(conf))); err != nil {
					die("DELETE_FAILED", "Could not move "+conf+" to trash.")
				}
				mb, _ := json.Marshal(map[string]any{
					"vm": name, "trashedAt": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
					"originConfDir": filepath.Dir(conf)})
				if err := os.WriteFile(filepath.Join(slot, "trash-meta.json"), mb, 0644); err != nil {
					die("DELETE_FAILED", "trash-meta: "+err.Error())
				}
				info(" - Moved " + name + " to trash (" + slot + "); restore with --trash-restore " + name)
			} else {
				os.RemoveAll(dir)
				os.Remove(conf)
				info(" - Deleted " + conf + " and " + dir + "/")
			}
			shortcutDelete(name)
		} else {
			info(" - " + dir + " not found. Doing nothing.")
		}
		okMap(map[string]any{"deleted": name})
	case "rename":
		name := ""
		if len(args) > 2 {
			name = args[2]
		}
		if !validVMName(name) {
			die("INVALID_ARGS", "rename needs a valid new name")
		}
		// Renaming a live VM moves the pidfile/sockets out from under
		// the running guest — liveness tracking then reports stopped
		// and the next disk op can run unguarded.
		vmStoppedOrDie(dir, "renaming it")
		dst := filepath.Join(filepath.Dir(dir), name)
		if err := os.Rename(dir, dst); err != nil {
			die("RENAME_FAILED", err.Error())
		}
		conf := vmConfPath(dst)
		if conf != "" {
			b, _ := os.ReadFile(conf)
			b = []byte(strings.ReplaceAll(string(b), "vm_name=\""+filepath.Base(dir)+"\"", "vm_name=\""+name+"\""))
			if err := os.WriteFile(conf, b, 0644); err != nil {
				die("RENAME_FAILED", "conf update: "+err.Error())
			}
		}
		okMap(map[string]any{"renamed": dst})
	case "move":
		dst := ""
		if len(args) > 2 {
			dst = args[2]
		}
		if dst == "" {
			die("INVALID_ARGS", "move needs a destination dir")
		}
		// Copying a live VM dir duplicates stale pidfiles/sockets —
		// the copy claims to be running and the source keeps mutating.
		vmStoppedOrDie(dir, "moving it")
		if err := cpDir(dir, dst); err != nil {
			die("MOVE_FAILED", err.Error())
		}
		okMap(map[string]any{"moved": dst})
	case "profile":
		if len(args) < 3 {
			die("INVALID_ARGS", "profile needs a name")
		}
		p, err := ProfileDefaults(args[2])
		if err != nil {
			die("UNKNOWN_PROFILE", err.Error())
		}
		j, _ := json.Marshal(p)
		ok(string(j))
	case "network-preset":
		if len(args) < 3 {
			die("INVALID_ARGS", "network-preset needs a name")
		}
		v, err := NetworkPreset(args[2])
		if err != nil {
			die("UNKNOWN_PRESET", err.Error())
		}
		okMap(map[string]any{"network": v})
	case "linked-clone":
		if len(args) < 4 {
			die("INVALID_ARGS", "linked-clone needs src dst")
		}
		cmd := execCommand("qemu-img", "create", "-f", "qcow2", "-b", args[2], "-F", "qcow2", args[3])
		if err := cmd.Run(); err != nil {
			die("LINKED_CLONE_FAILED", err.Error())
		}
		okMap(map[string]any{"linkedClone": args[3]})
	case "snapshot-create", "snapshot-apply", "snapshot-delete":
		// qemu-img snapshot -c/-a/-d <tag> <disk> — same as vm.sh.
		if len(args) < 3 {
			die("INVALID_ARGS", "snapshot op needs a tag")
		}
		tag := args[2]
		// vm_running guard: qemu-img snapshot on a live disk corrupts it.
		if c := vmConfPath(dir); c != "" {
			pf := filepath.Join(dir, strings.TrimSuffix(filepath.Base(c), ".conf")+".pid")
			if b, err := os.ReadFile(pf); err == nil && len(strings.TrimSpace(string(b))) > 0 {
				fmt.Fprintln(os.Stderr, " - ERROR! Stop the VM before managing snapshots.")
				return 1
			}
		}
		// --external/-external [desc]: qcow2 overlay chain instead of
		// an internal qemu-img snapshot (snapshot_create_external).
		for _, a := range args[3:] {
			if a == "--external" || a == "-external" {
				if op == "snapshot-create" {
					desc := ""
					for j, b := range args[3:] {
						if b != "--external" && b != "-external" && (j == 0 || args[3+j-1] == "--external" || args[3+j-1] == "-external") {
							desc = b
							break
						}
					}
					return snapshotCreateExternal(dir, tag, desc, vmConfPath(dir))
				}
				die("INVALID_ARGS", "--external only applies to snapshot-create")
			}
		}
		disk := confDisk(dir)
		if disk == "" {
			die("NO_DISK", "no disk_img in "+dir)
		}
		flag := map[string]string{"snapshot-create": "-c", "snapshot-apply": "-a", "snapshot-delete": "-d"}[op]
		cmd := execCommand("qemu-img", "snapshot", "-q", flag, tag, disk)
		if err := cmd.Run(); err != nil {
			die("SNAPSHOT_FAILED", err.Error())
		}
		okMap(map[string]any{"snapshot": tag, "op": op})
	case "snapshot-tree":
		return snapshotTree(dir)
	case "snapshot-verify":
		return snapshotVerify(dir)
	case "snapshot-consolidate":
		if len(args) < 3 {
			die("INVALID_ARGS", "snapshot-consolidate needs a tag")
		}
		return snapshotConsolidate(dir, args[2])
	case "snapshot-list":
		sdir := filepath.Join(dir, ".snapshots")
		ents, _ := filepath.Glob(filepath.Join(sdir, "*.json"))
		var snaps []map[string]any
		for _, e := range ents {
			var s map[string]any
			if b, err := os.ReadFile(e); err == nil && json.Unmarshal(b, &s) == nil {
				snaps = append(snaps, s)
			}
		}
		j, _ := json.Marshal(snaps)
		ok(string(j))
	default:
		fmt.Fprintf(os.Stderr, "unknown op %q\n", op)
		return 2
	}
	return 0
}

func xdgData() string { return qelib.XDGData() }

// confGet: read a bare key="value" line from the VM's .conf.
func confGet(dir, key string) string {
	conf := vmConfPath(dir)
	if conf == "" {
		return ""
	}
	f, err := os.Open(conf)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, key+"=") {
			return strings.Trim(strings.SplitN(line, "=", 2)[1], `"'`)
		}
	}
	return ""
}

func confDisk(dir string) string {
	conf := vmConfPath(dir)
	if conf == "" {
		return ""
	}
	f, err := os.Open(conf)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "disk_img=") {
			v := strings.SplitN(line, "=", 2)[1]
			v = strings.Trim(v, `\"'`)
			if filepath.IsAbs(v) {
				return v
			}
			// A relative disk must stay inside the VM dir — ops act on
			// the resolved path (compact/resize/delete), so '..' that
			// escapes would hit files outside the VM.
			joined := filepath.Join(dir, v)
			if rel, err := filepath.Rel(dir, joined); err != nil ||
				rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return ""
			}
			return joined
		}
	}
	return ""
}

// Command construction seam (ADR-0009): tests substitute stubs to
// observe/fake subprocesses. Restore via t.Cleanup; never t.Parallel
// while a stub is installed.
var execCommand = exec.Command

// ProfileDefaults mirrors vm_profile_defaults — conf overrides per preset.
func ProfileDefaults(name string) (map[string]string, error) {
	profiles := map[string]map[string]string{
		"windows-gaming": {"guest_os": "windows", "ram": "8G", "cpu_cores": "6", "video": "virtio-vga-gl"},
		"linux-desktop":  {"guest_os": "linux", "ram": "4G", "cpu_cores": "4"},
		"server":         {"guest_os": "linux", "ram": "4G", "display": "none", "network": "user"},
		"minimal":        {"guest_os": "linux", "ram": "512M", "cpu_cores": "1"},
		"ci":             {"guest_os": "linux", "ram": "2G", "display": "none", "network": "none"},
	}
	p, ok := profiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q", name)
	}
	return p, nil
}

// NetworkPreset mirrors vm_network_preset — maps preset name to network= value.
func NetworkPreset(name string) (string, error) {
	switch name {
	case "nat", "user":
		return "user", nil
	case "bridge":
		return "bridge", nil
	case "hostonly":
		return "host", nil
	case "isolated", "none":
		return "none", nil
	}
	return "", fmt.Errorf("unknown preset %q", name)
}
