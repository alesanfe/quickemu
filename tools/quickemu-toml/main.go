// quickemu-toml: ADR-0001 phase 5 — native TOML parsing, replacing the
// line-oriented Bash parser. Same flattened dotted-key output contract
// as toml_parse so .conf parity and vm_config_load stay unchanged.
//
// Subcommands:
//
//	parse     --file f              flat dotted-key map (v1 envelope)
//	validate  --file f              schema-v1 diagnostics (line:col)
//	get       --file f --key k      one flattened value to stdout
//	from-conf --file f.conf         conf -> TOML text on stdout
//	to-conf   --file f.toml         TOML -> conf text on stdout
//	migrate   --file f.conf [--dry-run]
//	                                QE-111: .conf.bak + diff + write
//	set       --file f --key k --value v [--history n]
//	                                QE-112: transactional key patch
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const version = "0.2.0"

func die(c, m string) int {
	qelib.Emit("toml."+os.Args[1], "quickemu-toml/"+version, false, nil,
		[]qelib.QEError{{Code: c, Message: m, Severity: "error", Retryable: false}})
	return 1
}
func ok(d any) int {
	qelib.EmitOK("toml."+os.Args[1], "quickemu-toml/"+version, d)
	return 0
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
// lib/quickemu/config/toml.sh — the canonical emission order for
// declarative keys.
var tomlKeyOrder = []string{
	"guest_os", "arch", "disk_img", "disk_format", "disk_size", "iso", "img",
	"fixed_iso", "floppy", "ram", "cpu_cores", "network", "bridge", "macaddr",
	"ssh_port", "spice_port", "display", "monitor", "serial", "sound_card",
	"sound_duplex", "public_dir", "seed", "kernel", "initrd", "boot_args",
	"boot", "tpm", "secureboot", "gl", "width", "height", "vnc_listen",
	"monitor_cmd", "port_forwards", "gpu_passthrough", "viewer",
}

// fromConf: toml_from_conf parity — convert a legacy bash .conf into
// TOML on stdout (the conf is parsed as text, never executed).
// Unknown keys are preserved under [extra]; comments travel with the
// key they preceded.
func fromConf(file string) int {
	b, err := os.ReadFile(file)
	if err != nil {
		return die("TOML_PARSE", err.Error())
	}
	doc := parseConf(b)
	for _, w := range doc.warnings {
		fmt.Fprintln(os.Stderr, "warning: "+w)
	}
	fmt.Print(confToTOML(doc))
	return 0
}

// toConf: TOML -> .conf on stdout. [runtime]/foreign tables are
// skipped — runtime state never round-trips (QE-110).
func toConf(file string) int {
	b, err := os.ReadFile(file)
	if err != nil {
		return die("TOML_PARSE", err.Error())
	}
	out, warns := tomlToConf(b)
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "warning: "+w)
	}
	fmt.Print(out)
	return 0
}

// migrate: QE-111 — warnings (stderr), the diff shown BEFORE the
// write, then backup+atomic-write unless --dry-run.
func migrate(file string, dryRun bool) int {
	res, err := migrateConf(file, dryRun)
	if err != nil {
		return die("MIGRATE", err.Error())
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "warning: "+w)
	}
	fmt.Print(res.Diff)
	if dryRun {
		fmt.Println("Dry run — no files written.")
		return 0
	}
	if err := res.commit(); err != nil {
		return die("MIGRATE", err.Error())
	}
	if res.Changed {
		fmt.Printf(" - Migrated %s -> %s (backup %s)\n", file, res.Out, res.Backup)
	} else {
		fmt.Printf(" - %s already up to date (backup %s)\n", res.Out, res.Backup)
	}
	return 0
}

// set: QE-112 — validates and transactionally patches one key. The
// file extension picks ConfSet (.conf) or TomlSet (.toml).
func set(file, key, val string, history int) int {
	o := &SetOptions{History: history}
	if g, err := NewEditGuard(file); err == nil {
		o.Guard = g
	}
	var err error
	if strings.HasSuffix(file, ".toml") {
		err = TomlSet(file, key, val, o)
	} else {
		err = ConfSet(file, key, val, o)
	}
	if err != nil {
		return die("CONFIG_SET", err.Error())
	}
	return ok(map[string]any{"file": file, "key": key, "set": true})
}

// validate: schema-v1 semantic validation — data carries the full
// diagnostics list (file:line:col compiler style); error diagnostics
// also land in errors[] so contract callers see them.
func validate(file string) int {
	b, err := os.ReadFile(file)
	if err != nil {
		return die("TOML_PARSE", err.Error())
	}
	diags := validateTOMLDoc(file, b)
	var keys int
	var doc map[string]any
	if _, err := toml.Decode(string(b), &doc); err == nil {
		out := map[string]any{}
		flat("", doc, out)
		keys = len(out)
	}
	errs := []any{}
	for _, d := range diags {
		if d.Severity == "error" {
			errs = append(errs, qelib.QEError{
				Code: "SCHEMA", Message: d.String(), Severity: "error"})
		}
	}
	valid := !hasErrors(diags)
	qelib.Emit("toml.validate", "quickemu-toml/"+version, valid,
		map[string]any{"valid": valid, "keys": keys, "diagnostics": diags}, errs)
	if valid {
		return 0
	}
	return 1
}

func main() { os.Exit(run()) }

func run() int {
	os.Args = qelib.NormArgs(os.Args)
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-toml <parse|validate|get|from-conf|to-conf|migrate|set> --file <f> [--key k] [--value v] [--history n] [--dry-run]")
		return 2
	}
	sub := os.Args[1]
	var file, key, val string
	dryRun, history := false, 0
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--file":
			if i+1 < len(os.Args) {
				file = os.Args[i+1]
				i++
			}
		case "--key":
			if i+1 < len(os.Args) {
				key = os.Args[i+1]
				i++
			}
		case "--value":
			if i+1 < len(os.Args) {
				val = os.Args[i+1]
				i++
			}
		case "--history":
			if i+1 < len(os.Args) {
				fmt.Sscan(os.Args[i+1], &history)
				i++
			}
		case "--dry-run":
			dryRun = true
		}
	}
	if file == "" {
		return die("BAD_ARGS", "--file required")
	}
	switch sub {
	case "from-conf":
		return fromConf(file)
	case "to-conf":
		return toConf(file)
	case "migrate":
		return migrate(file, dryRun)
	case "set":
		if key == "" {
			return die("BAD_ARGS", "set needs --key")
		}
		return set(file, key, val, history)
	case "parse", "validate", "get":
		// shared decode below
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
		return 2
	}
	if sub == "validate" {
		return validate(file)
	}
	var doc map[string]any
	if _, err := toml.DecodeFile(file, &doc); err != nil {
		return die("TOML_PARSE", err.Error())
	}
	// map[string]any accepts every key; strict-key diagnostics live in
	// the validate path (diagnose.go).
	out := map[string]any{}
	flat("", doc, out)
	switch sub {
	case "parse":
		return ok(out)
	case "get":
		if key == "" {
			return die("BAD_ARGS", "get needs --key")
		}
		if v, ok2 := out[key]; ok2 {
			fmt.Printf("%v\n", v)
			return 0
		}
		return 1
	}
	return 2
}
