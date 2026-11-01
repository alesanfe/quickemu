// sockpath_test.go — the .s/ socket layout: bind paths stay short,
// readers resolve .s first then live legacy sockets, and SockOwner
// prefers the sidecar over the dir basename.
package qelib

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSockBindShortAndFixed(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "debian-cloud-13-genericcloud")
	for _, k := range []string{SockAgent, SockQMP, SockMonitor, SockSerial,
		SockSpice, SockVirtiofsd, SockSwtpm} {
		p := SockBind(dir, k)
		if filepath.Base(p) != k {
			t.Fatalf("SockBind(%q) = %q — basename must be the kind", k, p)
		}
		// The whole point of .s/: path length no longer carries the
		// VM name, so it fits sun_path even for deep dirs+long names.
		if len(p) > 107 {
			t.Fatalf("SockBind(%q) = %d bytes — over the 107 unix limit", k, len(p))
		}
	}
}

func TestSockPathPrefersDotS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets")
	}
	dir := t.TempDir()
	p := SockBind(dir, SockQMP)
	os.MkdirAll(filepath.Dir(p), 0o700)
	ln, err := net.Listen("unix", p)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if got := SockPath(dir, "vm", SockQMP); got != p {
		t.Fatalf("SockPath = %q, want %q", got, p)
	}
}

func TestSockPathLegacyFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets")
	}
	dir := t.TempDir()
	legacy := filepath.Join(dir, "vm-qmp.socket")
	ln, err := net.Listen("unix", legacy)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	// No .s socket — a pre-.s launch stays reachable.
	if got := SockPath(dir, "vm", SockQMP); got != legacy {
		t.Fatalf("SockPath legacy = %q, want %q", got, legacy)
	}
	// Once .s is live it wins over a lingering legacy socket.
	p := SockBind(dir, SockQMP)
	os.MkdirAll(filepath.Dir(p), 0o700)
	ln2, err := net.Listen("unix", p)
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
	if got := SockPath(dir, "vm", SockQMP); got != p {
		t.Fatalf("SockPath .s = %q, want %q", got, p)
	}
}

func TestSockPathCanonicalWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	want := SockBind(dir, SockAgent)
	if got := SockPath(dir, "vm", SockAgent); got != want {
		t.Fatalf("SockPath absent = %q, want %q", got, want)
	}
	// A legacy REGULAR file is not a socket — ignored, .s returned.
	os.WriteFile(filepath.Join(dir, "vm-agent.sock"), []byte("x"), 0o644)
	if got := SockPath(dir, "vm", SockAgent); got != want {
		t.Fatalf("SockPath regular-file = %q, want %q", got, want)
	}
}

func TestSockOwner(t *testing.T) {
	dir := t.TempDir()
	// No sidecar: falls back to the vmdir basename (per-VM dir
	// convention).
	if got := SockOwner(dir); got != filepath.Base(dir) {
		t.Fatalf("SockOwner = %q, want %q", got, filepath.Base(dir))
	}
	// The .s/vm sidecar wins when basename(dir) != vm name.
	s := filepath.Join(dir, SockSubdir)
	os.MkdirAll(s, 0o700)
	os.WriteFile(filepath.Join(s, "vm"), []byte("realvm\n"), 0o600)
	if got := SockOwner(dir); got != "realvm" {
		t.Fatalf("SockOwner sidecar = %q, want realvm", got)
	}
}
