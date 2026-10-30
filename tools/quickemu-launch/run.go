// run: spawn QEMU with the generated args — the launcher migration.
// Writes the .sh launch script, publishes instance identity, and
// spawns a detached reaper that runs the vm_runtime_cleanup equivalent
// when QEMU exits on its own (Bash disown'd watcher parity).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// instanceUUID: vm_instance_uuid parity — kernel uuid source, or a
// timestamp+pid+random fallback when /proc is unreadable.
func instanceUUID() string {
	if b, err := os.ReadFile("/proc/sys/kernel/random/uuid"); err == nil {
		return strings.TrimSpace(string(b))
	}
	return fmt.Sprintf("%d-%d-%d", time.Now().UnixNano(), os.Getpid(), time.Now().Unix()%100000)
}

// procStartTime: vm_proc_start_time — jiffies-since-boot from
// /proc/<pid>/stat field 22 ("" on non-Linux / unreadable).
func procStartTime(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	f := strings.Fields(string(b))
	if len(f) < 22 {
		return ""
	}
	return f[21]
}

// detach puts the child in its own session — the Bash `&` + `disown`
// equivalent. Without it QEMU dies on SIGHUP when the spawning
// terminal/session goes away.
func detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}

// shq: POSIX single-quote — the launch .sh embeds conf paths, the vm
// name and every QEMU arg; an apostrophe or $(...) in any of them
// would otherwise execute when the script is run.
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func runQEMU(confPath string, vmdir string, args []string, conf ...Conf) error {
	dir := vmdir
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	name := strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
	// launch script (execution artifact, same as bash — the swtpm
	// companion line precedes the QEMU invocation like configure_tpm).
	script := filepath.Join(dir, name+".sh")
	pre := ""
	// Companion-daemon lines precede the QEMU invocation in configure
	// order: virtiofsd (start_virtiofsd) then swtpm (configure_tpm).
	if len(conf) > 0 && conf[0]["_virtiofsd_bin"] != "" {
		var vb strings.Builder
		for _, a := range virtiofsdArgv(conf[0]["_virtiofsd_bin"], conf[0]) {
			vb.WriteString(shq(a) + " ")
		}
		pre = vb.String() + "&\n"
	}
	if _, err := os.Stat(filepath.Join(dir, name+".swtpm-pid")); err == nil {
		pre += "swtpm socket --ctrl type=unixio,path=" + shq(filepath.Join(dir, name+".swtpm-sock")) +
			" --terminate --tpmstate dir=" + shq(dir) + " --tpm2 &\n"
	}
	var sb strings.Builder
	for i, arg := range args {
		if i > 0 {
			sb.WriteString(" \\\n    ")
		}
		sb.WriteString(shq(arg))
	}
	scriptBody := "#!/bin/sh\n" + pre + sb.String() + " 2>/dev/null\n"
	if err := os.WriteFile(script, []byte(scriptBody), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: launch script %s: %v\n", script, err)
	}
	logPath := filepath.Join(dir, name+".log")
	logF, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logF.Close()
	cmd := execCommand(args[0], args[1:]...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	// configure_cpu_pinning runs right after spawn (vm_boot): maps the
	// guest's CPU threads onto the conf's host core list via taskset.
	if len(conf) > 0 {
		applyCPUPinning(pid, conf[0])
	}
	// Reap in background — without Wait() a dead QEMU stays a zombie
	// and kill(pid,0) still succeeds, so the liveness probe below
	// would report "Started" for a guest that already crashed.
	go cmd.Wait()
	// wait 250ms — if it died, qemu_spawn prints the ERROR line, drops
	// the boot-state files and cats the log (all stdout, echo contract).
	time.Sleep(250 * time.Millisecond)
	if err := syscall.Kill(pid, 0); err != nil {
		fmt.Printf(" - Process:  ERROR! Failed to start %s as %s\n", confPath, name)
		os.Remove(filepath.Join(dir, name+".pid"))
		os.Remove(filepath.Join(dir, name+".spice"))
		os.Remove(dotdir(dir) + name + ".sock")
		fmt.Println()
		logB, _ := os.ReadFile(logPath)
		fmt.Println(string(logB))
		return fmt.Errorf("qemu died on start")
	}
	// publish instance identity (same JSON contract as vm_instance_publish):
	// uuid + proc start-time + uid + command fingerprint, atomic 0600.
	rt := filepath.Join(dir, name+".runtime.json")
	fpv := sha256.Sum256([]byte(args[0] + "|" + confPath + "|" + name))
	rec := map[string]any{
		"schemaVersion":      1,
		"instanceId":         instanceUUID(),
		"vmId":               name,
		"pid":                pid,
		"uid":                os.Getuid(),
		"commandFingerprint": "sha256:" + hex.EncodeToString(fpv[:]),
		"configPath":         confPath,
		"createdAt":          time.Now().UTC().Format(time.RFC3339),
	}
	if st := procStartTime(pid); st != "" {
		rec["processStartTime"] = st
	} else {
		rec["processStartTime"] = nil
	}
	rtJSON, _ := json.Marshal(rec)
	tmp := filepath.Join(dir, "."+name+".runtime.json.tmp")
	if err := os.WriteFile(tmp, append(rtJSON, '\n'), 0600); err == nil {
		if err := os.Rename(tmp, rt); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: runtime.json rename failed: %v\n", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "WARNING: runtime.json write failed: %v — API/CLI state will show stopped\n", err)
	}
	// Detached reaper: quickemu exits right after spawning QEMU, so a
	// separate session performs the cleanup when the guest dies —
	// Bash's disowned `while kill -0 $VM_PID` watcher. Re-exec self.
	if self, err := os.Executable(); err == nil {
		reaper := execCommand(self, "--reap",
			fmt.Sprint(pid), dir, name, fmt.Sprint(os.Getpid()))
		reaper.Stdout, reaper.Stderr = logF, logF
		detach(reaper)
		if err := reaper.Start(); err == nil {
			reaper.Process.Release()
		}
	}
	fmt.Printf(" - Process:  Started %s as %s (%d)\n", confPath, name, pid)
	return nil
}

// reapVM: --reap <pid> <dir> <name> — independent-process equivalent of
// the Bash disowned watcher + vm_runtime_cleanup. Polls until the QEMU
// pid is gone, then stops companion daemons (comm-guarded TERM, then
// KILL) and removes every runtime state file.
// launcherPid is set by the --reap path (4th arg) so the reaper can
// drop the launcher's port-registry reservations at cleanup time —
// _port_registry_release parity.
var launcherPid int

func reapVM(pid int, dir, name string) {
	for alivePid(pid) {
		time.Sleep(2 * time.Second)
	}
	// Ownership guard (fail-closed): the pidfile may now belong to a
	// relaunched instance — either recording a DIFFERENT pid or caught
	// mid-write with unparseable content. Only clean when the pidfile
	// still says "this reaper's pid"; anything else means the state
	// belongs to someone else (or is unknown — never delete on doubt).
	// A missing pidfile is ambiguous (QEMU unlinks it on clean exit,
	// but a relaunch may not have written it yet), so it earns a
	// re-check before the runtime.json fallback decides.
	pf := filepath.Join(dir, name+".pid")
	owned := false
	for i := 0; i < 2; i++ {
		b, err := os.ReadFile(pf)
		if err != nil {
			if i == 0 {
				time.Sleep(time.Second)
			}
			continue
		}
		cur := 0
		fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &cur)
		if cur != pid {
			return
		}
		owned = true
		break
	}
	if !owned {
		// Still no pidfile — QEMU may run without one. A live foreign
		// pid in runtime.json is the last proof the dir changed hands.
		if b, err := os.ReadFile(filepath.Join(dir, name+".runtime.json")); err == nil {
			var rt struct {
				Pid int `json:"pid"`
			}
			if json.Unmarshal(b, &rt) == nil && rt.Pid > 0 && rt.Pid != pid && alivePid(rt.Pid) {
				return
			}
		}
	}
	if lp := launcherPid; lp > 0 {
		portRegistryRelease(lp)
	}
	stopDaemon(dir, name, "virtiofsd")
	stopDaemon(dir, name, "swtpm")
	for _, ext := range []string{
		".pid", ".spice", ".sock",
		"-monitor.socket", "-qmp.socket", "-serial.socket", "-agent.sock",
		".ports", ".paused", ".runtime.json",
		".virtiofsd-pid", ".virtiofsd-sock",
		".swtpm-pid", ".swtpm-sock",
	} {
		os.Remove(filepath.Join(dir, name+ext))
	}
	os.Remove(filepath.Join(dir, name+".lock"))
	os.RemoveAll(filepath.Join(dir, name+".lock.shared"))
	// vm_runtime_cleanup tail: registry + event. Without this a QEMU
	// that exits on its own leaves no vm.stopped transition — the CLI
	// only emits for user-initiated stops.
	inventoryTouchStopped(dir, name, pid)
	qeEventAt(dir, name, "vm.stopped", pid, "")
}

