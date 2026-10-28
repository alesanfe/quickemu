package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
			cerr = execCommand(qi, "convert", "-O", fmt_, srcAbs, dst).Run()
		} else {
			cerr = execCommand(qi, "create", "-f", "qcow2", "-b", srcAbs, "-F", fmt_, dst).Run()
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
