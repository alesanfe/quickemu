// quickemu-vm: filesystem/state VM operations migrated from vm.sh.
// Covers backup, clone, rename, delete (trash), snapshot metadata, and
// move — the operations that don't need a running QEMU.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
		"snapshot-list-json": "vm.snapshot.list", "resize": "vm.disk-resize",
		"snapshot-tree": "vm.snapshot.tree", "vfio-scan": "vm.vfio.scan",
		"stats": "vm.stats", "doctor": "vm.doctor", "plan": "vm.plan",
		"compact": "vm.disk-compact", "resize-commit": "vm.disk-resize-commit",
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
		// DEST="${2:-${RESTORE_DIR:-.}}" — env supplies the default
		// when no positional dest is given.
		f, dest := "", "."
		if v := os.Getenv("RESTORE_DIR"); v != "" {
			dest = v
		}
		if len(args) > 2 {
			f = args[2]
		}
		if len(args) > 3 {
			dest = args[3]
		}
		if f == "" {
			die("INVALID_ARGS", "backup-restore needs a file")
		}
		return backupRestore(f, dest, dir)
	case "backup-prune":
		keep := 5
		if len(args) > 2 {
			fmt.Sscanf(args[2], "%d", &keep)
		}
		if k := os.Getenv("BACKUP_KEEP"); k != "" {
			fmt.Sscanf(k, "%d", &keep)
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
		return importVm(args[2], ".", dir)
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
		// NAME="${RESTORE_VM:-${1}}" — env wins over the positional.
		name := filepath.Base(dir)
		if len(args) > 2 {
			name = args[2]
		}
		if v := os.Getenv("RESTORE_VM"); v != "" {
			name = v
		}
		// The name lands in filepath.Join for BOTH the slot source
		// and the restore destination — "../x" moves a crafted
		// slot's files outside the conf dir.
		if !validVMName(name) {
			die("INVALID_ARGS", "trash-restore needs a valid VM name")
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
		vmStoppedOrDie(dir, "resizing")
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
	case "resize-commit":
		// qemu-img commit — merge the disk's changes back into its
		// backing file (external-snapshot commit). VM must be stopped.
		vmStoppedOrDie(dir, "committing its disk")
		disk := confDisk(dir)
		if _, err := os.Stat(disk); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", disk)
			return 1
		}
		if out, err := execCommand("qemu-img", "info", "--output=json", disk).Output(); err == nil {
			var qi struct {
				BackingFilename string `json:"backing-filename"`
			}
			if json.Unmarshal(out, &qi) == nil && qi.BackingFilename == "" {
				die("NO_BACKING", disk+" has no backing file to commit.")
			}
		}
		if err := execCommand("qemu-img", "commit", disk).Run(); err != nil {
			die("COMMIT_FAILED", "qemu-img commit failed for "+disk+".")
		}
		info(" - Committed " + disk + " into its backing file.")
		okMap(map[string]any{"committed": disk})
	case "compact":
		// disk_compact: qemu-img convert to .compact then atomic swap.
		vmStoppedOrDie(dir, "compacting its disk")
		disk := confDisk(dir)
		// ${disk_img:-disk_img} — an unset key reports the literal name.
		shown := disk
		if shown == "" {
			shown = "disk_img"
		}
		if _, err := os.Stat(disk); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", shown)
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
		cname := filepath.Base(dir)
		if c := vmConfPath(dir); c != "" {
			cname = strings.TrimSuffix(filepath.Base(c), ".conf")
		}
		vmEvent(dir, cname, "vm.disk-compacted", nil)
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
		runDelete(dir, args[2:])
	case "rename":
		runRename(dir, args[2:])
	case "move":
		runMove(dir, args[2:])
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
		// clone_vm_linked: [ -f SRC ] check, then qemu-img create -b —
		// its output goes to the user (Bash's `2>&1`), failure is a
		// fixed message, not the raw exec error.
		if _, err := os.Stat(args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! source not found: %s\n", args[2])
			return 1
		}
		cmd := execCommand("qemu-img", "create", "-f", "qcow2", "-b", args[2], "-F", "qcow2", args[3])
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Print(string(out))
			fmt.Fprintln(os.Stderr, "ERROR! linked clone failed.")
			return 1
		}
		okMap(map[string]any{"linkedClone": args[3]})
	case "snapshot-create", "snapshot-apply", "snapshot-delete":
		return runSnapshotOp(op, dir, args)
	case "snapshot-register":
		// QE-021: record a monitor-created snapshot (savevm) — the
		// CLI does the monitor work; the sidecar lives here.
		if len(args) < 3 {
			die("INVALID_ARGS", "snapshot-register needs a tag")
		}
		typ, desc := "vmstate", ""
		if len(args) > 3 {
			typ = args[3]
		}
		if len(args) > 4 {
			desc = args[4]
		}
		return snapshotRegister(dir, args[2], typ, desc)
	case "snapshot-unregister":
		if len(args) < 3 {
			die("INVALID_ARGS", "snapshot-unregister needs a tag")
		}
		return snapshotUnregister(dir, args[2])
	case "snapshot-policy":
		verb, dry := "run", false
		for _, a := range args[2:] {
			if a == "--dry" {
				dry = true
			} else {
				verb = a
			}
		}
		return snapshotPolicy(dir, verb, dry)
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
		snaps = append(snaps, internalSnapshots(dir)...)
		j, _ := json.Marshal(snaps)
		ok(string(j))
		if !outputJSON {
			// Text mode mirrors vm.sh: qemu-img's own table for the
			// internal snapshots, one line per external sidecar.
			if disk := confDisk(dir); disk != "" {
				if qi, err := exec.LookPath("qemu-img"); err == nil {
					if out, err := execCommand(qi, "snapshot", "-l", disk).Output(); err == nil {
						info(strings.TrimRight(string(out), "\n"))
					}
				}
			}
			for _, e := range ents {
				var s map[string]any
				if b, err := os.ReadFile(e); err == nil && json.Unmarshal(b, &s) == nil {
					infof("%v\t%s\n", s["id"], s["tag"])
				}
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown op %q\n", op)
		return 2
	}
	return 0
}
