// qmp2_test.go — watch EOF paths and async-event sink coverage over
// real unix sockets.
package main

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// watchEOF: server that sends one event then closes → QMP_EOF rc=1.
func TestWatchEOF(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "w.qmp")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		defer c.Close()
		c.Write([]byte(`{"QMP":{"version":{}}}` + "\n"))
		bufio.NewReader(c).ReadString('\n') // qmp_capabilities
		c.Write([]byte(`{"event":"STOP"}` + "\n"))
	}()
	if rc := runWatch(sock); rc != 1 {
		t.Fatalf("watch eof rc=%d", rc)
	}
	if rc := runWatch(filepath.Join(t.TempDir(), "none")); rc != 1 {
		t.Fatalf("watch unavailable rc=%d", rc)
	}
}

// TestEventSinkDurable: an async event interleaved before the command
// reply lands in <vmdir>/<vm>.events.jsonl as qemu.<EVENT>, while the
// reply itself still correlates to our command id.
func TestEventSinkDurable(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "vm-qmp.socket")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		defer c.Close()
		r := bufio.NewReader(c)
		c.Write([]byte(`{"QMP":{"version":{}}}` + "\n"))
		r.ReadString('\n') // qmp_capabilities
		c.Write([]byte(`{"return":{}}` + "\n"))
		r.ReadString('\n') // the command
		c.Write([]byte(`{"event":"STOP","data":{"guest":true}}` + "\n"))
		time.Sleep(10 * time.Millisecond)
		c.Write([]byte(`{"return":{"status":"running"},"id":"qe-1"}` + "\n"))
	}()
	rc := runExecute(sock, `{"execute":"query-status"}`, 5*time.Second)
	if rc != 0 {
		t.Fatalf("execute rc=%d", rc)
	}
	deadline := time.Now().Add(2 * time.Second)
	var b []byte
	for time.Now().Before(deadline) {
		b, _ = os.ReadFile(filepath.Join(dir, "vm.events.jsonl"))
		if strings.Contains(string(b), `"qemu.STOP"`) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(string(b), `"qemu.STOP"`) {
		t.Fatalf("events.jsonl missing qemu.STOP: %s", b)
	}
}

// TestEventSinkSkipsForeignSocket: a socket not named <vm>-qmp.socket
// has no VM identity to attribute — the sink must be a no-op.
func TestEventSinkSkipsForeignSocket(t *testing.T) {
	dir := t.TempDir()
	qmpEventSink(filepath.Join(dir, "x.sock"), map[string]any{"event": "STOP"})
	if _, err := os.Stat(filepath.Join(dir, "x.events.jsonl")); err == nil {
		t.Fatal("foreign socket must not emit events")
	}
}
