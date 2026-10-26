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
// contract fields.
func TestEnvelopeShape(t *testing.T) {
	var e envelope
	if json.Unmarshal([]byte(`{"schemaVersion":1,"command":"qmp.execute","success":true,"data":{},"warnings":[],"errors":[],"meta":{"backend":"quickemu-qmp","backendVersion":"0.1.0"}}`), &e) != nil {
		t.Fatal("envelope schema drifted")
	}
	if e.SchemaVersion != 1 || !strings.Contains(e.Meta.Backend, "quickemu") {
		t.Fatal("envelope contract broken")
	}
}
