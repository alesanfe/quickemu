// tools/quickemu-cli/lifecycle_state.go: VM state introspection: pid/proc/lock/qmp state + instance verify
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	return strings.ReplaceAll(
		strings.TrimRight(string(procCmdlineRaw(pid)), "\x00"), "\x00", " ")
}

// procCmdlineRaw: the verbatim /proc/<pid>/cmdline bytes — the hash
// input for the args fingerprint. Empty when unreadable.
func procCmdlineRaw(pid int) []byte {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return nil
	}
	return b
}

// cmdlineArgv0: argv[0] of a live process — the executable name QEMU
// was exec'd as (the commandFingerprint input, not the resolved path).
func cmdlineArgv0(pid int) string {
	raw := procCmdlineRaw(pid)
	if i := bytes.IndexByte(raw, 0); i > 0 {
		return string(raw[:i])
	}
	return ""
}

// procExe: /proc/<pid>/exe readlink — the REAL binary on disk backing
// the process. Second return reports a " (deleted)" link (the binary
// was unlinked/replaced after exec — the recorded fingerprint can no
// longer be reproduced on disk, so the process is unverifiable).
func procExe(pid int) (string, bool) {
	ex, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "", false
	}
	if strings.HasSuffix(ex, " (deleted)") {
		return strings.TrimSuffix(ex, " (deleted)"), true
	}
	return ex, false
}

// procArgsHash: sha256 over the raw cmdline bytes — any argv change
// (a different VM, a different qemu build flag set) flips the hash.
func procArgsHash(pid int) string {
	raw := procCmdlineRaw(pid)
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
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

// runtimeRec: the runtime.json identity record as the launcher
// publishes it, plus the strong-fingerprint fields the CLI augments
// (exe real path + full-args hash — QE fingerprint extension).
type runtimeRec struct {
	Pid              int    `json:"pid"`
	Uid              int    `json:"uid"`
	ProcessStartTime any    `json:"processStartTime"`
	Exe              string `json:"exe"`
	ArgsHash         string `json:"argsHash"`
	CommandFp        string `json:"commandFingerprint"`
	ConfigPath       string `json:"configPath"`
	VmId             string `json:"vmId"`
}

// instanceVerify: vm_instance_verify parity — pidfile + sibling
// runtime.json identity → a typed status word. Exit status in Bash is
// folded into the word itself; callers only need the pair (word, pid).
// A live pid is NOT enough (QE fingerprint): uid, process start-time,
// the real exe path and the recorded args hash must all hold before
// the process counts as ours — a recycled or forged pid is refused.
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
	var rec runtimeRec
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
	if w := fingerprintCheck(pid, &rec); w != "" {
		return w, 0
	}
	if !vmProcOwned(pid) {
		return "foreign-process", 0
	}
	fingerprintAugment(pid)
	return "alive", pid
}

// fingerprintCheck: the QE-001 strong-identity layer — everything
// provable from /proc must match the recorded fingerprint before the
// pid is trusted. Unreadable evidence is skipped (macOS / foreign
// owners fall back to the comm heuristic, exactly like vmProcOwned);
// contradicting evidence rejects.
func fingerprintCheck(pid int, rec *runtimeRec) string {
	cl := procCmdlineRaw(pid)
	if len(cl) == 0 {
		return "" // no procfs evidence — legacy path decides
	}
	ex, deleted := procExe(pid)
	switch {
	case deleted:
		// The recorded binary no longer exists on disk — the process
		// outlived its image, so nothing on disk can corroborate it.
		return "exe-deleted"
	case ex == "":
		// cmdline readable but exe not: a process we can partially
		// see is worse than one we can't see at all — refuse.
		return "exe-unverifiable"
	}
	// The real binary must be the one argv[0] claims — an exec'd
	// process whose image was swapped is not the recorded one.
	if a0 := cmdlineArgv0(pid); a0 != "" && filepath.Base(a0) != filepath.Base(ex) {
		return "exe-mismatch"
	}
	if rec.Exe != "" && rec.Exe != ex {
		return "exe-mismatch"
	}
	// The args hash is the strongest check the CLI records itself
	// (sha256 of the raw cmdline — any argv drift flips it).
	if strings.HasPrefix(rec.ArgsHash, "sha256:") {
		if h := procArgsHash(pid); h != "" && h != rec.ArgsHash {
			return "fingerprint-mismatch"
		}
	} else if strings.HasPrefix(rec.CommandFp, "sha256:") &&
		rec.ConfigPath != "" && rec.VmId != "" {
		// Launcher fingerprint: sha256(argv0 | configPath | vmId) —
		// recompute against both the raw argv[0] and the resolved
		// exe so either spelling verifies.
		ok := false
		for _, a := range []string{cmdlineArgv0(pid), ex} {
			if a == "" {
				continue
			}
			sum := sha256.Sum256([]byte(a + "|" + rec.ConfigPath + "|" + rec.VmId))
			if "sha256:"+hex.EncodeToString(sum[:]) == rec.CommandFp {
				ok = true
				break
			}
		}
		if !ok {
			return "fingerprint-mismatch"
		}
	}
	return ""
}

// fingerprintAugment: fills the strong-fingerprint fields the Bash
// runtime.json predates (exe real path, full-args hash — plus any
// missing legacy fields) into the live record. Runs after a
// successful verify and after spawn; atomic rewrite so a concurrent
// reader never sees a torn record. Best-effort, never fatal.
func fingerprintAugment(pid int) {
	rt := filepath.Join(vmdir(), vmname()+".runtime.json")
	b, err := os.ReadFile(rt)
	if err != nil {
		return
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return
	}
	if p, _ := m["pid"].(float64); int(p) != pid {
		return // record belongs to another instance — never edit it
	}
	set := func(k string, v any) {
		if _, ok := m[k]; !ok || m[k] == nil || m[k] == "" {
			m[k] = v
		}
	}
	if ex, deleted := procExe(pid); ex != "" && !deleted {
		set("exe", ex)
	}
	if h := procArgsHash(pid); h != "" {
		set("argsHash", h)
	}
	if _, ok := m["uid"]; !ok {
		set("uid", os.Getuid())
	}
	if rtStartTime(m["processStartTime"]) == "" {
		if st := procStartTimeJ(pid); st != "" {
			if n, err := strconv.ParseInt(st, 10, 64); err == nil {
				set("processStartTime", n)
			}
		}
	}
	set("configPath", vmConf)
	set("vmId", vmname())
	out, err := json.Marshal(m)
	if err != nil {
		return
	}
	tmp := rt + ".tmp"
	if os.WriteFile(tmp, append(out, '\n'), 0600) == nil {
		os.Rename(tmp, rt)
	} else {
		os.Remove(tmp)
	}
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
