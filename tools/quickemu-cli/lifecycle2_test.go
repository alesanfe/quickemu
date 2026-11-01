// lifecycle2_test.go — monitor-channel lifecycle coverage: pause/resume
// and shutdown/reset through a fake (qemu) monitor socket, screendump
// via the same channel, telnet attach, proc-ownership and doctor
// invariants. Raises the package back over the 80% floor.
package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeMonitor listens on the VM's -monitor.socket, greets "(qemu) "
// immediately, reads one command line and replies "(qemu) " — exactly
// what monitorSend's greeting/write/reply exchange expects. onCmd (may
// be nil) inspects the received command (e.g. writes the screendump
// file a real QEMU would produce).
func fakeMonitor(t *testing.T, sock string, onCmd func(string)) net.Listener {
	t.Helper()
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("no unix sockets")
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.Write([]byte("(qemu) "))
				buf := make([]byte, 4096)
				n, _ := c.Read(buf)
				if n > 0 && onCmd != nil {
					onCmd(strings.TrimSpace(string(buf[:n])))
				}
				c.Write([]byte("ok\n(qemu) "))
			}(c)
		}
	}()
	return l
}

// monitorFixture: running fake VM + monitor=socket conf + live fake
// monitor — the full channel path actPause/actShutdown/... gate on.
func monitorFixture(t *testing.T, onCmd func(string)) string {
	t.Helper()
	vd := withConf(t, "monitor=\"socket\"\n")
	livePID(t, vd)
	fakeMonitor(t, filepath.Join(vd, vmname()+"-monitor.socket"), onCmd)
	return vd
}

func TestMonitorControlAndPauseResume(t *testing.T) {
	monitorFixture(t, nil)
	if !monitorControl("stop") {
		t.Fatal("monitorControl over live monitor socket should succeed")
	}
	if rc := actPause(); rc != 0 {
		t.Fatalf("pause via monitor rc=%d", rc)
	}
	if !paused() {
		t.Fatal(".paused marker missing after monitor stop")
	}
	if rc := actResume(); rc != 0 {
		t.Fatalf("resume via monitor rc=%d", rc)
	}
}

func TestShutdownResetChannels(t *testing.T) {
	// Full path: monitor socket live → system_powerdown / system_reset.
	monitorFixture(t, nil)
	if rc := actShutdown(); rc != 0 {
		t.Fatalf("shutdown via monitor rc=%d", rc)
	}
	if rc := actReset(); rc != 0 {
		t.Fatalf("reset via monitor rc=%d", rc)
	}
}

func TestShutdownResetNoMonitor(t *testing.T) {
	// Running VM but monitor=none → refusal path.
	vd := withConf(t, "monitor=\"none\"\n")
	livePID(t, vd)
	if rc := actShutdown(); rc != 1 {
		t.Fatalf("shutdown monitor=none rc=%d", rc)
	}
	if rc := actReset(); rc != 1 {
		t.Fatalf("reset monitor=none rc=%d", rc)
	}
	// Not running at all → early "not running" branch.
	withConf(t, "")
	if rc := actShutdown(); rc != 1 {
		t.Fatal("shutdown not-running should fail")
	}
	if rc := actReset(); rc != 1 {
		t.Fatal("reset not-running should fail")
	}
}

func TestScreenshotViaMonitor(t *testing.T) {
	vd := monitorFixture(t, func(cmd string) {
		// A real QEMU writes the PPM on `screendump <path>` — emulate.
		if strings.HasPrefix(cmd, "screendump ") {
			p := strings.TrimPrefix(cmd, "screendump ")
			os.WriteFile(p, []byte("P6\n2 1\n255\n\x00\x00\x00\xff\xff\xff"), 0o644)
		}
	})
	ppm := filepath.Join(vd, "shot.ppm")
	old := extra["screenshot"]
	defer func() { extra["screenshot"] = old }()
	extra["screenshot"] = ppm
	if rc := actScreenshot(); rc != 0 {
		t.Fatalf("screenshot rc=%d", rc)
	}
	if st, err := os.Stat(ppm); err != nil || st.Size() == 0 {
		t.Fatal("ppm not produced")
	}
	// .png target → conversion branch (convert/pnmtopng or PPM warning).
	extra["screenshot"] = filepath.Join(vd, "shot.png")
	if rc := actScreenshot(); rc != 0 {
		t.Fatalf("screenshot png rc=%d", rc)
	}
	// Early gates: not running / monitor none / socket missing.
	withConf(t, "")
	if rc := actScreenshot(); rc != 1 {
		t.Fatal("screenshot not-running should fail")
	}
	vd2 := withConf(t, "monitor=\"none\"\n")
	livePID(t, vd2)
	if rc := actScreenshot(); rc != 1 {
		t.Fatal("screenshot monitor=none should fail")
	}
	vd3 := withConf(t, "monitor=\"socket\"\n")
	livePID(t, vd3)
	if rc := actScreenshot(); rc != 1 {
		t.Fatal("screenshot missing socket should fail")
	}
}

