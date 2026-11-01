package main

import (
	"fmt"
	"os"
	"sort"
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
		if outFmt == "json" {
			emitFail("vm.config", "not_found", vmConf+" not found.")
		} else {
			fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", vmConf)
		}
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
	entries := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		kv := strings.SplitN(l, "=", 2)
		if len(kv) == 2 && keys[strings.TrimSpace(kv[0])] {
			k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
			entries[k] = v
			if outFmt != "json" {
				fmt.Printf("%s=%s\n", k, v)
			}
		}
	}
	if outFmt == "json" {
		emitOK("vm.config", map[string]any{"conf": vmConf, "entries": entries})
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
			if outFmt == "json" {
				if terr != nil {
					emitFail("vm.migrate-conf", "CONVERT_FAILED", terr.Error())
					return 1
				}
				emitOK("vm.migrate-conf", map[string]any{
					"conf": vmConf, "toml": t, "dryRun": true})
				return 0
			}
			fmt.Print(t)
			if terr != nil {
				return 1
			}
		}
		return 0
	}
	if _, err := os.Stat(out); err == nil {
		if outFmt == "json" {
			emitFail("vm.migrate-conf", "ALREADY_EXISTS",
				out+" already exists")
		} else {
			fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", out)
		}
		return 1
	}
	t, err := tool("quickemu-toml", "from-conf", "--file", vmConf)
	if err != nil {
		if outFmt == "json" {
			emitFail("vm.migrate-conf", "CONVERT_FAILED", err.Error())
		} else {
			fmt.Print(t)
		}
		return 1
	}
	if err := os.WriteFile(out, []byte(t), 0644); err != nil {
		if outFmt == "json" {
			emitFail("vm.migrate-conf", "WRITE_FAILED", err.Error())
		} else {
			fmt.Fprintln(os.Stderr, "ERROR! "+err.Error())
		}
		return 1
	}
	if outFmt == "json" {
		emitOK("vm.migrate-conf", map[string]any{
			"conf": vmConf, "toml": out})
	} else {
		fmt.Println(" - Migrated " + vmConf + " -> " + out + " (original kept)")
	}
	return 0
}

// actStatusQuo: `--status-quo` with no action — persist the CLI conf
// overrides into the .conf (the Bash "save state" the GUI menu
// invokes) instead of booting the VM.
func actStatusQuo() int {
	b, err := os.ReadFile(vmConf)
	if err != nil {
		if outFmt == "json" {
			emitFail("vm.status-quo", "not_found", vmConf+" not found.")
		} else {
			fmt.Println("ERROR! " + vmConf + " not found.")
		}
		return 1
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	seen := map[string]bool{}
	for i, l := range lines {
		k := strings.SplitN(l, "=", 2)[0]
		if v, ok := confOpts[k]; ok {
			lines[i] = k + "=\"" + v + "\""
			seen[k] = true
		}
	}
	keys := make([]string, 0, len(confOpts))
	for k := range confOpts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !seen[k] {
			lines = append(lines, k+"=\""+confOpts[k]+"\"")
		}
	}
	if err := os.WriteFile(vmConf, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		if outFmt == "json" {
			emitFail("vm.status-quo", "write_failed", err.Error())
		} else {
			fmt.Println("ERROR! could not write " + vmConf + ": " + err.Error())
		}
		return 1
	}
	if outFmt == "json" {
		emitOK("vm.status-quo", map[string]any{"vm": vmname(), "written": keys})
	} else {
		fmt.Printf(" - Saved %d option(s) to %s.\n", len(keys), vmConf)
	}
	return 0
}
