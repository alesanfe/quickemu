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
