// run_test.go — daemon lifecycle coverage with real processes:
// renamed sleep as a fake virtiofsd/swtpm, python3 unix-socket stubs,
// dead-pid reap, and the runQEMU spawn path.
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAlivePidAndStopDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix")
	}
	if !alivePid(os.Getpid()) {
		t.Fatal("own pid should be alive")
	}
	if alivePid(99999999) {
		t.Fatal("dead pid should be false")
	}
	dir := t.TempDir()
	name := "vm"
	// Missing pidfile → early return.
	stopDaemon(dir, name, "virtiofsd")
	// Dead pid → early return.
	pf := filepath.Join(dir, name+".virtiofsd-pid")
	os.WriteFile(pf, []byte("99999999"), 0o600)
	stopDaemon(dir, name, "virtiofsd")
	// Comm mismatch → return without signalling.
	os.WriteFile(pf, []byte("1"), 0o600) // pid 1 exists, comm != virtiofsd
	stopDaemon(dir, name, "virtiofsd")
	// Real process with matching comm: renamed sleep.
	bin := filepath.Join(dir, "virtiofsd")
	data, _ := os.ReadFile("/bin/sleep")
	os.WriteFile(bin, data, 0o755)
	cmd := exec.Command(bin, "60")
	if err := cmd.Start(); err != nil {
		t.Skip("spawn: " + err.Error())
	}
	defer cmd.Process.Kill()
	os.WriteFile(pf, []byte(itoa(cmd.Process.Pid)), 0o600)
	stopDaemon(dir, name, "virtiofsd")
	// Reap the zombie so the liveness probe sees it as dead.
	cmd.Process.Wait()
	if alivePid(cmd.Process.Pid) {
		t.Fatal("daemon still alive after stopDaemon")
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestReapVM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix")
	}
	dir := t.TempDir()
	name := "vm"
	// Dead pid → immediate cleanup: all runtime artifacts removed.
	// The pidfile must record the reaper's own pid for the ownership
	// guard to allow cleanup.
	os.WriteFile(filepath.Join(dir, name+".pid"), []byte("99999999"), 0o600)
	for _, ext := range []string{".sock", ".ports", ".runtime.json", "-qmp.socket", ".paused"} {
		os.WriteFile(filepath.Join(dir, name+ext), []byte("x"), 0o600)
	}
	os.WriteFile(filepath.Join(dir, name+".lock"), []byte("x"), 0o600)
	os.MkdirAll(filepath.Join(dir, name+".lock.shared"), 0o755)
	reapVM(99999999, dir, name)
	for _, ext := range []string{".pid", ".sock", ".ports", ".runtime.json", "-qmp.socket", ".paused", ".lock"} {
		if _, err := os.Stat(filepath.Join(dir, name+ext)); err == nil {
			t.Fatalf("%s not reaped", ext)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, name+".lock.shared")); err == nil {
		t.Fatal("lock.shared not reaped")
	}
	// Ownership guard: pidfile holds a DIFFERENT live pid → nothing removed.
	os.WriteFile(filepath.Join(dir, name+".pid"), []byte("1"), 0o600)
	os.WriteFile(filepath.Join(dir, name+".ports"), []byte("x"), 0o600)
	reapVM(99999998, dir, name)
	if _, err := os.Stat(filepath.Join(dir, name+".ports")); err != nil {
		t.Fatal("foreign instance state was reaped")
	}
	// Corrupt pidfile → fail-closed: an existing-but-unparseable
	// pidfile may belong to a relaunched instance mid-write, so its
	// state must NOT be reaped.
	os.WriteFile(filepath.Join(dir, name+".pid"), []byte("garbage\n"), 0o600)
	reapVM(99999997, dir, name)
	if _, err := os.Stat(filepath.Join(dir, name+".ports")); err != nil {
		t.Fatal("corrupt pidfile treated as unowned — state was reaped")
	}
	// Missing pidfile is ambiguous: a relaunch may not have written
	// it yet. A live foreign runtime.json proves the dir changed
	// hands → preserve.
	os.Remove(filepath.Join(dir, name+".pid"))
	rt, _ := json.Marshal(map[string]any{"pid": 1}) // pid 1 = live foreign
	os.WriteFile(filepath.Join(dir, name+".runtime.json"), rt, 0o600)
	reapVM(99999996, dir, name)
	if _, err := os.Stat(filepath.Join(dir, name+".ports")); err != nil {
		t.Fatal("missing pidfile + foreign live runtime.json was reaped")
	}
	// A pidfile appearing mid-reap (relaunch writing it late) must be
	// honored on the ownership re-check.
	os.Remove(filepath.Join(dir, name+".runtime.json"))
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.WriteFile(filepath.Join(dir, name+".pid"), []byte("2"), 0o600)
	}()
	reapVM(99999995, dir, name)
	if _, err := os.Stat(filepath.Join(dir, name+".ports")); err != nil {
		t.Fatal("pidfile written mid-reap (relaunch) was ignored")
	}
	// Truly absent state — clean-exit semantics — → cleanup proceeds.
	os.Remove(filepath.Join(dir, name+".pid"))
	reapVM(99999994, dir, name)
	if _, err := os.Stat(filepath.Join(dir, name+".ports")); err == nil {
		t.Fatal("state survived a legit reap")
	}
}

