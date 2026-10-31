// snapmem_test.go: QE-021 memory snapshots (savevm/loadvm/delvm via
// human-monitor-command) + QE-022 policy routing. The QMP channel
// and the quickemu-vm sidecar are scripted stubs.
package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// snapMemStubs: quickemu-qmp answers human-monitor-command by the
// embedded line (savevm/loadvm/delvm ack, "badcmd" errors), the vm
// sidecar reports one vmstate snapshot named "m1", and launch
// captures its conf for -loadvm assertions.
func snapMemStubs(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("posix stub")
	}
	d := t.TempDir()
	qmp := `#!/bin/sh
case "$*" in
  *savevm\ bad*) echo '{"data":{"return":"Error: duplicate snapshot"}}' ;;
  *savevm*) echo '{"data":{"return":""}}' ;;
  *loadvm*) echo '{"data":{"return":""}}' ;;
  *delvm*) echo '{"data":{"return":""}}' ;;
  *) echo '{"data":{"return":""}}' ;;
esac
exit 0`
	vm := `#!/bin/sh
case "$1" in
  snapshot-list-json)
    echo '{"data":{"snapshots":[{"name":"m1","vm-state-size":1024},{"name":"d1","vm-state-size":0}]}}' ;;
  *)
    echo "vm-stub $@"
    echo "$@" >> "$QE_VM_CALLS" ;;
esac
exit 0`
	launch := `#!/bin/sh
if [ -n "$QE_LAUNCH_CONF" ]; then cat "$1" > "$QE_LAUNCH_CONF" 2>/dev/null; fi
echo "launch stub $@"
exit 0`
	for n, s := range map[string]string{
		"quickemu-qmp": qmp, "quickemu-vm": vm, "quickemu-launch": launch,
	} {
		if err := os.WriteFile(filepath.Join(d, n), []byte(s), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func snapMemReset(t *testing.T) {
	t.Helper()
	oldA, oldT, oldM, oldQ, oldD, oldP, oldAct :=
		actions, snapshotTag, memFlag, quiesceFlag, snapshotDesc, policyVerb, snapshotAct
	t.Cleanup(func() {
		actions, snapshotTag, memFlag, quiesceFlag, snapshotDesc, policyVerb, snapshotAct =
			oldA, oldT, oldM, oldQ, oldD, oldP, oldAct
	})
}

func TestSnapshotMemParse(t *testing.T) {
	snapMemReset(t)
	actions, memFlag, quiesceFlag, snapshotDesc, snapshotTag = nil, false, false, "", ""
	parseArgs([]string{"--vm", "x.conf", "--snapshot", "create", "m1",
		"--memory", "--quiesce", "--snapshot-desc", "post-install"})
	if memFlag != true || quiesceFlag != true || snapshotDesc != "post-install" ||
		snapshotTag != "m1" || snapshotAct != "create" {
		t.Fatalf("flags: mem=%v quiesce=%v desc=%q tag=%q act=%q",
			memFlag, quiesceFlag, snapshotDesc, snapshotTag, snapshotAct)
	}
	actions = nil
	parseArgs([]string{"--vm", "x.conf", "--snapshot-policy", "status"})
	if len(actions) != 1 || actions[0] != "snapshot_policy" || policyVerb != "status" {
		t.Fatalf("policy: %v verb=%q", actions, policyVerb)
	}
	// Bare --snapshot-policy defaults to "run".
	actions, policyVerb = nil, ""
	parseArgs([]string{"--vm", "x.conf", "--snapshot-policy"})
	if policyVerb != "run" {
		t.Fatalf("default verb %q", policyVerb)
	}
}

func TestSnapshotMemCreateGates(t *testing.T) {
	snapMemStubs(t)
	withConf(t, "")
	snapMemReset(t)
	// Bad tag refused before any monitor traffic.
	memFlag, snapshotTag = true, "bad tag!"
	if rc := actSnapshotMemCreate(); rc != 1 {
		t.Fatalf("bad tag rc=%d", rc)
	}
	// Not running.
	snapshotTag = "m1"
	if rc := actSnapshotMemCreate(); rc != 1 {
		t.Fatalf("stopped VM rc=%d", rc)
	}
	// Non-qcow2 disk refused while running.
	withConf(t, "disk_format=\"raw\"\n")
	vd := vmdir()
	livePID(t, vd)
	if rc := actSnapshotMemCreate(); rc != 1 {
		t.Fatalf("raw disk rc=%d", rc)
	}
}

func TestSnapshotMemCreateHappyAndError(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("no /bin/sleep")
	}
	snapMemStubs(t)
	vd := withConf(t, "")
	livePID(t, vd)
	monSock(t, "-qmp.socket")
	machineTransition(stRunning, "test", "t")
	calls := filepath.Join(vd, "vm-calls.log")
	t.Setenv("QE_VM_CALLS", calls)
	snapMemReset(t)
	memFlag, snapshotTag, snapshotDesc = true, "m1", "post-install"
	if rc := actSnapshotMemCreate(); rc != 0 {
		t.Fatalf("create rc=%d", rc)
	}
	// The sidecar registers the vmstate type.
	b, _ := os.ReadFile(calls)
	if !strings.Contains(string(b), "snapshot-register") ||
		!strings.Contains(string(b), "vmstate") {
		t.Fatalf("register not called: %s", b)
	}
	// Monitor-reported error fails the op.
	snapshotTag = "bad"
	if rc := actSnapshotMemCreate(); rc != 1 {
		t.Fatalf("savevm error rc=%d", rc)
	}
}

func TestSnapshotMemApplyStopped(t *testing.T) {
	snapMemStubs(t)
	vd := withConf(t, "")
	t.Setenv("QE_VM_CALLS", filepath.Join(vd, "vm-calls.log"))
	machineTransition(stStopped, "test", "t")
	cap := filepath.Join(vd, "cap.conf")
	t.Setenv("QE_LAUNCH_CONF", cap)
	snapMemReset(t)
	snapshotTag = "m1"
	if rc := actSnapshotMemApply(); rc != 0 {
		t.Fatalf("apply rc=%d", rc)
	}
	b, err := os.ReadFile(cap)
	if err != nil || !strings.Contains(string(b), `loadvm="m1"`) {
		t.Fatalf("launch conf missing loadvm: %v %q", err, b)
	}
	if r := machineState(); r == nil || r.State != stRunning {
		t.Fatalf("state = %+v", r)
	}
}

func TestSnapshotMemApplyRunning(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("no /bin/sleep")
	}
	snapMemStubs(t)
	vd := withConf(t, "")
	livePID(t, vd)
	monSock(t, "-qmp.socket")
	machineTransition(stRunning, "test", "t")
	snapMemReset(t)
	snapshotTag = "m1"
	if rc := actSnapshotMemApply(); rc != 0 {
		t.Fatalf("apply running rc=%d", rc)
	}
}

