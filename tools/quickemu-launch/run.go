// run: spawn QEMU with the generated args — the launcher migration.
// Writes the .sh launch script, publishes instance identity, starts
// the cleanup watcher (reaps companion daemons when QEMU exits).
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

func runQEMU(confPath string, args []string) error {
	dir := filepath.Dir(confPath)
	name := strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
	// launch script (execution artifact, same as bash)
	script := filepath.Join(dir, name+".sh")
	scriptBody := "#!/usr/bin/env bash\n" + strings.Join(args, " \\n    ") + " 2>/dev/null\n"
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
		"pid": pid, "name": name, "startedAt": time.Now().UTC().Format(time.RFC3339), "qmp": filepath.Join(dir, "qmp.sock"),
	})
	os.WriteFile(rt, rtJSON, 0644)
	// cleanup watcher: reap companions when QEMU exits
	go func() {
		cmd.Wait()
		os.Remove(rt)
		os.Remove(filepath.Join(dir, "qmp.sock"))
		os.Remove(filepath.Join(dir, "monitor.sock"))
		os.Remove(filepath.Join(dir, "serial.sock"))
		os.Remove(filepath.Join(dir, "vm.pid"))
	}()
	fmt.Printf(" - Process: Started %s (%d)\n", name, pid)
	return nil
}

// startVirtiofsd: spawn the standalone virtiofsd before QEMU and poll
// for its socket (start_virtiofsd parity). The caller falls back to 9p
// when this returns false.
func startVirtiofsd(bin string, c Conf) bool {
	sock := filepath.Join(c["_vmdir"], c["_vmname"]+".virtiofsd-sock")
	os.Remove(sock)
	logF, _ := os.OpenFile(filepath.Join(c["_vmdir"], c["_vmname"]+".log"),
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
	if err := cmd.Start(); err != nil {
		return false
	}
	for i := 0; i < 20; i++ {
		if st, err := os.Stat(sock); err == nil && st.Mode()&os.ModeSocket != 0 {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	cmd.Process.Kill()
	return false
}
