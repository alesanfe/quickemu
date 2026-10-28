// act2_test.go — second sweep over the action layer: conf parsing,
// inspect/migrate/events/plugins/stats/hostinfo/capabilities, monitor
// over a real loopback listener, snapshot dispatch, hotplug persist
// and the qeEvent/actEvents pair.
package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfKVAndHelpers(t *testing.T) {
	root, _ := mkLayout(t, "kv")
	conf := filepath.Join(root, "kv.conf")
	os.WriteFile(conf, []byte(
		"# comment\nguest_os=\"linux\"\nram='4G'\nbare=x\nnoequal\n"), 0o600)
	kv := confKV()
	if kv["guest_os"] != "linux" || kv["ram"] != "4G" || kv["bare"] != "x" {
		t.Fatalf("confKV %v", kv)
	}
	if !fileExists(conf) || fileExists(filepath.Join(root, "no")) {
		t.Fatal("fileExists")
	}
	for _, c := range []struct {
		s string
		n bool
	}{{"42", true}, {"-3", true}, {"+7", true}, {"", false}, {"-", false}, {"a1", false}} {
		if isNum(c.s) != c.n {
			t.Fatalf("isNum(%q)", c.s)
		}
	}
	if memMB("4G") <= 0 || hostFreeMemMB() < 0 || unameArch() == "" {
		t.Fatal("qelib delegation")
	}
	if uf := unameFields("-m"); uf == "" {
		t.Fatal("unameFields")
	}
	if g := glob(filepath.Join(root, "*.conf")); len(g) != 1 {
		t.Fatal("glob")
	}
	if got := jsonStr(map[string]any{"a": 1}); !strings.Contains(got, `"a":1`) {
		t.Fatal("jsonStr")
	}
	if q := qmpCmd("query-status", map[string]any{"x": 1}); !strings.Contains(q, "query-status") {
		t.Fatal("qmpCmd")
	}
}

func TestMonitorSendLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback")
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write([]byte("QEMU monitor\n(qemu) "))
		buf := make([]byte, 256)
		n, _ := c.Read(buf)
		_ = n
		c.Write([]byte("info status\nVM status: running\n(qemu) "))
	}()
	addr := ln.Addr().String()
	out, err := monitorSend("tcp", addr, "info status")
	if err != nil || !strings.Contains(out, "running") {
		t.Fatalf("monitorSend %q %v", out, err)
	}
	// Dial failure path.
	if _, err := monitorSend("tcp", "127.0.0.1:1", "x"); err == nil {
		t.Fatal("expected dial error")
	}
}

func TestInspectAndMigrate(t *testing.T) {
	root, _ := mkLayout(t, "mi")
	conf := filepath.Join(root, "mi.conf")
	os.WriteFile(conf, []byte(
		"guest_os=\"linux\"\narch=\"x86_64\"\nunknown_key=\"z\"\n"), 0o600)
	if rc := actInspectConf(); rc != 0 {
		t.Fatalf("inspect rc=%d", rc)
	}
	old := vmConf
	vmConf = filepath.Join(root, "missing.conf")
	if rc := actInspectConf(); rc != 1 {
		t.Fatalf("inspect missing rc=%d", rc)
	}
	vmConf = old
	dryRunFlag = true
	defer func() { dryRunFlag = false }()
	// No toml tool on PATH → exercises the fallback branch.
	_ = actMigrateConf()
}

func TestEventsAndQEEvent(t *testing.T) {
	_, dir := mkLayout(t, "ev")
	qeEvent("vm.test", `{"k":1}`)
	qeEvent("vm.test2", "")
	f := filepath.Join(dir, "ev.events.jsonl")
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("events file %v", err)
	}
	var seqs []int
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var e struct {
			Seq int `json:"seq"`
		}
		if json.Unmarshal([]byte(l), &e) == nil {
			seqs = append(seqs, e.Seq)
		}
	}
	if len(seqs) != 2 || seqs[0] != 1 || seqs[1] != 2 {
		t.Fatalf("seqs %v", seqs)
	}
	// actEvents replays lines > cursor.
	old := extra["events"]
	extra["events"] = "0"
	if rc := actEvents(); rc != 0 {
		t.Fatalf("events rc=%d", rc)
	}
	extra["events"] = "99" // nothing newer — still rc 0
	if rc := actEvents(); rc != 0 {
		t.Fatalf("events99 rc=%d", rc)
	}
	extra["events"] = old
}

func TestInventoryStatsCaps(t *testing.T) {
	mkLayout(t, "inv")
	if rc := actInventory(); rc != 0 {
		t.Fatalf("inventory rc=%d", rc)
	}
	old := outFmt
	outFmt = "json"
	if rc := actInventory(); rc != 0 {
		t.Fatalf("inventory json rc=%d", rc)
	}
	if rc := actStats(); rc != 1 {
		t.Fatalf("stats no-sock rc=%d", rc)
	}
	outFmt = ""
	if rc := actStats(); rc != 1 {
		t.Fatalf("stats text rc=%d", rc)
	}
	outFmt = old
	if rc := actCapabilities(); rc != 0 {
		t.Fatalf("caps rc=%d", rc)
	}
	_ = actHostInfo()
}

