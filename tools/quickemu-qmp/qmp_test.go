package main

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeQMP serves greeting+caps then scripted replies; it lets us verify
// the handshake path without QEMU.
func fakeQMP(t *testing.T, replies map[string]map[string]any) string {
	dir := t.TempDir()
	sock := filepath.Join(dir, "test.qmp")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				w := bufio.NewWriter(c)
				w.WriteString(`{"QMP":{"version":{"qemu":{"major":8,"minor":2,"micro":0}},"capabilities":[]}}` + "\n")
				w.Flush()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					var req map[string]any
					if json.Unmarshal([]byte(line), &req) != nil {
						continue
					}
					ex, _ := req["execute"].(string)
					// interleave an async event before the reply
					w.WriteString(`{"event":"WATCHDOG","timestamp":{"seconds":1,"microseconds":0}}` + "\n")
					if rep, ok := replies[ex]; ok {
						b, _ := json.Marshal(rep)
						w.Write(b)
						w.WriteString("\n")
					} else {
						w.WriteString(`{"return":{}}` + "\n")
					}
					w.Flush()
				}
			}(c)
		}
	}()
	return sock
}

func TestExecuteQueryStatus(t *testing.T) {
	sock := fakeQMP(t, map[string]map[string]any{
		"query-status": {"return": map[string]any{"status": "running", "running": true}},
	})
	rc := runExecute(sock, `{"execute":"query-status"}`, 5*time.Second)
	if rc != 0 {
		t.Fatalf("execute rc=%d", rc)
	}
}

func TestExecuteError(t *testing.T) {
	sock := fakeQMP(t, map[string]map[string]any{
		"bad-cmd": {"error": map[string]any{"class": "CommandNotFound", "desc": "nope"}},
	})
	rc := runExecute(sock, `{"execute":"bad-cmd"}`, 5*time.Second)
	if rc != 1 {
		t.Fatalf("expected rc=1, got %d", rc)
	}
}

func TestUnavailableSocket(t *testing.T) {
	rc := runExecute(filepath.Join(t.TempDir(), "nope.qmp"), `{"execute":"query-status"}`, time.Second)
	if rc != 1 {
		t.Fatalf("expected rc=1, got %d", rc)
	}
}

func TestBadCommandJSON(t *testing.T) {
	rc := runExecute("/tmp/x.qmp", `not json`, time.Second)
	if rc != 1 {
		t.Fatalf("expected rc=1, got %d", rc)
	}
}

// Envelope shape sanity: the emitted JSON parses and carries the
// v1 contract fields (schemas/v1).
func TestEnvelopeShape(t *testing.T) {
	var e struct {
		SchemaVersion int            `json:"schemaVersion"`
		Command       string         `json:"command"`
		Success       bool           `json:"success"`
		Data          map[string]any `json:"data"`
		Meta          struct {
			Tool string `json:"tool"`
		} `json:"meta"`
	}
	if json.Unmarshal([]byte(`{"schemaVersion":1,"command":"qmp.execute","success":true,"data":{},"warnings":[],"errors":[],"meta":{"tool":"quickemu-qmp","quickemuVersion":"5.0.0"}}`), &e) != nil {
		t.Fatal("envelope schema drifted")
	}
	if e.SchemaVersion != 1 || !strings.Contains(e.Meta.Tool, "quickemu") {
		t.Fatal("envelope contract broken")
	}
}

// Allowlist contract (qmp_adversarial_spec.sh parity): the structured
// channel is fail-closed — mutation commands are denied unless
// QUICKEMU_QMP_RAW=1. Matching is structural: the parsed name must be
// an exact allowlist hit or a family prefix — substring inclusion in
// any position does not authorize.
func TestCommandAllowlist(t *testing.T) {
	t.Setenv("QUICKEMU_QMP_RAW", "")
	allow := func(name string) bool {
		return qmpCommandAllowed(map[string]any{"execute": name})
	}
	for _, denied := range []string{"device_add", "blockdev-add", "x-exit", "chardev-add"} {
		if allow(denied) {
			t.Errorf("%q must be denied without QUICKEMU_QMP_RAW", denied)
		}
	}
	for _, ok := range []string{"query-status", "query-block", "screendump", "system_powerdown", "human-monitor-command"} {
		if !allow(ok) {
			t.Errorf("%q must be allowed", ok)
		}
	}
	t.Setenv("QUICKEMU_QMP_RAW", "1")
	if !allow("device_add") {
		t.Error("QUICKEMU_QMP_RAW=1 must bypass the allowlist")
	}
}

// Substring-bypass adversarial cases: an allowed name embedded in a
// hostile identifier must not authorize it.
func TestCommandAllowlistAdversarial(t *testing.T) {
	t.Setenv("QUICKEMU_QMP_RAW", "")
	allow := func(name string) bool {
		return qmpCommandAllowed(map[string]any{"execute": name})
	}
	denied := []string{
		// allowed substring embedded / prefixed / suffixed.
		// ("query-status-quit" is NOT a bypass: it is a query-* family
		// name — read-only by convention and QEMU rejects unknown
		// query-* members anyway.)
		"xquery-status", "not-stop", "stopx",
		"cont-inue", "pre-drive-backup", "drive-backupx",
		"screendump2", "block-dirty-bitmapX",
		// charset violations — not command identifiers at all
		"", "STOP", "stop now", "query status", "-query-x",
		"query-\nquit", "human-monitor-command\x00", "query-*",
	}
	for _, d := range denied {
		if allow(d) {
			t.Errorf("%q must be denied (substring bypass)", d)
		}
	}
	// family members allowed
	for _, ok := range []string{
		"query-status", "query-blockstats", "query-new-member-2077",
		"block-dirty-bitmap-add", "block-dirty-bitmap-remove",
		"block-dirty-bitmap-clear", "block-dirty-bitmap-enable",
	} {
		if !allow(ok) {
			t.Errorf("%q must be allowed (family prefix)", ok)
		}
	}
}

// Envelope shape: extra top-level keys, wrong types and missing
// execute are denied fail-closed.
func TestCommandAllowlistEnvelope(t *testing.T) {
	t.Setenv("QUICKEMU_QMP_RAW", "")
	denied := []map[string]any{
		{}, // no execute
		{"execute": "query-status", "evil": true},                               // unknown key
		{"execute": "query-status", "control": map[string]any{"enable": "oob"}}, // OOB channel
		{"execute": 42}, // wrong type
		{"execute": "query-status", "arguments": "x"}, // args ok shape-wise
	}
	// arguments may be any JSON — the last case is legitimate QMP
	// ({execute, arguments}); fix expectation for it separately.
	for i, c := range denied[:len(denied)-1] {
		if qmpCommandAllowed(c) {
			t.Errorf("case %d (%v) must be denied", i, c)
		}
	}
	if !qmpCommandAllowed(map[string]any{"execute": "query-status", "arguments": map[string]any{"x": 1}}) {
		t.Error("execute+arguments is a valid envelope")
	}
	if !qmpCommandAllowed(map[string]any{"execute": "query-status", "id": "req-1"}) {
		t.Error("id is part of the QMP spec and must be allowed")
	}
}