// qeEventAt: qe_event parity for the detached reaper — appends a JSONL
// record to <dir>/<name>.events.jsonl (same schema as the CLI's
// qeEvent: seq, ts, event, vm, pid, data; ~1MiB rotation to .1).
func qeEventAt(dir, name, typ string, pid int, data string) {
	f := filepath.Join(dir, name+".events.jsonl")
	if st, err := os.Stat(f); err == nil && st.Size() >= 1048576 {
		os.Rename(f, f+".1")
	}
	seq := 1
	if b, err := os.ReadFile(f); err == nil {
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		var last struct {
			Seq int `json:"seq"`
		}
		if json.Unmarshal([]byte(lines[len(lines)-1]), &last) == nil {
			seq = last.Seq + 1
		}
	}
	if data == "" {
		data = "null"
	}
	rec, _ := json.Marshal(map[string]any{
		"seq": seq, "ts": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"event": typ, "vm": name, "pid": pid,
		"data": json.RawMessage(data)})
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer fh.Close()
	fh.Write(append(rec, '\n'))
}

// inventoryTouchStopped: inventory_touch parity — upsert this
// instance into the global vms.json registry keyed by QE_INSTANCE_ID
// (the reaper inherits the launcher's env) with the watched pid as
// fallback.
func inventoryTouchStopped(dir, name string, pid int) {
	db := filepath.Join(qelib.XDGData(), "quickemu", "vms.json")
	if err := os.MkdirAll(filepath.Dir(db), 0755); err != nil {
		return
	}
	id := os.Getenv("QE_INSTANCE_ID")
	if id == "" {
		id = fmt.Sprint(pid)
	}
	m := map[string]any{}
	if b, err := os.ReadFile(db); err == nil {
		json.Unmarshal(b, &m)
	}
	m[id] = map[string]any{"vm": name, "state": "stopped",
		"dir": dir, "lastSeen": time.Now().UTC().Format(time.RFC3339)}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	tmp := db + ".tmp"
	if os.WriteFile(tmp, b, 0644) == nil {
		os.Rename(tmp, db)
	} else {
		os.Remove(tmp)
	}
}

