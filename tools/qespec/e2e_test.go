package qespec

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ============ e2e specs ported from spec/e2e/ ============
// e2e_lib.sh equivalents: Sandbox=TempDir, VMFixture, WaitFor, VMState,
// SpawnFakeVM, UnixDial (replaces socat).

// ---- e2e/compatibility/entrypoint_parity_spec.sh ----

func TestEntrypointOpsParity(t *testing.T) {
	d := t.TempDir()
	conf := VMFixture(t, d)

	cli := Run(t, "quickemu", nil, "--vm", conf, "--op-status", "no-such-op", "--format", "json")
	goR := Run(t, "quickemu-ops", nil, "status", "--vmdir", d, "no-such-op")
	// Empty output is itself a contract break — fail, don't skip:
	// a backend that emits nothing produces a false pass otherwise.
	if cli.Stdout == "" || goR.Stdout == "" {
		t.Fatalf("empty envelope — cli %d bytes, ops %d bytes",
			len(cli.Stdout), len(goR.Stdout))
	}
	for name, r := range map[string]Result{"cli": cli, "ops": goR} {
		m := r.JSON(t)
		if v, _ := m["schemaVersion"].(float64); v != 1 {
			t.Errorf("%s envelope schemaVersion=%v", name, m["schemaVersion"])
		}
		if _, ok := m["success"].(bool); !ok {
			t.Errorf("%s envelope success not bool", name)
		}
		if _, ok := m["errors"].([]any); !ok {
			t.Errorf("%s envelope errors not array", name)
		}
	}
}

// ---- e2e/contracts/cli_json_spec.sh ----

func TestE2EJSONEnvelope(t *testing.T) {
	d := t.TempDir()
	conf := VMFixture(t, d)
	for _, args := range [][]string{
		{"--vm", conf, "--status", "--format", "json"},
		{"--vm", conf, "--backup-list", "--format", "json"},
		{"--vm", conf, "--host-info", "--format", "json"},
		{"--capabilities", "--format", "json"},
	} {
		r := Run(t, "quickemu", nil, args...)
		r.Contains(t, `"schemaVersion":1`, `"success":`)
	}
	Run(t, "quickemu", nil, "--vm", conf, "--status", "--format", "json").
		Contains(t, "allowedOps")
}

// ---- e2e/download/resume_spec.sh ----

func TestDownloadResumeContract(t *testing.T) {
	src := SourceFile(t, "tools/quickget-vm/main.go")
	for _, p := range []string{"Range", "bytes=%d-", "O_APPEND", "StatusPartialContent"} {
		if !strings.Contains(src, p) {
			t.Errorf("resume contract: %q absent", p)
		}
	}
}

// ---- e2e/recovery/stale_lock_spec.sh ----

func TestStaleLockRecovered(t *testing.T) {
	d := t.TempDir()
	conf := VMFixture(t, d)
	lock := filepath.Join(d, "vm.lock")
	os.MkdirAll(lock, 0o755)
	os.WriteFile(filepath.Join(lock, "pid"), []byte("999999"), 0o644)
	if got := VMState(t, conf); got != "stopped" {
		t.Fatalf("stale lock blocked status: %q", got)
	}
}

// ---- e2e/security/path_confinement_spec.sh ----

func TestDeleteNeverEscapesVMDir(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "vm", "allowed"), 0o755)
	conf := Write(t, d, "vm.conf",
		"guest_os=\"linux\"\ndisk_img=\"vm/disk.qcow2\"\nram=\"256M\"\n")
	Write(t, filepath.Join(d, "vm"), "disk.qcow2", "x")
	Write(t, d, "must-survive.txt", "canary")

	Run(t, "quickemu", nil, "--vm", conf, "--delete-vm")
	if st, err := os.Stat(filepath.Join(d, "vm")); err == nil && st.IsDir() {
		t.Error("vm dir still present after delete")
	}
	if b, err := os.ReadFile(filepath.Join(d, "must-survive.txt")); err != nil || string(b) != "canary" {
		t.Error("canary deleted — confinement breach")
	}
}

// ---- e2e/import_export/roundtrip_spec.sh ----