func TestStartVirtiofsdAndSwtpm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix")
	}
	dir := t.TempDir()
	conf := Conf{"_vmdir": dir, "_vmname": "vm", "_public": dir}
	// Daemon that never creates the socket → poll exhausts → false.
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "vfsd"),
		[]byte("#!/bin/sh\nsleep 5\n"), 0o755)
	if startVirtiofsd(filepath.Join(stub, "vfsd"), conf) {
		t.Fatal("no-socket daemon should fail")
	}
	// Daemon that binds a real unix socket (python3) → true.
	if _, err := exec.LookPath("python3"); err == nil {
		os.WriteFile(filepath.Join(stub, "vfsd2"), []byte(`#!/bin/sh
sock=""
for a in "$@"; do case $a in --socket-path=*) sock=${a#*=};; esac; done
exec python3 -c "import socket,time;s=socket.socket(socket.AF_UNIX);s.bind('$sock');time.sleep(30)"
`), 0o755)
		if !startVirtiofsd(filepath.Join(stub, "vfsd2"), conf) {
			t.Fatal("socket daemon should succeed")
		}
		if b, err := os.ReadFile(filepath.Join(dir, "vm.virtiofsd-pid")); err != nil || len(b) == 0 {
			t.Fatal("pidfile not written")
		}
	}
	// swtpm: stub that fails to make the socket → false.
	os.WriteFile(filepath.Join(stub, "swtpm"),
		[]byte("#!/bin/sh\nsleep 5\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	if startSwtpm(conf) {
		t.Fatal("no-socket swtpm should fail")
	}
	// stub that binds the ctrl socket → true.
	if _, err := exec.LookPath("python3"); err == nil {
		os.WriteFile(filepath.Join(stub, "swtpm"), []byte(`#!/bin/sh
sock=""
prev=""
for a in "$@"; do case $prev in --ctrl) sock=${a#type=unixio,path=};; esac; prev=$a; done
exec python3 -c "import socket,time;s=socket.socket(socket.AF_UNIX);s.bind('$sock');time.sleep(30)"
`), 0o755)
		if !startSwtpm(conf) {
			t.Fatal("stub swtpm should succeed")
		}
		if b, err := os.ReadFile(filepath.Join(dir, "vm.swtpm-pid")); err != nil || len(b) == 0 {
			t.Fatal("swtpm pidfile missing")
		}
	}
}

func TestRunQEMU(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix")
	}
	dir := t.TempDir()
	name := "vm"
	conf := filepath.Join(dir, name+".conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o644)
	// Dead-on-start: /bin/true exits before the liveness probe → error.
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "qtrue"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	if err := runQEMU(conf, dir, []string{filepath.Join(stub, "qtrue")}); err == nil {
		t.Fatal("dead qemu should error")
	}
	// Live qemu: script writes pidfile? runQEMU publishes runtime.json
	// and spawns the detached reaper — use sleep so it stays alive.
	os.WriteFile(filepath.Join(stub, "qsleep"), []byte("#!/bin/sh\nsleep 3\n"), 0o755)
	if err := runQEMU(conf, dir, []string{filepath.Join(stub, "qsleep")}); err != nil {
		t.Fatalf("runQEMU %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, name+".sh")); err != nil {
		t.Fatal("launch script not written")
	}
	if _, err := os.Stat(filepath.Join(dir, name+".runtime.json")); err != nil {
		t.Fatal("runtime.json not published")
	}
	if _, err := os.Stat(filepath.Join(dir, name+".log")); err != nil {
		t.Fatal("log not created")
	}
	// swtpm-pid preexisting → script preamble includes companion line.
	os.WriteFile(filepath.Join(dir, name+".swtpm-pid"), []byte("1"), 0o600)
	os.WriteFile(filepath.Join(stub, "qsleep2"), []byte("#!/bin/sh\nsleep 3\n"), 0o755)
	if err := runQEMU(conf, dir, []string{filepath.Join(stub, "qsleep2")}); err != nil {
		t.Fatalf("runQEMU2 %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, name+".sh")); !strings.Contains(string(b), "swtpm socket") {
		t.Fatal("companion line missing from launch script")
	}
}