// alivePid: EPERM-aware liveness — a foreign-owned daemon counts as
// alive (its pidfile must not be cleaned up from under it).
func alivePid(pid int) bool { return qelib.Alive(pid) }

// stopDaemon: stop_virtiofsd/stop_swtpm parity — read the recorded
// pidfile, verify /proc comm matches (PID-reuse guard), TERM then KILL.
func stopDaemon(dir, name, comm string) {
	b, err := os.ReadFile(filepath.Join(dir, name+"."+comm+"-pid"))
	if err != nil {
		return
	}
	pid := 0
	fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid)
	if !alivePid(pid) {
		return
	}
	if cb, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err != nil ||
		strings.TrimSpace(string(cb)) != comm {
		return
	}
	syscall.Kill(pid, syscall.SIGTERM)
	for i := 0; i < 5 && alivePid(pid); i++ {
		time.Sleep(200 * time.Millisecond)
	}
	if alivePid(pid) {
		syscall.Kill(pid, syscall.SIGKILL)
	}
}

// virtiofsdArgv: the daemon invocation line — shared by startVirtiofsd
// and the .sh preamble (start_virtiofsd appends it with a trailing &).
func virtiofsdArgv(bin string, c Conf) []string {
	dir, name := c["_vmdir"], c["_vmname"]
	return []string{bin,
		"--socket-path=" + filepath.Join(dir, name+".virtiofsd-sock"),
		"--shared-dir=" + c["_public"],
		"--announce-submounts",
		fmt.Sprintf("--translate-uid=squash-guest:0:%d:65536", os.Getuid()),
		fmt.Sprintf("--translate-gid=squash-guest:0:%d:65536", os.Getgid())}
}

// virtiofsdFail: start_virtiofsd failure reporting — the "Operation
// not permitted" case gets its own two-line warning.
func virtiofsdFail(errB []byte) {
	if strings.Contains(string(errB), "Operation not permitted") {
		fmt.Println(" - WARNING! virtiofsd failed to start (insufficient permissions); falling back to 9p.")
		fmt.Println("            Install the standalone virtiofsd package to enable virtiofs support.")
	} else {
		fmt.Println(" - WARNING! virtiofsd failed to start; falling back to 9p.")
	}
}

