package qespec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- spec/functional/quickgetc_entry_spec.sh ----
// Hermetic: catalog override is plain JSON at XDG_CACHE_HOME and image
// URLs resolve to a local httptest server.

type qgcEnv struct {
	dir   string
	cache string
	srv   string
}

func qgcSetup(t *testing.T) *qgcEnv {
	t.Helper()
	e := &qgcEnv{dir: t.TempDir()}
	e.cache = filepath.Join(e.dir, "cache")
	www := filepath.Join(e.dir, "www")
	for _, p := range []string{e.cache, www} {
		os.MkdirAll(p, 0o755)
	}
	for n, c := range map[string]string{
		"t1.iso": "iso1", "t2k.iso": "iso2k", "t2g.iso": "iso2g", "o9.iso": "iso9",
	} {
		Write(t, www, n, c)
	}
	e.srv = HTTPServer(t, www).URL

	Write(t, e.cache, "quickget_data.json", fmt.Sprintf(`[
 {"name":"testos","pretty_name":"TestOS","guest_os":"linux","homepage":"https://testos.example","description":"A test OS",
  "releases":[
    {"release":"1.0","edition":null,"arch":"x86_64","ram":4294967296,
     "iso":[{"web":{"url":"%s/t1.iso","file_name":"t1.iso"}}]},
    {"release":"2.0","edition":"kde","arch":"x86_64",
     "iso":[{"web":{"url":"%s/t2k.iso","file_name":"t2k.iso"}}]},
    {"release":"2.0","edition":"gnome","arch":"x86_64",
     "iso":[{"web":{"url":"%s/t2g.iso","file_name":"t2g.iso"}}]}
  ]},
 {"name":"otheros","pretty_name":"OtherOS","guest_os":"linux","homepage":"https://other.example","description":"Other",
  "releases":[{"release":"9","edition":null,"arch":"x86_64",
     "iso":[{"web":{"url":"%s/o9.iso","file_name":"o9.iso"}}]}]}
]`, e.srv, e.srv, e.srv, e.srv))

	WithFakeBin(t)
	return e
}

func (e *qgcEnv) run(t *testing.T, args ...string) Result {
	t.Helper()
	return Run(t, "quickgetc",
		[]RunOpt{InDir(e.dir), Env("XDG_CACHE_HOME=" + e.cache)},
		args...)
}

func TestQuickgetcInformational(t *testing.T) {
	e := qgcSetup(t)
	e.run(t, "--version").Contains(t, "0.1.0")
	e.run(t, "--help").Contains(t, "quickgetc <os> <release>", "--download")
	e.run(t, "--list").Contains(t, "testos", "otheros")
	e.run(t, "--list-csv").Contains(t, "Display Name,OS,Release", "TestOS,testos,1.0")
	e.run(t, "--list-json").Contains(t, `"OS": "testos"`)
	e.run(t, "--show", "testos").Contains(t, "TestOS", "https://testos.example")
	e.run(t, "--show").Contains(t, "TestOS", "OtherOS")
	if r := e.run(t, "--open-homepage", "testos"); r.Failed() {
		t.Fatalf("open-homepage rc=%d", r.Code)
	}
	r := e.run(t, "--open-homepage", "ghost-os")
	if r.OK() {
		t.Error("ghost-os homepage succeeded")
	}
	r.Contains(t, "not a supported OS")
}

func TestQuickgetcURLOps(t *testing.T) {
	e := qgcSetup(t)
	e.run(t, "--url", "testos").Contains(t,
		"/t1.iso", "/t2k.iso")
	e.run(t, "--url").Contains(t, "testos-1.0", "otheros-9")
	e.run(t, "--check", "testos").Contains(t, "testos-1.0")
}

func TestQuickgetcVMCreation(t *testing.T) {
	e := qgcSetup(t)

	e.run(t, "testos", "1.0").Contains(t, "Making testos-1.0.conf")
	conf := filepath.Join(e.dir, "testos-1.0.conf")
	b, err := os.ReadFile(conf)
	if err != nil {
		t.Fatalf("conf missing: %v", err)
	}
	for _, s := range []string{`guest_os="linux"`, "ram=4"} {
		if !strings.Contains(string(b), s) {
			t.Errorf("conf missing %q", s)
		}
	}

	if r := e.run(t, "testos", "2.0"); r.OK() {
		t.Error("missing edition accepted")
	} else {
		r.Contains(t, "must specify an edition")
	}

	e.run(t, "testos", "2.0", "kde").Contains(t, "Making testos-2.0-kde.conf")
	if _, err := os.Stat(filepath.Join(e.dir, "testos-2.0-kde.conf")); err != nil {
		t.Error("kde conf missing")
	}

	if r := e.run(t, "testos", "1.0", "kde"); r.OK() {
		t.Error("edition on uneditioned release accepted")
	} else {
		r.Contains(t, "does not require an edition")
	}
	if r := e.run(t, "testos", "2.0", "bogus"); r.OK() {
		t.Error("bad edition accepted")
	} else {
		r.Contains(t, "not a supported TestOS 2.0 edition")
	}
	if r := e.run(t, "nosuchos", "1.0"); r.OK() {
		t.Error("unknown OS accepted")
	} else {
		r.Contains(t, "not a supported OS")
	}
	if r := e.run(t, "testos", "99"); r.OK() {
		t.Error("unknown release accepted")
	} else {
		r.Contains(t, "not a supported release")
	}

	// Fresh env: the earlier cases already wrote testos-1.0.conf here.
	e2 := qgcSetup(t)
	e2.run(t, "--download", "testos", "1.0")
	if _, err := os.Stat(filepath.Join(e2.dir, "testos-1.0.conf")); err == nil {
		t.Error("--download generated a conf")
	}

	e.run(t, "--arch", "x86_64", "testos", "1.0").Contains(t, "testos-1.0-x86_64")
	if _, err := os.Stat(filepath.Join(e.dir, "testos-1.0-x86_64.conf")); err != nil {
		t.Error("arch conf missing")
	}
	if r := e.run(t, "--arch", "sparc", "testos", "1.0"); r.OK() {
		t.Error("sparc accepted")
	} else {
		r.Contains(t, "not a supported architecture")
	}
	if r := e.run(t, "--arch"); r.OK() {
		t.Error("--arch without value accepted")
	} else {
		r.Contains(t, "must specify an architecture")
	}
}

