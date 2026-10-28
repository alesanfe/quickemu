package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// xdgUserDir resolves an XDG user dir natively — same source as the
// xdg-user-dir(1) binary: $XDG_CONFIG_HOME/user-dirs.dirs (defaults
// ~/.config/user-dirs.dirs), honouring enabled=false in
// user-dirs.conf. Removes the subprocess dependency.
func xdgUserDir(name string) string {
	cfg := qelib.XDGConfig()
	if b, err := os.ReadFile(filepath.Join(cfg, "user-dirs.conf")); err == nil && strings.Contains(string(b), "enabled=False") {
		return ""
	}
	f, err := os.Open(filepath.Join(cfg, "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	defer f.Close()
	home := os.Getenv("HOME")
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 || kv[0] != "XDG_"+name+"_DIR" {
			continue
		}
		v := strings.Trim(strings.TrimSpace(kv[1]), `"'`)
		v = strings.Replace(v, "$HOME", home, 1)
		return v
	}
	return ""
}
