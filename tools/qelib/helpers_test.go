// helpers_test.go — the helpers deduplicated into qelib from the
// consumer tools: CopyFile/MoveFile, QuickgetArch, XDG dirs,
// UnameFields, the events writer (AppendEvent/EventsMaxBytes), the
// Apple osrecovery surface (MacGenID/AppleSession/AppleInfo),
// DataFile sibling lookup and the Fail exit path.
package qelib

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuickgetArch(t *testing.T) {
	if QuickgetArch("arm64") != "arm64" {
		t.Fatal("arm64 must pass through")
	}
	for _, a := range []string{"amd64", "x86_64", "aarch64", ""} {
		if got := QuickgetArch(a); got != "amd64" {
			t.Fatalf("QuickgetArch(%q)=%q", a, got)
		}
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.bin")
	os.WriteFile(src, []byte("payload"), 0o600)
	dst := filepath.Join(dir, "b.bin")
	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "payload" {
		t.Fatalf("copied %q", b)
	}
	if err := CopyFile(src+"-missing", dst); err == nil {
		t.Fatal("missing src must fail")
	}
	if err := CopyFile(src, filepath.Join(dir, "no", "b")); err == nil {
		t.Fatal("unwritable dst must fail")
	}
}

func TestMoveFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.bin")
	os.WriteFile(src, []byte("payload"), 0o600)
	dst := filepath.Join(dir, "b.bin")
	if err := MoveFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("src must be gone after move")
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "payload" {
		t.Fatalf("moved %q", b)
	}
	if err := MoveFile(src, dst); err == nil {
		t.Fatal("missing src must fail")
	}
}

func TestXDGDirs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xd")
	t.Setenv("XDG_CACHE_HOME", "/xc")
	t.Setenv("XDG_CONFIG_HOME", "/xg")
	if XDGData() != "/xd" || XDGCache() != "/xc" || XDGConfig() != "/xg" {
		t.Fatalf("env must win: %s %s %s", XDGData(), XDGCache(), XDGConfig())
	}
}

func TestXDGDirsFallback(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/h")
	if XDGData() != filepath.Join("/h", ".local", "share") {
		t.Fatalf("data %s", XDGData())
	}
	if XDGCache() != filepath.Join("/h", ".cache") {
		t.Fatalf("cache %s", XDGCache())
	}
	if XDGConfig() != filepath.Join("/h", ".config") {
		t.Fatalf("config %s", XDGConfig())
	}
}

func TestUnameFields(t *testing.T) {
	fs := strings.Fields(UnameFields("-s -r -m"))
	if len(fs) != 3 {
		t.Fatalf("want 3 fields, got %q", fs)
	}
	if fs[0] != "Linux" {
		t.Fatalf("sysname %q", fs[0])
	}
	if got := UnameFields(""); got != "" {
		t.Fatalf("no flags %q", got)
	}
}

func TestHostArchValue(t *testing.T) {
	if a := HostArch(); a != "x86_64" && a != "aarch64" {
		t.Fatalf("arch %q", a)
	}
}

func TestHostFreeMemMBValue(t *testing.T) {
	if HostFreeMemMB() < 0 {
		t.Fatal("free mem must be >= 0")
	}
}

func TestPciAttrMiss(t *testing.T) {
	if PciAttr("no-such-dev", "vendor") != "" {
		t.Fatal("unreadable attr must be empty")
	}
	if PciHex("no-such-dev", "vendor") != "" {
		t.Fatal("unreadable hex attr must be empty")
	}
}

func TestEventsMaxBytes(t *testing.T) {
	t.Setenv("QE_EVENTS_MAX_BYTES", "")
	if EventsMaxBytes() != 1048576 {
		t.Fatalf("default %d", EventsMaxBytes())
	}
	t.Setenv("QE_EVENTS_MAX_BYTES", "123")
	if EventsMaxBytes() != 123 {
		t.Fatalf("env %d", EventsMaxBytes())
	}
}

func TestAppendEvent(t *testing.T) {
	dir := t.TempDir()
	AppendEvent("", "vm", 1, "x", "") // no-op, must not panic
	AppendEvent(dir, "vm1", 1234, "vm.started", `{"k":1}`)
	f := filepath.Join(dir, "vm1.events.jsonl")
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["seq"].(float64) != 1 || rec["event"] != "vm.started" || rec["vm"] != "vm1" ||
		rec["pid"].(float64) != 1234 {
		t.Fatalf("record %v", rec)
	}
	if rec["data"].(map[string]any)["k"].(float64) != 1 {
		t.Fatalf("data %v", rec["data"])
	}
	// Second append continues the sequence; pid<=0 and empty data → null.
	AppendEvent(dir, "vm1", 0, "vm.stopped", "")
	b, _ = os.ReadFile(f)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines %v", lines)
	}
	var rec2 map[string]any
	json.Unmarshal([]byte(lines[1]), &rec2)
	if rec2["seq"].(float64) != 2 || rec2["pid"] != nil || rec2["data"] != nil {
		t.Fatalf("record2 %v", rec2)
	}
	// Rotation: cap of 1 byte forces the rename to <file>.1.
	t.Setenv("QE_EVENTS_MAX_BYTES", "1")
	AppendEvent(dir, "vm1", 1, "vm.rotate", "")
	if _, err := os.Stat(f + ".1"); err != nil {
		t.Fatal("rotation must leave a .1 generation")
	}
}