func TestPluginsAndSnapshotDispatch(t *testing.T) {
	mkLayout(t, "pl")
	// Stub plugin on PATH.
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "quickemu-plugin-demo"),
		[]byte("#!/bin/sh\nexit 7\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if rc := actPlugins(); rc != 0 {
		t.Fatalf("plugins rc=%d", rc)
	}
	if rc := actPlugin("demo", nil); rc != 7 {
		t.Fatalf("plugin rc=%d", rc)
	}
	if rc := actPlugin("missing", nil); rc != 1 {
		t.Fatalf("plugin missing rc=%d", rc)
	}
	old := outFmt
	outFmt = "json"
	if rc := actPlugin("missing", nil); rc != 1 {
		t.Fatalf("plugin missing json rc=%d", rc)
	}
	outFmt = old

	// Snapshot dispatch: vmOp shells to quickemu-vm — absent → nonzero,
	// but every case must be exercised. Unknown action → rc 1.
	for _, act := range []string{"tree", "verify", "info", "bogus"} {
		snapshotAct = act
		if act == "bogus" && actSnapshotCmd() != 1 {
			t.Fatalf("snapshot bogus")
		}
		_ = actSnapshotCmd()
	}
	snapshotAct = ""
}

func TestHotplugPersist(t *testing.T) {
	root, _ := mkLayout(t, "hp")
	conf := filepath.Join(root, "hp.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o600)
	hotplugPersist("usb", "0123:abcd")
	b, _ := os.ReadFile(conf)
	if !strings.Contains(string(b), `usb_devices=("0123:abcd")`) {
		t.Fatalf("usb persist %s", b)
	}
	hotplugPersist("disk", "/x.qcow2")
	b, _ = os.ReadFile(conf)
	if !strings.Contains(string(b), `extra_disks=("/x.qcow2")`) {
		t.Fatalf("disk persist %s", b)
	}
	// Append to existing array.
	hotplugPersist("usb", "ffff:ffff")
	b, _ = os.ReadFile(conf)
	if !strings.Contains(string(b), `"0123:abcd" "ffff:ffff"`) {
		t.Fatalf("usb append %s", b)
	}
	// Missing conf → silent no-op.
	vmConf = filepath.Join(root, "none.conf")
	hotplugPersist("usb", "0:0")
}

func TestEmitAndSendPaths(t *testing.T) {
	mkLayout(t, "em")
	emitOK("test.cmd", map[string]any{"a": 1})
	emitFail("test.cmd", "CODE", "msg")
	emitEnvelope("test.cmd", true, nil, nil)

	// actType/actSendKeys/actScreenshot exercise hmp→qmp→missing tool.
	old := extra["type"]
	// Contract: monitor actions on a VM that is not running must
	// report failure — hmp cannot reach a monitor that does not
	// exist, so every action returns non-zero.
	extra["type"] = "Az -"
	if rc := actType(); rc == 0 {
		t.Errorf("actType succeeded with no running VM")
	}
	extra["type"] = old
	old = extra["send-keys"]
	extra["send-keys"] = "ret"
	if rc := actSendKeys(); rc == 0 {
		t.Errorf("actSendKeys succeeded with no running VM")
	}
	extra["send-keys"] = old
	old = extra["screenshot"]
	extra["screenshot"] = ""
	if rc := actScreenshot(); rc == 0 {
		t.Errorf("actScreenshot succeeded with no running VM")
	}
	extra["screenshot"] = old
}

func TestDispatchTable(t *testing.T) {
	for _, a := range []string{"status_vm", "list_vms", "pause_vm",
		"resume_vm", "kill_vm", "stop_vm", "monitor_cmd", "send_keys",
		"type_text", "screenshot_vm", "shutdown_vm", "reset_vm",
		"delete_vm", "clone_vm", "linked_clone_vm", "rename_vm",
		"move_vm", "backup_vm", "backup_list", "trash_list",
		"snapshot_create", "snapshot_apply", "snapshot_delete",
		"snapshot_list", "snapshot_tree", "snapshot_verify",
		"snapshot_cmd", "disk_compact", "disk_resize", "delete_disk",
		"export_vm", "import_vm", "backup_verify", "backup_restore",
		"backup_prune", "trash_empty", "trash_restore", "vfio_scan",
		"autostart_vm", "shortcut_create", "delete_shortcut",
		"host_info", "capabilities", "inventory_show", "vm_stats",
		"inspect_conf", "migrate_conf", "events_read", "plugin_list",
		"vm_hotplug_usb", "vm_hotplug_disk", "vm_hotunplug",
		"operation_status", "operation_list", "operation_cancel"} {
		if fn := dispatch(a); fn == nil {
			t.Fatalf("dispatch(%q) nil", a)
		}
	}
	if dispatch("no-such-action") != nil {
		t.Fatal("unknown action must be nil")
	}
}
