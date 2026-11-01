package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// runtimeCleanup: vm_runtime_cleanup parity — when a pidfile exists but
// its process is dead the VM died uncleanly; drop every runtime
// artefact a boot can leave behind plus stale op locks, and record a
// vm.stopped event. Runs before actions and before boot, like Bash's
// main flow.
// requireRunning: the shared " - <vm> is not running." guard
// used by the lifecycle actions (vm_running parity).
func requireRunning() bool {
	if !vmRunning() {
		fmt.Println(" - " + vmname() + " is not running.")
		return false
	}
	return true
}

// requireRunningEmit: requireRunning for structured callers — the
// v1 contract needs a typed failure envelope, not a bare text line
// (stdout text would read as EMPTY_RESPONSE → legacy double-run).
func requireRunningEmit(cmd string) bool {
	if !vmRunning() {
		if outFmt == "json" {
			emitFail(cmd, "not_running", vmname()+" is not running.")
		} else {
			fmt.Println(" - " + vmname() + " is not running.")
		}
		return false
	}
	return true
}

func runtimeCleanup() {
	// Boot reconcile (QE-003): sync the persisted machine record
	// against pidfile/fingerprint/socket/QMP truth BEFORE anything
	// acts on it — crash vs requested-stop is decided here.
	machineReconcile()
	// Op-journal reconnect (QE-004): ops whose worker died — or whose
	// bootId predates this boot — are marked failed; output stays in
	// the journal, not on the CLI's stdout contract.
	if fileExists(filepath.Join(vmdir(), "ops")) {
		tool("quickemu-ops", "recover", "--vmdir", vmdir())
	}
	if !(readPID() > 0 && !vmPidAlive()) {
		return
	}
	dir, name := vmdir(), vmname()
	for _, ext := range []string{
		".pid", ".paused", ".spice", ".sock", ".ports", ".runtime.json",
		"-qmp.socket", "-monitor.socket", "-serial.socket", "-agent.sock",
	} {
		os.Remove(filepath.Join(dir, name+ext))
	}
	os.RemoveAll(filepath.Join(dir, qelib.SockSubdir))
	// <vm>.state.json is NOT reaped — the machine record survives
	// cleanup so the next boot can tell crashed from stopped.
	os.RemoveAll(filepath.Join(dir, name+".lock"))
	os.RemoveAll(filepath.Join(dir, name+".lock.shared"))
	inventoryTouch("stopped")
	qeEvent("vm.stopped", "")
}

// monitorReady: a live monitor channel exists (QMP or HMP socket) —
// the vm_qmp_socket_ready || vm_monitor_socket_ready disjunction.
func monitorReady() bool {
	if st, err := os.Stat(qmpSock()); err == nil && st.Mode()&os.ModeSocket != 0 {
		return true
	}
	return monitorSockReady()
}

// monitorControl: vm_control parity for a bare HMP verb (stop/cont) —
// QMP execute first, human monitor as the fallback path.
func monitorControl(cmd string) bool {
	return vmControl(cmd, qmpCmd(cmd, nil)) == 0
}

func actPause() int {
	if !requireRunningEmit("vm.paused") {
		return 1
	}
	conf := confKV()
	if conf["monitor"] != "none" && monitorReady() {
		if !monitorControl("stop") {
			if outFmt == "json" {
				emitFail("vm.paused", "monitor_send_failed", "pause request was not accepted")
			}
			return 1
		}
	} else if err := syscall.Kill(readPID(), syscall.SIGSTOP); err != nil {
		if outFmt == "json" {
			emitFail("vm.paused", "signal_failed", err.Error())
		}
		return 1
	}
	if werr := os.WriteFile(filepath.Join(vmdir(), vmname()+".paused"), nil, 0644); werr != nil {
		fmt.Fprintf(os.Stderr, "WARNING: paused but marker write failed: %v\n", werr)
	}
	machineTransition(stPaused, "pause", "quickemu-cli")
	qeEvent("vm.paused", "")
	if outFmt == "json" {
		emitOK("vm.paused", map[string]any{"vm": vmname(), "pid": readPID()})
	} else {
		fmt.Printf(" - %s (%d) paused.\n", vmname(), readPID())
	}
	return 0
}