func TestImportExportRoundtrip(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "src")
	os.MkdirAll(src, 0o755)
	conf := Write(t, src, "vm.conf",
		"guest_os=\"linux\"\ndisk_img=\"disk.qcow2\"\nram=\"256M\"\ndisplay=\"none\"\nboot=\"legacy\"\n")
	Write(t, src, "disk.qcow2", "x")
	arc := filepath.Join(d, "vm.tar.gz")

	if r := Run(t, "quickemu", nil, "--vm", conf, "--export", arc); r.Failed() {
		t.Fatalf("export rc=%d\n%s", r.Code, r.Combined())
	}
	if st, err := os.Stat(arc); err != nil || st.Size() == 0 {
		t.Fatal("archive empty")
	}
	dst := filepath.Join(d, "dst")
	os.MkdirAll(dst, 0o755)
	if r := Run(t, "quickemu", []RunOpt{InDir(dst)}, "--import", arc); r.Failed() {
		t.Fatalf("import rc=%d\n%s", r.Code, r.Combined())
	}
	found := false
	filepath.Walk(dst, func(p string, i os.FileInfo, e error) error {
		if e == nil && !i.IsDir() && strings.HasSuffix(p, ".conf") {
			found = true
		}
		return nil
	})
	if !found {
		t.Error("no .conf after import")
	}
}

// ---- tools/e2e-contract.sh (QuickGUI CLI sequence) ----
// capabilities → status → delete-plan → planId → guarded delete →
// op-status NOT_FOUND → events JSONL → inventory.

func TestE2EContractGUISequence(t *testing.T) {
	d := t.TempDir()

	env := Run(t, "quickemu", nil, "--capabilities", "--format", "json").Envelope(t)
	if env["command"] != "capabilities" {
		t.Fatalf("command=%v", env["command"])
	}

	conf := Write(t, d, "e2e.conf",
		"guest_os=\"linux\"\ndisk_img=\"disk.qcow2\"\nram=\"256M\"\n")
	env = Run(t, "quickemu", []RunOpt{InDir(d)}, "--vm", conf, "--status", "--format", "json").Envelope(t)
	if env["command"] != "vm.status" {
		t.Fatalf("command=%v", env["command"])
	}

	r := Run(t, "quickemu", []RunOpt{InDir(d)}, "--vm", conf, "--delete-plan", "--format", "json")
	m := r.Envelope(t)
	if m["command"] != "vm.delete-plan" {
		t.Fatalf("command=%v", m["command"])
	}
	plan, _ := m["data"].(map[string]any)["planId"].(string)
	if plan != "" {
		r = Run(t, "quickemu", []RunOpt{InDir(d)}, "--vm", conf,
			"--delete-vm", "--plan-id", plan, "--trash", "--format", "json")
		r.Envelope(t)
	}

	r = Run(t, "quickemu", []RunOpt{InDir(d)}, "--vm", conf, "--op-status", "nope-123", "--format", "json")
	if r.OK() {
		t.Error("op-status on missing op succeeded")
	}
	if !strings.Contains(r.Stdout, "OPERATION_NOT_FOUND") && !strings.Contains(r.Stdout, `"success":false`) {
		t.Errorf("expected OPERATION_NOT_FOUND: %s", r.Stdout)
	}

	// events: JSONL lines or empty body — every line must be a valid object.
	r = Run(t, "quickemu", []RunOpt{InDir(d)}, "--vm", conf, "--events", "0", "--format", "json")
	for _, l := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		if l == "" {
			continue
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(l), &v); err != nil {
			t.Errorf("events line not JSON: %q", l)
		}
	}

	r = Run(t, "quickemu", []RunOpt{InDir(d)}, "--vm", conf, "--inventory", "--format", "json")
	r.JSON(t) // must at least parse
}

// ---- e2e/uat_scenarios_spec.sh ----

