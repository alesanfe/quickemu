// qe090_test.go: the hostile-archive corpus — absolute paths, ..
// traversal, symlink escapes, member-count bombs, disk-full,
// no-clobber imports, the recovery journal and v2 manifests.
// Seam-level tests call runExtraction in-process (stubs never reach
// a re-exec'd helper); wire-level tests go through runTool.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
)

// link member — the shared member struct has no linkname field, so
// hostile links are built with raw headers here.
func makeArchiveFull(t *testing.T, ms []member, links [][2]string) string {
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
	for _, l := range links {
		tf := byte(tar.TypeSymlink)
		if strings.HasPrefix(l[0], "hard:") {
			tf = tar.TypeLink
			l[0] = strings.TrimPrefix(l[0], "hard:")
		}
		if err := tw.WriteHeader(&tar.Header{
			Name: l[0], Linkname: l[1], Typeflag: tf}); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	w.Close()
	return f
}

func v2JSON(vm string, blobs map[string][2]string) []byte {
	var bs []map[string]any
	for p, meta := range blobs {
		bs = append(bs, map[string]any{
			"path": p, "sha256": meta[0], "size": mustAtoi(meta[1]), "kind": "file"})
	}
	b, _ := json.Marshal(map[string]any{
		"format_version": 2, "vm": vm, "created_at": "t",
		"quickemu_version": "5.0.0", "min_qemu_version": "4.0.0",
		"conf":  map[string]string{"guest_os": "linux"},
		"blobs": bs,
		"disks": []map[string]any{{
			"path": vm + "/disk.qcow2", "format": "qcow2",
			"virtual_size": 1024, "snapshots": []map[string]any{{"name": "s1"}},
		}},
	})
	return b
}

func mustAtoi(s string) int64 {
	var n int64
	fmt.Sscan(s, &n)
	return n
}

func envErr(t *testing.T, out string) (code string) {
	t.Helper()
	var env struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("bad envelope %s", out)
	}
	if env.Success || len(env.Errors) == 0 {
		t.Fatalf("expected error envelope %s", out)
	}
	return env.Errors[0].Code
}

// --- member-name gates, via the binary boundary ---

func TestHostileMembers(t *testing.T) {
	cases := []struct {
		name string
		ms   []member
	}{
		{"abs-unix", []member{{"quickvm-manifest.json", manifestJSON(), 0}, {"/etc/passwd", []byte("x"), 0}}},
		{"dotdot", []member{{"quickvm-manifest.json", manifestJSON(), 0}, {"../x", []byte("x"), 0}}},
		{"dotdot-deep", []member{{"quickvm-manifest.json", manifestJSON(), 0}, {"a/../../x", []byte("x"), 0}}},
		{"backslash-escape", []member{{"quickvm-manifest.json", manifestJSON(), 0}, {`a\..\..\x`, []byte("x"), 0}}},
		{"drive", []member{{"quickvm-manifest.json", manifestJSON(), 0}, {"c:/x", []byte("x"), 0}}},
		{"trailing-dot", []member{{"quickvm-manifest.json", manifestJSON(), 0}, {".", []byte("x"), 0}}},
		{"equiv-dup", []member{{"quickvm-manifest.json", manifestJSON(), 0},
			{"a/../b", []byte("x"), 0}, {"b", []byte("y"), 0}}},
		{"fifo", []member{{"quickvm-manifest.json", manifestJSON(), 0}, {"p", []byte(""), tar.TypeFifo}}},
		{"dev", []member{{"quickvm-manifest.json", manifestJSON(), 0}, {"d", []byte(""), tar.TypeChar}}},
	}
	for _, c := range cases {
		arc := makeArchive(t, c.ms)
		out, rc := runTool(t, "verify", "--archive", arc)
		if rc != 1 {
			t.Fatalf("%s: rc=%d out=%s", c.name, rc, out)
		}
	}
}

func TestHostileLinks(t *testing.T) {
	cases := [][2]string{
		{"l", "/etc/passwd"},    // absolute target
		{"l", "../escape"},      // dotdot escape
		{"d/l", "../../escape"}, // deep escape
		{"l", "a/../../b"},      // resolves outside
		{"l", "c:/win"},         // drive
		{"hard:l", "outside"},   // hardlink to member not shipped
		{"hard:l", "../x"},      // hardlink escape
		{"hard:l", "/abs"},      // hardlink absolute
	}
	for _, l := range cases {
		arc := makeArchiveFull(t, []member{
			{"quickvm-manifest.json", manifestJSON(), 0},
			{"f", []byte("x"), 0},
		}, [][2]string{l})
		out, rc := runTool(t, "verify", "--archive", arc)
		if rc != 1 {
			t.Fatalf("link %v: rc=%d out=%s", l, rc, out)
		}
	}
}

