// quickemu-vm: filesystem/state VM operations migrated from vm.sh.
// Covers backup, clone, rename, delete (trash), snapshot metadata, and
// move — the operations that don't need a running QEMU.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	var items []string
	conf := vmConfPath(dir)
	if conf != "" {
		items = append(items, conf)
	}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || p == dir {
			return nil
		}
		if excl[filepath.Ext(p)] || excl[filepath.Base(p)] {
			return nil
		}
		items = append(items, p)
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
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(p, ".qcow2") || info.IsDir() {
			return nil
		}
		b, err := exec.Command("qemu-img", "info", "--output=json", p).Output()
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

func main() {
	args := os.Args[1:]
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-vm <op> <vmdir> [args]")
		os.Exit(2)
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
		os.Exit(2)
	}
	op, dir := args[0], args[1]
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
			dep, _ := json.Marshal(dependents)
			ok(fmt.Sprintf("{\"vm\":%q,\"planId\":%q,\"delete\":%s,\"dependents\":%s}", name, planID, arr, dep))
			return
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
		ok(fmt.Sprintf("{\"network\":%q}", v))
	case "linked-clone":
		if len(args) < 4 {
			die("INVALID_ARGS", "linked-clone needs src dst")
		}
		cmd := execCommand("qemu-img", "create", "-f", "qcow2", "-b", args[2], "-F", "qcow2", args[3])
		if err := cmd.Run(); err != nil {
			die("LINKED_CLONE_FAILED", err.Error())
		}
		ok(fmt.Sprintf("{\"linkedClone\":%q}", args[3]))
	case "snapshot-create", "snapshot-apply", "snapshot-delete":
		// qemu-img snapshot -c/-a/-d <tag> <disk> — same as vm.sh.
		if len(args) < 3 {
			die("INVALID_ARGS", "snapshot op needs a tag")
		}
		tag := args[2]
		disk := confDisk(dir)
		if disk == "" {
			die("NO_DISK", "no disk_img in "+dir)
		}
		flag := map[string]string{"snapshot-create": "-c", "snapshot-apply": "-a", "snapshot-delete": "-d"}[op]
		cmd := execCommand("qemu-img", "snapshot", "-q", flag, tag, disk)
		if err := cmd.Run(); err != nil {
			die("SNAPSHOT_FAILED", err.Error())
		}
		ok(fmt.Sprintf("{\"snapshot\":%q,\"op\":%q}", tag, op))
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
			return filepath.Join(dir, v)
		}
	}
	return ""
}

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

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
