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
		{"pause", []string{"--vm", conf, "--pause"}, 0},
		{"resume", []string{"--vm", conf, "--resume"}, 0},
		{"stop", []string{"--vm", conf, "--stop"}, 0},
		{"kill", []string{"--vm", conf, "--kill"}, 0},
		{"monitor-cmd", []string{"--vm", conf, "--monitor-cmd", "info vms"}, 0},
		{"type", []string{"--vm", conf, "--type", "hi"}, 0},
		{"send-keys", []string{"--vm", conf, "--send-keys", "ret"}, 0},
		{"screenshot", []string{"--vm", conf, "--screenshot"}, 0},
		{"publicdir", []string{"--vm", conf, "--publicdir"}, 1},
		{"vnc", []string{"--vm", conf, "--vnc"}, 1},
		{"serial-console", []string{"--vm", conf, "--serial-console"}, 1},
		{"monitor-console", []string{"--vm", conf, "--monitor-console"}, 1},
		{"hotplug-usb", []string{"--vm", conf, "--hotplug-usb", "05ac:12a8"}, 0},
		{"hotunplug", []string{"--vm", conf, "--hotunplug", "x"}, 0},
		{"migrate-conf", []string{"--vm", conf, "--migrate-conf"}, 0},
		{"stats", []string{"--vm", conf, "--stats"}, 1},
		{"events", []string{"--vm", conf, "--events"}, 0},
		{"plugins", []string{"--vm", conf, "--plugins"}, 0},
		{"op-list", []string{"--vm", conf, "--op-list"}, 0},
		{"inspect-conf", []string{"--vm", conf, "--inspect-conf"}, 0},
		{"list-vms", []string{"--list-vms"}, 0},
		{"host-info", []string{"--host-info"}, 0},
		{"inventory", []string{"--inventory"}, 0},
		{"vfio-scan", []string{"--vfio-scan"}, 0},
		{"fix", []string{"--vm", conf, "--fix"}, 0},
		{"delete-plan", []string{"--vm", conf, "--delete-plan"}, 0},
		{"start", []string{"--vm", conf, "--start"}, 0},
		{"echo", []string{"--vm", conf, "--echo"}, 0},
		{"json-events", []string{"--vm", conf, "--events", "--format", "json"}, 0},
		{"clone", []string{"--vm", conf, "--clone", "c2"}, 0},
		{"rename", []string{"--vm", conf, "--rename", "r2"}, 0},
		{"backup", []string{"--vm", conf, "--backup"}, 0},
		{"export", []string{"--vm", conf, "--export"}, 0},
		{"trash-list", []string{"--vm", conf, "--trash-list"}, 0},
		{"backup-list", []string{"--vm", conf, "--backup-list"}, 0},
		{"disk-compact", []string{"--vm", conf, "--disk-compact"}, 0},
		{"ignore-msrs", []string{"--ignore-msrs-always"}, -1},
		{"display-opt", []string{"--vm", conf, "--display", "sdl", "--status"}, 0},
		{"ssh-port", []string{"--vm", conf, "--ssh-port", "2222", "--status"}, 0},
		{"shutdown", []string{"--vm", conf, "--shutdown"}, -1},
		{"reset", []string{"--vm", conf, "--reset"}, -1},
		{"restart", []string{"--vm", conf, "--restart"}, -1},
		{"status-quo", []string{"--vm", conf, "--status-quo"}, -1},
		{"delete-disk", []string{"--vm", conf, "--delete-disk"}, -1},
		{"delete-vm", []string{"--vm", conf, "--delete-vm"}, -1},
		{"move", []string{"--vm", conf, "--move", "/tmp/x"}, -1},
		{"linked-clone", []string{"--vm", conf, "--linked-clone", "lc"}, -1},
		{"autostart", []string{"--vm", conf, "--autostart"}, -1},
		{"shortcut", []string{"--vm", conf, "--shortcut"}, -1},
		{"delete-shortcut", []string{"--vm", conf, "--delete-shortcut"}, -1},
		{"trash", []string{"--vm", conf, "--trash"}, -1},
		{"trash-empty", []string{"--vm", conf, "--trash-empty"}, -1},
		{"trash-restore", []string{"--vm", conf, "--trash-restore", "m1"}, -1},
		{"import", []string{"--vm", conf, "--import", "x.tar.gz"}, -1},
		{"backup-restore", []string{"--vm", conf, "--backup-restore", "f"}, -1},
		{"backup-verify", []string{"--vm", conf, "--backup-verify", "f"}, -1},
		{"backup-prune", []string{"--vm", conf, "--backup-prune", "2"}, -1},
		{"disk-resize", []string{"--vm", conf, "--disk-resize", "20G"}, -1},
		{"snapshot-tree", []string{"--vm", conf, "--snapshot-tree"}, -1},
		{"snapshot-verify", []string{"--vm", conf, "--snapshot-verify"}, -1},
		{"op-status", []string{"--vm", conf, "--op-status", "op1"}, -1},
		{"op-watch", []string{"--vm", conf, "--op-watch", "op1"}, -1},
		{"plugin", []string{"--vm", conf, "--plugin", "p1", "a"}, -1},
		{"check", []string{"--vm", conf, "--check"}, -1},
		{"snapshot-tag", []string{"--vm", conf, "--snapshot=create", "t1"}, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, rc := runMain(t, c.args...)
			if c.rc >= 0 && rc != c.rc {
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

func TestDoctorBranches(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "iso=\"/nonexistent.iso\"\nboot=\"efi\"\nram=\"1G\"\ntpm=\"on\"\nnetwork=\"no-such-br\"\ndisk_img=\"/nonexistent.qcow2\"\ndisk_size=\"16G\"\n")
	old := doctorFix
	doctorFix = true
	defer func() { doctorFix = old }()
	if rc := actDoctor(); rc != 0 {
		t.Fatalf("doctor rc=%d", rc)
	}
	// invariant: pidfile alive but missing runtime.json → false.
	livePID(t, vd)
	os.Remove(filepath.Join(vd, vmname()+".runtime.json"))
	if vmVerifyInvariants() {
		t.Fatal("running without runtime.json should violate invariant")
	}
}

func TestStartViewerVariants(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "")
	get := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	// unix socket path from .spice file.
	os.WriteFile(filepath.Join(vd, vmname()+".spice"),
		[]byte("/tmp/spice.sock\n"), 0o644)
	startViewer(get(map[string]string{"viewer": "spicy", "display": "spice"}))
	startViewer(get(map[string]string{"viewer": "remote-viewer", "display": "spice"}))
	// port in .spice.
	os.WriteFile(filepath.Join(vd, vmname()+".spice"), []byte("5931\n"), 0o644)
	startViewer(get(map[string]string{"viewer": "spicy", "display": "spice"}))
	startViewer(get(map[string]string{"viewer": "remote-viewer", "display": "spice"}))
	// no .spice: local access socket path, remote access port, plus
	// macos/fullscreen/extra-args/shared-dir branches.
	os.Remove(filepath.Join(vd, vmname()+".spice"))
	startViewer(get(map[string]string{"viewer": "spicy", "display": "spice",
		"access": "local"}))
	startViewer(get(map[string]string{"viewer": "remote-viewer", "display": "spice",
		"access": "remote", "spice_port": "5932"}))
	startViewer(get(map[string]string{"viewer": "spicy", "display": "spice",
		"guest_os": "macos", "fullscreen": "on", "public_dir": "/tmp",
		"viewer_extra_args": "--foo bar"}))
	// viewer not on PATH → silent return.
	startViewer(get(map[string]string{"viewer": "spicy", "display": "spice"}))
}

func TestIgnoreMSRs(t *testing.T) {
	// sudo stub fails fast → deterministic failure path coverage.
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "sudo"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = actIgnoreMSRsAlways() // rc depends on host modprobe.d presence
}
