// act4_test.go — doctor/stats/snapshot/migrate/hotplug/main coverage.
package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		os.Args = append([]string{"quickemu"},
			strings.Split(os.Getenv("QC_ARGS"), "\x1f")...)
		main()
		return
	}
	os.Exit(m.Run())
}

func runMain(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QC_ARGS="+strings.Join(args, "\x1f"))
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	}
	return out.String() + errb.String(), rc
}

func TestMainDispatch(t *testing.T) {
	stubTools(t)
	dir := t.TempDir()
	conf := filepath.Join(dir, "m1.conf")
	os.WriteFile(conf, []byte("viewer=\"none\"\n"), 0o644)
	cases := []struct {
		name string
		args []string
		rc   int
	}{
		{"version", []string{"--version"}, 0},
		{"help", []string{"--help"}, 0},
		{"capabilities", []string{"--capabilities"}, 0},
		{"status", []string{"--vm", conf, "--status"}, 0},
		{"status-json", []string{"--vm", conf, "--status", "--format", "json"}, 0},
		{"doctor", []string{"--vm", conf, "--doctor"}, 0},
		{"dry-run", []string{"--vm", conf, "--dry-run"}, 0},
		{"snapshot-info", []string{"--vm", conf, "--snapshot=info"}, 0},
		{"snapshot-bad", []string{"--vm", conf, "--snapshot=nope"}, 1},
		{"bad-flag", []string{"--bogus"}, 1},
		{"no-args", []string{}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, rc := runMain(t, c.args...)
			if rc != c.rc {
				t.Fatalf("rc=%d want %d out=%s", rc, c.rc, out)
			}
		})
	}
}

func TestDoctorFixtures(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "")
	name := vmname()
	// corrupt runtime.json (no schemaVersion key)
	os.WriteFile(filepath.Join(vd, name+".runtime.json"), []byte("{}"), 0o644)
	// stale pidfile (impossible pid)
	os.WriteFile(filepath.Join(vd, name+".pid"), []byte("999999999"), 0o644)
	// stale lock dir holding dead owner pid
	lk := filepath.Join(vd, name+".lock")
	os.MkdirAll(lk, 0o755)
	os.WriteFile(filepath.Join(lk, "pid"), []byte("999999999"), 0o644)
	// stale socket: real unix socket with no live pid
	sock := filepath.Join(vd, name+".socket")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("no unix sockets: " + err.Error())
	}
	defer l.Close()
	// truncated events log
	os.WriteFile(filepath.Join(vd, name+".events.jsonl"),
		[]byte("{\"a\":1}\n{\"b\":2"), 0o644)
	// orphan op: state=running with dead workerPid
	os.MkdirAll(filepath.Join(vd, "ops"), 0o755)
	os.WriteFile(filepath.Join(vd, "ops", "o1.json"),
		[]byte(`{"state":"running","workerPid":999999999}`), 0o644)

	old := outFmt
	outFmt = "json"
	defer func() { outFmt = old }()
	qeDoctor(false) // report only — findings survive
	// fix pass actually repairs.
	qeDoctor(true)
	if _, err := os.Stat(filepath.Join(vd, name+".pid")); !os.IsNotExist(err) {
		t.Fatal("stale pidfile not removed")
	}
	if _, err := os.Stat(lk); !os.IsNotExist(err) {
		t.Fatal("stale lock not reclaimed")
	}
	if b, _ := os.ReadFile(filepath.Join(vd, "ops", "o1.json")); !strings.Contains(string(b), "interrupted") {
		t.Fatal("orphan op not marked interrupted")
	}
	outFmt = "text"
	qeDoctor(true)
}

func TestStatsAndSnapshots(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "")
	// actStats needs the qmp socket to exist (content irrelevant).
	sock := qmpSock()
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("no unix sockets")
	}
	defer l.Close()
	old := outFmt
	outFmt = "json"
	defer func() { outFmt = old }()
	if rc := actStats(); rc != 0 {
		t.Fatalf("stats rc=%d", rc)
	}
	outFmt = "text"
	if rc := actStats(); rc != 0 {
		t.Fatalf("stats text rc=%d", rc)
	}
	l.Close()
	os.Remove(sock)
	if rc := actStats(); rc != 1 {
		t.Fatal("stats without qmp should fail")
	}

	// snapshot dispatch — stub quickemu-vm answers 0 for everything.
	d := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	os.WriteFile(filepath.Join(d, "quickemu-vm"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	for _, act := range []string{"info", "list", "tree", "verify", "create",
		"apply", "delete", "consolidate", "bogus"} {
		snapshotAct = act
		externalFlag = act == "create"
		rc := actSnapshotCmd()
		if act == "bogus" {
			if rc != 1 {
				t.Fatal("bogus snapshot act should fail")
			}
		} else if rc != 0 {
			t.Fatalf("snapshot %s rc=%d", act, rc)
		}
	}
	snapshotAct = ""
	_ = vd
}

func TestMigrateConfAndHotplug(t *testing.T) {
	stubTools(t)
	withConf(t, "guest_os=\"linux\"\nram=\"2G\"\n")
	// stub quickemu-toml emitting TOML.
	d := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	os.WriteFile(filepath.Join(d, "quickemu-toml"),
		[]byte("#!/bin/sh\necho 'guest_os = \"linux\"'\n"), 0o755)

	old := dryRunFlag
	dryRunFlag = true
	if rc := actMigrateConf(); rc != 0 {
		t.Fatalf("migrate dry-run rc=%d", rc)
	}
	dryRunFlag = false
	if rc := actMigrateConf(); rc != 0 {
		t.Fatalf("migrate rc=%d", rc)
	}
	// second run → toml exists → fail.
	if rc := actMigrateConf(); rc != 1 {
		t.Fatal("existing toml should fail")
	}
	dryRunFlag = old

	// hotplug: extra map + stubbed qmp → success paths.
	for k, v := range map[string]string{
		"hotplug-usb": "05ac:12a8", "hotunplug": "hotusb05ac12a8"} {
		extra[k] = v
	}
	persistFlag = true
	if rc := actHotplugUSB(); rc != 0 {
		t.Fatalf("hotplug-usb rc=%d", rc)
	}
	extra["hotplug-usb"] = "zz:zz"
	if rc := actHotplugUSB(); rc != 1 {
		t.Fatal("bad usb id should fail")
	}
	f := filepath.Join(t.TempDir(), "d.qcow2")
	os.WriteFile(f, []byte("x"), 0o644)
	extra["hotplug-disk"] = f
	if rc := actHotplugDisk(); rc != 0 {
		t.Fatalf("hotplug-disk rc=%d", rc)
	}
	extra["hotplug-disk"] = "/nonexistent.iso"
	if rc := actHotplugDisk(); rc != 1 {
		t.Fatal("missing disk should fail")
	}
	if rc := actHotunplug(); rc != 0 {
		t.Fatalf("hotunplug rc=%d", rc)
	}
	persistFlag = false

	// opsOp/qmpOp via stubs.
	if rc := opsOp("list")(); rc != 0 {
		t.Fatalf("opsOp rc=%d", rc)
	}
	if rc := qmpOp(func() string { return "query-status" },
		func() string { return "ok" })(); rc != 0 {
		t.Fatalf("qmpOp rc=%d", rc)
	}
	// monitorSendBest with no socket → hmp stub.
	monitorSendBest("sendkey ret")
}

func TestIgnoreMSRs(t *testing.T) {
	// sudo stub fails fast → deterministic failure path coverage.
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "sudo"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = actIgnoreMSRsAlways() // rc depends on host modprobe.d presence
}
