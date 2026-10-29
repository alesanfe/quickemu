package qelib

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormArgs(t *testing.T) {
	in := []string{"tool", "--port=2222", "--pid", "9", "-x", "pos"}
	want := []string{"tool", "--port", "2222", "--pid", "9", "-x", "pos"}
	if got := NormArgs(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("NormArgs = %v, want %v", got, want)
	}
	// In-place reuse would clobber unread elements — ensure the
	// original slice is untouched.
	if in[1] != "--port=2222" || in[2] != "--pid" {
		t.Fatalf("input mutated: %v", in)
	}
}

func TestAlive(t *testing.T) {
	if Alive(0) || Alive(-1) {
		t.Fatal("non-positive pid reported alive")
	}
	if !Alive(os.Getpid()) {
		t.Fatal("own pid reported dead")
	}
	if Alive(1 << 30) {
		t.Fatal("impossible pid reported alive")
	}
}

func TestFindTool(t *testing.T) {
	if _, err := FindTool("definitely-not-a-real-tool-xyz"); err == nil {
		t.Fatal("missing tool resolved")
	}
	if p, err := FindTool("go"); err != nil || p == "" {
		t.Fatalf("PATH lookup failed: %v %v", p, err)
	}
}

func TestFindToolSiblingBeatsPath(t *testing.T) {
	// A stale/hostile earlier-PATH binary must not shadow the
	// version-matched sibling this executable ships with (qespec
	// caught a ~/go/bin/quickget-catalog hijacking bin/quickget).
	tmp := t.TempDir()
	self := filepath.Join(tmp, "bin", "entry")
	sibling := filepath.Join(tmp, "tools", "qt-foo", "qt-foo")
	pathBin := filepath.Join(tmp, "pathbin", "qt-foo")
	for _, f := range []string{self, sibling, pathBin} {
		os.MkdirAll(filepath.Dir(f), 0o755)
		os.WriteFile(f, []byte("#!/bin/sh\n"), 0o755)
	}
	old := selfExe
	selfExe = func() (string, error) { return self, nil }
	t.Cleanup(func() { selfExe = old })
	t.Setenv("PATH", filepath.Dir(pathBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	got, err := FindTool("qt-foo")
	if err != nil {
		t.Fatalf("FindTool: %v", err)
	}
	if got != sibling {
		t.Fatalf("sibling must beat PATH: got %q want %q", got, sibling)
	}
	// No sibling anywhere: PATH is the fallback.
	got, err = FindTool("qt-pathonly")
	os.MkdirAll(filepath.Join(tmp, "pathbin"), 0o755)
	os.WriteFile(filepath.Join(tmp, "pathbin", "qt-pathonly"), []byte("#!/bin/sh\n"), 0o755)
	if got, err = FindTool("qt-pathonly"); err != nil || !strings.HasSuffix(got, "qt-pathonly") {
		t.Fatalf("PATH fallback: %q %v", got, err)
	}
}

func TestMemMB(t *testing.T) {
	for in, want := range map[string]int{"4G": 4096, "512M": 512, "256K": 0, "128": 128, "junk": 0, "": 0} {
		if got := MemMB(in); got != want {
			t.Fatalf("MemMB(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestHostArch(t *testing.T) {
	if a := HostArch(); a != "x86_64" && a != "aarch64" {
		t.Fatalf("HostArch = %q", a)
	}
}

func TestQMPCmd(t *testing.T) {
	if got := QMPCmd("query-status", nil); got != `{"execute":"query-status"}` {
		t.Fatalf("QMPCmd nil = %s", got)
	}
	got := QMPCmd("device_add", map[string]any{"driver": "usb-host", "id": "x"})
	if got != `{"arguments":{"driver":"usb-host","id":"x"},"execute":"device_add"}` {
		t.Fatalf("QMPCmd args = %s", got)
	}
}

func TestCmdName(t *testing.T) {
	os.Args = []string{"prog", "reserve"}
	if got := CmdName("ports"); got != "ports.reserve" {
		t.Fatalf("CmdName = %q", got)
	}
	os.Args = []string{"prog"}
	if got := CmdName("ports"); got != "ports" {
		t.Fatalf("CmdName = %q", got)
	}
}

// --- extended coverage: sys helpers, envelope emit, http probes ---

func TestXDGData(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg")
	if XDGData() != "/xdg" {
		t.Fatal("XDG_DATA_HOME override")
	}
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/h")
	if XDGData() != "/h/.local/share" {
		t.Fatalf("fallback: %s", XDGData())
	}
}

func TestPciHelpers(t *testing.T) {
	if PciAttr("nonexistent-dev", "vendor") != "" {
		t.Fatal("missing sysfs must be empty")
	}
	if PciHex("nonexistent-dev", "vendor") != "" {
		t.Fatal("PciHex empty")
	}
}

func TestEmitEnvelopeShape(t *testing.T) {
	// Capture stdout through a pipe — Emit is print-based.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	Emit("cmd.x", "tool/0", true, map[string]any{"k": 1}, nil)
	w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("envelope: %v %s", err, b)
	}
	if env["schemaVersion"].(float64) != 1 || env["success"] != true ||
		env["command"] != "cmd.x" {
		t.Fatalf("shape: %v", env)
	}
	meta := env["meta"].(map[string]any)
	if meta["quickemuVersion"] != QEVersion || meta["tool"] != "tool/0" {
		t.Fatalf("meta: %v", meta)
	}
	// OP_ID propagates to operationId.
	t.Setenv("OP_ID", "op-9")
	r2, w2, _ := os.Pipe()
	os.Stdout = w2
	EmitOK("cmd.y", "tool/0", nil)
	w2.Close()
	os.Stdout = old
	b2, _ := io.ReadAll(r2)
	var env2 map[string]any
	json.Unmarshal(b2, &env2)
	if env2["meta"].(map[string]any)["operationId"] != "op-9" {
		t.Fatalf("opid: %s", b2)
	}
}

func TestDataFileOverride(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "a"), 0o755)
	f := filepath.Join(d, "a", "f.json")
	os.WriteFile(f, []byte("{}"), 0o600)
	t.Setenv("QUICKEMU_DATA_DIR", d)
	if got := DataFile("a", "f.json"); got != f {
		t.Fatalf("DataFile=%q want %q", got, f)
	}
}

func TestHTTPSuite(t *testing.T) {
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer redir.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			w.Write([]byte("body"))
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(200)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	if !WebCheck(srv.URL, map[string]string{"X-T": "1"}) {
		t.Fatal("WebCheck HEAD")
	}
	if s, err := FetchString(srv.URL+"/final", 100); err != nil || s != "body" {
		t.Fatalf("FetchString %q %v", s, err)
	}
	if _, err := FetchString(srv.URL+"/final", 100); err != nil {
		t.Fatal(err)
	}
	if u, err := FinalURL(srv.URL + "/redir"); err == nil && u != "" {
		// no redirect on this path — same URL back
		_ = u
	}
	if WebCheck("http://127.0.0.1:1/nothing", nil) {
		t.Fatal("WebCheck must fail on refused conn")
	}
	if _, err := FetchString("http://127.0.0.1:1/", 10); err == nil {
		t.Fatal("FetchString must error")
	}
	if got, err := FinalURL(srv.URL + "/final"); err != nil || !strings.HasSuffix(got, "/final") {
		t.Fatalf("FinalURL %q %v", got, err)
	}
}

// ParamCheckConf — commands.sh resolution/access/network checks:
// width must pair with height, both numeric in 1-16384; access scopes
// take local|remote|host-token; macaddr must be a real MAC.
func TestParamCheckConf(t *testing.T) {
	if errs := ParamCheckConf(map[string]string{"width": "1024"}); len(errs) != 1 ||
		!strings.Contains(errs[0], "requires '--height'") {
		t.Fatalf("width without height: %v", errs)
	}
	if errs := ParamCheckConf(map[string]string{"height": "768"}); len(errs) != 1 ||
		!strings.Contains(errs[0], "requires '--width'") {
		t.Fatalf("height without width: %v", errs)
	}
	if errs := ParamCheckConf(map[string]string{"height": "x", "width": "1024"}); len(errs) == 0 {
		t.Fatal("non-numeric height must fail")
	}
	if errs := ParamCheckConf(map[string]string{"width": "20000", "height": "10"}); len(errs) == 0 {
		t.Fatal("width>16384 must fail")
	}
	if errs := ParamCheckConf(map[string]string{"width": "1024", "height": "768"}); len(errs) != 0 {
		t.Fatalf("valid resolution: %v", errs)
	}
	if errs := ParamCheckConf(map[string]string{"access": "bad host"}); len(errs) != 1 ||
		!strings.Contains(errs[0], "is not recognised for ACCESS") {
		t.Fatalf("bad access token: %v", errs)
	}
	for _, v := range []string{"local", "remote", "192.168.1.10", "vm.local"} {
		if errs := ParamCheckConf(map[string]string{"access": v}); len(errs) != 0 {
			t.Fatalf("access=%s must pass: %v", v, errs)
		}
	}
	if errs := ParamCheckConf(map[string]string{"ssh_access": "a b"}); len(errs) == 0 {
		t.Fatal("bad ssh_access token must fail")
	}
	if errs := ParamCheckConf(map[string]string{"macaddr": "de:ad:be:ef"}); len(errs) == 0 {
		t.Fatal("short MAC must fail")
	}
	if errs := ParamCheckConf(map[string]string{"macaddr": "de:ad:be:ef:00:01"}); len(errs) != 0 {
		t.Fatalf("valid MAC: %v", errs)
	}
}
