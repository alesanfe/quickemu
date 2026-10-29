// main_test.go — main.go surface: createVM/makeVMConfig over local
// sources, webGet against httptest (200/206/404), createConfig, and
// the display/testResult helpers.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"testing"
)

// helperExec re-runs the test binary so main() (and its os.Exit calls)
// execute in-process under coverage. QE_HELPER=1 is the dedicated mode
// flag; -test.run=^$ prevents the helper from re-running the suite.
func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		os.Args = append([]string{os.Args[0]},
			strings.Split(os.Getenv("QGC_HELPER_ARGS"), "\x1f")...)
		main()
		return
	}
	os.Exit(m.Run())
}

func TestMainArgs(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "cat.json")
	b, _ := json.Marshal(catFixture())
	os.WriteFile(fixture, b, 0o600)
	t.Setenv("QUICKGETC_CATALOG", fixture)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("QGC_WORK", t.TempDir())

	cases := []struct {
		name string
		args []string
		rc   int
		want string
	}{
		{"help", []string{"--help"}, 0, "quickgetc"},
		{"version", []string{"--version"}, 0, ""},
		{"list-csv", []string{"--list-csv"}, 0, "alpine"},
		{"list-json", []string{"--list-json"}, 0, `"OS"`},
		{"list", []string{"--list"}, 0, "alpine"},
		{"show-all", []string{"--show"}, 0, "Alpine"},
		{"show-os", []string{"--show", "alpine"}, 0, ""},
		{"show-missing", []string{"--show", "nope"}, 1, ""},
		{"bad-flag", []string{"--bogus"}, 1, "Not supported"},
		{"no-os", []string{}, 1, "supported"},
		{"check-all", []string{"--check"}, 0, ""},
		{"arch-missing", []string{"--arch"}, 1, "architecture"},
		{"config-short", []string{"--create-config"}, 1, ""},
		{"open-missing-arg", []string{"--open-homepage"}, 1, "OS"},
		{"pos-no-rel", []string{"alpine"}, 1, ""},
		{"pos-missing-rel", []string{"alpine", "9.9"}, 1, "not a supported release"},
		{"pos-ok", []string{"alpine", "3.19"}, 0, ""},
		{"pos-mixed-edition", []string{"alpine", "3.20"}, 0, ""},
		{"pos-unknown-os", []string{"zzz", "1"}, 1, "not a supported OS"},
		{"pos-extra-edition", []string{"alpine", "3.19", "x"}, 1, "does not require"},
		{"arch-valid", []string{"--arch", "aarch64", "--list"}, 0, "alpine"},
		{"arch-bad", []string{"--arch", "mips", "--list"}, 1, ""},
		{"arch-riscv", []string{"-a", "riscv64", "--list-csv"}, 0, "alpine"},
		{"download-pos", []string{"--download", "alpine", "3.19"}, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, _, rc := runToolEnv(t, nil, c.args...)
			if rc != c.rc {
				t.Fatalf("rc=%d want %d out=%s", rc, c.rc, out)
			}
			if c.want != "" && !strings.Contains(out, c.want) {
				t.Fatalf("out=%q missing %q", out, c.want)
			}
		})
	}
}

func runToolEnv(t *testing.T, extra []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if w := os.Getenv("QGC_WORK"); w != "" {
		cmd.Dir = w
	}
	cmd.Env = append(os.Environ(), append(extra, "QE_HELPER=1",
		"QGC_HELPER_ARGS="+strings.Join(args, "\x1f"))...)
	var out, errBuf strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	err := cmd.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	}
	return out.String(), errBuf.String(), rc
}

func TestWebGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file.iso":
			w.Write([]byte("iso-content"))
		case "/resumable":
			if strings.HasPrefix(r.Header.Get("Range"), "bytes=4-") {
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte("-rest"))
				return
			}
			w.Write([]byte("data-rest"))
		case "/missing":
			w.WriteHeader(404)
		default:
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	if rc := webGet(srv.URL+"/file.iso", dir, "f.iso", nil); rc != 0 {
		t.Fatalf("webGet rc=%d", rc)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "f.iso")); string(b) != "iso-content" {
		t.Fatal("content")
	}
	// 404 → rc 1.
	if rc := webGet(srv.URL+"/missing", dir, "m.iso", nil); rc != 1 {
		t.Fatalf("404 rc=%d", rc)
	}
	// Resume: partial file + Range → appended.
	os.WriteFile(filepath.Join(dir, "r.bin"), []byte("data"), 0o600)
	if rc := webGet(srv.URL+"/resumable", dir, "r.bin", nil); rc != 0 {
		t.Fatalf("resume rc=%d", rc)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "r.bin")); string(b) != "data-rest" {
		t.Fatalf("resume content %q", b)
	}
	// Bad URL → rc 1.
	if rc := webGet("::bad::", dir, "x", nil); rc != 1 {
		t.Fatalf("bad url rc=%d", rc)
	}
	// Hostile file_name from a foreign catalog must not escape dir —
	// the download destination is joined raw, so "../" writes outside.
	for _, name := range []string{"../evil", "a/../../evil", "/abs/evil"} {
		if rc := webGet(srv.URL+"/file.iso", dir, name, nil); rc == 0 {
			t.Fatalf("hostile file name %q must fail", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil")); err == nil {
		t.Fatal("download escaped the VM dir")
	}
}

func TestCreateVMLocal(t *testing.T) {
	dir := t.TempDir()
	e := &OSEntry{Name: "local", PrettyName: "Local", GuestOS: "linux"}
	// file_name source — no network.
	entry := &Release{Release: "1", Arch: "x86_64",
		IMG: []Source{{FileName: "local.img"}}}
	old := operation
	operation = "create"
	defer func() { operation = old }()
	vmPath := filepath.Join(dir, "localvm")
	os.MkdirAll(vmPath, 0o755)
	if rc := createVM(e, entry, vmPath); rc != 0 {
		t.Fatalf("createVM rc=%d", rc)
	}
	conf := vmPath + ".conf"
	b, err := os.ReadFile(conf)
	if err != nil || !strings.Contains(string(b), `guest_os="linux"`) ||
		!strings.Contains(string(b), "img=") {
		t.Fatalf("conf %v %s", err, b)
	}
	// Second run → conf exists → early return.
	if rc := makeVMConfig(e, entry, vmPath, map[string]string{"img": "x"}); rc != 0 {
		t.Fatalf("existing conf rc=%d", rc)
	}
	// Custom source → error.
	entry2 := &Release{Release: "1", ISO: []Source{{Custom: "x"}}}
	if rc := createVM(e, entry2, filepath.Join(dir, "v2")); rc != 1 {
		t.Fatalf("custom rc=%d", rc)
	}
}

// A foreign catalog (QUICKGETC_CATALOG) controls file_name/guest_os
// verbatim — they land inside quoted conf lines, so a value carrying
// quotes or newlines injects extra conf entries (e.g. iso='/owned').
func TestCreateVMConfInjection(t *testing.T) {
	dir := t.TempDir()
	e := &OSEntry{Name: "evil", GuestOS: "linux"}
	old := operation
	operation = "create"
	defer func() { operation = old }()

	entry := &Release{Release: "1", Arch: "x86_64",
		IMG: []Source{{FileName: "x.img'\niso='/owned"}}}
	vmPath := filepath.Join(dir, "evilvm")
	os.MkdirAll(vmPath, 0o755)
	rc := createVM(e, entry, vmPath)
	b, _ := os.ReadFile(vmPath + ".conf")
	if strings.Contains(string(b), "iso=") || strings.Count(string(b), "img=") > 1 {
		t.Fatalf("hostile file_name injected conf lines (rc=%d):\n%s", rc, b)
	}

	// guest_os from the catalog is interpolated inside quotes too.
	e2 := &OSEntry{Name: "evil2", GuestOS: "linux\"\niso=\"/owned"}
	entry2 := &Release{Release: "1", Arch: "x86_64",
		IMG: []Source{{FileName: "ok.img"}}}
	vmPath2 := filepath.Join(dir, "evilvm2")
	os.MkdirAll(vmPath2, 0o755)
	createVM(e2, entry2, vmPath2)
	b, _ = os.ReadFile(vmPath2 + ".conf")
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "iso=") {
			t.Fatalf("hostile guest_os injected conf line: %s", l)
		}
	}
}

func TestHandleWebSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("web-bytes"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	var s Source
	if err := json.Unmarshal([]byte(`{"web":{"url":"`+srv.URL+`/f.iso","file_name":"f.iso"}}`), &s); err != nil {
		t.Fatal(err)
	}
	old := operation
	operation = "download"
	defer func() { operation = old }()
	if got := handleWebSource("iso", s, dir); got == "" {
		t.Fatal("web source")
	}
	if _, err := os.Stat(filepath.Join(dir, "f.iso")); err != nil {
		t.Fatal("downloaded file")
	}
	// Bad checksum → "".
	var s2 Source
	json.Unmarshal([]byte(`{"web":{"url":"`+srv.URL+`/f.iso","file_name":"f2.iso",
	 "checksum":"`+fmt.Sprintf("%064x", 1)+`"}}`), &s2)
	if got := handleWebSource("iso", s2, dir); got != "" {
		t.Fatal("bad checksum must fail")
	}
}

func TestCreateConfig(t *testing.T) {
	dir := t.TempDir()
	if rc := createConfig("", ""); rc != 1 {
		t.Fatalf("usage rc=%d", rc)
	}
	// Local ISO → moved + conf written.
	iso := filepath.Join(dir, "windows-11.iso")
	os.WriteFile(iso, []byte("x"), 0o600)
	vm := filepath.Join(dir, "winvm")
	if rc := createConfig(vm, iso); rc != 0 {
		t.Fatalf("createConfig rc=%d", rc)
	}
	b, err := os.ReadFile(vm + ".conf")
	if err != nil || !strings.Contains(string(b), `guest_os="windows"`) {
		t.Fatalf("conf %v %s", err, b)
	}
	if _, err := os.Stat(filepath.Join(vm, "windows-11.iso")); err != nil {
		t.Fatal("iso not moved")
	}
	// Existing dir → rc 1.
	if rc := createConfig(vm, iso); rc != 1 {
		t.Fatalf("exists rc=%d", rc)
	}
	// Missing input → rc 1.
	if rc := createConfig(filepath.Join(dir, "v2"), filepath.Join(dir, "none.iso")); rc != 1 {
		t.Fatalf("missing rc=%d", rc)
	}
	// Unsupported extension → rc 1.
	bad := filepath.Join(dir, "f.txt")
	os.WriteFile(bad, []byte("x"), 0o600)
	if rc := createConfig(filepath.Join(dir, "v3"), bad); rc != 1 {
		t.Fatalf("ext rc=%d", rc)
	}
	// macOS detection via filename.
	mac := filepath.Join(dir, "macos-14.img")
	os.WriteFile(mac, []byte("x"), 0o600)
	if rc := createConfig(filepath.Join(dir, "macvm"), mac); rc != 0 {
		t.Fatalf("mac rc=%d", rc)
	}
}

