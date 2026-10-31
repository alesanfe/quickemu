// quicktest: automated OS install/provision/verify runs inside
// Quickemu VMs. The engine Quickgui drives — declarative TOML profile
// in, JSONL event stream out, ops-journal state for watchers.
//
//	quicktest plan   --profile <file.toml>
//	quicktest run    --profile <file.toml> [--dir <workdir>]
//	quicktest status --vmdir <dir> --id <op-id>
//	quicktest cancel --vmdir <dir> --id <op-id>
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func main() {
	os.Exit(dispatch(qelib.NormArgs(os.Args)))
}

func dispatch(args []string) int {
	if len(args) < 2 {
		usage()
		return 2
	}
	switch args[1] {
	case "plan":
		return cmdPlan(args[2:])
	case "run":
		return cmdRun(args[2:])
	case "status":
		return cmdStatus(args[2:])
	case "cancel":
		return cmdCancel(args[2:])
	case "resume":
		return cmdResume(args[2:])
	case "--version", "version":
		fmt.Println("quicktest " + version)
		return 0
	case "--help", "help", "-h":
		usage()
		return 0
	}
	fmt.Fprintf(os.Stderr, "quicktest: unknown command %s\n", args[1])
	usage()
	return 2
}

func usage() {
	fmt.Println(`quicktest ` + version + ` — automated VM install/provision/verify for Quickemu

Usage:
  quicktest plan   --profile <file.toml>   Resolve a profile into a step plan (v1 envelope)
  quicktest run    --profile <file.toml>   Execute a run; JSONL events on stdout
                   [--dir <workdir>]         where quickget places the VM (default: .)
                   [--keep]                  leave the VM running on failure
  quicktest status --vmdir <dir> --id <op>  Journal entry for a run
  quicktest cancel --vmdir <dir> --id <op>  Request cancellation (checked per-step)
  quicktest resume --vmdir <dir>            Resume the newest unfinished run
  quicktest --version | --help`)
}

// flag pulls "--name value" (post-NormArgs, --name=value already
// split) from args.
func flag(args []string, name, def string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return def
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

// emitErr is the pre-run failure path — the v1 envelope, no JSONL
// stream because no run exists yet.
func emitErr(code, msg string) {
	qelib.Fail(qelib.CmdName("quicktest"), "quicktest/"+version, code, msg)
}

func cmdPlan(args []string) int {
	pf := flag(args, "--profile", "")
	if pf == "" {
		emitErr("BAD_ARGS", "--profile required")
		return 1
	}
	p, err := loadProfile(pf)
	if err != nil {
		emitErr("BAD_PROFILE", err.Error())
		return 1
	}
	var warnings []string
	prov, err := pickProvider(p)
	steps := []string{"resolve", "acquire-image", "prepare"}
	if err != nil {
		warnings = append(warnings, err.Error())
	} else {
		steps = planSteps(prov)
	}
	data := map[string]any{
		"profile":  p.Profile.Name,
		"os":       p.System.OS,
		"release":  p.System.Release,
		"edition":  p.System.Edition,
		"arch":     p.System.Arch,
		"provider": p.Automation.Provider,
		"steps":    steps,
		"resources": map[string]any{
			"cpus": p.Machine.CPUs, "ram": p.Machine.RAM,
			"diskSize": p.Machine.DiskSize,
		},
		"identity": map[string]any{
			"hostname": p.Identity.Hostname, "user": p.Identity.Username,
		},
	}
	if len(warnings) > 0 {
		data["warnings"] = warnings
	}
	qelib.EmitOK("quicktest.plan", "quicktest/"+version, data)
	return 0
}

func cmdRun(args []string) int {
	pf := flag(args, "--profile", "")
	if pf == "" {
		emitErr("BAD_ARGS", "--profile required")
		return 1
	}
	dir := flag(args, "--dir", ".")
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	p, err := loadProfile(pf)
	if err != nil {
		emitErr("BAD_PROFILE", err.Error())
		return 1
	}
	if _, err := pickProvider(p); err != nil {
		emitErr("NO_PROVIDER", err.Error())
		return 1
	}
	abs, _ := filepath.Abs(pf)
	return run(p, filepath.Dir(abs), dir, hasFlag(args, "--keep"))
}

func cmdStatus(args []string) int {
	vmdir := flag(args, "--vmdir", "")
	id := flag(args, "--id", "")
	if vmdir == "" || id == "" {
		emitErr("BAD_ARGS", "--vmdir and --id required")
		return 1
	}
	out, err := toolOut("quickemu-ops", "status", "--vmdir", vmdir, "--id", id)
	if err != nil {
		emitErr("OP_LOOKUP", err.Error())
		return 1
	}
	fmt.Print(out)
	if !strings.HasSuffix(out, "\n") {
		fmt.Println()
	}
	return 0
}

func cmdCancel(args []string) int {
	vmdir := flag(args, "--vmdir", "")
	id := flag(args, "--id", "")
	if vmdir == "" || id == "" {
		emitErr("BAD_ARGS", "--vmdir and --id required")
		return 1
	}
	// ops set --state cancelled maps to the cancelRequested transition;
	// the runner checks the journal at each step boundary.
	_, err := toolOut("quickemu-ops", "set", "--vmdir", vmdir, "--id", id,
		"--state", "cancelled")
	if err != nil {
		emitErr("OP_CANCEL", err.Error())
		return 1
	}
	return 0
}

// cmdResume finds the newest run under <vmdir>/.quicktest whose state
// is not "completed" and re-enters the pipeline at the first step that
// never finished — same run id, key and SSH port, so the event stream
// continues where the dead run left off.
func cmdResume(args []string) int {
	vmdir := flag(args, "--vmdir", "")
	if vmdir == "" {
		emitErr("BAD_ARGS", "--vmdir required")
		return 1
	}
	st, err := latestUnfinishedRun(vmdir)
	if err != nil {
		emitErr("RESUME", err.Error())
		return 1
	}
	p, err := loadProfile(st.Profile)
	if err != nil {
		emitErr("RESUME", "profile: "+err.Error())
		return 1
	}
	if _, err := pickProvider(p); err != nil {
		emitErr("NO_PROVIDER", err.Error())
		return 1
	}
	return runSteps(p, filepath.Dir(st.Profile), st.Workdir,
		hasFlag(args, "--keep"), st)
}

// latestUnfinishedRun scans <vmdir>/.quicktest/*/state.json and picks
// the most recent run that did not complete.
func latestUnfinishedRun(vmdir string) (*runState, error) {
	matches, err := filepath.Glob(filepath.Join(vmdir, ".quicktest", "*", "state.json"))
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("no runs under %s", vmdir)
	}
	var best *runState
	var bestMod time.Time
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil {
			continue
		}
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var st runState
		if json.Unmarshal(b, &st) != nil || st.Status == "completed" ||
			st.RunID == "" || st.Profile == "" {
			continue
		}
		if best == nil || fi.ModTime().After(bestMod) {
			best, bestMod = &st, fi.ModTime()
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no unfinished runs under %s", vmdir)
	}
	return best, nil
}
