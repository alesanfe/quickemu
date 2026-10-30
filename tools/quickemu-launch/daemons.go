// daemons.go — companion-daemon spawns extracted from run.go:
// virtiofsd (file share) + swtpm (TPM), with their socket/pid
// discovery and stderr classification.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "fuser", sock).Output(); err == nil {
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
