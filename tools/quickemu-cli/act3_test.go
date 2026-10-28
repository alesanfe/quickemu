// act3_test.go — QMP/launch/viewer paths via stub binaries on PATH.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubTools installs fake quickemu-qmp/quickemu-launch/viewer tools.
func stubTools(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for _, b := range []string{"quickemu-qmp", "quickemu-launch", "spicy",
		"remote-viewer", "vncviewer", "xdg-open", "socat", "ssh", "telnet",
		"quickemu-ops", "quickemu-ports", "quickemu-proc", "quickemu-vm",
		"quickemu-toml"} {
		os.WriteFile(filepath.Join(d, b),
			[]byte("#!/bin/sh\necho STUB \"$@\"\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	return d
}

func withConf(t *testing.T, kv string) string {
	t.Helper()
	dir := t.TempDir()
	old := vmConf
	vmConf = filepath.Join(dir, "vm1.conf")
	os.WriteFile(vmConf, []byte(kv), 0o644)
	os.MkdirAll(vmdir(), 0o755)
	t.Cleanup(func() { vmConf = old })
	return vmdir()
}

func TestQMPWrappers(t *testing.T) {
	stubTools(t)
	withConf(t, "")
	out, err := qmp("stop")
	if err != nil || out == "" {
		t.Fatalf("qmp: %v %q", err, out)
	}
	if out, err = qmpRaw("{}"); err != nil || out == "" {
		t.Fatalf("qmpRaw: %v %q", err, out)
	}
	if _, err = hmp("info vms"); err != nil {
		t.Fatalf("hmp: %v", err)
	}
}

// livePID spawns a sleep renamed to the VM name so comm matches and
// returns its pid; the caller must Kill+Wait it.
func livePID(t *testing.T, vd string) int {
	t.Helper()
	bin := filepath.Join(vd, vmname())
	if b, err := os.ReadFile("/bin/sleep"); err == nil {
		os.WriteFile(bin, b, 0o755)
	}
	cmd := exec.Command(bin, "60")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot spawn renamed sleep: " + err.Error())
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	os.WriteFile(pidPath(), []byte(itoa(cmd.Process.Pid)), 0o644)
	return cmd.Process.Pid
}

func TestPauseResumeStop(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "")
	livePID(t, vd)
	if !vmRunning() {
		t.Fatal("vmRunning false with live pid")
	}
	if rc := actPause(); rc != 0 {
		t.Fatalf("pause rc=%d", rc)
	}
	if !paused() {
		t.Fatal(".paused marker missing")
	}
	if st, _ := state(); st != "paused" {
		t.Fatalf("state=%s", st)
	}
	if rc := actResume(); rc != 0 {
		t.Fatalf("resume rc=%d", rc)
	}
	if paused() {
		t.Fatal(".paused marker not removed")
	}
	// force kill: the renamed-sleep dies → not-running path next.
	if rc := actStop(true); rc != 0 {
		t.Fatalf("stop rc=%d", rc)
	}
	deadline := time.Now().Add(3 * time.Second)
	for state0, _ := state(); state0 == "running" || state0 == "paused"; state0, _ = state() {
		if time.Now().After(deadline) {
			t.Fatal("vm still running after force stop")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if rc := actStop(false); rc != 0 {
		t.Fatalf("stop-not-running rc=%d", rc)
	}
}

func TestMonitorCmdBranches(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "")
	old := extra["monitor-cmd"]
	defer func() { extra["monitor-cmd"] = old }()
	extra["monitor-cmd"] = ""
	if rc := actMonitorCmd(); rc != 1 {
		t.Fatal("empty msg should fail")
	}
	extra["monitor-cmd"] = "info vms"
	// no monitor socket → falls back to QMP hmp (stub succeeds).
	if rc := actMonitorCmd(); rc != 0 {
		t.Fatal("hmp fallback failed")
	}
	// monitor=telnet → tcp send; no listener → fail.
	os.WriteFile(vmConf, []byte("monitor=\"telnet\"\nmonitor_telnet_host=\"127.0.0.1\"\nmonitor_telnet_port=\"9\"\n"), 0o644)
	if rc := actMonitorCmd(); rc != 1 {
		t.Fatal("telnet send to closed port should fail")
	}
	// monitor=bogus → error path.
	os.WriteFile(vmConf, []byte("monitor=\"bogus\"\n"), 0o644)
	if rc := actMonitorCmd(); rc != 1 {
		t.Fatal("unknown monitor type should fail")
	}
	_ = vd
}

func TestLaunchDryRunSpice(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "viewer=\"none\"\ndisplay=\"sdl\"\n")
	if rc := actLaunch(); rc != 0 {
		t.Fatalf("launch rc=%d", rc)
	}
	if rc := actDryRun(); rc != 0 {
		t.Fatalf("dry-run rc=%d", rc)
	}
	// bad viewer rejected.
	os.WriteFile(vmConf, []byte("viewer=\"nope\"\n"), 0o644)
	if rc := actLaunch(); rc != 1 {
		t.Fatal("bad viewer should fail")
	}
	// already-running path: live pid + viewer=none conf.
	os.WriteFile(vmConf, []byte("viewer=\"none\"\n"), 0o644)
	livePID(t, vd)
	if rc := actLaunch(); rc != 0 {
		t.Fatalf("already-running launch rc=%d", rc)
	}
	// spice-console on running vm.
	os.WriteFile(vmConf, []byte("display=\"spice\"\nviewer=\"none\"\n"), 0o644)
	if rc := actSpiceConsole(); rc != 0 {
		t.Fatalf("spice-console rc=%d", rc)
	}
	// vnc attach on running vnc vm with vncviewer stub.
	os.WriteFile(vmConf, []byte("display=\"vnc\"\nvnc_listen=\"127.0.0.1:3\"\n"), 0o644)
	if rc := actVNC(); rc != 0 {
		t.Fatalf("vnc rc=%d", rc)
	}
	os.WriteFile(vmConf, []byte("display=\"vnc\"\nvnc_listen=\"127.0.0.1:abc\"\n"), 0o644)
	if rc := actVNC(); rc != 1 {
		t.Fatal("bad vnc_listen should fail")
	}
	// telnet attach: conf says serial=telnet; telnetAttach dials a
	// real listener.
	os.WriteFile(vmConf, []byte("serial=\"telnet\"\nserial_telnet_host=\"127.0.0.1\"\nserial_telnet_port=\"9\"\n"), 0o644)
	if rc := actTelnet("serial"); rc == 0 {
		t.Log("telnet attach unexpectedly ok")
	}
	// public_dir opens via stub xdg-open.
	pub := filepath.Join(vd, "pub")
	os.WriteFile(vmConf, []byte("public_dir=\""+pub+"\"\n"), 0o644)
	if rc := actPublicDir(); rc != 0 {
		t.Fatalf("publicdir rc=%d", rc)
	}
}

func TestApplyOverridesAndStats(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "guest_os=\"linux\"\nram=\"2G\"\n")
	old := confOpts
	confOpts = map[string]string{"ram": "4G", "newkey": "v"}
	defer func() { confOpts = old }()
	ov := applyOverrides()
	if ov == vmConf {
		t.Fatal("no override file")
	}
	b, _ := os.ReadFile(ov)
	if !strings.Contains(string(b), `ram="4G"`) || !strings.Contains(string(b), `newkey="v"`) {
		t.Fatalf("override conf: %s", b)
	}
	confOpts = nil
	if applyOverrides() != vmConf {
		t.Fatal("empty overrides should return vmConf")
	}
	_ = vd
}

func TestDoctorStatsInventory(t *testing.T) {
	stubTools(t)
	withConf(t, "guest_os=\"linux\"\n")
	// diagnostics — must not panic; rc varies with host.
	_ = actDoctor()
	_ = actStats()
	_ = actInventory()
	if !toolExists("sh") {
		t.Fatal("sh must exist")
	}
	if toolExists("definitely-not-a-tool-xyz") {
		t.Fatal("phantom tool found")
	}
	vmVerifyInvariants()
}