func TestUATScenarios(t *testing.T) {
	Run(t, "quickget", nil, "--list").Contains(t, "ubuntu")
	if r := Run(t, "quickget", nil, "--download", "notarealos", "1.0"); r.OK() {
		t.Error("uat-02 fake OS accepted")
	} else {
		r.Contains(t, "not a supported OS")
	}
	if r := Run(t, "quickget", nil, "--download", "debian", "9999"); r.OK() {
		t.Error("uat-03 fake release accepted")
	} else {
		r.Contains(t, "not a supported")
		r.ErrContains(t, "integrity")
	}
	Run(t, "quickemu", nil, "--version").Contains(t, ".")
	Run(t, "quickemu", nil, "--capabilities", "--format", "json").
		Contains(t, `"schemaVersion":1`)
	if r := Run(t, "quickget", nil, "--download", "--minimum-trust", "Z", "debian", "13"); r.OK() {
		t.Error("uat-06 trust Z accepted")
	} else {
		r.ErrContains(t, "invalid trust level")
	}
	if r := Run(t, "quickget", nil, "--check", "totallyfake", "1.0"); r.OK() {
		t.Error("uat-07 fake OS check succeeded")
	} else {
		r.Contains(t, "not a supported OS")
	}
}

// ---- e2e/contracts/ops_flags_spec.sh ----

func TestOpsFlagsContract(t *testing.T) {
	d := t.TempDir()
	conf := VMFixture(t, d)

	Run(t, "quickemu", nil, "--vm", conf, "--snapshot-verify").
		Contains(t, "Snapshot chain verified")
	Run(t, "quickemu", nil, "--vm", conf, "--trash-empty").
		Contains(t, "Trash is empty")
	if r := Run(t, "quickemu", nil, "--vm", conf, "--plugins"); r.Failed() {
		t.Errorf("--plugins rc=%d", r.Code)
	}
	if r := Run(t, "quickemu", nil, "--vm", conf, "--operation-status", "op-999", "--format", "json"); r.OK() {
		t.Error("op-status on missing op succeeded")
	} else {
		r.Contains(t, "OPERATION_NOT_FOUND")
	}
	if r := Run(t, "quickemu", nil, "--vm", conf, "--operation-cancel", "op-999", "--format", "json"); r.OK() {
		t.Error("op-cancel on missing op succeeded")
	} else {
		r.Contains(t, "OPERATION_NOT_FOUND")
	}
	if r := Run(t, "quickemu", []RunOpt{Timeout(15 * time.Second)},
		"--vm", conf, "--plugin", "bogus-plugin"); r.OK() {
		t.Error("bogus plugin accepted")
	} else {
		r.ErrContains(t, "not found on PATH")
	}
	if r := Run(t, "quickemu", nil, "--vm", conf, "--backup-verify", "/nonexistent.tar.zst"); r.OK() {
		t.Error("backup-verify on missing archive succeeded")
	} else {
		r.ErrContains(t, "not found")
	}
	if r := Run(t, "quickemu", nil, "--vm", conf, "--hotunplug", "usb1"); r.OK() {
		t.Error("hotunplug on stopped VM succeeded")
	} else {
		if !strings.Contains(r.Combined(), "QMP") {
			t.Errorf("hotunplug error lacks QMP: %s", r.Combined())
		}
	}
	Run(t, "quickemu", nil, "--vm", conf, "--echo").Contains(t, "qemu-system")
}

