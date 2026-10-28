package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// confKV caches the parsed conf keyed on path+mtime+size — ~10
// call sites per command no longer re-read and re-parse the file
// each time, while an on-disk change is still picked up.
var confKVCache = struct {
	sync.Mutex
	path    string
	modTime int64
	size    int64
	m       map[string]string
}{}

func confKV() map[string]string {
	confKVCache.Lock()
	defer confKVCache.Unlock()
	var mt, sz int64
	if st, err := os.Stat(vmConf); err == nil {
		mt, sz = st.ModTime().UnixNano(), st.Size()
	}
	if confKVCache.path == vmConf && confKVCache.modTime == mt &&
		confKVCache.size == sz && confKVCache.m != nil {
		return confKVCache.m
	}
	m := map[string]string{}
	if b, err := os.ReadFile(vmConf); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if i := strings.Index(l, "="); i > 0 && !strings.HasPrefix(l, "#") {
				m[strings.TrimSpace(l[:i])] = strings.Trim(strings.TrimSpace(l[i+1:]), `"'`)
			}
		}
	}
	confKVCache.path, confKVCache.modTime, confKVCache.size, confKVCache.m = vmConf, mt, sz, m
	return m
}

// actInspectConf: inspect_conf parity — read the conf WITHOUT
// executing it (confs are data to Go; foreign confs must be inspected).
func actInspectConf() int {
	b, err := os.ReadFile(vmConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", vmConf)
		return 1
	}
	keys := map[string]bool{}
	for _, k := range []string{"guest_os", "arch", "boot", "tpm", "secureboot", "disk_img",
		"disk_format", "disk_size", "iso", "img", "fixed_iso", "floppy", "ram", "cpu_cores",
		"network", "bridge", "macaddr", "port_forwards", "display", "monitor", "serial",
		"sound_card", "public_dir", "seed", "kernel", "initrd", "boot_args",
		"gpu_passthrough", "vnc_listen"} {
		keys[k] = true
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		kv := strings.SplitN(l, "=", 2)
		if len(kv) == 2 && keys[strings.TrimSpace(kv[0])] {
			fmt.Printf("%s=%s\n", strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
	}
	return 0
}

// actMigrateConf: migrate_conf parity — write a TOML migration of the
// conf next to it; --dry-run prints the TOML (or a diff vs existing).
func actMigrateConf() int {
	out := strings.TrimSuffix(vmConf, ".conf") + ".toml"
	if dryRunFlag {
		if _, err := os.Stat(out); err == nil {
			t, _ := tool("quickemu-toml", "from-conf", "--file", vmConf)
			fmt.Println("Diff (existing " + out + " vs regenerated):")
			fmt.Print(t)
		} else {
			t, terr := tool("quickemu-toml", "from-conf", "--file", vmConf)
			fmt.Print(t)
			if terr != nil {
				return 1
			}
		}
		return 0
	}
	if _, err := os.Stat(out); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", out)
		return 1
	}
	t, err := tool("quickemu-toml", "from-conf", "--file", vmConf)
	if err != nil {
		fmt.Print(t)
		return 1
	}
	if err := os.WriteFile(out, []byte(t), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! "+err.Error())
		return 1
	}
	fmt.Println(" - Migrated " + vmConf + " -> " + out + " (original kept)")
	return 0
}