func actResume() int {
	// QE-020: a suspended VM resumes by cold-booting from the saved
	// state image — the same verb, the state machine picks the path.
	if suspendedState() {
		return actResumeSuspended()
	}
	if !requireRunningEmit("vm.resumed") {
		return 1
	}
	conf := confKV()
	if conf["monitor"] != "none" && monitorReady() {
		if !monitorControl("cont") {
			if outFmt == "json" {
				emitFail("vm.resumed", "monitor_send_failed", "resume request was not accepted")
			}
			return 1
		}
		qeEvent("vm.resumed", "")
	} else if err := syscall.Kill(readPID(), syscall.SIGCONT); err != nil {
		if outFmt == "json" {
			emitFail("vm.resumed", "signal_failed", err.Error())
		}
		return 1
	}
	machineTransition(stRunning, "resume", "quickemu-cli")
	os.Remove(filepath.Join(vmdir(), vmname()+".paused"))
	if outFmt == "json" {
		emitOK("vm.resumed", map[string]any{"vm": vmname(), "pid": readPID()})
	} else {
		fmt.Printf(" - %s (%d) resumed.\n", vmname(), readPID())
	}
	return 0
}

// actShutdown: shutdown_vm — graceful ACPI system_powerdown over the
// monitor; errors when the VM has no monitor channel (--kill forces).
// A successful request records running→stopping (guest-shutdown) so a
// later dead pid reconciles as a completed stop, never a crash.
func actShutdown() int {
	rc := monitorAction("system_powerdown", "vm.shutdown-requested", "shutdown", "vm.shutdown")
	if rc == 0 {
		machineTransition(stStopping, "guest-shutdown", "quickemu-cli")
	}
	return rc
}

// actPowerdown: --powerdown — same ACPI system_powerdown request as
// --shutdown (the GUI's batch stop sends this spelling); the envelope
// reports `vm.powerdown` so the caller's command-match resolves.
func actPowerdown() int {
	rc := monitorAction("system_powerdown", "vm.powerdown-requested", "powerdown", "vm.powerdown")
	if rc == 0 {
		machineTransition(stStopping, "guest-powerdown", "quickemu-cli")
	}
	return rc
}

// actReset: reset_vm — immediate system_reset over the monitor.
func actReset() int {
	return monitorAction("system_reset", "vm.reset-requested", "reset", "vm.reset")
}

// monitorAction: Bash monitor_send_cmd && { event + echo } — a failed
// send surfaces only the monitor warning, no success line. emit is
// the v1 command name the envelope reports in --format json mode.
func monitorAction(cmd, event, verb, emit string) int {
	if !requireRunningEmit(emit) {
		return 1
	}
	conf := confKV()
	if conf["monitor"] == "none" || !monitorSockReady() {
		if outFmt == "json" {
			emitFail(emit, "no_monitor", vmname()+": no monitor channel; use --kill to force stop.")
		} else {
			fmt.Printf(" - %s: no monitor channel; use --kill to force stop.\n", vmname())
		}
		return 1
	}
	if monitorSendCmdQuiet(cmd) != 0 {
		if outFmt == "json" {
			emitFail(emit, "monitor_send_failed", "monitor command was not accepted")
		}
		return 1
	}
	qeEvent(event, "")
	if outFmt == "json" {
		emitOK(emit, map[string]any{"vm": vmname(), "pid": readPID(), "request": cmd})
	} else {
		fmt.Printf(" - %s (%d) %s requested.\n", vmname(), readPID(), verb)
	}
	return 0
}

// vmOp runs `quickemu-vm <sub> <rest...>`; args are produced lazily at
// dispatch time so vmdir()/vmname() see the parsed --vm flag.
// vmLockedOps: the mutating ops run under the EXCLUSIVE VM lock —
// delete/clone/rename/export like Bash's vm_lock_acquire set, plus
// the QE-002 exclusive set: import, disk ops, snapshot apply and
// every other state-changing vm op.
var vmLockedOps = map[string]bool{
	"delete": true, "delete-disk": true, "delete-plan": true,
	"clone": true, "linked-clone": true, "rename": true, "move": true,
	"export": true, "import": true, "autostart": true,
	"shortcut": true, "shortcut-delete": true,
	"backup": true, "backup-restore": true, "backup-prune": true,
	"trash-empty": true, "trash-restore": true,
	"compact": true, "resize": true, "resize-commit": true,
	"snapshot-create": true, "snapshot-apply": true,
	"snapshot-delete": true, "snapshot-consolidate": true,
	"snapshot-register": true, "snapshot-unregister": true,
	"snapshot-policy": true,
}