func TestDoctorFixRepairs(t *testing.T) {
	d := t.TempDir()
	conf := VMFixture(t, d)
	vmDir := filepath.Dir(conf)
	os.MkdirAll(filepath.Join(vmDir, "ops"), 0o755)
	os.MkdirAll(filepath.Join(vmDir, "vm.lock"), 0o755)
	os.WriteFile(filepath.Join(vmDir, "vm.runtime.json"), []byte("garbage"), 0o644)
	os.WriteFile(filepath.Join(vmDir, "vm.lock", "pid"), []byte("999999"), 0o644)
	Write(t, filepath.Join(vmDir, "ops"), "op-1.json",
		`{"state":"running","workerPid":999999}`)

	r := Run(t, "quickemu", nil, "--vm", conf, "--doctor", "--fix")
	r.Contains(t, "fixed:")
	for _, gone := range []string{"vm.runtime.json", "vm.lock"} {
		if _, err := os.Stat(filepath.Join(vmDir, gone)); err == nil {
			t.Errorf("%s survived doctor --fix", gone)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(vmDir, "ops", "op-1.json")); !strings.Contains(string(b), "interrupted") {
		t.Error("op record not marked interrupted")
	}
}

func TestSnapshotExternalChain(t *testing.T) {
	d := t.TempDir()
	conf := VMFixture(t, d)
	if r := Run(t, "quickemu", nil, "--vm", conf, "--snapshot", "create", "e1", "--external"); r.Failed() {
		t.Fatalf("snapshot create rc=%d\n%s", r.Code, r.Combined())
	}
	if _, err := os.Stat(filepath.Join(d, "vm-e1.qcow2")); err != nil {
		t.Error("overlay vm-e1.qcow2 missing")
	}
	Run(t, "quickemu", nil, "--vm", conf, "--snapshot-tree").Contains(t, "e1")
	if r := Run(t, "quickemu", nil, "--vm", conf, "--snapshot-verify"); r.Failed() {
		t.Error("verify failed after external snapshot")
	}
}

func TestHotplugDiskPersist(t *testing.T) {
	d := t.TempDir()
	conf := VMFixture(t, d)
	hot := filepath.Join(d, "hot.qcow2")
	if QemuImgAvailable() {
		exec.Command("qemu-img", "create", "-f", "qcow2", hot, "16M").Run()
	} else {
		os.WriteFile(hot, []byte{0}, 0o644)
	}
	Run(t, "quickemu", nil, "--vm", conf, "--hotplug-disk", hot, "--persist")
	if b, _ := os.ReadFile(conf); !strings.Contains(string(b), "extra_disks") {
		t.Error("extra_disks not persisted to conf")
	}
}

// ---- e2e/qemu_boot_spec.sh: raw QEMU + QMP over unix socket ----
// socat replaced by net.Dial("unix").

func TestRawQemuBootQMP(t *testing.T) {
	if !QemuAvailable() {
		t.Skip("qemu-system-x86_64 unavailable")
	}
	d := t.TempDir()
	sock := filepath.Join(d, "vm-qmp.socket")
	cmd := exec.Command("qemu-system-x86_64",
		"-m", "128", "-machine", "type=pc,accel=tcg",
		"-nographic", "-serial", "none", "-display", "none",
		"-qmp", "unix:"+sock+",server,nowait",
		"-pidfile", filepath.Join(d, "vm.pid"))
	if err := cmd.Start(); err != nil {
		t.Skipf("qemu start: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	WaitFor(t, 10*time.Second, func() bool { return SocketExists(sock) },
		"QMP socket never appeared")

	reply := UnixDial(t, sock,
		"{\"execute\":\"qmp_capabilities\"}\n{\"execute\":\"query-status\"}\n",
		3*time.Second)
	if !strings.Contains(reply, `"return"`) {
		t.Fatalf("QMP handshake failed: %s", reply)
	}
	if !strings.Contains(reply, `"status": "running"`) && !strings.Contains(reply, `"status":"running"`) {
		t.Fatalf("VM not running: %s", reply)
	}

	b, err := os.ReadFile(filepath.Join(d, "vm.pid"))
	if err != nil {
		t.Fatal("pidfile missing")
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if !PidAlive(pid) {
		t.Fatal("pidfile pid not alive")
	}
	cmd.Process.Signal(syscall.SIGTERM)
	// Reap the child ourselves — a zombie still answers kill(pid,0).
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("QEMU did not exit on SIGTERM")
	}
}

// ---- e2e/qmp/qmp_socket_spec.sh + lifecycle/start_stop_spec.sh ----
// Boot via the real CLI (TCG), verify socket/status, kill cleanly.

func quickemuBoot(t *testing.T, conf string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(Bin(t, "quickemu"), "--vm", conf)
	cmd.Env = append(os.Environ(), "QEMU_ACCEL=tcg", "CPU_MODEL=qemu64")
	if err := cmd.Start(); err != nil {
		t.Fatalf("quickemu start: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		Run(t, "quickemu", nil, "--vm", conf, "--kill")
	})
	return cmd
}

func TestQMPSocketAppears(t *testing.T) {
	if !QemuAvailable() {
		t.Skip("qemu-system-x86_64 unavailable")
	}
	d := t.TempDir()
	conf := VMFixture(t, d)
	sock := filepath.Join(d, "vm-qmp.socket")
	quickemuBoot(t, conf)
	WaitFor(t, 90*time.Second, func() bool { return SocketExists(sock) },
		"QMP socket never appeared")
	Run(t, "quickemu", nil, "--vm", conf, "--kill")
}

func TestLifecycleStartStop(t *testing.T) {
	if !QemuAvailable() {
		t.Skip("qemu-system-x86_64 unavailable")
	}
	d := t.TempDir()
	conf := VMFixture(t, d)
	quickemuBoot(t, conf)
	WaitFor(t, 90*time.Second, func() bool { return VMState(t, conf) == "running" },
		"VM never reached running")
	// The boot CLI may still hold the VM lock; retry --kill until the
	// state flips instead of assuming the first attempt landed.
	var kr Result
	WaitFor(t, 20*time.Second, func() bool {
		if VMState(t, conf) == "running" {
			kr = Run(t, "quickemu", nil, "--vm", conf, "--kill")
			return false
		}
		return true
	}, "VM did not stop; last --kill: "+kr.Combined())
	if out, _ := filepath.Glob(filepath.Join(d, "*.sock*")); len(out) > 0 {
		t.Errorf("residual sockets: %v", out)
	}
}

// ---- e2e/go_cli_boot_spec.sh: full lifecycle through the Go CLI ----

func TestGoCLIBootCycle(t *testing.T) {
	if !QemuAvailable() || !QemuImgAvailable() {
		t.Skip("qemu/qemu-img unavailable")
	}
	d := t.TempDir()
	vm := filepath.Join(d, "govm")
	os.MkdirAll(vm, 0o755)
	conf := Write(t, vm, "govm.conf",
		"guest_os=\"linux\"\ndisk_img=\"disk.qcow2\"\ndisplay=\"none\"\nram=\"128M\"\n")
	exec.Command("qemu-img", "create", "-f", "qcow2",
		filepath.Join(vm, "disk.qcow2"), "32M").Run()
	os.WriteFile(filepath.Join(vm, "stub.iso"), []byte("x"), 0o644)

	qmpSock := filepath.Join(vm, "govm-qmp.socket")
	quickemuBoot(t, conf)
	WaitFor(t, 90*time.Second, func() bool { return SocketExists(qmpSock) },
		"no QMP socket")
	WaitFor(t, 10*time.Second, func() bool { return VMState(t, conf) == "running" },
		"status not running")

	// pause -> paused marker; resume -> running.
	WaitFor(t, 10*time.Second, func() bool {
		Run(t, "quickemu", nil, "--vm", conf, "--pause")
		_, err := os.Stat(filepath.Join(vm, "govm.paused"))
		return err == nil
	}, "pause failed")
	if got := VMState(t, conf); got != "paused" {
		t.Fatalf("status not paused: %s", got)
	}
	Run(t, "quickemu", nil, "--vm", conf, "--resume")
	WaitFor(t, 10*time.Second, func() bool { return VMState(t, conf) == "running" },
		"status not running after resume")

	// Arbitrary HMP command via the raw monitor socket.
	Run(t, "quickemu", nil, "--vm", conf, "--monitor-cmd", "info status").
		Contains(t, "VM status: running")

	// QMP allowlist: query-* ok, quit denied without RAW.
	qmp := Bin(t, "quickemu-qmp")
	RunArgv(t, qmp, nil, "execute", "--socket", qmpSock,
		"--command", `{"execute":"query-status"}`).Contains(t, `"success":true`)
	RunArgv(t, qmp, nil, "execute", "--socket", qmpSock,
		"--command", `{"execute":"quit"}`).Contains(t, "QMP_DENIED")

	// Clean stop.
	Run(t, "quickemu", nil, "--vm", conf, "--stop")
	WaitFor(t, 10*time.Second, func() bool {
		b, err := os.ReadFile(filepath.Join(vm, "govm.pid"))
		if err != nil {
			return true
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		return !PidAlive(pid)
	}, "VM did not stop")
}

var _ = fmt.Sprint
