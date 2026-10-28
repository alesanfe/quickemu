// main_test.go: run() in-process via re-exec — real tar.gz fixtures are
// built per test so the whitelist, checksum and manifest rules are
// exercised against actual bytes, not mocks.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		a := os.Getenv("QE_HELPER_ARGS")
		os.Args = []string{"quickemu-archive"}
		if a != "" {
			os.Args = append(os.Args, strings.Split(a, "\x1f")...)
		}
		rc := run()
		_ = os.WriteFile(os.Getenv("QE_HELPER_RC"),
			[]byte(strconv.Itoa(rc)), 0o600)
		return
	}
	os.Exit(m.Run())
}

func runTool(t *testing.T, args ...string) (string, int) {
	t.Helper()
	rcf := filepath.Join(t.TempDir(), "rc")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QE_HELPER_ARGS="+strings.Join(args, "\x1f"),
		"QE_HELPER_RC="+rcf)
	var so bytes.Buffer
	cmd.Stdout = &so
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper: %v", err)
	}
	rc := -1
	if b, err := os.ReadFile(rcf); err == nil {
		rc, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	return so.String(), rc
}

type member struct {
	name    string
	body    []byte
	typeflg byte
}

func makeArchive(t *testing.T, ms []member) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "x.quickvm.tar.gz")
	w, _ := os.Create(f)
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, m := range ms {
		tf := m.typeflg
		if tf == 0 {
			tf = tar.TypeReg
		}
		if err := tw.WriteHeader(&tar.Header{
			Name: m.name, Mode: 0o600, Size: int64(len(m.body)),
			Typeflag: tf}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(m.body); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	w.Close()
	return f
}

func manifestJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"schemaVersion": 1, "vm": "v", "createdAt": "t",
		"members": []string{"a"}})
	return b
}

func TestVerifyOK(t *testing.T) {
	body := []byte("payload")
	sum := sha256.Sum256(body)
	sums := hex.EncodeToString(sum[:]) + "  f\n"
	arc := makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"quickvm-checksums.sha256", []byte(sums), 0},
		{"f", body, 0},
	})
	out, rc := runTool(t, "verify", "--archive", arc)
	if rc != 0 {
		t.Fatalf("rc=%d out=%s", rc, out)
	}
	var env map[string]any
	json.Unmarshal([]byte(strings.TrimSpace(out)), &env)
	if env["success"] != true {
		t.Fatalf("%s", out)
	}
}

func TestExtractWritesFiles(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	arc := makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"dir/disk.qcow2", []byte("img"), 0},
	})
	_, rc := runTool(t, "extract", "--archive", arc, "--dest", dest)
	if rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "dir/disk.qcow2")); err != nil || string(b) != "img" {
		t.Fatalf("extracted=%v %v", b, err)
	}
}

func TestRejections(t *testing.T) {
	cases := []struct {
		name string
		ms   []member
	}{
		{"slip", []member{
			{"quickvm-manifest.json", manifestJSON(), 0},
			{"../evil", []byte("x"), 0}}},
		{"abs", []member{
			{"quickvm-manifest.json", manifestJSON(), 0},
			{"/abs", []byte("x"), 0}}},
		{"link", []member{
			{"quickvm-manifest.json", manifestJSON(), 0},
			{"ln", []byte(""), tar.TypeSymlink}}},
		{"nomanifest", []member{
			{"f", []byte("x"), 0}}},
	}
	for _, c := range cases {
		arc := makeArchive(t, c.ms)
		if _, rc := runTool(t, "verify", "--archive", arc); rc != 1 {
			t.Fatalf("%s: rc=%d want 1", c.name, rc)
		}
	}
}

func TestChecksumMismatchAndDup(t *testing.T) {
	bad := makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"quickvm-checksums.sha256", []byte(strings.Repeat("0", 64) + "  f\n"), 0},
		{"f", []byte("real"), 0},
	})
	if _, rc := runTool(t, "verify", "--archive", bad); rc != 1 {
		t.Fatal("mismatch must fail")
	}
	dup := makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"d/f", []byte("a"), 0},
		{"d/f", []byte("a"), 0},
	})
	if _, rc := runTool(t, "verify", "--archive", dup); rc != 1 {
		t.Fatal("duplicate member must fail")
	}
}

func TestBadArgsAndMissing(t *testing.T) {
	if _, rc := runTool(t); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
	if _, rc := runTool(t, "verify", "--archive", "/nonexistent"); rc != 1 {
		t.Fatalf("rc")
	}
	bad := filepath.Join(t.TempDir(), "x.tgz")
	os.WriteFile(bad, []byte("not gzip"), 0o600)
	if _, rc := runTool(t, "verify", "--archive", bad); rc != 1 {
		t.Fatalf("not-gzip rc")
	}
}