func TestSnapshotMemDelete(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("no /bin/sleep")
	}
	snapMemStubs(t)
	vd := withConf(t, "")
	calls := filepath.Join(vd, "vm-calls.log")
	t.Setenv("QE_VM_CALLS", calls)
	livePID(t, vd)
	monSock(t, "-qmp.socket")
	machineTransition(stRunning, "test", "t")
	snapMemReset(t)
	snapshotTag = "m1"
	if rc := actSnapshotMemDelete(); rc != 0 {
		t.Fatalf("delvm rc=%d", rc)
	}
	// Stopped VM falls back to the qemu-img path.
	b, _ := os.ReadFile(calls)
	if !strings.Contains(string(b), "snapshot-unregister") {
		t.Fatalf("unregister not called: %s", b)
	}
}

func TestSnapshotIsMemAndRouting(t *testing.T) {
	snapMemStubs(t)
	withConf(t, "")
	t.Setenv("QE_VM_CALLS", filepath.Join(vmdir(), "vm-calls.log"))
	if !snapshotIsMem("m1") {
		t.Fatal("m1 must read as vmstate")
	}
	if snapshotIsMem("d1") {
		t.Fatal("d1 is disk-only")
	}
	if snapshotIsMem("missing") {
		t.Fatal("unknown tag must not route to the mem path")
	}
	// Policy routing reaches quickemu-vm with the verb.
	snapMemReset(t)
	policyVerb = "status"
	if rc := actSnapshotPolicy(); rc != 0 {
		t.Fatalf("policy rc=%d", rc)
	}
	b, _ := os.ReadFile(filepath.Join(vmdir(), "vm-calls.log"))
	if !strings.Contains(string(b), "snapshot-policy") ||
		!strings.Contains(string(b), "status") {
		t.Fatalf("policy not routed: %s", b)
	}
}

func TestHmpOutParse(t *testing.T) {
	snapMemStubs(t)
	withConf(t, "")
	monSock(t, "-qmp.socket")
	if out, err := hmpOut("savevm m1"); err != nil || out != "" {
		t.Fatalf("savevm out=%q err=%v", out, err)
	}
	if out, err := hmpOut("savevm bad"); err == nil && !strings.Contains(out, "Error") {
		t.Fatalf("bad must surface error text: %q %v", out, err)
	}
}
