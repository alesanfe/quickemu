// act_test.go: in-package coverage of the action layer — state
// detection (stopped/running/paused/stale), allowedOps matrix, list
// enumeration, vmdir/vmname resolution and runtime cleanup, all on
// temp VM layouts.
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mkLayout builds <root>/<name>/ + <root>/<name>.conf and points the
// package globals at it.
func mkLayout(t *testing.T, name string) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, name)
	os.MkdirAll(dir, 0o755)
	conf := filepath.Join(root, name+".conf")
	os.WriteFile(conf, []byte("guest_os=\"windows\"\n"), 0o600)
	old := vmConf
	vmConf = conf
	t.Cleanup(func() { vmConf = old })
	return root, dir
}

func TestStateMatrix(t *testing.T) {
	_, dir := mkLayout(t, "s1")

	if st, pid := state(); st != "stopped" || pid != 0 {
		t.Fatalf("stopped: %q %d", st, pid)
	}
	// stale: pidfile with dead pid.
	os.WriteFile(filepath.Join(dir, "s1.pid"), []byte("99999999"), 0o600)
	if st, _ := state(); st != "stale" {
		t.Fatalf("stale: %q", st)
	}
	// running: a process whose comm matches the VM name. /bin/sleep
	// copied under the vm name gives exactly that identity.
	b, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Skip("no /bin/sleep")
	}
	sp := filepath.Join(dir, "s1")
	if err := os.WriteFile(sp, b, 0o755); err != nil {
		t.Skip("sleep copy failed")
	}
	cmd := exec.Command(sp, "60")
	if err := cmd.Start(); err != nil {
		t.Skip("sleep spawn failed")
	}
	defer cmd.Process.Kill()
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "s1.pid"), []byte(itoa(pid)), 0o600)
	if st, got := state(); st != "running" || got != pid {
		t.Fatalf("running: %q %d", st, got)
	}
	// paused wins over running.
	os.WriteFile(filepath.Join(dir, "s1.paused"), []byte(""), 0o600)
	if st, _ := state(); st != "paused" {
		t.Fatalf("paused: %q", st)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return strings.TrimSpace(string(b))
}

func TestAllowedOpsMatrix(t *testing.T) {
	cases := map[string]string{
		"running":  "pause",
		"paused":   "resume",
		"stopped":  "start",
		"stale":    "start",
		"archived": "restore",
		"unknown":  "inspect-conf",
	}
	for st, want := range cases {
		found := false
		for _, o := range allowedOps(st) {
			if o == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s must allow %q: %v", st, want, allowedOps(st))
		}
	}
	// Cross-checks: stopped must NOT offer pause; running must NOT offer start.
	for _, o := range allowedOps("stopped") {
		if o == "pause" {
			t.Fatal("stopped must not offer pause")
		}
	}
	for _, o := range allowedOps("running") {
		if o == "start" {
			t.Fatal("running must not offer start")
		}
	}
}

func TestStatusAction(t *testing.T) {
	_, dir := mkLayout(t, "st")
	os.WriteFile(filepath.Join(dir, "st.pid"), []byte("99999999"), 0o600)
	if rc := actStatus(); rc != 0 {
		t.Fatalf("status rc=%d", rc)
	}
}

func TestListVMs(t *testing.T) {
	root, _ := mkLayout(t, "l1")
	// quickget layout: nested name/name.conf.
	os.MkdirAll(filepath.Join(root, "l2"), 0o755)
	os.WriteFile(filepath.Join(root, "l2", "l2.conf"),
		[]byte("guest_os=\"macos\"\n"), 0o600)
	cwd, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(cwd)

	outFmt = "json"
	defer func() { outFmt = "" }()
	if rc := actListVMs(); rc != 0 {
		t.Fatalf("list rc=%d", rc)
	}
}

func TestVmdirVmname(t *testing.T) {
	_, dir := mkLayout(t, "nm")
	if vmname() != "nm" {
		t.Fatalf("vmname=%q", vmname())
	}
	if vmdir() != dir {
		t.Fatalf("vmdir=%q want %q", vmdir(), dir)
	}
	// disk_img in conf redirects vmdir.
	os.WriteFile(filepath.Join(filepath.Dir(dir), "nm.conf"),
		[]byte("disk_img=\"/x/y/z.qcow2\"\n"), 0o600)
	if vmdir() != "/x/y" {
		t.Fatalf("vmdir disk_img=%q", vmdir())
	}
}

func TestRuntimeCleanup(t *testing.T) {
	_, dir := mkLayout(t, "rc1")
	// Dead-pid layout: every runtime artifact must be reaped.
	for _, f := range []string{"rc1.pid", "rc1.paused", "rc1.ports",
		"rc1-qmp.socket", "rc1-monitor.socket"} {
		os.WriteFile(filepath.Join(dir, f), []byte("99999999"), 0o600)
	}
	runtimeCleanup()
	for _, f := range []string{"rc1.pid", "rc1.paused", "rc1.ports",
		"rc1-qmp.socket", "rc1-monitor.socket"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Fatalf("%s must be reaped", f)
		}
	}
}

func TestReadPID(t *testing.T) {
	_, dir := mkLayout(t, "p")
	if readPID() != 0 {
		t.Fatal("missing pidfile = 0")
	}
	os.WriteFile(filepath.Join(dir, "p.pid"), []byte(" 4242\n"), 0o600)
	if readPID() != 4242 {
		t.Fatalf("readPID=%d", readPID())
	}
}