// vmSharedOps: read-only vm ops — concurrent with other readers, never
// concurrent with an exclusive writer (QE-002 shared lock).
var vmSharedOps = map[string]bool{
	"snapshot-list": true, "snapshot-list-json": true,
	"snapshot-tree": true, "snapshot-info": true, "snapshot-verify": true,
	"backup-list": true, "backup-verify": true, "trash-list": true,
}

func actStop(force bool) int {
	// kill_vm gates on the pidfile alone, but going through the
	// instance verify keeps a foreign or recycled pid safe from a
	// blind SIGKILL — same refusal, minus the hazard.
	if force && outFmt != "json" {
		fmt.Println("Killing " + vmname())
	}
	pid := readPID()
	if !vmPidAlive() {
		if outFmt == "json" {
			emitOK("vm.stopped", map[string]any{"vm": vmname(), "alreadyStopped": true})
		} else {
			fmt.Println(" - " + vmname() + " is not running.")
		}
		runtimeCleanup()
		return 0
	}
	if force {
		machineTransition(stStopping, "quickemu-kill", "quickemu-cli")
		killed := false
		if p, err := os.FindProcess(pid); err == nil {
			killed = p.Signal(syscall.SIGKILL) == nil
		}
		if !killed {
			if outFmt == "json" {
				emitFail("vm.stopped", "kill_failed", fmt.Sprintf("%s (%d) was not killed.", vmname(), pid))
			} else {
				fmt.Printf(" - %s (%d) was not killed.\n", vmname(), pid)
			}
			return 0
		}
		for i := 0; i < 40 && alive(pid); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		if outFmt == "json" {
			emitOK("vm.stopped", map[string]any{"vm": vmname(), "pid": pid, "forced": true})
		} else {
			fmt.Printf(" - %s (%d) killed.\n", vmname(), pid)
		}
		machineTransition(stStopped, "killed", "quickemu-cli")
		runtimeCleanup()
		return 0
	}
	// stop_vm is a Go-only action (Bash maps --kill/--shutdown): "quit"
	// is a privileged monitor write — the human monitor first, QMP
	// human-monitor-command when the VM runs monitor=none with only
	// the control socket up.
	machineTransition(stStopping, "quickemu-stop", "quickemu-cli")
	if monitorSendCmdQuiet("quit") != 0 {
		hmp("quit")
	}
	for i := 0; i < 40 && alive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if alive(pid) {
		// quit never landed (or QEMU ignored it) — reporting
		// "stopped" and wiping runtime state would orphan a live VM.
		if outFmt == "json" {
			emitFail("vm.stopped", "stop_failed", fmt.Sprintf("%s (%d) did not stop.", vmname(), pid))
		} else {
			fmt.Printf(" - %s (%d) did not stop.\n", vmname(), pid)
		}
		return 1
	}
	machineTransition(stStopped, "stopped", "quickemu-cli")
	runtimeCleanup()
	if outFmt == "json" {
		emitOK("vm.stopped", map[string]any{"vm": vmname(), "pid": pid})
	} else {
		fmt.Println(" - " + vmname() + " stopped.")
	}
	return 0
}