// manifestJSONNamed: the shared fixture names the VM "v"; import
// tests need the manifest to match the "vm/" member namespace.
func manifestJSONNamed(vm string) []byte {
	b, _ := json.Marshal(map[string]any{
		"schemaVersion": 1, "vm": vm, "createdAt": "t",
		"members": []string{"a"}})
	return b
}

// Intra-VM links are permitted and materialise on extract.
func TestAllowedLinksMaterialise(t *testing.T) {
	arc := makeArchiveFull(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"vm/disk.qcow2", []byte("img"), 0},
		{"vm/sub/f", []byte("x"), 0},
	}, [][2]string{
		{"vm/rel", "sub/f"},                // relative to member dir
		{"hard:vm/alias", "vm/disk.qcow2"}, // root-relative member
	})
	out, rc := runTool(t, "verify", "--archive", arc)
	if rc != 0 {
		t.Fatalf("verify rc=%d out=%s", rc, out)
	}
	var env struct {
		Data struct {
			Links []map[string]string `json:"links"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(strings.TrimSpace(out)), &env)
	if len(env.Data.Links) != 2 {
		t.Fatalf("links not registered: %s", out)
	}
	dest := filepath.Join(t.TempDir(), "d")
	arc2 := makeArchiveFull(t, []member{
		{"quickvm-manifest.json", manifestJSONNamed("vm"), 0},
		{"vm.conf", []byte("guest_os=linux"), 0},
		{"vm/disk.qcow2", []byte("img"), 0},
		{"vm/sub/f", []byte("x"), 0},
	}, [][2]string{{"vm/rel", "sub/f"}})
	if out, rc := runTool(t, "import", "--archive", arc2, "--dest", dest); rc != 0 {
		t.Fatalf("import rc=%d out=%s", rc, out)
	}
	p := filepath.Join(dest, "vm", "rel")
	tgt, err := os.Readlink(p)
	if err != nil || tgt != "sub/f" {
		t.Fatalf("symlink %q -> %q err %v", p, tgt, err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "vm", "sub", "f")); err != nil || string(b) != "x" {
		t.Fatalf("link deref failed: %v", err)
	}
}

// Entry-count bomb: >maxMembers tiny members must fail even though
// byte limits are nowhere near the cap. Generating 100k headers at
// test time is slow through the helper boundary, so the seam path
// tests the counter directly.
func TestEntryBomb(t *testing.T) {
	dir := t.TempDir()
	arc := filepath.Join(dir, "bomb.tgz")
	f, _ := os.Create(arc)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	man := manifestJSON()
	for i := 0; i <= maxMembers; i++ {
		name := fmt.Sprintf("f%06d", i)
		if i == 0 {
			name = "quickvm-manifest.json"
		}
		var body []byte
		if i == 0 {
			body = man
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600,
			Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if len(body) > 0 {
			tw.Write(body)
		}
	}
	tw.Close()
	gz.Close()
	f.Close()
	out, rc := runTool(t, "verify", "--archive", arc)
	if rc != 1 {
		t.Fatalf("bomb rc=%d", rc)
	}
	if code := envErr(t, out); code != "ARCHIVE_BOMB" {
		t.Fatalf("code=%s", code)
	}
}

// Disk-full: the statfs seam reports less than the declared bytes —
// the import must fail before writing anything, leaving no staging.
func TestDiskFullSeam(t *testing.T) {
	body := []byte("payload")
	sum := sha256.Sum256(body)
	arc := makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"quickvm-checksums.sha256", []byte(hex.EncodeToString(sum[:]) + "  vm/f\n"), 0},
		{"vm.conf", []byte("x"), 0},
		{"vm/f", body, 0},
	})
	dest := filepath.Join(t.TempDir(), "d")
	old := freeSpace
	freeSpace = func(string) (int64, error) { return 1, nil }
	defer func() { freeSpace = old }()
	if _, err := runExtraction(arc, dest, true, false); err == nil {
		t.Fatal("disk-full import must fail")
	} else if qe, ok := err.(*qeErr); !ok || qe.code != "INSUFFICIENT_SPACE" {
		t.Fatalf("err=%v", err)
	}
	ents, _ := os.ReadDir(dest)
	for _, e := range ents {
		if strings.Contains(e.Name(), ".import.") {
			t.Fatalf("staging left behind: %s", e.Name())
		}
	}
}

func TestStatfsFreeReal(t *testing.T) {
	free, err := statfsFree(t.TempDir())
	if err != nil || free <= 0 {
		t.Fatalf("statfs %d %v", free, err)
	}
}

// No-clobber: an existing <vm> must refuse without --force and
// replace with it.
func TestImportNoClobber(t *testing.T) {
	build := func() string {
		return makeArchive(t, []member{
			{"quickvm-manifest.json", manifestJSONNamed("vm"), 0},
			{"vm.conf", []byte("guest_os=linux"), 0},
			{"vm/disk.qcow2", []byte("img"), 0},
		})
	}
	dest := t.TempDir()
	arc := build()
	out, rc := runTool(t, "import", "--archive", arc, "--dest", dest)
	if rc != 0 {
		t.Fatalf("import rc=%d %s", rc, out)
	}
	if _, err := os.Stat(filepath.Join(dest, "vm", "disk.qcow2")); err != nil {
		t.Fatalf("vm dir missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "vm.conf")); err != nil {
		t.Fatalf("conf missing: %v", err)
	}
	// Second import without force must refuse BEFORE touching dest.
	out, rc = runTool(t, "import", "--archive", arc, "--dest", dest)
	if rc != 1 {
		t.Fatalf("reimport rc=%d", rc)
	}
	if code := envErr(t, out); code != "IMPORT_CONFLICT" {
		t.Fatalf("code=%s", code)
	}
	// With force it replaces.
	os.WriteFile(filepath.Join(dest, "vm", "stale"), []byte("old"), 0o600)
	if out, rc := runTool(t, "import", "--archive", arc, "--dest", dest, "--force"); rc != 0 {
		t.Fatalf("force import rc=%d %s", rc, out)
	}
	if _, err := os.Stat(filepath.Join(dest, "vm", "stale")); err == nil {
		t.Fatal("stale content survived forced import")
	}
}

// Recovery journal: an interrupted "installing" import completes;
// an interrupted "extracting" one cleans up.
func TestRecoverCompletesStaged(t *testing.T) {
	dest := t.TempDir()
	staging := filepath.Join(dest, "vm.import.tmp")
	os.MkdirAll(staging, 0o755)
	os.WriteFile(filepath.Join(staging, "disk.qcow2"), []byte("img"), 0o600)
	os.WriteFile(staging+".conf", []byte("guest_os=linux"), 0o600)
	j := &journal{Version: 1, VM: "vm", DestDir: dest, State: jStateInstalling,
		Staging: staging, ConfTmp: staging + ".conf", ConfDest: "vm.conf",
		Members: []string{"disk.qcow2"}}
	if err := writeJournal(filepath.Join(dest, "vm.import.journal"), j); err != nil {
		t.Fatal(err)
	}
	out, rc := runTool(t, "recover", "--dest", dest)
	if rc != 0 {
		t.Fatalf("recover rc=%d %s", rc, out)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "vm", "disk.qcow2")); err != nil || string(b) != "img" {
		t.Fatalf("completed import missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "vm.conf")); err != nil {
		t.Fatalf("conf not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "vm.import.journal")); err == nil {
		t.Fatal("journal not removed after completion")
	}
}

func TestRecoverCleansPartial(t *testing.T) {
	dest := t.TempDir()
	staging := filepath.Join(dest, "vm.import.tmp")
	os.MkdirAll(staging, 0o755)
	os.WriteFile(filepath.Join(staging, "half.qcow2"), []byte("partial"), 0o600)
	j := &journal{Version: 1, VM: "vm", DestDir: dest, State: jStateExtracting,
		Staging: staging, Members: []string{"disk.qcow2", "other"}}
	writeJournal(filepath.Join(dest, "vm.import.journal"), j)
	// An orphan dir with no journal is also cleaned.
	os.MkdirAll(filepath.Join(dest, "old.import.tmp"), 0o755)
	if out, rc := runTool(t, "recover", "--dest", dest); rc != 0 {
		t.Fatalf("recover rc=%d %s", rc, out)
	}
	for _, p := range []string{"vm.import.tmp", "vm.import.journal", "old.import.tmp"} {
		if _, err := os.Stat(filepath.Join(dest, p)); err == nil {
			t.Fatalf("%s left behind", p)
		}
	}
}

// Incomplete installing-state staging (a member never landed) cleans
// rather than installing a partial VM.
func TestRecoverIncompleteInstall(t *testing.T) {
	dest := t.TempDir()
	staging := filepath.Join(dest, "vm.import.tmp")
	os.MkdirAll(staging, 0o755)
	os.WriteFile(filepath.Join(staging, "disk.qcow2"), []byte("img"), 0o600)
	j := &journal{Version: 1, VM: "vm", DestDir: dest, State: jStateInstalling,
		Staging: staging, ConfTmp: staging + ".conf", ConfDest: "vm.conf",
		Members: []string{"disk.qcow2", "missing.qcow2"}} // absent -> incomplete
	writeJournal(filepath.Join(dest, "vm.import.journal"), j)
	if _, rc := runTool(t, "recover", "--dest", dest); rc != 0 {
		t.Fatalf("recover rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(dest, "vm")); err == nil {
		t.Fatal("partial VM was installed")
	}
}

// --- v2 manifest ---

func TestV2VerifyAndInspect(t *testing.T) {
	body := []byte("imgdata")
	sum := sha256.Sum256(body)
	csum := sha256.Sum256([]byte("guest_os=linux"))
	arc := makeArchive(t, []member{
		{"manifest.json", v2JSON("vm", map[string][2]string{
			"vm.conf":       {hex.EncodeToString(csum[:]), "14"},
			"vm/disk.qcow2": {hex.EncodeToString(sum[:]), "7"},
		}), 0},
		{"vm.conf", []byte("guest_os=linux"), 0},
		{"vm/disk.qcow2", body, 0},
	})
	out, rc := runTool(t, "verify", "--archive", arc)
	if rc != 0 {
		t.Fatalf("verify rc=%d %s", rc, out)
	}
	var env struct {
		Data struct {
			FV  int    `json:"format_version"`
			VM  string `json:"vm"`
			Ver int    `json:"verified"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(strings.TrimSpace(out)), &env)
	if env.Data.FV != 2 || env.Data.VM != "vm" || env.Data.Ver != 2 {
		t.Fatalf("data=%s", out)
	}
	out, rc = runTool(t, "inspect", "--archive", arc)
	if rc != 0 {
		t.Fatalf("inspect rc=%d", rc)
	}
	var ienv struct {
		Data struct {
			FV    int               `json:"format_version"`
			Conf  map[string]string `json:"conf"`
			Disks []map[string]any  `json:"disks"`
			Names []string          `json:"member_names"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(strings.TrimSpace(out)), &ienv)
	if ienv.Data.FV != 2 || ienv.Data.Conf["guest_os"] != "linux" ||
		len(ienv.Data.Disks) != 1 || len(ienv.Data.Names) != 3 {
		t.Fatalf("inspect data=%s", out)
	}
	// Manifest metadata must not land in the imported VM.
	dest := filepath.Join(t.TempDir(), "d")
	if out, rc := runTool(t, "import", "--archive", arc, "--dest", dest); rc != 0 {
		t.Fatalf("import rc=%d %s", rc, out)
	}
	if _, err := os.Stat(filepath.Join(dest, "vm", "manifest.json")); err == nil {
		t.Fatal("manifest.json installed into the VM dir")
	}
}

func TestV2RejectsBadBlobs(t *testing.T) {
	sum := sha256.Sum256([]byte("imgdata"))
	cases := []map[string][2]string{
		{"vm/evil": {"bad", "7"}},                         // missing member
		{"vm/disk.qcow2": {strings.Repeat("0", 64), "7"}}, // wrong sha
		{"../outside": {hex.EncodeToString(sum[:]), "7"}}, // unsafe path
	}
	for _, blobs := range cases {
		arc := makeArchive(t, []member{
			{"manifest.json", v2JSON("vm", blobs), 0},
			{"vm.conf", []byte("x"), 0},
			{"vm/disk.qcow2", []byte("imgdata"), 0},
		})
		if out, rc := runTool(t, "verify", "--archive", arc); rc != 1 {
			t.Fatalf("blobs %v accepted rc=%d out=%s", blobs, rc, out)
		}
	}
	// Unsupported format_version.
	arc := makeArchive(t, []member{
		{"manifest.json", []byte(`{"format_version":99,"vm":"vm"}`), 0},
		{"vm/disk.qcow2", []byte("imgdata"), 0},
	})
	if out, rc := runTool(t, "verify", "--archive", arc); rc != 1 {
		t.Fatalf("v99 accepted %s", out)
	} else if code := envErr(t, out); code != "UNSUPPORTED_FORMAT" {
		t.Fatalf("code=%s", code)
	}
}

// TPM carrying archives warn on stderr (envelope stays clean).
func TestV2TPMWarning(t *testing.T) {
	body := []byte("imgdata")
	sum := sha256.Sum256(body)
	var m map[string]any
	json.Unmarshal(v2JSON("vm", map[string][2]string{
		"vm/disk.qcow2": {hex.EncodeToString(sum[:]), "7"},
	}), &m)
	m["includes_tpm"] = true
	mb, _ := json.Marshal(m)
	arc := makeArchive(t, []member{
		{"manifest.json", mb, 0},
		{"vm/disk.qcow2", body, 0},
	})
	_, se, rc := qeRunFull(t, "verify", "--archive", arc)
	if rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if !strings.Contains(se, "TPM") {
		t.Fatalf("no tpm warning on stderr: %q", se)
	}
}

// qeRunFull wraps qetest.RunToolFull for stderr capture.
func qeRunFull(t *testing.T, args ...string) (string, string, int) {
	return qetest.RunToolFull(t, nil, "", args...)
}
