package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func readPID() int {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return 0
	}
	pid := 0
	fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid)
	return pid
}

// alive reports whether pid is a live process (EPERM-aware — a
// foreign owner's lock must never be reclaimed).
func alive(pid int) bool { return qelib.Alive(pid) }

// vmLockAcquire/vmLockRelease: vm_lock_* parity — mkdir-based
// exclusive lock at <vmdir>/<vm>.lock with a pid file for stale-owner
// reclaim. Bash defaults VM_LOCK_TRIES=1: one attempt, then LOCK_BUSY.
func vmLockAcquire() bool {
	lockDir := filepath.Join(vmdir(), vmname()+".lock")
	if err := os.MkdirAll(vmdir(), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! Cannot create VM dir %s: %v\n", vmdir(), err)
		return false
	}
	tries := 1
	if t := os.Getenv("VM_LOCK_TRIES"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			tries = n
		}
	}
	writeOwner := func() bool {
		// A lock without a readable owner is reclaimable by the next
		// acquirer — on write failure drop the lock rather than hold
		// an anonymous one.
		if os.WriteFile(filepath.Join(lockDir, "pid"),
			[]byte(strconv.Itoa(os.Getpid())), 0644) != nil {
			os.RemoveAll(lockDir)
			return false
		}
		return true
	}
	for i := 0; i < tries; i++ {
		if err := os.Mkdir(lockDir, 0755); err == nil {
			if writeOwner() {
				return true
			}
		}
		// Reclaim a dead writer (crash-safe, like Bash's kill -0 check)
		// then retry the mkdir immediately — a reclaimed lock is
		// ours to take, not a reason to fail this attempt.
		if b, err := os.ReadFile(filepath.Join(lockDir, "pid")); err == nil {
			owner, _ := strconv.Atoi(strings.TrimSpace(string(b)))
			switch {
			case owner > 0 && !alive(owner):
				// Dead owner — safe to reclaim.
				os.RemoveAll(lockDir)
				if err := os.Mkdir(lockDir, 0755); err == nil && writeOwner() {
					return true
				}
			case owner <= 0:
				// Corrupt/torn pid write — reclaim only once the
				// lock is old enough that the writer cannot be
				// mid-creation (same 2s window as a missing file).
				if st, serr := os.Stat(lockDir); serr == nil &&
					time.Since(st.ModTime()) > 2*time.Second {
					os.RemoveAll(lockDir)
					if err := os.Mkdir(lockDir, 0755); err == nil && writeOwner() {
						return true
					}
				}
			}
		} else if st, err := os.Stat(lockDir); err == nil &&
			time.Since(st.ModTime()) > 2*time.Second {
			// Pid-less lock older than ~2s: crash remnant between
			// mkdir and the pid write.
			os.RemoveAll(lockDir)
			if err := os.Mkdir(lockDir, 0755); err == nil && writeOwner() {
				return true
			}
		}
		if i+1 < tries {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if outFmt == "json" {
		emitFail("vm.lock", "VM_LOCKED", vmname()+" is locked by another operation.")
	} else {
		fmt.Fprintf(os.Stderr, "ERROR! %s is locked by another operation.\n", vmname())
	}
	return false
}

func vmLockRelease() {
	os.RemoveAll(filepath.Join(vmdir(), vmname()+".lock"))
}

// procComm: /proc/<pid>/comm (linux) — vm_proc_comm parity; empty on
// non-linux hosts where the fallback is plain liveness.
func procComm(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// vmPidAlive: vm_pid_alive parity — pid alive AND the process belongs
// to this VM (comm matches the vm name, its 15-char truncation or
// qemu*). When comm is unreadable (macOS, other users' procs) liveness
// alone decides — same as the Bash legacy path.
// procCmdline: /proc/<pid>/cmdline, NUL-separated argv joined with
// spaces; "" when the kernel hides it (non-linux, foreign owner) or
// for kernel threads.
func procCmdline(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(strings.TrimRight(string(b), "\x00"), "\x00", " ")
}

func vmPidAlive() bool {
	pid := readPID()
	if !alive(pid) {
		return false
	}
	name := vmname()
	// Hard ownership proof first: a quickemu qemu always carries this
	// VM's disk dir in its drive args and the -name process=<vm> token.
	// A recycled pid running anything else is foreign — comm matching
	// "qemu*" alone would let stop --force SIGKILL another VM.
	if cl := procCmdline(pid); cl != "" {
		return strings.Contains(cl, vmdir()+string(filepath.Separator)) ||
			strings.Contains(cl, "process="+name+",") ||
			strings.HasSuffix(cl, "process="+name)
	}
	comm := procComm(pid)
	if comm == "" {
		return true
	}
	short := name
	if len(short) > 15 {
		short = short[:15]
	}
	return comm == name || comm == short || strings.HasPrefix(comm, "qemu")
}

func vmRunning() bool { return vmPidAlive() }

func paused() bool {
	_, err := os.Stat(filepath.Join(vmdir(), vmname()+".paused"))
	return err == nil
}

func state() (string, int) {
	pid := readPID()
	if !vmPidAlive() {
		if pid > 0 {
			return "stale", 0
		}
		return "stopped", 0
	}
	if paused() {
		return "paused", pid
	}
	return "running", pid
}

// runtimeCleanup: vm_runtime_cleanup parity — when a pidfile exists but
// its process is dead the VM died uncleanly; drop every runtime
// artefact a boot can leave behind plus stale op locks, and record a
// vm.stopped event. Runs before actions and before boot, like Bash's
// main flow.
func runtimeCleanup() {
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
	os.RemoveAll(filepath.Join(dir, name+".lock"))
	os.RemoveAll(filepath.Join(dir, name+".lock.shared"))
	qeEvent("vm.stopped", "")
}

func actPause() int {
	out, err := qmp("stop")
	if err == nil {
		if werr := os.WriteFile(filepath.Join(vmdir(), vmname()+".paused"), nil, 0644); werr != nil {
			fmt.Fprintf(os.Stderr, "WARNING: paused but marker write failed: %v\n", werr)
		}
		fmt.Println(" - " + vmname() + " paused.")
	}
	fmt.Print(out)
	return rc(err)
}

func actResume() int {
	out, err := qmp("cont")
	if err == nil {
		os.Remove(filepath.Join(vmdir(), vmname()+".paused"))
		fmt.Println(" - " + vmname() + " resumed.")
	}
	fmt.Print(out)
	return rc(err)
}

// vmOp runs `quickemu-vm <sub> <rest...>`; args are produced lazily at
// dispatch time so vmdir()/vmname() see the parsed --vm flag.
// vmLockedOps: the mutating ops Bash wraps in vm_lock_acquire in its
// main dispatch (delete_vm, clone_vm, export_vm, autostart_vm,
// create/delete_shortcut, delete_disk, rename_vm).
var vmLockedOps = map[string]bool{
	"delete": true, "delete-disk": true, "clone": true, "linked-clone": true,
	"rename": true, "export": true, "autostart": true,
	"shortcut": true, "shortcut-delete": true,
}

func actStop(force bool) int {
	st, pid := state()
	if st != "running" && st != "paused" {
		fmt.Println(vmname() + " is not running")
		return 0
	}
	if force {
		if p, err := os.FindProcess(pid); err == nil {
			p.Signal(syscall.SIGKILL)
		}
	} else {
		// "quit" is not allowlisted on the structured QMP channel — the
		// HMP passthrough is how Bash issues it.
		hmp("quit")
	}
	for i := 0; i < 40 && alive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	for _, ext := range []string{".pid", ".paused", "-qmp.socket", "-monitor.socket", "-serial.socket", "-agent.sock"} {
		os.Remove(filepath.Join(vmdir(), vmname()+ext))
	}
	fmt.Println(" - " + vmname() + " stopped.")
	return 0
}

func actLaunch() int {
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
		fmt.Println(vmname())
		fmt.Printf(" - Process:  Already running %s as %s (%d)\n",
			vmConf, vmname(), readPID())
		startViewer(get)
		if mc := get("monitor_cmd"); mc != "" {
			monitorSendBest(mc)
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
	// vm_lock_acquire parity: the boot runs under the exclusive VM
	// lock so a concurrent launch fails LOCK_BUSY instead of racing
	// the pidfile/sockets window (release right after spawn, like
	// Bash's vm_lock_release post-qemu_spawn).
	if !vmLockAcquire() {
		return 1
	}
	out, err := tool("quickemu-launch", applyOverrides(), "--run")
	fmt.Print(out)
	vmLockRelease()
	if err != nil {
		return 1
	}
	startViewer(get)
	// Auto-skip the press-any-key prompt on uninstalled Windows guests.
	if get("iso") != "" && strings.HasPrefix(get("guest_os"), "windows") {
		for i := 0; i < 5; i++ {
			time.Sleep(1 * time.Second)
			monitorSendBest("sendkey ret")
		}
	}
	// monitor_cmd conf key: Bash sends it after every boot.
	if mc := get("monitor_cmd"); mc != "" && extra["monitor-cmd"] == "" {
		monitorSendBest(mc)
	}
	return 0
}

func actDryRun() int {
	conf := applyOverrides()
	out, err := tool("quickemu-launch", conf)
	fmt.Print(out)
	if outFmt == "json" && err == nil {
		// execution-plan.schema parity: human plan lines above, then the
		// vm.plan envelope carrying the assembled QEMU command.
		emitOK("vm.plan", map[string]any{"vm": vmname(),
			"command": strings.TrimSpace(out)})
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
