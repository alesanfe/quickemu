// quickemu-cli: the unified `quickemu` entry point in Go — replaces
// bin/quickemu + lib/quickemu/cli/*. It parses the same flag surface,
// resolves actions, and dispatches to the Go backend tools
// (launch/vm/qmp/ops/archive) — zero Bash delegation.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	parseArgs(os.Args[1:])
	// commands.sh param-checks: resolution/access/network validation on
	// the merged CLI overrides — errors before any action runs.
	if errs := qelib.ParamCheckConf(confOpts); len(errs) > 0 {
		if jsonRequested() {
			emitFail(reqCmd(), "param_check", strings.Join(errs, "; "))
			os.Exit(1)
		}
		for _, e := range errs {
			fmt.Println("ERROR! " + e)
		}
		os.Exit(1)
	}
	if vmConf == "" && len(actions) == 0 {
		usage()
		os.Exit(2)
	}
	// --format propagates to delegated tools via the env (Bash reads
	// OUTPUT_FORMAT the same way; quickemu-vm honors it too).
	if outFmt == "json" {
		os.Setenv("OUTPUT_FORMAT", "json")
	}
	fixupVMConf()
	// default action: boot the VM (dry-run prints the argv)
	if len(actions) == 0 {
		if trashFlag {
			// `--trash` alone means "move the VM to trash" (the GUI's
			// Move-to-trash menu sends it bare) — never boot.
			os.Exit(actTrashVm())
		}
		if statusQuoFlag {
			// `--status-quo` alone means "save state to conf" — write
			// the CLI overrides back; never boot.
			os.Exit(actStatusQuo())
		}
		if dryRunFlag {
			os.Exit(actDryRun())
		}
		os.Exit(actLaunch())
	}
	os.Exit(runActions())
}

// fixupVMConf: fixes running VMs when PWD is not relative to the VM
// directory (bin/quickemu.bash parity): when the configured disk is
// not reachable from the cwd, operate from the conf's directory so
// relative paths in the conf resolve the way Bash resolves them.
func fixupVMConf() {
	if vmConf == "" {
		return
	}
	if abs, err := filepath.Abs(vmConf); err == nil {
		vmConf = abs
	}
	// conf-load parity: Bash dies here ("Virtual machine
	// configuration not found.") before any action runs — the
	// message goes to stdout like every Bash echo.
	if _, err := os.Stat(vmConf); err != nil {
		if jsonRequested() {
			emitFail(reqCmd(), "config_not_found",
				"virtual machine configuration not found")
			os.Exit(1)
		}
		fmt.Println("ERROR! Virtual machine configuration not found.")
		os.Exit(1)
	}
	disk := confKV()["disk_img"]
	if disk == "" {
		df := confKV()["disk_format"]
		if df == "" {
			df = "qcow2"
		}
		disk = filepath.Join(vmdir(), "disk."+df)
	}
	if _, err := os.Stat(disk); err != nil {
		os.Chdir(filepath.Dir(vmConf))
	}
}

// runActions: Bash main flow — a stale pidfile means an unclean
// shutdown, so runtime artefacts are cleaned before any action runs
// (vm_runtime_cleanup). Each queued action runs in order; the last
// non-zero rc wins the exit code.
//
// v1 contract: in --format json mode stdout carries exactly ONE
// envelope per invocation. Composite flags (--restart → stop+start,
// --backup → backup+shutdown) run with stdout captured so each
// child's own envelope doesn't concatenate; the run reports a single
// composite envelope afterwards.
func runActions() int {
	if outFmt == "json" && len(actions) > 1 {
		name := compositeEmitName(actions)
		out, rcv := captureStdout(runAllActions)
		if rcv == 0 {
			data := map[string]any{"vm": vmname(), "actions": actions}
			if t := strings.TrimSpace(out); t != "" {
				data["log"] = t
			}
			emitOK(name, data)
		} else {
			msg := "composite action failed"
			if t := strings.TrimSpace(out); t != "" {
				if i := strings.LastIndex(t, "\n"); i >= 0 {
					t = t[i+1:]
				}
				if len(t) > 400 {
					t = t[:400]
				}
				msg = t
			}
			emitFail(name, "action_failed", msg)
		}
		return rcv
	}
	return runAllActions()
}

// compositeEmitName: v1 command name for multi-action runs — the
// structured contract wants one envelope whose name the caller can
// predict ('restart' for --restart, 'backup' for --backup).
func compositeEmitName(actions []string) string {
	switch strings.Join(actions, "+") {
	case "stop_vm+start":
		return "vm.restart"
	case "backup_vm+shutdown_vm":
		return "vm.backup"
	}
	// Hand-chained flags fall back to the leading action's name.
	return "vm." + strings.ReplaceAll(actions[0], "_", "-")
}

// captureStdout diverts os.Stdout for the duration of fn and returns
// everything written — used to absorb per-action envelopes during a
// composite structured run (stdout stays a single-envelope channel).
func captureStdout(fn func() int) (string, int) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		out := fn()
		return "", out
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	rc := fn()
	w.Close()
	os.Stdout = old
	out := <-done
	r.Close()
	return out, rc
}

func runAllActions() int {
	runtimeCleanup()
	rcv := 0
	for _, act := range actions {
		if act == "start" {
			if dryRunFlag {
				rcv = actDryRun()
			} else {
				rcv = actLaunch()
			}
			continue
		}
		// Host-level actions do not need --vm (early-action parity:
		// Bash runs capabilities|list_vms|import_vm before conf load;
		// the rest are host-scope ops that only read machine state).
		hostOnly := map[string]bool{
			"capabilities": true, "host_info": true, "list_vms": true,
			"inventory_show": true, "plugin_list": true, "import_vm": true,
			"trash_list": true, "trash_empty": true, "trash_restore": true,
			"vfio_scan": true,
		}
		if vmConf == "" && !hostOnly[act] {
			fmt.Fprintln(os.Stderr, "ERROR! --vm is required for "+act)
			rcv = 2
			continue
		}
		// vm_lock parity: Bash runs mutating actions under the
		// exclusive VM lock (kill|pause|resume|shutdown|reset|
		// send_keys|type_text|screenshot; vm/fs ops lock inside vmOp).
		locked := map[string]bool{
			"kill_vm": true, "pause_vm": true, "resume_vm": true,
			"shutdown_vm": true, "reset_vm": true, "stop_vm": true,
			"send_keys": true, "type_text": true, "screenshot_vm": true,
			"vm_suspend":  true, // migrate mutates machine state
			"monitor_cmd": true, // HMP socket is single-client like QMP
			"backup_incr": true, // drive-backup block job mutates the live image
		}
		// QE-002: read-only actions take the shared lock — status,
		// conf reads and snapshot listings run concurrently with each
		// other but never under an exclusive writer.
		shared := map[string]bool{
			"status_vm": true, "inspect_conf": true, "events_read": true,
			"vm_stats": true,
		}
		if fn := dispatch(act); fn != nil {
			if locked[act] && !vmLockAcquireOp(act) {
				rcv = 1
				continue
			}
			if shared[act] && !locked[act] && !vmLockAcquireShared(act) {
				rcv = 1
				continue
			}
			// QE-104: structured JSONL action records — start/done|fail
			// per dispatch, independent of the human output stream.
			qlogActionStart(act)
			r := fn()
			qlogActionDone(act, r)
			if locked[act] {
				vmLockRelease()
			}
			if shared[act] && !locked[act] {
				vmLockReleaseShared()
			}
			if r != 0 {
				rcv = r
			}
		}
	}
	return rcv
}
