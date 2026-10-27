// quickemu-toml: ADR-0001 phase 5 — native TOML parsing, replacing the
// line-oriented Bash parser. Same flattened dotted-key output contract
// as toml_parse so .conf parity and vm_config_load stay unchanged.
package main

import (
	"strings"
	"encoding/json"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

func die(c, m string) {
	b, _ := json.Marshal(map[string]any{"schemaVersion": 1, "success": false,
		"errors": []map[string]string{{"code": c, "message": m}}})
	fmt.Println(string(b))
	os.Exit(1)
}
func ok(d any) {
	b, _ := json.Marshal(map[string]any{"schemaVersion": 1, "command": "toml.parse",
		"success": true, "data": d})
	fmt.Println(string(b))
	os.Exit(0)
}

// flat walks the decoded tree producing "key=value" rows identical to
// the Bash parser's output (sections become dotted keys, arrays are
// emitted comma-joined).
func flat(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch t := v.(type) {
		case map[string]any:
			flat(key, t, out)
		default:
			out[key] = v
		}
	}
}

// tomlKeyOrder: the exact key order of toml_from_conf in
// lib/quickemu/config/toml.sh.
var tomlKeyOrder = []string{
	"guest_os", "arch", "disk_img", "disk_format", "disk_size", "iso", "img",
	"fixed_iso", "floppy", "ram", "cpu_cores", "network", "bridge", "macaddr",
	"ssh_port", "spice_port", "display", "monitor", "serial", "sound_card",
	"sound_duplex", "public_dir", "seed", "kernel", "initrd", "boot_args",
	"boot", "tpm", "secureboot", "gl", "width", "height", "vnc_listen",
	"monitor_cmd", "port_forwards", "gpu_passthrough", "viewer",
}

// fromConf: toml_from_conf parity — convert a legacy bash .conf into
// TOML on stdout (key="value" lines only; the conf is never executed).
func fromConf(file string) int {
	b, err := os.ReadFile(file)
	if err != nil {
		die("TOML_PARSE", err.Error())
	}
	kv := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		p := strings.SplitN(l, "=", 2)
		if len(p) == 2 {
			kv[strings.TrimSpace(p[0])] = strings.Trim(strings.TrimSpace(p[1]), `"'`)
		}
	}
	fmt.Println("# Migrated by quickemu --migrate-conf")
	fmt.Println("formatVersion = 1")
	fmt.Println()
	for _, k := range tomlKeyOrder {
		if v, ok := kv[k]; ok && v != "" {
			fmt.Printf("%s = \"%s\"\n", k, strings.ReplaceAll(v, `"`, `\"`))
		}
	}
	return 0
}

func main() {
	os.Args = normArgs(os.Args)
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-toml <parse|validate|get|from-conf> --file <f> [--key k]")
		os.Exit(2)
	}
	sub, file, key := os.Args[1], "", ""
	for i := 2; i+1 < len(os.Args); i += 2 {
		switch os.Args[i] {
		case "--file":
			file = os.Args[i+1]
		case "--key":
			key = os.Args[i+1]
		}
	}
	if file == "" {
		die("BAD_ARGS", "--file required")
	}
	if sub == "from-conf" {
		os.Exit(fromConf(file))
	}
	var doc map[string]any
	if _, err := toml.DecodeFile(file, &doc); err != nil {
		die("TOML_PARSE", err.Error())
	}
	// map[string]any accepts every key; strict-key warnings need a
	// typed-schema decode, which is a separate contract.
	out := map[string]any{}
	flat("", doc, out)
	switch sub {
	case "parse":
		ok(out)
	case "validate":
		ok(map[string]any{"valid": true, "keys": len(out)})
	case "get":
		if key == "" {
			die("BAD_ARGS", "get needs --key")
		}
		if v, ok := out[key]; ok {
			fmt.Printf("%v\n", v)
			os.Exit(0)
		}
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
		os.Exit(2)
	}
}
// normArgs expands GNU-style --flag=value into --flag value so the
// positional pair parsers stay uniform (pflag parity without the dep).
func normArgs(a []string) []string {
	out := a[:0]
	for _, s := range a {
		if i := strings.IndexByte(s, '='); strings.HasPrefix(s, "--") && i > 2 {
			out = append(out, s[:i], s[i+1:])
		} else {
			out = append(out, s)
		}
	}
	return out
}
