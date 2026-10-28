// main_test.go — main() flag parsing via exitFn/serveAPI seams.
package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func runMain(t *testing.T, args ...string) (rc int, served bool) {
	t.Helper()
	oldExit, oldServe, oldArgs := exitFn, serveAPI, os.Args
	oldToken, oldData := token, dataDir
	exitFn = func(c int) { rc = c }
	serveAPI = func(ln net.Listener, listen string) bool {
		served = true
		ln.Close()
		return true
	}
	os.Args = append([]string{"quickemu-api"}, args...)
	t.Cleanup(func() {
		exitFn, serveAPI, os.Args = oldExit, oldServe, oldArgs
		token, dataDir = oldToken, oldData
	})
	main()
	return rc, served
}

func TestMainFlagParsing(t *testing.T) {
	// Missing flag value → rc 2.
	if rc, _ := runMain(t, "--token"); rc != 2 {
		t.Fatalf("missing value rc=%d", rc)
	}
	// Short token → rc 1.
	if rc, _ := runMain(t, "--token", "short"); rc != 1 {
		t.Fatalf("short token rc=%d", rc)
	}
	// Nonexistent token-file → rc 1.
	if rc, _ := runMain(t, "--token-file", "/nonexistent/x"); rc != 1 {
		t.Fatalf("token-file rc=%d", rc)
	}
	// Exposed bind without token and without the override → rc 1.
	if rc, _ := runMain(t, "--listen", "0.0.0.0:8490"); rc != 1 {
		t.Fatalf("exposed bind rc=%d", rc)
	}
	// Valid token file + loopback on a free port → serves.
	tf := filepath.Join(t.TempDir(), "tok")
	os.WriteFile(tf, []byte("0123456789abcdef"), 0o600)
	if rc, served := runMain(t, "--token-file", tf, "--listen", "127.0.0.1:0"); rc != 0 || !served {
		t.Fatalf("token-file serve rc=%d served=%v", rc, served)
	}
	// Exposed bind with the explicit waiver → serves.
	if rc, served := runMain(t, "--listen", "0.0.0.0:0", "--insecure-allow-exposed"); rc != 0 || !served {
		t.Fatalf("waived exposed rc=%d served=%v", rc, served)
	}
	// --data-dir sets the global.
	if rc, _ := runMain(t, "--data-dir", "/tmp/qapi-dd", "--listen", "127.0.0.1:0"); rc != 0 || dataDir != "/tmp/qapi-dd" {
		t.Fatalf("data-dir rc=%d dir=%q", rc, dataDir)
	}
	// Unix socket listen path → serves and removes the socket on close.
	if rc, served := runMain(t, "--unix", filepath.Join(t.TempDir(), "api.sock")); rc != 0 || !served {
		t.Fatalf("unix listen rc=%d served=%v", rc, served)
	}
}
