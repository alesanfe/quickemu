// quickemu-env: multi-VM orchestration without root. Reads an
// env.toml ([[vm]] conf/inline specs, depends_on DAG, [[net]] socket/
// mcast/user segments, [[share]] 9p mounts, [[healthcheck]] probes)
// and drives the existing quickemu/quickget/quickemu-ports siblings —
// see nets.go for the exact --extra_args mechanism.
//
//	quickemu-env validate -f env.toml
//	quickemu-env up [-f env.toml] [--rollback] [vm ...]
//	quickemu-env down [-f env.toml] [--force] [--grace 10s] [vm ...]
//	quickemu-env status [-f env.toml] [--format json]
//	quickemu-env restart [-f env.toml] [vm ...]
//	quickemu-env destroy [-f env.toml] [--yes]
//	quickemu-env hosts [-f env.toml]
//	quickemu-env inventory [-f env.toml] [--json | -o ansible]
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const version = "0.1.0"

// opts are the parsed command-line flags shared by every subcommand.
type opts struct {
	file     string
	format   string // --format json|human
	rollback bool
	yes      bool
	force    bool
	grace    time.Duration
	output   string // -o ansible
	jsonFlag bool   // --json (inventory shorthand)
	pos      []string
}

func usage() {
	fmt.Print(`quickemu-env — multi-VM environments without root

Usage:
  quickemu-env <command> [-f env.toml] [flags] [vm ...]

Commands:
  validate            check env.toml (schema, refs, cycles)
  up                  create missing VMs, start in dependency order
  down                stop VMs in reverse order
  status              per-VM state table (state, pid, ip)
  restart             down + up on the selected VMs
  destroy             stop and delete env-created artifacts (asks; --yes)
  hosts               /etc/hosts block with resolved addresses
  inventory           vm -> state/ip/ports/deps (--json, -o ansible)

Flags:
  -f, --file          env.toml path (default ./env.toml)
      --format        human|json output for status/validate
      --rollback      up: stop what was started when a VM fails
      --grace         down: graceful-stop budget before --kill (default 10s)
      --force         down: escalate to --kill after the grace period
  -y, --yes           destroy: skip the confirmation prompt
      --json          inventory: emit JSON
  -o, --output        inventory: output format (ansible)
`)
}

// parseArgs scans cmd-specific flags after the subcommand word.
// Unknown "-" tokens are errors; positionals select VMs.
func parseArgs(args []string) (*opts, error) {
	o := &opts{file: "env.toml", format: "human", grace: 10 * time.Second}
	for i := 0; i < len(args); i++ {
		a := args[i]
		val := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		// --flag=value form
		if strings.HasPrefix(a, "--") {
			if j := strings.IndexByte(a, '='); j > 0 {
				args = append(args[:i+1], append([]string{a[j+1:]}, args[i+1:]...)...)
				a = a[:j]
			}
		}
		switch a {
		case "-f", "--file":
			o.file = val()
		case "--format":
			o.format = val()
			if o.format != "json" && o.format != "human" && o.format != "text" {
				return nil, fmt.Errorf("--format must be human or json")
			}
		case "--rollback":
			o.rollback = true
		case "--force":
			o.force = true
		case "--grace":
			d, err := time.ParseDuration(val())
			if err != nil {
				return nil, fmt.Errorf("--grace: %v", err)
			}
			o.grace = d
		case "-y", "--yes":
			o.yes = true
		case "--json":
			o.jsonFlag = true
		case "-o", "--output":
			o.output = val()
		case "-h", "--help":
			usage()
			os.Exit(0)
		case "--version":
			fmt.Println(qelib.QEVersion)
			os.Exit(0)
		default:
			if strings.HasPrefix(a, "-") {
				return nil, fmt.Errorf("unknown flag %s", a)
			}
			o.pos = append(o.pos, a)
		}
	}
	return o, nil
}

// printErrs renders diagnostics as file:line:col — line 0 means a
// file-level error without a position.
func printErrs(file string, errs envErrs) int {
	for _, e := range errs {
		if e.line > 0 {
			fmt.Printf("%s:%d:%d: error: %s\n", file, e.line, e.col, e.msg)
		} else {
			fmt.Printf("%s: error: %s\n", file, e.msg)
		}
	}
	return 1
}

// loadOrDie loads + validates, printing diagnostics on failure.
// Validate-time conf existence is NOT checked — files appear during
// `up` (quickget) and `validate` must stay hermetic.
func loadOrDie(file string) (*Env, int) {
	e, errs := loadEnv(file)
	if len(errs) > 0 {
		return nil, printErrs(file, errs)
	}
	return e, 0
}

// run is the testable core of main — returns the process exit code.
func run(args []string) int {
	args = qelib.NormArgs(append([]string{"quickemu-env"}, args...))
	if len(args) < 2 {
		usage()
		return 2
	}
	cmd := args[1]
	switch cmd {
	case "-h", "--help", "help":
		usage()
		return 0
	case "--version":
		fmt.Println(qelib.QEVersion)
		return 0
	}
	o, err := parseArgs(args[2:])
	if err != nil {
		fmt.Fprintln(os.Stdout, "ERROR! "+err.Error())
		return 2
	}
	switch cmd {
	case "validate":
		e, errs := loadEnv(o.file)
		if len(errs) > 0 {
			if o.format == "json" {
				emitErrs("env.validate", errs)
			}
			return printErrs(o.file, errs)
		}
		if o.format == "json" {
			emitOK("env.validate", map[string]any{
				"file": o.file, "vms": len(e.VMs), "nets": len(e.Nets),
				"shares": len(e.Shares), "healthchecks": len(e.Health)})
		} else {
			fmt.Printf("%s: OK (%d vm(s), %d net(s), %d share(s), %d healthcheck(s))\n",
				o.file, len(e.VMs), len(e.Nets), len(e.Shares), len(e.Health))
		}
		return 0
	case "up", "down", "status", "restart", "destroy", "hosts", "inventory":
		e, rc := loadOrDie(o.file)
		if e == nil {
			return rc
		}
		switch cmd {
		case "up":
			return cmdUp(e, o.pos, o.rollback)
		case "down":
			return cmdDown(e, o.pos, o.grace, o.force)
		case "status":
			return cmdStatus(e, o.pos, o.format == "json")
		case "restart":
			return cmdRestart(e, o.pos, o.grace, o.force, o.rollback)
		case "destroy":
			return cmdDestroy(e, o.pos, o.yes, o.grace, o.force)
		case "hosts":
			return cmdHosts(e, o.pos)
		case "inventory":
			return cmdInventory(e, o.pos, o.jsonFlag || o.format == "json", o.output)
		}
	default:
		fmt.Fprintf(os.Stderr, "quickemu-env: unknown command %q\n", cmd)
		return 2
	}
	return 2
}

func main() { os.Exit(run(os.Args[1:])) }

// qelibProfileDefaults is the indirection keeping qelib's preset
// table out of the pure-parse tests.
func qelibProfileDefaults(name string) (map[string]string, bool) {
	return qelib.ProfileDefaults(name)
}