// daemonSockPid: resolve the daemon child's pid. virtiofsd forks, so
// cmd.Process.Pid is the launcher that exits immediately — the pidfile
// must hold the socket owner or the reaper cannot kill it. Prefer
// fuser (unambiguous owner); else scan /proc for comm==name whose
// cmdline carries the socket path.
func daemonSockPid(sock, name string, fallback int) int {
	if out, err := exec.Command("fuser", sock).Output(); err == nil {
		pid := 0
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &pid); err == nil && pid > 0 {
			return pid
		}
	}
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cb, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil || strings.TrimSpace(string(cb)) != name {
			continue
		}
		cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err == nil && strings.Contains(strings.ReplaceAll(string(cmdline), "\x00", " "), sock) {
			return pid
		}
	}
	return fallback
}

// startVirtiofsd: spawn the standalone virtiofsd before QEMU and poll
// for its socket (start_virtiofsd parity). The caller falls back to 9p
// when this returns false.
func startVirtiofsd(bin string, c Conf) bool {
	dir, name := c["_vmdir"], c["_vmname"]
	sock := filepath.Join(dir, name+".virtiofsd-sock")
	os.Remove(sock)
	logF, _ := os.OpenFile(filepath.Join(dir, name+".log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer logF.Close()
	// stderr captured apart (Bash mktemp): the "Operation not
	// permitted" probe must not race stale entries in the shared log.
	errF, _ := os.CreateTemp("", "virtiofsd-*.err")
	if errF != nil {
		defer os.Remove(errF.Name())
		defer errF.Close()
	}
	cmd := execCommand(bin, virtiofsdArgv(bin, c)[1:]...)
	if logF != nil {
		cmd.Stdout = logF
	}
	if errF != nil {
		cmd.Stderr = errF
	}
	detach(cmd)
	startErr := cmd.Start()
	readErr := func() []byte {
		if errF == nil {
			return nil
		}
		errF.Sync()
		b, _ := os.ReadFile(errF.Name())
		if logF != nil {
			logF.Write(b)
		}
		return b
	}
	if startErr != nil {
		virtiofsdFail(readErr())
		return false
	}
	go cmd.Wait()
	for i := 0; i < 20; i++ {
		if st, err := os.Stat(sock); err == nil && st.Mode()&os.ModeSocket != 0 {
			dpid := daemonSockPid(sock, "virtiofsd", cmd.Process.Pid)
			if err := os.WriteFile(filepath.Join(dir, name+".virtiofsd-pid"),
				[]byte(fmt.Sprint(dpid)), 0644); err != nil {
				// Without the pidfile the reaper cannot kill the
				// daemon at cleanup — fail rather than leak it.
				syscall.Kill(dpid, syscall.SIGKILL)
				return false
			}
			readErr()
			fmt.Printf(" - virtiofsd: %s (%d)\n", sock, dpid)
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	errB := readErr()
	cmd.Process.Kill()
	virtiofsdFail(errB)
	return false
}

// startSwtpm: configure_tpm parity — spawn `swtpm socket` with the
// same control/state flags before QEMU, record .swtpm-pid, and poll
// for the control socket. Returns false when the daemon never came up
// (the caller then skips the -chardev tpm args, matching
// swtpm_socket_ready).
func startSwtpm(c Conf) bool {
	dir, name := c["_vmdir"], c["_vmname"]
	// swtpm parses --ctrl type=unixio,path=X and --tpmstate dir=D by
	// splitting on ',' with no escape mechanism — a comma anywhere in
	// the dir or VM name would inject extra daemon options. Refuse.
	sock := filepath.Join(dir, name+".swtpm-sock")
	if strings.ContainsAny(sock+dir, ",") {
		fmt.Fprintf(os.Stderr, "ERROR! TPM unavailable: VM dir %q contains ',' (breaks swtpm options).\n", dir)
		return false
	}
	os.Remove(sock)
	bin, err := exec.LookPath("swtpm")
	if err != nil {
		return false
	}
	logF, _ := os.OpenFile(filepath.Join(dir, name+".log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer logF.Close()
	cmd := execCommand(bin, "socket",
		"--ctrl", "type=unixio,path="+sock,
		"--terminate",
		"--tpmstate", "dir="+dir,
		"--tpm2")
	if logF != nil {
		cmd.Stdout, cmd.Stderr = logF, logF
	}
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return false
	}
	if err := os.WriteFile(filepath.Join(dir, name+".swtpm-pid"),
		[]byte(fmt.Sprint(cmd.Process.Pid)), 0644); err != nil {
		cmd.Process.Kill()
		return false
	}
	for i := 0; i < 10; i++ {
		if st, err := os.Stat(sock); err == nil && st.Mode()&os.ModeSocket != 0 {
			fmt.Printf(" - TPM:      %s (%d)\n", sock, cmd.Process.Pid)
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	cmd.Process.Kill()
	return false
}