func TestQuickgetcCreateConfig(t *testing.T) {
	e := qgcSetup(t)

	Write(t, e.dir, "my-linux.iso", "iso-bytes")
	e.run(t, "--create-config", "custom",
		filepath.Join(e.dir, "my-linux.iso")).Contains(t, "Creating custom.conf")
	if b, err := os.ReadFile(filepath.Join(e.dir, "custom.conf")); err != nil ||
		!strings.Contains(string(b), `guest_os="linux"`) || !strings.Contains(string(b), "iso=") {
		t.Error("custom.conf wrong")
	}
	if _, err := os.Stat(filepath.Join(e.dir, "custom", "my-linux.iso")); err != nil {
		t.Error("iso not copied into vm dir")
	}

	Write(t, e.dir, "Win11_23H2.iso", "iso-bytes")
	r := e.run(t, "--create-config", "winvm", filepath.Join(e.dir, "Win11_23H2.iso"))
	if r.Failed() {
		t.Fatalf("winvm rc=%d", r.Code)
	}
	r.Contains(t, "guest_os=windows")
	if b, _ := os.ReadFile(filepath.Join(e.dir, "winvm.conf")); !strings.Contains(string(b), `guest_os="windows"`) {
		t.Error("winvm.conf missing windows guest_os")
	}

	Write(t, e.dir, "disk.qcow2", "qcow-bytes")
	e.run(t, "--create-config", "qcowvm",
		filepath.Join(e.dir, "disk.qcow2")).Contains(t, "Creating qcowvm.conf")
	if b, _ := os.ReadFile(filepath.Join(e.dir, "qcowvm.conf")); !strings.Contains(string(b), "disk_img=") {
		t.Error("qcow2 not mapped to disk_img")
	}

	if r := e.run(t, "--create-config", "newvm"); r.OK() {
		t.Error("missing input accepted")
	} else {
		r.Contains(t, "requires a VM name")
	}
	os.MkdirAll(filepath.Join(e.dir, "existing"), 0o755)
	Write(t, e.dir, "a.iso", "x")
	if r := e.run(t, "--create-config", "existing", filepath.Join(e.dir, "a.iso")); r.OK() {
		t.Error("existing dir accepted")
	} else {
		r.Contains(t, "Could not create directory")
	}
	Write(t, e.dir, "readme.txt", "x")
	if r := e.run(t, "--create-config", "txtvm", filepath.Join(e.dir, "readme.txt")); r.OK() {
		t.Error("txt accepted")
	} else {
		r.Contains(t, "ISO, IMG and QCOW2")
	}
	if r := e.run(t, "--create-config", "gonevm", filepath.Join(e.dir, "gone.iso")); r.OK() {
		t.Error("missing file accepted")
	} else {
		r.Contains(t, "valid URL or path")
	}
}

func TestQuickgetcArgValidation(t *testing.T) {
	e := qgcSetup(t)
	if r := e.run(t, "--bogus"); r.OK() {
		t.Error("--bogus accepted")
	} else {
		r.Contains(t, "Not supported argument")
	}
	if r := e.run(t); r.OK() {
		t.Error("no args accepted")
	} else {
		r.Contains(t, "must specify an operating system")
	}
	if r := e.run(t, "testos"); r.OK() {
		t.Error("missing release accepted")
	} else {
		r.Contains(t, "TestOS", "Releases")
	}
}

func TestQuickgetcCacheHandling(t *testing.T) {
	e := qgcSetup(t)

	// Fresh .json override wins over a stale .zst.
	zst := filepath.Join(e.cache, "quickget_data.json.zst")
	os.WriteFile(zst, []byte("not-real-zstd"), 0o644)
	os.Chtimes(zst, time.Now().Add(-72*time.Hour), time.Now().Add(-72*time.Hour))
	e.run(t, "--list-csv").Contains(t, "TestOS,testos,1.0")

	// Fresh-but-garbage .zst with no .json override: decode error path.
	os.Remove(filepath.Join(e.cache, "quickget_data.json"))
	os.WriteFile(zst, []byte("garbage-not-zstd"), 0o644)
	r := e.run(t, "--list")
	if r.OK() {
		t.Error("garbage .zst accepted")
	}
	r.Contains(t, "Failed to decompress")
}
