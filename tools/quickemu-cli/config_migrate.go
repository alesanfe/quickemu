// config_migrate.go: `quickemu config <verb>` — the QE-111/112
// config-file command surface. Verbs delegate to the quickemu-toml
// sibling binary, which owns the TOML schema, migration and
// transactional editing; this file only maps argv.
//
//	quickemu config migrate   [--vm x.conf | x.conf] [--dry-run]
//	quickemu config set       [--vm x.conf] <key> <value> [--history n]
//	quickemu config get       [--vm x.conf] <key>
//	quickemu config validate  [--vm x.conf]
//
// The .conf (or .toml) target comes from --vm/--file, the global
// vmConf when set by a preceding --vm, or a positional *.conf.
package main

import (
	"fmt"
	"os"
	"strings"
)

// actConfig parses `quickemu config ...` tail args and runs the
// mapped quickemu-toml subcommand, streaming its stdout.
func actConfig(args []string) int {
	verb, rest := "", args
	if len(rest) > 0 {
		verb, rest = rest[0], rest[1:]
	}
	file, dry, history := vmConf, dryRunFlag, ""
	var positional []string
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--vm", "--file":
			if i+1 < len(rest) {
				i++
				file = rest[i]
			}
		case "--dry-run":
			dry = true
		case "--history":
			if i+1 < len(rest) {
				i++
				history = rest[i]
			}
		default:
			positional = append(positional, rest[i])
		}
	}
	// `config migrate x.conf` — a positional *.conf names the target.
	if file == "" && len(positional) > 0 &&
		(strings.HasSuffix(positional[0], ".conf") || strings.HasSuffix(positional[0], ".toml")) {
		file, positional = positional[0], positional[1:]
	}
	if file == "" {
		fmt.Fprintln(os.Stderr, "ERROR! config "+verb+" needs a target: --vm <conf>")
		return 2
	}
	switch verb {
	case "migrate":
		a := []string{"migrate", "--file", file}
		if dry {
			a = append(a, "--dry-run")
		}
		out, err := tool("quickemu-toml", a...)
		fmt.Print(out)
		return rc(err)
	case "set":
		if len(positional) < 2 {
			fmt.Fprintln(os.Stderr, "usage: quickemu config set [--vm x.conf] <key> <value> [--history n]")
			return 2
		}
		a := []string{"set", "--file", file, "--key", positional[0], "--value", positional[1]}
		if history != "" {
			a = append(a, "--history", history)
		}
		out, err := tool("quickemu-toml", a...)
		fmt.Print(out)
		return rc(err)
	case "get":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "usage: quickemu config get [--vm x.conf] <key>")
			return 2
		}
		out, err := tool("quickemu-toml", "get", "--file", file, "--key", positional[0])
		fmt.Print(out)
		return rc(err)
	case "validate":
		out, err := tool("quickemu-toml", "validate", "--file", file)
		fmt.Print(out)
		return rc(err)
	default:
		fmt.Fprintln(os.Stderr,
			"usage: quickemu config <migrate|set|get|validate> [--vm x.conf] [args]")
		return 2
	}
}
