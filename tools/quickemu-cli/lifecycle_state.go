// tools/quickemu-cli/lifecycle_state.go: VM state introspection: pid/proc/lock/qmp state + instance verify
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// vmProcOwned: ownership proof for a live pid — the process is THIS
// VM's qemu. Cmdline tokens are the strongest check (a quickemu qemu
// always carries the VM dir in its drive args and -name process=<vm>);
// comm matching (vmname, 15-char truncation, qemu*) is the legacy
// fallback when cmdline is unreadable.
func vmProcOwned(pid int) bool {
	name := vmname()
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

// procStateChar: /proc/<pid>/stat state letter (after the last ')' so
// comm values containing parens can't shift the field offsets).
func procStateChar(pid int) byte {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 || i+2 >= len(b) {
		return 0
	}
	f := strings.Fields(string(b[i+2:]))
	if len(f) == 0 {
		return 0
	}
	return f[0][0]
}

// procStartTimeJ: /proc/<pid>/stat starttime (jiffies) — the field the
// launcher records into runtime.json "processStartTime".
func procStartTimeJ(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 || i+2 >= len(b) {
		return ""
	}
	f := strings.Fields(string(b[i+2:]))
	if len(f) <= 19 {
		return ""
	}
	return f[19]
}

// instanceVerify: vm_instance_verify parity — pidfile + sibling
// runtime.json identity → a typed status word. Exit status in Bash is
// folded into the word itself; callers only need the pair (word, pid).
func instanceVerify() (string, int) {
	pid := readPID()
	if pid <= 0 {
		return "dead", 0 // missing or non-numeric pidfile
	}
	dir, name := vmdir(), vmname()
	rtb, err := os.ReadFile(filepath.Join(dir, name+".runtime.json"))
	if err != nil {
		// No runtime record: live process with VM-looking comm is a
		// pre-runtime (alive-legacy) boot, anything else is foreign.
		if !alive(pid) {
			return "dead", 0
		}
		if vmProcOwned(pid) {
			return "alive-legacy", pid
		}
		return "foreign-process", 0
	}
	var rec struct {
		Pid              int `json:"pid"`
		Uid              int `json:"uid"`
		ProcessStartTime any `json:"processStartTime"`
	}
	if json.Unmarshal(rtb, &rec) != nil || rec.Pid != pid {
		return "runtime-corrupt", 0
	}
	if !alive(pid) {
		return "dead", 0
	}
	if rec.Uid > 0 && rec.Uid != os.Getuid() {
		return "uid-mismatch", 0
	}
	if st := rtStartTime(rec.ProcessStartTime); st != "" {
		if cur := procStartTimeJ(pid); cur != "" && cur != st {
			return "pid-reused", 0
		}
	}
	if !vmProcOwned(pid) {
		return "foreign-process", 0
	}
	return "alive", pid
}

// rtStartTime: runtime.json "processStartTime" is a bare number or
// null (printf %s of the jiffies field) — normalise to a string.
func rtStartTime(v any) string {
	switch t := v.(type) {
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case string:
		return t
	}
	return ""
}

func vmPidAlive() bool {
	st, _ := instanceVerify()
	return st == "alive" || st == "alive-legacy"
}

func vmRunning() bool { return vmPidAlive() }

func paused() bool {
	_, err := os.Stat(filepath.Join(vmdir(), vmname()+".paused"))
	return err == nil
}

// qmpVMStatus: qmp_vm_status parity — query-status over the QMP
// socket; "" when the channel is absent or refuses (markers decide).
func qmpVMStatus() string {
	out, err := qmp("query-status")
	if err != nil {
		return ""
	}
	var rep struct {
		Return struct {
			Status string `json:"status"`
		} `json:"return"`
	}
	if json.Unmarshal([]byte(out), &rep) != nil {
		return ""
	}
	return rep.Return.Status
}

func state() (string, int) {
	// Archived = trashed: checked first, a trashed VM reports archived
	// regardless of pid debris (vm_state parity).
	if td := os.Getenv("QE_TRASH_DIR"); td != "" {
		if st, err := os.Stat(filepath.Join(td, vmname())); err == nil && st.IsDir() {
			return "archived", 0
		}
	}
	st, pid := instanceVerify()
	switch st {
	case "alive", "alive-legacy":
		// QMP query-status is authoritative for the guest lifecycle;
		// when the channel is gone the on-disk markers decide.
		switch qst := qmpVMStatus(); qst {
		case "paused", "inmigrate", "suspended":
			return "paused", pid
		case "shutdown":
			return "stopping", pid
		case "":
			if paused() || procStateChar(pid) == 'T' {
				return "paused", pid
			}
			return "running", pid
		default:
			return "running", pid
		}
	case "dead":
		if _, err := os.Stat(pidPath()); err == nil {
			return "stale", 0
		}
		return "stopped", 0
	default:
		// runtime-corrupt / pid-reused / foreign-process /
		// uid-mismatch: live but not provably ours.
		return "unknown", 0
	}
}
