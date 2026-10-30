// ops_lifecycle.go — destructive relocate ops extracted from run()'s
// op switch: delete (trash/rm with plan-id guard), rename, move.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runDelete(dir string, rest []string) {
	// delete_vm: confinement + plan-id guard, then conf+dir+shortcut
	// to trash (default) or rm -rf (--no-trash / --hard-delete).
	name := filepath.Base(dir)
	conf := dir + ".conf"
	planID, useTrash := "", false
	for _, a := range rest {
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
			shown := actual
			if actual == "" || actual == "sha256:empty" {
				code = "PLAN_EXPIRED"
			}
			if shown == "" {
				shown = "none"
			}
			die(code, fmt.Sprintf("delete plan is stale (expected %s, got %s); regenerate with --delete-plan", planID, shown))
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
			// qe_event "vm.trashed" — lands inside the trashed dir's
			// event log, inspectable via --trash-restore/--events.
			vmEvent(filepath.Join(slot, "vm", filepath.Base(dir)), name,
				"vm.trashed", map[string]any{"slot": slot})
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
}

func runRename(dir string, rest []string) {
	name := ""
	if len(rest) > 0 {
		name = rest[0]
	}
	if !validVMName(name) {
		die("INVALID_ARGS", "rename needs a valid new name")
	}
	// rename_vm: the conf file is renamed alongside the dir
	// (mv VM → <parent>/<new>.conf), with the old name's path
	// component rewritten (s|old/|new/|g) and every step rolled
	// back on failure.
	parent := filepath.Dir(dir)
	dst := filepath.Join(parent, name)
	newConf := filepath.Join(parent, name+".conf")
	if _, err := os.Stat(dst); err == nil {
		die("RENAME_FAILED", name+" already exists.")
	}
	if _, err := os.Stat(newConf); err == nil {
		die("RENAME_FAILED", name+" already exists.")
	}
	// Renaming a live VM moves the pidfile/sockets out from under
	// the running guest — liveness tracking then reports stopped
	// and the next disk op can run unguarded.
	vmStoppedOrDie(dir, "renaming")
	conf := vmConfPath(dir)
	oldName := filepath.Base(dir)
	if conf != "" {
		oldName = strings.TrimSuffix(filepath.Base(conf), ".conf")
	}
	movedDir := false
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		if err := os.Rename(dir, dst); err != nil {
			die("RENAME_FAILED", "Could not move "+dir+".")
		}
		movedDir = true
	}
	confOut := ""
	if conf != "" {
		if strings.HasPrefix(conf, dir+string(os.PathSeparator)) {
			// Conf inside the VM dir rode along with the dir move.
			confOut = filepath.Join(dst, filepath.Base(conf))
		} else {
			if err := os.Rename(conf, newConf); err != nil {
				if movedDir {
					os.Rename(dst, dir)
				}
				die("RENAME_FAILED", "Could not rename "+conf+"; rolled back.")
			}
			confOut = newConf
		}
		if b, err := os.ReadFile(confOut); err == nil {
			nb := []byte(strings.ReplaceAll(string(b), oldName+"/", name+"/"))
			if err := os.WriteFile(confOut, nb, 0644); err != nil {
				if movedDir {
					os.Rename(dst, dir)
				}
				if confOut == newConf {
					os.Rename(newConf, conf)
				}
				die("RENAME_FAILED", "Could not rewrite "+name+".conf; rolled back.")
			}
		}
	}
	// qe_event "vm.renamed" — the events file kept the old name and
	// moved with the dir, so seq continues inside the new dir.
	vmEvent(dst, oldName, "vm.renamed", map[string]any{"newName": name})
	info(" - Renamed " + oldName + " to " + name)
	okMap(map[string]any{"renamed": dst})
}

func runMove(dir string, rest []string) {
	dst := ""
	if len(rest) > 0 {
		dst = rest[0]
	}
	if dst == "" {
		die("INVALID_ARGS", "move needs a destination dir")
	}
	// move_vm: DEST is the new parent — the VM lands at
	// DEST/<vmdir-basename> + DEST/<vmname>.conf via mv (not a
	// copy), old-dir path prefixes in the conf are repointed, and
	// the conf move rolls the dir move back on failure.
	vmStoppedOrDie(dir, "moving")
	if st, err := os.Stat(dst); err != nil || !st.IsDir() {
		die("MOVE_FAILED", "Destination "+dst+" does not exist.")
	}
	conf := vmConfPath(dir)
	name := filepath.Base(dir)
	if conf != "" {
		name = strings.TrimSuffix(filepath.Base(conf), ".conf")
	}
	newDir := filepath.Join(dst, filepath.Base(dir))
	newConf := filepath.Join(dst, name+".conf")
	if _, err := os.Stat(newDir); err == nil {
		die("MOVE_FAILED", name+" already exists in "+dst+".")
	}
	if _, err := os.Stat(newConf); err == nil {
		die("MOVE_FAILED", name+" already exists in "+dst+".")
	}
	movedDir := false
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		if err := os.Rename(dir, newDir); err != nil {
			// Cross-filesystem move: Bash's mv falls back to
			// copy+remove transparently.
			if cpDir(dir, newDir) == nil && os.RemoveAll(dir) == nil {
				movedDir = true
			}
		} else {
			movedDir = true
		}
		if !movedDir {
			die("MOVE_FAILED", "Could not move "+dir+".")
		}
	}
	if conf != "" {
		confDst := newConf
		if strings.HasPrefix(conf, dir+string(os.PathSeparator)) {
			confDst = filepath.Join(newDir, filepath.Base(conf))
		} else if err := os.Rename(conf, newConf); err != nil {
			if movedDir {
				os.Rename(newDir, dir)
			}
			die("MOVE_FAILED", "Could not move "+conf+"; rolled back.")
		}
		// sed -i s|OLD_DIR/|NEW_DIR/|g — Bash marks it || true:
		// a rewrite failure must not roll the move back.
		if b, err := os.ReadFile(confDst); err == nil {
			nb := []byte(strings.ReplaceAll(string(b), dir+"/", newDir+"/"))
			os.WriteFile(confDst, nb, 0644)
		}
	}
	// qe_event "vm.moved" — lands in the moved dir's event log
	// (Bash's post-mv emit is dead code; landing it is fix-forward).
	vmEvent(newDir, name, "vm.moved", map[string]any{"dest": dst})
	info(" - Moved " + name + " to " + dst)
	okMap(map[string]any{"moved": dst})
}
