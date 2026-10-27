// run: spawn QEMU with the generated args — the launcher migration.
// Writes the .sh launch script, publishes instance identity, and
// spawns a detached reaper that runs the vm_runtime_cleanup equivalent
// when QEMU exits on its own (Bash disown'd watcher parity).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// detach puts the child in its own session — the Bash `&` + `disown`
// equivalent. Without it QEMU dies on SIGHUP when the spawning
// terminal/session goes away.
func detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}

func runQEMU(confPath string, args []string) error {
	dir := filepath.Dir(confPath)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	name := strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
	// launch script (execution artifact, same as bash — the swtpm
	// companion line precedes the QEMU invocation like configure_tpm).
	script := filepath.Join(dir, name+".sh")
	pre := ""
	if _, err := os.Stat(filepath.Join(dir, name+".swtpm-pid")); err == nil {
		pre = "swtpm socket --ctrl type=unixio,path=" + filepath.Join(dir, name+".swtpm-sock") +
			" --terminate --tpmstate dir=" + dir + " --tpm2 &\n"
	}
	scriptBody := "#!/usr/bin/env bash\n" + pre + strings.Join(args, " \\\n    ") + " 2>/dev/null\n"
	os.WriteFile(script, []byte(scriptBody), 0755)
	logPath := filepath.Join(dir, name+".log")
	logF, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logF.Close()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	// wait 250ms — if it died, print the log and fail (bash parity)
	time.Sleep(250 * time.Millisecond)
	if err := syscall.Kill(pid, 0); err != nil {
		logB, _ := os.ReadFile(logPath)
		fmt.Fprintln(os.Stderr, string(logB))
		return fmt.Errorf("qemu died on start")
	}
	// publish instance identity (same JSON contract as vm_instance_publish)
	rt := filepath.Join(dir, name+".runtime.json")
	rtJSON, _ := json.Marshal(map[string]interface{}{
		"pid": pid, "name": name, "startedAt": time.Now().UTC().Format(time.RFC3339),
		"qmp": filepath.Join(dir, name+"-qmp.socket"),
	})
	os.WriteFile(rt, rtJSON, 0644)
	// Detached reaper: quickemu exits right after spawning QEMU, so a
	// separate session performs the cleanup when the guest dies —
	// Bash's disowned `while kill -0 $VM_PID` watcher. Re-exec self.
	if self, err := os.Executable(); err == nil {
		reaper := exec.Command(self, "--reap",
			fmt.Sprint(pid), dir, name, fmt.Sprint(os.Getpid()))
		reaper.Stdout, reaper.Stderr = logF, logF
		detach(reaper)
		if err := reaper.Start(); err == nil {
			reaper.Process.Release()
		}
	}
	fmt.Printf(" - Process: Started %s (%d)\n", name, pid)
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
}

func alivePid(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

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
	cmd := exec.Command(bin,
		"--socket-path="+sock,
		"--shared-dir="+c["_public"],
		"--announce-submounts",
		fmt.Sprintf("--translate-uid=squash-guest:0:%d:65536", os.Getuid()),
		fmt.Sprintf("--translate-gid=squash-guest:0:%d:65536", os.Getgid()))
	if logF != nil {
		cmd.Stdout, cmd.Stderr = logF, logF
	}
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return false
	}
	for i := 0; i < 20; i++ {
		if st, err := os.Stat(sock); err == nil && st.Mode()&os.ModeSocket != 0 {
			os.WriteFile(filepath.Join(dir, name+".virtiofsd-pid"),
				[]byte(fmt.Sprint(cmd.Process.Pid)), 0644)
			fmt.Printf(" - virtiofsd: %s (%d)\n", sock, cmd.Process.Pid)
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	cmd.Process.Kill()
	return false
}

// startSwtpm: configure_tpm parity — spawn `swtpm socket` with the
// same control/state flags before QEMU, record .swtpm-pid, and poll
// for the control socket. Returns false when the daemon never came up
// (the caller then skips the -chardev tpm args, matching
// swtpm_socket_ready).
func startSwtpm(c Conf) bool {
	dir, name := c["_vmdir"], c["_vmname"]
	sock := filepath.Join(dir, name+".swtpm-sock")
	os.Remove(sock)
	bin, err := exec.LookPath("swtpm")
	if err != nil {
		return false
	}
	logF, _ := os.OpenFile(filepath.Join(dir, name+".log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer logF.Close()
	cmd := exec.Command(bin, "socket",
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
	os.WriteFile(filepath.Join(dir, name+".swtpm-pid"),
		[]byte(fmt.Sprint(cmd.Process.Pid)), 0644)
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