func actLaunch() int {
	// actLaunch runs without runActions (the no-actions default path)
	// — the boot reconcile has to happen here too.
	machineReconcile()
	conf := confKV()
	get := func(k string) string {
		if v := confOpts[k]; v != "" {
			return v
		}
		return conf[k]
	}
	// Already-running parity: a second `quickemu --vm x.conf` never
	// spawns a second QEMU — Bash attaches the viewer to the live
	// instance instead. Without this the two QEMU processes race over
	// the pidfile/sockets in the VM dir.
	if vmRunning() {
		if outFmt == "json" {
			emitOK("vm.started", map[string]any{
				"vm": vmname(), "pid": readPID(), "alreadyRunning": true})
		} else {
			fmt.Println(vmname())
			fmt.Printf(" - Process:  Already running %s as %s (%d)\n",
				vmConf, vmname(), readPID())
		}
		startViewer(get)
		if mc := get("monitor_cmd"); mc != "" {
			monitorSendCmd(mc)
		}
		return 0
	}
	// vm_runtime_cleanup parity: a stale pidfile means an unclean
	// shutdown — remove leftover state before booting (Bash does this
	// in its main flow before any launch).
	runtimeCleanup()
	// viewer_param_check (launch path only — skipped on --dry-run):
	// viewer must be none|spicy|remote-viewer and the binary installed.
	viewer := get("viewer")
	if viewer == "" {
		viewer = "spicy"
	}
	switch viewer {
	case "none", "spicy", "remote-viewer":
	default:
		fmt.Fprintf(os.Stderr, "ERROR! Requested viewer '%s' is not recognised.\n", viewer)
		return 1
	}
	if viewer != "none" {
		if _, err := exec.LookPath(viewer); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! Requested '%s' as viewer, but '%s' is not installed.\n", viewer, viewer)
			return 1
		}
	}
	machineTransition(stPreparing, "launch", "quickemu-cli")
	// vm_lock_acquire parity: the boot runs under the exclusive VM
	// lock so a concurrent launch fails LOCK_BUSY instead of racing
	// the pidfile/sockets window (release right after spawn, like
	// Bash's vm_lock_release post-qemu_spawn).
	if !vmLockAcquireOp("start") {
		machineTransition(stStopped, "lock-busy", "quickemu-cli")
		if outFmt == "json" {
			emitFail("vm.started", "lock_busy", "another quickemu operation holds the VM lock")
		}
		return 1
	}
	machineTransition(stStarting, "spawn", "quickemu-cli")
	out, err := tool("quickemu-launch", applyOverrides(), "--run")
	if outFmt != "json" {
		fmt.Print(out)
	}
	vmLockRelease()
	if err != nil {
		machineTransition(stStopped, "launch-failed", "quickemu-cli")
		if outFmt == "json" {
			msg := strings.TrimSpace(out)
			if msg == "" {
				msg = err.Error()
			}
			emitFail("vm.started", "launch_failed", msg)
		}
		return 1
	}
	machineTransition(stRunning, "spawned", "quickemu-cli")
	inventoryTouch("running")
	// qemu_spawn emits vm.started once the QEMU pid is published.
	if p := readPID(); p > 0 {
		// Strong fingerprint (QE-001): record the real exe path and
		// the full-args hash into runtime.json while the freshly
		// spawned pid is provably ours.
		fingerprintAugment(p)
		qeEvent("vm.started", fmt.Sprintf(`{"pid":%d}`, p))
	}
	startViewer(get)
	// Auto-skip the press-any-key prompt on uninstalled Windows guests.
	if get("iso") != "" && strings.HasPrefix(get("guest_os"), "windows") {
		for i := 0; i < 5; i++ {
			time.Sleep(1 * time.Second)
			monitorSendCmd("sendkey ret")
		}
	}
	// monitor_cmd conf key: Bash sends it after every boot.
	if mc := get("monitor_cmd"); mc != "" && extra["monitor-cmd"] == "" {
		monitorSendCmd(mc)
	}
	if outFmt == "json" {
		emitOK("vm.started", map[string]any{"vm": vmname(), "pid": readPID()})
	}
	return 0
}

func actDryRun() int {
	conf := applyOverrides()
	out, err := tool("quickemu-launch", conf)
	if outFmt == "json" {
		// execution-plan.schema parity: the envelope IS the output —
		// human plan lines would corrupt the single-envelope contract.
		if err == nil {
			emitOK("vm.plan", map[string]any{"vm": vmname(),
				"command": strings.TrimSpace(out)})
		} else {
			emitFail("vm.plan", "plan_failed", strings.TrimSpace(out))
		}
	} else {
		fmt.Print(out)
	}
	return rc(err)
}

// applyOverrides writes CLI conf overrides into a temp conf alongside
// the original (Bash exports them as env vars; quickemu-launch reads
// only the file, so overrides are materialised there).
func applyOverrides() string {
	if len(confOpts) == 0 {
		return vmConf
	}
	b, _ := os.ReadFile(vmConf)
	c := string(b)
	for k, v := range confOpts {
		// The value lands verbatim inside key="v" — a quote or control
		// char injects extra conf lines the launcher then honours.
		if strings.ContainsAny(v, "\"\n\r") || strings.IndexFunc(v, unicode.IsControl) >= 0 {
			fmt.Fprintf(os.Stderr, "WARNING: --%s value contains quote/control characters — ignored\n", k)
			continue
		}
		kv := k + `="` + v + `"`
		if strings.Contains(c, k+"=") {
			lines := strings.Split(c, "\n")
			for i, l := range lines {
				if strings.HasPrefix(l, k+"=") {
					lines[i] = kv
				}
			}
			c = strings.Join(lines, "\n")
		} else {
			c += kv + "\n"
		}
	}
	tmp := filepath.Join(vmdir(), "."+vmname()+".cli.conf")
	if err := os.WriteFile(tmp, []byte(c), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: override conf write failed (%v) — launching without overrides\n", err)
		return vmConf
	}
	return tmp
}