func TestOpenHomepageAndTestAll(t *testing.T) {
	cat := []OSEntry{{Name: "x", Homepage: "https://x.test",
		Releases: []Release{{Release: "1"}}},
		{Name: "nohome"}}
	if rc := openHomepage(cat, "missing"); rc != 1 {
		t.Fatalf("unknown os rc=%d", rc)
	}
	if rc := openHomepage(cat, "nohome"); rc != 1 {
		t.Fatalf("no homepage rc=%d", rc)
	}
	// xdg-open exists on most desktops; either result is a covered path.
	_ = openHomepage(cat, "x")
	if rc := testAll(cat, "missing"); rc != 1 {
		t.Fatalf("testAll missing rc=%d", rc)
	}
	// show mode lists URLs without network.
	old := operation
	operation = "show"
	if rc := testAll(catFixture(), "alpine"); rc != 0 {
		t.Fatalf("testAll rc=%d", rc)
	}
	operation = old
	usage()
}

func TestCreateVMWebSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("imagedata"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte("imagedata")))
	var s Source
	json.Unmarshal([]byte(`{"web":{"url":"`+srv.URL+`/w.iso","file_name":"w.iso","checksum":"`+sum+`"}}`), &s)
	e := findOSEntry(catFixture(), "alpine")
	entry := &Release{Release: "3.20", Arch: "x86_64", ISO: []Source{s}}
	vm := filepath.Join(dir, "wvm")
	if rc := createVM(e, entry, vm); rc != 0 {
		t.Fatalf("createVM web rc=%d", rc)
	}
	b, err := os.ReadFile(filepath.Join(vm, "w.iso"))
	if err != nil || string(b) != "imagedata" {
		t.Fatalf("downloaded %v %q", err, b)
	}
	if _, err := os.Stat(vm + ".conf"); err != nil {
		t.Fatal("conf not written")
	}
	// download-only mode: fetch succeeds, no conf written.
	old := operation
	operation = "download"
	vm2 := filepath.Join(dir, "dvm")
	if rc := createVM(e, entry, vm2); rc != 0 {
		t.Fatalf("download-mode rc=%d", rc)
	}
	operation = old
	if _, err := os.Stat(vm2 + ".conf"); err == nil {
		t.Fatal("conf written in download mode")
	}
	// Bad checksum → handleWebSource fails → rc 1.
	bad := s
	bad.Web.Checksum = fmt.Sprintf("%x", sha256.Sum256([]byte("other")))
	entry2 := &Release{Release: "3.20", ISO: []Source{bad}}
	if rc := createVM(e, entry2, filepath.Join(dir, "v2")); rc != 1 {
		t.Fatalf("checksum rc=%d", rc)
	}
	// Custom source → rc 1.
	entry3 := &Release{Release: "3.20", ISO: []Source{{Custom: "x"}}}
	if rc := createVM(e, entry3, filepath.Join(dir, "v3")); rc != 1 {
		t.Fatalf("custom rc=%d", rc)
	}
}

func TestPopulateCacheDownload(t *testing.T) {
	b, _ := json.Marshal(catFixture())
	var buf strings.Builder
	zw, _ := zstd.NewWriter(&buf)
	zw.Write(b)
	zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(buf.String()))
	}))
	defer srv.Close()
	dir := t.TempDir()
	t.Setenv("QUICKGETC_CATALOG", "")
	t.Setenv("XDG_CACHE_HOME", dir)
	old := cacheURL
	cacheURL = srv.URL + "/q.zst"
	defer func() { cacheURL = old }()
	cat := populateCache()
	if len(cat) != 2 || cat[0].Name != "alpine" {
		t.Fatalf("populateCache %v", cat)
	}
	// Second call hits the fresh on-disk cache.
	if len(populateCache()) != 2 {
		t.Fatal("cached read failed")
	}
}

func TestDisplayHelpers(t *testing.T) {
	e := &OSEntry{Name: "x", PrettyName: "X OS", Homepage: "https://x",
		Description: "desc", Releases: []Release{{Release: "1"}}}
	showOSInfo(e)
	testResult("x", "1", "", "https://u", "PASS")
	listJSON(catFixture())
	_ = hostArch()
	if s := setArch("x86_64"); s != "x86_64" {
		t.Fatal("setArch")
	}
}