// CryptSalt is the deduplicated salt source for the two former
// copies in quickget-vm and quicktest — same SHA512-crypt charset.
func TestCryptSalt(t *testing.T) {
	s := CryptSalt(8)
	if len(s) != 8 {
		t.Fatalf("len %d", len(s))
	}
	for _, c := range s {
		if !strings.ContainsRune(
			"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./", c) {
			t.Fatalf("non-crypt char %c in %q", c, s)
		}
	}
	a, b := CryptSalt(24), CryptSalt(24)
	if a == b {
		t.Fatal("salts must be random")
	}
	if CryptSalt(0) != "" {
		t.Fatal("empty salt")
	}
}

func TestMacGenID(t *testing.T) {
	s := MacGenID(16)
	if len(s) != 16 {
		t.Fatalf("len %d", len(s))
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789ABCDEF", c) {
			t.Fatalf("non-hex char %c in %q", c, s)
		}
	}
	a, b := MacGenID(32), MacGenID(32)
	if a == b {
		t.Fatal("ids must be random")
	}
}

// rtFunc intercepts the osrecovery.apple.com calls so the handshake is
// exercised offline — the *http.Client is the injected seam.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func rtResp(r *http.Request, h http.Header, body string) *http.Response {
	return &http.Response{
		StatusCode: 200, Header: h, Request: r,
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestAppleSession(t *testing.T) {
	cli := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "osrecovery.apple.com" || r.Method != "GET" {
			t.Fatalf("req %s %s", r.Method, r.URL)
		}
		return rtResp(r, http.Header{
			"Set-Cookie": {"session=SESS123; Path=/"}}, ""), nil
	})}
	got, err := AppleSession(cli)
	if err != nil || got != "SESS123" {
		t.Fatalf("session %q %v", got, err)
	}
	// Cookie-less reply: no session to scrape.
	cli = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return rtResp(r, http.Header{}, ""), nil
	})}
	if _, err := AppleSession(cli); err == nil {
		t.Fatal("no session cookie must fail")
	}
	// Transport failure propagates.
	cli = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("net down")
	})}
	if _, err := AppleSession(cli); err == nil {
		t.Fatal("transport error must propagate")
	}
}

func TestAppleInfo(t *testing.T) {
	var body string
	cli := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/InstallationPayload/RecoveryImage" || r.Method != "POST" {
			t.Fatalf("req %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Cookie") != `session="SESS"` {
			t.Fatalf("cookie %q", r.Header.Get("Cookie"))
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		return rtResp(r, http.Header{}, "token http://cdn/img.dmg extra"), nil
	})}
	fields, err := AppleInfo(cli, "SESS", "MLB001", "Mac-TEST", "latest")
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 || fields[0] != "token" || fields[2] != "extra" {
		t.Fatalf("fields %v", fields)
	}
	for _, want := range []string{"sn=MLB001", "bid=Mac-TEST", "os=latest", "cid=", "k=", "fg="} {
		if !strings.Contains(body, want) {
			t.Fatalf("body %q missing %s", body, want)
		}
	}
	cli = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("net down")
	})}
	if _, err := AppleInfo(cli, "s", "m", "b", "d"); err == nil {
		t.Fatal("transport error must propagate")
	}
}

func TestDataFileEnv(t *testing.T) {
	t.Setenv("QUICKEMU_DATA_DIR", "/droot")
	if got := DataFile("catalog", "data.json"); got != filepath.Join("/droot", "catalog", "data.json") {
		t.Fatalf("env path %s", got)
	}
}

// The sibling search walks up from os.Executable() checking
// <base>/share/quickemu/<rel>: create the file next to the test
// binary so the found-branch is exercised.
func TestDataFileSibling(t *testing.T) {
	t.Setenv("QUICKEMU_DATA_DIR", "")
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no test exe")
	}
	share := filepath.Join(filepath.Dir(exe), "share", "quickemu")
	os.MkdirAll(share, 0o755)
	defer os.RemoveAll(filepath.Join(filepath.Dir(exe), "share"))
	want := filepath.Join(share, "probe.txt")
	os.WriteFile(want, []byte("x"), 0o600)
	if got := DataFile("probe.txt"); got != want {
		t.Fatalf("sibling %s want %s", got, want)
	}
	// Miss falls back to the dev-tree guess (still a valid path).
	if got := DataFile("no-such", "file.txt"); got == "" {
		t.Fatal("miss must still return a path")
	}
}

// Fail prints the v1 error envelope and exits 1 — exercised via the
// classic self-reexec helper-process pattern.
func TestFailExit(t *testing.T) {
	if os.Getenv("QE_HELPER_FAIL") == "1" {
		Fail("testcmd", "qelib", "E_BROKEN", "boom")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestFailExit")
	cmd.Env = append(os.Environ(), "QE_HELPER_FAIL=1")
	out, err := cmd.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 {
		t.Fatalf("rc=%v out=%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &env); err != nil {
		t.Fatalf("out %s", out)
	}
	if env["success"] != false || env["command"] != "testcmd" {
		t.Fatalf("envelope %s", out)
	}
	errs := env["errors"].([]any)
	if errs[0].(map[string]any)["code"] != "E_BROKEN" {
		t.Fatalf("errors %v", errs)
	}
}
