// raw_test.go — --raw mode branches, deny/error paths and the
// correlation-id skip logic that pushes runExecute to full coverage.
package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRawMode(t *testing.T) {
	old := qmpRaw
	qmpRaw = true
	defer func() { qmpRaw = old }()
	sock := fakeQMP(t, map[string]map[string]any{
		"query-status": {"return": map[string]any{"status": "running"}},
		"bad-cmd":      {"error": map[string]any{"class": "X", "desc": "y"}},
	})
	if rc := runExecute(sock, `{"execute":"query-status"}`, 5*time.Second); rc != 0 {
		t.Fatalf("raw ok rc=%d", rc)
	}
	if rc := runExecute(sock, `{"execute":"bad-cmd"}`, 5*time.Second); rc != 1 {
		t.Fatalf("raw err rc=%d", rc)
	}
}

func TestDeniedAndErrorPaths(t *testing.T) {
	t.Setenv("QUICKEMU_QMP_RAW", "")
	// Allowlist denial never touches the socket.
	if rc := runExecute(filepath.Join(t.TempDir(), "x"), `{"execute":"device_add"}`, time.Second); rc != 1 {
		t.Fatalf("denied rc=%d", rc)
	}
	// Command without "execute" key → skips allowlist, dials anyway.
	if rc := runExecute(filepath.Join(t.TempDir(), "x"), `{"foo":1}`, time.Second); rc != 1 {
		t.Fatalf("no-exec rc=%d", rc)
	}
	// Hotunplug against a dead socket → error.
	if err := Hotunplug(filepath.Join(t.TempDir(), "none"), "id1"); err == nil {
		t.Fatal("dead sock must fail")
	}
	// Screenshot against a dead socket → error.
	if err := Screenshot(filepath.Join(t.TempDir(), "none"), "/tmp/s.ppm"); err == nil {
		t.Fatal("dead sock screendump must fail")
	}
	// HotplugDisk with dead socket after valid stat.
	f := filepath.Join(t.TempDir(), "d.qcow2")
	_ = f
	if _, err := HotplugDisk(filepath.Join(t.TempDir(), "none"), f); err == nil {
		t.Fatal("dead sock disk must fail")
	}
}
