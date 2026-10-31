// tools/quickemu-launch/conf_parse.go: VM .conf/.toml parsing + conf key helpers (Conf type)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// qemuOptEsc: QEMU option lists split on ',' — a conf path or name
// containing a comma would otherwise inject bogus options
// (file=d,ro=on opens "d"; hostname=x,hostfwd=... adds a forward).
// QEMU's documented escape inside option strings is ',,'.
func qemuOptEsc(v string) string {
	return strings.ReplaceAll(v, ",", ",,")
}

func parseConf(path string) Conf {
	if strings.HasSuffix(path, ".toml") {
		return parseTomlConf(path)
	}
	c := Conf{}
	b, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		v := strings.Trim(strings.TrimSpace(kv[1]), `"'`)
		c[strings.TrimSpace(kv[0])] = v
	}
	return c
}

func (c Conf) get(k, def string) string {
	if v, ok := c[k]; ok && v != "" {
		return v
	}
	return def
}

// parseTomlConf loads a .toml VM config via BurntSushi/toml — the same
// library quickemu-toml wraps (Bash toml.sh parity). The flat config
// model keeps scalar top-level keys only; nested tables/arrays are
// ignored with the same warning as loader.sh.
func parseTomlConf(path string) Conf {
	c := Conf{}
	var doc map[string]any
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		return c
	}
	for k, v := range doc {
		// QE-060/061: [[share]] tables decode as []map[string]any —
		// flatten into share_dirs/share_tags/share_in_dirs before the
		// generic nested-key warning fires.
		if k == "share" {
			flattenShares(c, v)
			continue
		}
		switch x := v.(type) {
		case string:
			c[k] = x
		case bool:
			if x {
				c[k] = "true"
			}
		case int64, float64:
			c[k] = fmt.Sprintf("%v", x)
		case []any:
			// TOML array -> bash-array literal so confList parses both.
			parts := make([]string, 0, len(x))
			for _, e := range x {
				parts = append(parts, fmt.Sprintf("%q", fmt.Sprint(e)))
			}
			c[k] = "(" + strings.Join(parts, " ") + ")"
		default:
			fmt.Fprintf(os.Stderr, "WARNING! Ignoring nested TOML key %q (flat config model).\n", k)
		}
	}
	return c
}

// flattenShares: [[share]] path="..." tag="..." direction="in|out"
// accumulates into space-separated flat keys (matching what a plain
// .conf writes by hand). direction="in" is the QE-061 guest→host
// inbox; anything else is a host→guest share. Accepts either decode
// shape ([]map[string]any from BurntSushi, []any in tests).
func flattenShares(c Conf, v any) {
	var arr []any
	switch t := v.(type) {
	case []map[string]any:
		for _, m := range t {
			arr = append(arr, m)
		}
	case []any:
		arr = t
	default:
		fmt.Fprintf(os.Stderr, "WARNING! Ignoring malformed share key (flat config model).\n")
		return
	}
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		path, _ := m["path"].(string)
		if path == "" {
			fmt.Fprintf(os.Stderr, "WARNING! [[share]] without path — ignored.\n")
			continue
		}
		if dir, _ := m["direction"].(string); dir == "in" {
			c["share_in_dirs"] = strings.TrimSpace(c["share_in_dirs"] + " " + path)
			continue
		}
		c["share_dirs"] = strings.TrimSpace(c["share_dirs"] + " " + path)
		if tag, _ := m["tag"].(string); tag != "" {
			c["share_tags"] = strings.TrimSpace(c["share_tags"] + " " + tag)
		}
	}
}

// dotdir: Bash launches with cwd=vmdir so artifacts render as ./name —
// emit the same prefix when the conf sits in the current directory.
func dotdir(dir string) string {
	if dir == "." {
		return "./"
	}
	return dir + "/"
}

// vmDir: Bash vm_identify parity — runtime state (pidfile, sockets,
// .ports, .log, .runtime.json, locks) lives in the disk's directory:
// dirname(disk_img) resolved against the conf directory for relative
// paths. With no disk_img configured the quickget convention applies:
// a per-VM directory named after the conf, alongside it.
func vmDir(conf Conf, confPath string) string {
	dir := filepath.Dir(confPath)
	name := strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
	if d := strings.TrimSpace(conf.get("disk_img", "")); d != "" {
		if filepath.IsAbs(d) {
			return filepath.Dir(d)
		}
		// Relative disk_img must stay inside the conf dir — "../x"
		// would scatter .pid/.sock/.ports outside the VM tree, and
		// diverge from the CLI's vmdir() which clamps the same way.
		j := filepath.Join(dir, d)
		if rel, err := filepath.Rel(dir, j); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return filepath.Dir(j)
		}
	}
	return filepath.Join(dir, name)
}

// confOn: quickemu conf booleans are "on" (with "true"/"yes" tolerated).
func confOn(c Conf, key string) bool {
	v := c.get(key, "")
	return v == "on" || v == "true" || v == "yes"
}
