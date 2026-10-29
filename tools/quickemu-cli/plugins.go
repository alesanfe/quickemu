package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// actPlugins: plugin_list parity — quickemu-plugin-* on PATH.
func actPlugins() int {
	seen := map[string]bool{}
	var names []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if n := e.Name(); strings.HasPrefix(n, "quickemu-plugin-") && !seen[n] {
				seen[n] = true
				names = append(names, strings.TrimPrefix(n, "quickemu-plugin-"))
			}
		}
	}
	for _, n := range names {
		fmt.Println(n)
	}
	return 0
}

// actPlugin: plugin_dispatch parity — exec quickemu-plugin-<name> with
// the plugin env contract.
func actPlugin(name string, pargs []string) int {
	// The name joins the binary name — a separator or leading dash
	// turns LookPath into a relative-path exec.
	if name == "" || strings.ContainsAny(name, `/\`) ||
		strings.Contains(name, "..") || strings.HasPrefix(name, "-") {
		if outFmt == "json" {
			emitFail("vm.plugin", "INVALID_ARGUMENT", "invalid plugin name")
		} else {
			fmt.Fprintf(os.Stderr, "ERROR! Invalid plugin name %q.\n", name)
		}
		return 1
	}
	bin, err := exec.LookPath("quickemu-plugin-" + name)
	if err != nil {
		if outFmt == "json" {
			emitFail("vm.plugin", "NOT_FOUND", "plugin '"+name+"' not installed")
		} else {
			fmt.Fprintf(os.Stderr, "ERROR! Plugin 'quickemu-plugin-%s' not found on PATH.\n", name)
		}
		return 1
	}
	cmd := execCommand(bin, pargs...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	cmd.Env = append(os.Environ(),
		"VMDIR="+vmdir(), "VMNAME="+vmname(),
		"OUTPUT_FORMAT="+outFmt, "QE_SCHEMA_VERSION=1")
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}
