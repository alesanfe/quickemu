// quickemu-vm: filesystem/state VM operations migrated from vm.sh.
// Covers backup, clone, rename, delete (trash), snapshot metadata, and
// move — the operations that don't need a running QEMU.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func die(code, msg string) {
	fmt.Printf("{\"errors\":[{\"code\":\"%s\",\"message\":%q}],\"schemaVersion\":1,\"success\":false}\n", code, msg)
	os.Exit(1)
}

func ok(data string) {
	fmt.Printf("{\"data\":%s,\"errors\":[],\"schemaVersion\":1,\"success\":true}\n", data)
}

func vmConfPath(dir string) string {
	ents, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
	if len(ents) > 0 {
		return ents[0]
	}
	return ""
}

func main() {
	args := os.Args[1:]
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-vm <op> <vmdir> [args]")
		os.Exit(2)
	}
	op, dir := args[0], args[1]
	switch op {
	case "backup":
		// backup_vm: tar the vmdir into .backups/<ts>.tar.gz
		bdir := filepath.Join(dir, ".backups")
		if err := os.MkdirAll(bdir, 0755); err != nil {
			die("BACKUP_FAILED", err.Error())
		}
		ts := time.Now().UTC().Format("20060102-150405")
		dst := filepath.Join(bdir, "backup-"+ts+".tar.gz")
		if err := tarGz(dir, dst); err != nil {
			die("BACKUP_FAILED", err.Error())
		}
		ok(fmt.Sprintf("{\"backup\":%q}", dst))
	case "clone":
		// clone_vm: copy dir + rewrite vm_name in .conf
		name := ""
		if len(args) > 2 {
			name = args[2]
		}
		if name == "" {
			die("INVALID_ARGS", "clone needs a new name")
		}
		dst := filepath.Join(filepath.Dir(dir), name)
		if err := cpDir(dir, dst); err != nil {
			die("CLONE_FAILED", err.Error())
		}
		conf := vmConfPath(dst)
		if conf != "" {
			b, _ := os.ReadFile(conf)
			b = []byte(strings.ReplaceAll(string(b), "vm_name=\""+filepath.Base(dir)+"\"", "vm_name=\""+name+"\""))
			os.WriteFile(conf, b, 0644)
		}
		ok(fmt.Sprintf("{\"clone\":%q}", dst))
	case "delete":
		// delete_vm: move to XDG trash root — same semantics as vm.sh.
		trashRoot := filepath.Join(xdgData(), "quickemu", "trash")
		slot := filepath.Join(trashRoot, time.Now().UTC().Format("20060102-150405")+"-"+filepath.Base(dir))
		if err := os.MkdirAll(filepath.Join(slot, "vm"), 0755); err != nil {
			die("DELETE_FAILED", err.Error())
		}
		dst := filepath.Join(slot, "vm", filepath.Base(dir))
		if err := os.Rename(dir, dst); err != nil {
			die("DELETE_FAILED", err.Error())
		}
		meta := fmt.Sprintf("{\"vm\":%q,\"trashedAt\":%q,\"originConfDir\":%q}", filepath.Base(dir), time.Now().UTC().Format(time.RFC3339), filepath.Dir(dir))
		os.WriteFile(filepath.Join(slot, "trash-meta.json"), []byte(meta), 0644)
		ok(fmt.Sprintf("{\"deleted\":%q,\"restore\":\"--trash-restore %s\"}", dst, filepath.Base(dir)))
	case "rename":
		name := ""
		if len(args) > 2 {
			name = args[2]
		}
		if name == "" {
			die("INVALID_ARGS", "rename needs a new name")
		}
		dst := filepath.Join(filepath.Dir(dir), name)
		if err := os.Rename(dir, dst); err != nil {
			die("RENAME_FAILED", err.Error())
		}
		conf := vmConfPath(dst)
		if conf != "" {
			b, _ := os.ReadFile(conf)
			b = []byte(strings.ReplaceAll(string(b), "vm_name=\""+filepath.Base(dir)+"\"", "vm_name=\""+name+"\""))
			os.WriteFile(conf, b, 0644)
		}
		ok(fmt.Sprintf("{\"renamed\":%q}", dst))
	case "move":
		dst := ""
		if len(args) > 2 {
			dst = args[2]
		}
		if dst == "" {
			die("INVALID_ARGS", "move needs a destination dir")
		}
		if err := cpDir(dir, dst); err != nil {
			die("MOVE_FAILED", err.Error())
		}
		ok(fmt.Sprintf("{\"moved\":%q}", dst))
	case "snapshot-list":
		sdir := filepath.Join(dir, ".snapshots")
		ents, _ := filepath.Glob(filepath.Join(sdir, "*.json"))
		var snaps []map[string]interface{}
		for _, e := range ents {
			var s map[string]interface{}
			if b, err := os.ReadFile(e); err == nil && json.Unmarshal(b, &s) == nil {
				snaps = append(snaps, s)
			}
		}
		j, _ := json.Marshal(snaps)
		ok(string(j))
	default:
		fmt.Fprintf(os.Stderr, "unknown op %q\n", op)
		os.Exit(2)
	}
}

func xdgData() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share")
}
