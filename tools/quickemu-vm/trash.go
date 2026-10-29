package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	// Literal suffix match, not a glob: a name like "a[bc]" is a valid
	// VM name but a metachar pattern — Glob would match other VMs'
	// slots (same fix as the doctor patterns).
	var m []string
	if ents, err := os.ReadDir(root); err == nil {
		for _, e := range ents {
			if e.IsDir() && strings.HasSuffix(e.Name(), "-"+name) {
				m = append(m, filepath.Join(root, e.Name()))
			}
		}
	}
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
		if err := os.Rename(srcDir, filepath.Join(confDir, name)); err != nil {
			die("RESTORE_FAILED", err.Error())
		}
	}
	if err := os.Rename(srcConf, filepath.Join(confDir, name+".conf")); err != nil {
		die("RESTORE_FAILED", err.Error())
	}
	os.RemoveAll(slot)
	info(" - Restored " + name + " from trash into " + confDir)
	okMap(map[string]any{"restored": name})
	return 0
}