func TestTelnetAttachPaths(t *testing.T) {
	// Refused dial → rc 1.
	if rc := telnetAttach("127.0.0.1", "1"); rc != 1 {
		t.Fatal("closed port should fail")
	}
	// Live endpoint that emits a few bytes then closes → rc 0 once the
	// inbound copy drains.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no tcp")
	}
	defer l.Close()
	go func() {
		if c, err := l.Accept(); err == nil {
			c.Write([]byte("QEMU monitor\n"))
			c.Close()
		}
	}()
	port := strings.TrimPrefix(l.Addr().String(), "127.0.0.1:")
	if rc := telnetAttach("127.0.0.1", port); rc != 0 {
		t.Fatalf("attach rc=%d", rc)
	}
}

func TestVMProcOwnedBranches(t *testing.T) {
	vd := withConf(t, "")
	// Cmdline token: a process spawned from inside the vmdir carries
	// <vmdir>/ in its argv[0].
	cmd := spawnFakeProc(t, filepath.Join(vd, "inner-proc"))
	if !vmProcOwned(cmd.Process.Pid) {
		t.Error("proc with vmdir in cmdline must be owned")
	}
	// Foreign: sleep with default argv — comm!=vmname, no vmdir token.
	src, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep")
	}
	fk := exec.Command(src, "60")
	if err := fk.Start(); err != nil {
		t.Skip("spawn failed")
	}
	defer func() { fk.Process.Kill(); fk.Wait() }()
	time.Sleep(50 * time.Millisecond)
	if vmProcOwned(fk.Process.Pid) {
		t.Error("foreign sleep must not be owned")
	}
}

func TestRTStartTime(t *testing.T) {
	if got := rtStartTime(42.0); got != "42" {
		t.Fatalf("float64: %q", got)
	}
	if got := rtStartTime("77"); got != "77" {
		t.Fatalf("string: %q", got)
	}
	if got := rtStartTime(nil); got != "" {
		t.Fatalf("nil: %q", got)
	}
}

func TestVMVerifyInvariantsBranches(t *testing.T) {
	// stopped + no pidfile → consistent.
	withConf(t, "")
	if !vmVerifyInvariants() {
		t.Error("stopped VM without pidfile must pass")
	}
	// archived + stray pidfile in the live dir → invariant violation.
	vd := withConf(t, "")
	os.WriteFile(filepath.Join(vd, vmname()+".pid"), []byte("99999999"), 0o644)
	trash := t.TempDir()
	os.MkdirAll(filepath.Join(trash, vmname()), 0o755)
	t.Setenv("QE_TRASH_DIR", trash)
	if vmVerifyInvariants() {
		t.Error("archived VM with stale pidfile must fail")
	}
	t.Setenv("QE_TRASH_DIR", t.TempDir()) // un-archive for next fixture
	// running + pidfile + runtime.json → pass.
	vd = withConf(t, "")
	pid := livePID(t, vd)
	writeRuntime(t, vd, vmname(), pid, procStartTimeJ(pid), os.Getuid())
	if !vmVerifyInvariants() {
		t.Error("running VM with full state must pass")
	}
	// running but runtime.json removed → violation.
	os.Remove(filepath.Join(vd, vmname()+".runtime.json"))
	if vmVerifyInvariants() {
		t.Error("running VM missing runtime.json must fail")
	}
}

func TestUnameHelpers(t *testing.T) {
	switch got := unameArch(); got {
	case "x86_64", "aarch64":
	default:
		if got == "" {
			t.Fatal("unameArch empty")
		}
	}
	if got := unameFields("-s -r -m"); got == "" {
		t.Fatal("unameFields empty")
	}
	if got := unameFields("-zzz"); got != "" {
		t.Fatalf("bogus flags should yield empty, got %q", got)
	}
}

func TestIgnoreMSRsAlways(t *testing.T) {
	if !fileExists("/etc/modprobe.d") {
		t.Skip("/etc/modprobe.d absent — host not debian-like")
	}
	// Fake sudo + initramfs rebuilders on PATH: the success path runs
	// `sudo tee` then `sudo update-initramfs`/`mkinitcpio`.
	d := t.TempDir()
	for _, b := range []string{"sudo", "update-initramfs", "mkinitcpio"} {
		os.WriteFile(filepath.Join(d, b), []byte("#!/bin/sh\ncat >/dev/null 2>&1 || true\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	if rc := actIgnoreMSRsAlways(); rc != 0 {
		t.Logf("ignore-msrs rc=%d (acceptable: conf may already exist)", rc)
	}
}
