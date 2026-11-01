// contract_emit_test.go: v1-envelope contract regressions — every
// structured call must produce exactly ONE envelope whose command
// name the QuickGUI normalizer can resolve. Text on stdout or a
// concatenated second envelope reads as MALFORMED_JSON/EMPTY_RESPONSE
// on the GUI side and triggers a legacy double-run.
package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envOut decodes a single emitted envelope — asserts stdout is one
// JSON object (never text lines or concatenated envelopes).
func envOut(t *testing.T, fn func()) map[string]any {
	t.Helper()
	out := captureOut(t, fn)
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("stdout is not a single envelope: %v\nout=%q", err, out)
	}
	return env
}

func withJSON(t *testing.T) {
	t.Helper()
	old := outFmt
	outFmt = "json"
	t.Cleanup(func() { outFmt = old })
}

func TestEmitPauseNotRunningJSON(t *testing.T) {
	stubTools(t)
	withConf(t, "")
	withJSON(t)
	var rc int
	env := envOut(t, func() { rc = actPause() })
	if rc != 1 || env["command"] != "vm.paused" || env["success"] != false {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
	errs, _ := env["errors"].([]any)
	if len(errs) == 0 || !strings.Contains(errs[0].(map[string]any)["code"].(string), "not_running") {
		t.Fatalf("want not_running error: %v", env)
	}
}

func TestEmitStopNotRunningJSON(t *testing.T) {
	stubTools(t)
	withConf(t, "")
	withJSON(t)
	var rc int
	env := envOut(t, func() { rc = actStop(true) })
	// Idempotent stop: not-running is still a successful 'stopped'.
	if rc != 0 || env["command"] != "vm.stopped" || env["success"] != true {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
	if env["data"].(map[string]any)["alreadyStopped"] != true {
		t.Fatalf("want alreadyStopped: %v", env)
	}
}

func TestEmitStopKilledJSON(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "")
	livePID(t, vd)
	withJSON(t)
	var rc int
	env := envOut(t, func() { rc = actStop(true) })
	if rc != 0 || env["command"] != "vm.stopped" || env["success"] != true {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
	if env["data"].(map[string]any)["forced"] != true {
		t.Fatalf("want forced kill data: %v", env)
	}
}

func TestEmitMonitorCmdJSON(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "")
	withJSON(t)
	old := extra["monitor-cmd"]
	defer func() { extra["monitor-cmd"] = old }()
	extra["monitor-cmd"] = "info vms"
	ln, err := net.Listen("unix", filepath.Join(vd, vmname()+"-monitor.socket"))
	if err != nil {
		t.Skip("no unix sockets: " + err.Error())
	}
	defer ln.Close()
	go func() {
		for c, err := ln.Accept(); err == nil; c, err = ln.Accept() {
			c.Close()
		}
	}()
	var rc int
	env := envOut(t, func() { rc = actMonitorCmd() })
	if rc != 0 || env["command"] != "vm.monitor-cmd" || env["success"] != true {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
	if env["data"].(map[string]any)["request"] != "info vms" {
		t.Fatalf("want request echo in data: %v", env)
	}
}

func TestEmitSendKeysJSON(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "")
	livePID(t, vd)
	withJSON(t)
	old := extra["send-keys"]
	defer func() { extra["send-keys"] = old }()
	extra["send-keys"] = "ctrl-alt-delete"
	// No monitor socket → HMP path warns but the send is best-effort —
	// the contract only needs the single envelope.
	var rc int
	env := envOut(t, func() { rc = actSendKeys() })
	if env["command"] != "vm.send-keys" {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
}

func TestCompositeEmitName(t *testing.T) {
	cases := map[string]string{
		"stop_vm+start":          "vm.restart",
		"backup_vm+shutdown_vm":  "vm.backup",
		"delete_vm+trash":        "vm.delete-vm",
		"snapshot_create+status": "vm.snapshot-create",
	}
	for join, want := range cases {
		got := compositeEmitName(strings.Split(join, "+"))
		if got != want {
			t.Fatalf("%s → %s want %s", join, got, want)
		}
	}
}

func TestRunActionsCompositeJSON(t *testing.T) {
	stubTools(t)
	withConf(t, "")
	withJSON(t)
	old := actions
	actions = []string{"stop_vm", "start"}
	defer func() { actions = old }()
	var rc int
	env := envOut(t, func() { rc = runActions() })
	// One envelope for the whole composite — never stopped+started pair.
	if env["command"] != "vm.restart" {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
}

func TestEmitDryRunJSON(t *testing.T) {
	stubTools(t)
	withConf(t, `guest_os="linux"`)
	withJSON(t)
	var rc int
	env := envOut(t, func() { rc = actDryRun() })
	if rc != 0 || env["command"] != "vm.plan" || env["success"] != true {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
	if env["data"].(map[string]any)["command"] == nil {
		t.Fatal("plan must carry the assembled command")
	}
}

func TestEmitStartAlreadyRunningJSON(t *testing.T) {
	stubTools(t)
	vd := withConf(t, "")
	livePID(t, vd)
	withJSON(t)
	var rc int
	env := envOut(t, func() { rc = actLaunch() })
	if rc != 0 || env["command"] != "vm.started" || env["success"] != true {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
	if env["data"].(map[string]any)["alreadyRunning"] != true {
		t.Fatalf("want alreadyRunning flag: %v", env)
	}
}

func TestEmitTrashBareJSON(t *testing.T) {
	stubTools(t)
	withConf(t, `guest_os="linux"`)
	withJSON(t)
	old := trashFlag
	trashFlag = true
	t.Cleanup(func() { trashFlag = old })
	var rc int
	env := envOut(t, func() { rc = actTrashVm() })
	if rc != 0 || env["command"] != "vm.trash" || env["success"] != true {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
	if env["data"].(map[string]any)["trashed"] != true {
		t.Fatalf("want trashed flag: %v", env)
	}
}

func TestAllowedOpsCanonicalNames(t *testing.T) {
	have := map[string]bool{}
	for _, o := range allowedOps("running") {
		have[o] = true
	}
	for _, want := range []string{"stop", "pause", "reset", "screenshot",
		"send-keys", "monitor-cmd", "suspend", "shutdown", "kill"} {
		if !have[want] {
			t.Fatalf("running allowedOps missing %q", want)
		}
	}
	stopped := map[string]bool{}
	for _, o := range allowedOps("stopped") {
		stopped[o] = true
	}
	for _, want := range []string{"start", "trash", "delete-vm",
		"snapshot-create", "snapshot-apply", "disk-resize", "backup"} {
		if !stopped[want] {
			t.Fatalf("stopped allowedOps missing %q", want)
		}
	}
	// No stale bare aliases may leak — the GUI gates on canonical names.
	for _, o := range append(allowedOps("running"), allowedOps("stopped")...) {
		switch o {
		case "create", "apply", "delete", "info", "list", "snapshot",
			"snapshot-clone", "resize":
			t.Fatalf("stale bare op name %q", o)
		}
	}
}

func TestDiskResizeCommitFlag(t *testing.T) {
	for _, f := range []string{"--resize-commit", "--disk-resize-commit"} {
		old := actions
		actions = nil
		p := &argParser{args: []string{f}}
		if !parseVMOpFlags(p, f) {
			t.Fatalf("%s not parsed", f)
		}
		if len(actions) != 1 || actions[0] != "disk_resize_commit" {
			t.Fatalf("%s -> %v", f, actions)
		}
		actions = old
	}
}

func TestPowerdownFlagAndAction(t *testing.T) {
	old := actions
	actions = nil
	p := &argParser{args: []string{"--powerdown"}}
	if !parseLifecycleFlags(p, "--powerdown") || actions[0] != "powerdown_vm" {
		t.Fatalf("--powerdown -> %v", actions)
	}
	actions = old
	if dispatch("powerdown_vm") == nil {
		t.Fatal("powerdown_vm has no dispatch")
	}
}

func TestEmitStatusQuoBareJSON(t *testing.T) {
	withConf(t, "guest_os=\"linux\"\n")
	withJSON(t)
	oldQ, oldO := statusQuoFlag, confOpts
	statusQuoFlag = true
	confOpts = map[string]string{"snapshot": "on", "ram": "4G"}
	t.Cleanup(func() { statusQuoFlag, confOpts = oldQ, oldO })
	var rc int
	env := envOut(t, func() { rc = actStatusQuo() })
	if rc != 0 || env["command"] != "vm.status-quo" || env["success"] != true {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
	b, _ := os.ReadFile(vmConf)
	if !strings.Contains(string(b), `snapshot="on"`) ||
		!strings.Contains(string(b), `ram="4G"`) {
		t.Fatalf("conf not persisted: %q", b)
	}
}
