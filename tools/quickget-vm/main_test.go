// main_test.go — helper-exec harness covering the mkconf/get/fetch/
// customconf subcommand dispatch.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestServer(payload []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
}

func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		os.Args = append([]string{"quickget-vm"},
			strings.Split(os.Getenv("QV_ARGS"), "\x1f")...)
		main()
		return
	}
	os.Exit(m.Run())
}

func runVM(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QV_ARGS="+strings.Join(args, "\x1f"))
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	}
	return out.String() + errb.String(), rc
}

func TestChunkcheckSubcommand(t *testing.T) {
	// Missing arg -> usage rc2; empty dir -> rc1 (files absent).
	if _, rc := runVM(t, "chunkcheck"); rc != 2 {
		t.Fatalf("missing vmdir rc=%d want 2", rc)
	}
	empty := t.TempDir()
	if _, rc := runVM(t, "chunkcheck", empty); rc != 1 {
		t.Fatalf("empty dir rc=%d want 1", rc)
	}
	// Valid CNKL pair (signature_method=2) -> rc0 "Verification passed".
	dir := t.TempDir()
	dmg := []byte("chunkcheck fixture payload")
	h := sha256.New()
	hdr := make([]byte, 36)
	copy(hdr[:4], "CNKL")
	binary.LittleEndian.PutUint32(hdr[4:8], 36)
	hdr[8], hdr[9], hdr[10] = 1, 1, 2
	binary.LittleEndian.PutUint64(hdr[12:20], 1)
	binary.LittleEndian.PutUint64(hdr[20:28], 36)
	binary.LittleEndian.PutUint64(hdr[28:36], 72)
	h.Write(hdr)
	var rec [36]byte
	binary.LittleEndian.PutUint32(rec[:4], uint32(len(dmg)))
	sum := sha256.Sum256(dmg)
	copy(rec[4:36], sum[:])
	h.Write(rec[:])
	cl := append(append(hdr, rec[:]...), h.Sum(nil)...)
	os.WriteFile(filepath.Join(dir, "RecoveryImage.dmg"), dmg, 0o644)
	os.WriteFile(filepath.Join(dir, "RecoveryImage.chunklist"), cl, 0o644)
	out, rc := runVM(t, "chunkcheck", dir)
	if rc != 0 || !strings.Contains(out, "Verification passed") {
		t.Fatalf("valid pair rc=%d out=%q", rc, out)
	}
	// QuickGUI passes the configured iso= path — the .dmg file
	// itself — which must resolve to its containing vmdir.
	out, rc = runVM(t, "chunkcheck", filepath.Join(dir, "RecoveryImage.dmg"))
	if rc != 0 || !strings.Contains(out, "Verification passed") {
		t.Fatalf("dmg-path arg rc=%d out=%q", rc, out)
	}
}

func TestMainSubcommands(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "x.iso")
	os.WriteFile(img, []byte("x"), 0o644)

	// real HTTP — the URL crosses into the helper process via args.
	payload := []byte("img-bytes")
	srv := newTestServer(payload)
	defer srv.Close()

	// quickget-catalog stub for the `get` path.
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-catalog"),
		[]byte("#!/bin/sh\necho '"+srv.URL+"/img.iso'\n"), 0o755)
	os.WriteFile(filepath.Join(stub, "zsync"),
		[]byte("#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in -o) shift; touch \"$1\";; esac; shift; done\nexit 0\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))

	cases := []struct {
		name string
		args []string
		rc   int
	}{
		{"none", []string{}, 2},
		{"bad-sub", []string{"nope"}, 2},
		{"fetch-noargs", []string{"fetch"}, 2},
		{"fetch-ok", []string{"fetch", "--url", srv.URL + "/f.iso",
			"--dest", filepath.Join(dir, "f.iso")}, 0},
		{"get-noargs", []string{"get"}, 2},
		{"get", []string{"get", "testos", "1.0", "--dir",
			filepath.Join(dir, "g")}, 0},
		{"get-arch", []string{"get", "testos", "1.0", "--arch", "arm64",
			"--dir", filepath.Join(dir, "g2"), "--no-conf"}, 0},
		{"customconf-noargs", []string{"customconf"}, -1},
		{"customconf", []string{"customconf",
			filepath.Join(dir, "vm1"), img}, 0},
		{"mkconf-noargs", []string{"mkconf"}, 2},
		{"mkconf", []string{"mkconf", "--os", "reactos", "--release", "0.4",
			"--image", "reactos-0.4.iso", "--dir", filepath.Join(dir, "mk")}, 0},
		{"mkconf-ci", []string{"mkconf", "--os", "ubuntu", "--release", "24.04",
			"--image", "u.iso", "--dir", filepath.Join(dir, "mk2"),
			"--cloud-init"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, rc := runVM(t, c.args...)
			if c.rc >= 0 && rc != c.rc {
				t.Fatalf("rc=%d want %d out=%s", rc, c.rc, out)
			}
		})
	}
}

func TestStallWatchReader(t *testing.T) {
	var last atomic.Int64
	last.Store(1) // epoch — stale
	src := &stallWatchReader{r: strings.NewReader("payload"), last: &last}
	buf := make([]byte, 16)
	n, err := src.Read(buf)
	if n <= 0 || err != nil {
		t.Fatalf("read n=%d err=%v", n, err)
	}
	if time.Since(time.Unix(0, last.Load())) > time.Second {
		t.Fatal("reader must ping lastRead on every chunk")
	}
	// EOF must not refresh the timestamp (a closed-but-not-stalled
	// body would keep the watchdog quiet).
	last.Store(1)
	src.Read(buf)
	if last.Load() != 1 {
		t.Fatal("EOF/0-byte read must not refresh the stall timer")
	}
}
