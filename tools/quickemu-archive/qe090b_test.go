// qe090b_test.go: coverage for the uncovered branches — link
// materialisation failures, stageFile error paths, layout mapping,
// journal/recover variants, installOne force semantics, usage and
// seam stubs (mkdirTemp, freeSpace).
package main

import (
	"archive/tar"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- safe.go leaf helpers ---

func TestAllowedType(t *testing.T) {
	ok := []byte{tar.TypeReg, tar.TypeDir, tar.TypeSymlink, tar.TypeLink}
	bad := []byte{tar.TypeChar, tar.TypeBlock, tar.TypeFifo, tar.TypeXHeader,
		tar.TypeXGlobalHeader, tar.TypeGNUSparse, tar.TypeGNULongName, 'z'}
	for _, ty := range ok {
		if !allowedType(ty) {
			t.Fatalf("type %q must be allowed", ty)
		}
	}
	for _, ty := range bad {
		if allowedType(ty) {
			t.Fatalf("type %q must be refused", ty)
		}
	}
}

func TestQEErrError(t *testing.T) {
	e := qerr("CODE_X", "the %s", "message")
	if e.Error() != "the message" || e.code != "CODE_X" {
		t.Fatalf("qeErr = %q code %q", e.Error(), e.code)
	}
}

// --- layoutRel / layOut ---

func TestLayoutRel(t *testing.T) {
	cases := []struct {
		clean, vm string
		vmMode    bool
		want      string
		ok        bool
	}{
		{"a/b", "vm", false, "a/b", true},  // generic keeps verbatim
		{"", "vm", false, "", false},       // empty generic name
		{"vm", "vm", true, "", false},      // the dir root itself
		{"vm.conf", "vm", true, "", false}, // routed conf
		{"vm/disk.qcow2", "vm", true, "disk.qcow2", true},
		{"vm/sub/f", "vm", true, "sub/f", true},
		{"other/f", "vm", true, "", false},   // nested path outside vm/
		{"other", "vm", true, "other", true}, // top-level stray file ok
	}
	for _, c := range cases {
		got, ok := layoutRel(c.clean, c.vm, c.vmMode)
		if got != c.want || ok != c.ok {
			t.Fatalf("layoutRel(%q,%q,%v) = %q,%v want %q,%v",
				c.clean, c.vm, c.vmMode, got, ok, c.want, c.ok)
		}
	}
}

func TestLayOutErrors(t *testing.T) {
	staging := filepath.Join(t.TempDir(), "st")
	// Member outside the VM namespace → IMPORT_LAYOUT.
	s := &scan{members: []memberRef{{name: "evil/x"}}}
	if _, err := s.layOut(staging, "vm", true); err == nil {
		t.Fatal("outside member accepted")
	} else if qe := err.(*qeErr); qe.code != "IMPORT_LAYOUT" {
		t.Fatalf("code=%s", qe.code)
	}
	// The <vm>/ root dir entry itself is skipped, not an error.
	s = &scan{members: []memberRef{{name: "vm", typ: tar.TypeDir}, {name: "vm/f"}}}
	ex, err := s.layOut(staging, "vm", true)
	if err != nil || len(ex.rel) != 1 {
		t.Fatalf("layout %v %+v", err, ex)
	}
	// A member that maps to an unsafe staging-rel path → TAR_SLIP.
	s = &scan{members: []memberRef{{name: "vm/../evil"}}}
	if _, err := s.layOut(staging, "vm", true); err == nil {
		t.Fatal("unsafe rel accepted")
	} else if qe := err.(*qeErr); qe.code != "TAR_SLIP" {
		t.Fatalf("code=%s", qe.code)
	}
	// Routed conf member is recorded, not staged.
	s = &scan{members: []memberRef{{name: "vm.conf"}, {name: "vm/f"}}}
	ex, err = s.layOut(staging, "vm", true)
	if err != nil || ex.conf != "vm.conf" || len(ex.rel) != 1 {
		t.Fatalf("conf routing %+v %v", ex, err)
	}
	// Metadata members are consumed, never laid out.
	s = &scan{members: []memberRef{{name: "vm/manifest.json"}, {name: "vm/f"}}}
	ex, err = s.layOut(staging, "vm", true)
	if err != nil || len(ex.rel) != 1 || ex.rel["vm/f"] != "f" {
		t.Fatalf("meta layout %+v %v", ex, err)
	}
}

// --- stageFile error paths ---

func TestStageFileErrors(t *testing.T) {
	dir := t.TempDir()
	// Short read: fewer bytes than the header declared.
	dst := filepath.Join(dir, "f")
	err := stageFile(strings.NewReader("abc"), dst, "", 0o600, 10)
	if qe, ok := err.(*qeErr); !ok || qe.code != "ARCHIVE_BAD" {
		t.Fatalf("short read err=%v", err)
	}
	// Default mode applies when mode==0.
	if err := stageFile(strings.NewReader("x"), dst, "", 0, 1); err != nil {
		t.Fatalf("default mode write: %v", err)
	}
	// MkdirAll failure: parent component is a regular file.
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o600)
	err = stageFile(strings.NewReader("x"), filepath.Join(blocker, "f"), "", 0o600, 1)
	if qe, ok := err.(*qeErr); !ok || qe.code != "EXTRACT_FAIL" {
		t.Fatalf("mkdir fail err=%v", err)
	}
	// OpenFile failure: destination path is a directory.
	subdir := filepath.Join(dir, "adir")
	os.MkdirAll(subdir, 0o755)
	err = stageFile(strings.NewReader("x"), subdir, "", 0o600, 1)
	if qe, ok := err.(*qeErr); !ok || qe.code != "EXTRACT_FAIL" {
		t.Fatalf("open dir err=%v", err)
	}
	// Root escape: a symlinked parent resolving outside root.
	outside := filepath.Join(dir, "outside")
	os.MkdirAll(outside, 0o755)
	root := filepath.Join(dir, "root")
	os.MkdirAll(root, 0o755)
	link := filepath.Join(root, "linkdir")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	err = stageFile(strings.NewReader("x"), filepath.Join(link, "f"), root, 0o600, 1)
	if qe, ok := err.(*qeErr); !ok || qe.code != "TAR_SLIP" {
		t.Fatalf("root escape err=%v", err)
	}
}

func TestMustRel(t *testing.T) {
	root := t.TempDir()
	if r := mustRel(root, filepath.Join(root, "a", "b")); r != filepath.Join("a", "b") {
		t.Fatalf("mustRel=%q", r)
	}
	if r := mustRel(root, filepath.Join(root, "..", "up")); r != filepath.Join("..", "up") {
		t.Fatalf("mustRel outside=%q", r)
	}
}

// --- materialiseLinks failure branches (scan structs built directly) ---

func matLinksSetup(t *testing.T) (*extracted, string) {
	t.Helper()
	staging := filepath.Join(t.TempDir(), "st")
	os.MkdirAll(staging, 0o755)
	ex := &extracted{staging: staging, rel: map[string]string{}}
	return ex, staging
}

func TestMaterialiseLinksFailures(t *testing.T) {
	ex, staging := matLinksSetup(t)

	// Hardlink whose target was not staged.
	s := &scan{links: []linkEntry{{member: "vm/alias", target: "vm/missing", hardlink: true}}}
	ex.rel["vm/alias"] = "alias"
	err := s.materialiseLinks(ex, "vm", true)
	if qe, ok := err.(*qeErr); !ok || qe.code != "ARCHIVE_LINK" {
		t.Fatalf("unstaged target err=%v", err)
	}

	// Hardlink target staged but not a regular file (a dir).
	tgt := filepath.Join(staging, "tdir")
	os.MkdirAll(tgt, 0o755)
	s = &scan{links: []linkEntry{{member: "vm/alias", target: "vm/tdir", hardlink: true}}}
	ex.rel["vm/tdir"] = "tdir"
	err = s.materialiseLinks(ex, "vm", true)
	if qe, ok := err.(*qeErr); !ok || qe.code != "ARCHIVE_LINK" {
		t.Fatalf("dir target err=%v", err)
	}

	// Symlink whose namespace target escapes the VM dir.
	ex2, _ := matLinksSetup(t)
	ex2.rel["vm/a/l"] = "a/l"
	s = &scan{links: []linkEntry{{member: "vm/a/l", target: "../../../y/z"}}}
	err = s.materialiseLinks(ex2, "vm", true)
	if qe, ok := err.(*qeErr); !ok || qe.code != "ARCHIVE_LINK" ||
		!strings.Contains(qe.msg, "escapes") {
		t.Fatalf("escape err=%v", err)
	}

	// Symlink whose stored text no longer matches the remapped
	// layout (member vm/l target ../x resolves to member x, which is
	// fine lexically but lives outside vm/ after stripping).
	ex3, _ := matLinksSetup(t)
	ex3.rel["vm/l"] = "l"
	s = &scan{links: []linkEntry{{member: "vm/l", target: "../x"}}}
	err = s.materialiseLinks(ex3, "vm", true)
	if qe, ok := err.(*qeErr); !ok || qe.code != "ARCHIVE_LINK" ||
		!strings.Contains(qe.msg, "remapped") {
		t.Fatalf("remapped err=%v", err)
	}

	// Happy hardlink: staged regular target links through.
	ex4, staging4 := matLinksSetup(t)
	os.WriteFile(filepath.Join(staging4, "disk.qcow2"), []byte("img"), 0o600)
	ex4.rel["vm/disk.qcow2"] = "disk.qcow2"
	ex4.rel["vm/alias"] = "alias"
	s = &scan{links: []linkEntry{{member: "vm/alias", target: "vm/disk.qcow2", hardlink: true}}}
	if err := s.materialiseLinks(ex4, "vm", true); err != nil {
		t.Fatalf("hardlink materialise: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(staging4, "alias")); err != nil || string(b) != "img" {
		t.Fatalf("hardlink read %v", err)
	}

	// Link dropped by layout is skipped silently.
	ex5, _ := matLinksSetup(t)
	s = &scan{links: []linkEntry{{member: "vm/gone", target: "x"}}}
	if err := s.materialiseLinks(ex5, "vm", true); err != nil {
		t.Fatalf("dropped link err=%v", err)
	}
}

// --- journal / install / recover variants ---

func TestJournalIOErrors(t *testing.T) {
	dir := t.TempDir()
	// writeJournal fails when the tmp path is a directory.
	block := filepath.Join(dir, "j.import.journal.tmp")
	os.MkdirAll(block, 0o755)
	j := &journal{Version: 1, DestDir: dir, Staging: dir}
	if err := writeJournal(filepath.Join(dir, "j.import.journal"), j); err == nil {
		t.Fatal("writeJournal over dir-tmp succeeded")
	}
	// loadJournal rejects corrupt JSON.
	bad := filepath.Join(dir, "bad.import.journal")
	os.WriteFile(bad, []byte("{nope"), 0o600)
	if _, err := loadJournal(bad); err == nil {
		t.Fatal("corrupt journal parsed")
	}
	if _, err := loadJournal(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing journal loaded")
	}
}

func TestInstallOne(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.WriteFile(src, []byte("new"), 0o600)
	os.WriteFile(dst, []byte("old"), 0o600)
	// Existing dst without force refuses.
	if err := installOne(src, dst, false); err == nil ||
		!strings.Contains(err.Error(), "--force") {
		t.Fatalf("no-force install err=%v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "old" {
		t.Fatal("dst clobbered without force")
	}
	// With force the dst is replaced.
	if err := installOne(src, dst, true); err != nil {
		t.Fatalf("force install: %v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "new" {
		t.Fatal("dst not replaced")
	}
	if _, err := os.Stat(src); err == nil {
		t.Fatal("src left behind after rename")
	}
}

func TestStagedCompleteGeneric(t *testing.T) {
	dir := t.TempDir()
	// Generic extract: any existing staging dir counts as complete.
	j := &journal{Version: 1, DestDir: dir, Staging: filepath.Join(dir, "st"), State: jStateInstalling}
	if stagedComplete(j) {
		t.Fatal("missing staging reported complete")
	}
	os.MkdirAll(j.Staging, 0o755)
	if !stagedComplete(j) {
		t.Fatal("existing staging incomplete")
	}
	// VM import with a missing conf sidecar is incomplete.
	j2 := &journal{Version: 1, VM: "vm", DestDir: dir,
		Staging: filepath.Join(dir, "vm.import.tmp"), ConfTmp: filepath.Join(dir, "nope.conf"),
		Members: []string{}}
	os.MkdirAll(j2.Staging, 0o755)
	if stagedComplete(j2) {
		t.Fatal("missing confTmp reported complete")
	}
	os.WriteFile(j2.ConfTmp, []byte("x"), 0o600)
	if !stagedComplete(j2) {
		t.Fatal("complete VM staging incomplete")
	}
}

func TestRecoverVariants(t *testing.T) {
	dest := t.TempDir()

	// Corrupt journal is removed.
	os.WriteFile(filepath.Join(dest, "a.import.journal"), []byte("{bad"), 0o600)

	// Installing journal whose install cannot complete (dst exists,
	// no force) → cleaned-incomplete: staging and journal removed,
	// pre-existing dst untouched.
	staging := filepath.Join(dest, "vm.import.tmp")
	os.MkdirAll(staging, 0o755)
	os.WriteFile(filepath.Join(staging, "disk.qcow2"), []byte("img"), 0o600)
	os.MkdirAll(filepath.Join(dest, "vm"), 0o755)
	os.WriteFile(filepath.Join(dest, "vm", "keep"), []byte("k"), 0o600)
	j := &journal{Version: 1, VM: "vm", DestDir: dest, State: jStateInstalling,
		Staging: staging, Members: []string{"disk.qcow2"}, Force: false}
	writeJournal(filepath.Join(dest, "vm.import.journal"), j)

	// Generic extract journal completing: entries land in dest.
	gstaging := filepath.Join(dest, ".qe-extract-g1.tmp")
	os.MkdirAll(gstaging, 0o755)
	os.WriteFile(filepath.Join(gstaging, "f"), []byte("x"), 0o600)
	gj := &journal{Version: 1, DestDir: dest, State: jStateInstalling, Staging: gstaging}
	writeJournal(filepath.Join(dest, "g1.qe-extract.journal"), gj)

	// Orphans of every flavour.
	os.MkdirAll(filepath.Join(dest, "o1.import.tmp"), 0o755)
	os.WriteFile(filepath.Join(dest, "o2.import.conf"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(dest, "o3.journal.tmp"), []byte("x"), 0o600)
	os.MkdirAll(filepath.Join(dest, ".qe-extract-o4.tmp"), 0o755)

	actions, err := recoverDest(dest)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	got := map[string]bool{}
	for _, a := range actions {
		got[a["action"]] = true
	}
	for _, want := range []string{"removed-corrupt-journal", "cleaned-incomplete",
		"completed", "removed-orphan"} {
		if !got[want] {
			t.Fatalf("action %s missing in %v", want, actions)
		}
	}
	// The blocked VM dir survives the failed completion.
	if b, err := os.ReadFile(filepath.Join(dest, "vm", "keep")); err != nil || string(b) != "k" {
		t.Fatal("existing vm dir damaged")
	}
	if _, err := os.Stat(staging); err == nil {
		t.Fatal("blocked staging left behind")
	}
	// Generic staged file installed.
	if b, err := os.ReadFile(filepath.Join(dest, "f")); err != nil || string(b) != "x" {
		t.Fatal("generic install missing")
	}
	for _, p := range []string{"o1.import.tmp", "o2.import.conf", "o3.journal.tmp", ".qe-extract-o4.tmp"} {
		if _, err := os.Stat(filepath.Join(dest, p)); err == nil {
			t.Fatalf("orphan %s left", p)
		}
	}
}

// --- runExtraction uncovered branches ---

func TestRunExtractionBranches(t *testing.T) {
	dest := t.TempDir()

	// vmMode with no VM name anywhere → MANIFEST_MISSING.
	arc := makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSONNamed(""), 0},
		{"f", []byte("x"), 0},
	})
	if _, err := runExtraction(arc, dest, true, false); err == nil {
		t.Fatal("nameless import accepted")
	} else if qe := err.(*qeErr); qe.code != "MANIFEST_MISSING" {
		t.Fatalf("code=%s", qe.code)
	}

	// Manifest naming an unsafe VM.
	for _, vm := range []string{"..", "a/b", "a\\b"} {
		arc := makeArchive(t, []member{
			{"quickvm-manifest.json", manifestJSONNamed(vm), 0},
			{"f", []byte("x"), 0},
		})
		if _, err := runExtraction(arc, dest, true, false); err == nil {
			t.Fatalf("vm %q accepted", vm)
		} else if qe := err.(*qeErr); qe.code != "IMPORT_LAYOUT" {
			t.Fatalf("vm %q code=%s", vm, qe.code)
		}
	}

	// Generic extract with mkdirTemp seam failing.
	arc = makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"f", []byte("x"), 0},
	})
	old := mkdirTemp
	mkdirTemp = func(dir, pat string) (string, error) { return "", errors.New("boom") }
	defer func() { mkdirTemp = old }()
	if _, err := runExtraction(arc, filepath.Join(t.TempDir(), "d"), false, false); err == nil {
		t.Fatal("mkdirTemp failure swallowed")
	}
	mkdirTemp = old

	// Journal write failure: a non-empty dir squats on
	// <vm>.import.journal — recover cannot remove it and the atomic
	// rename onto it fails.
	arc = makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSONNamed("vm"), 0},
		{"vm.conf", []byte("x"), 0},
		{"vm/f", []byte("y"), 0},
	})
	d2 := t.TempDir()
	blockJ := filepath.Join(d2, "vm.import.journal")
	os.MkdirAll(blockJ, 0o755)
	os.WriteFile(filepath.Join(blockJ, "in"), []byte("x"), 0o600)
	if _, err := runExtraction(arc, d2, true, false); err == nil {
		t.Fatal("journal write failure swallowed")
	}

	// ensureSpace statfs error → SPACE_CHECK.
	oldFs := freeSpace
	freeSpace = func(string) (int64, error) { return 0, errors.New("no fs") }
	defer func() { freeSpace = oldFs }()
	if _, err := runExtraction(arc, filepath.Join(t.TempDir(), "d"), true, false); err == nil {
		t.Fatal("statfs error swallowed")
	} else if qe := err.(*qeErr); qe.code != "SPACE_CHECK" {
		t.Fatalf("code=%s", qe.code)
	}
	freeSpace = oldFs
}

// --- run()/parseArgs usage paths ---

func TestUsageAndPositional(t *testing.T) {
	for _, sub := range []string{"verify", "inspect", "extract", "import"} {
		if _, rc := runTool(t, sub); rc != 2 {
			t.Fatalf("%s without archive rc=%d", sub, rc)
		}
	}
	if _, rc := runTool(t, "frobnicate"); rc != 2 {
		t.Fatalf("unknown sub rc")
	}
	// Positional archive works too.
	arc := makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"f", []byte("x"), 0},
	})
	if _, rc := runTool(t, "verify", arc); rc != 0 {
		t.Fatal("positional archive rejected")
	}
}

func TestParseArgs(t *testing.T) {
	a := parseArgs([]string{"--archive", "x.tgz", "--force", "pos1", "--dest"}, "force")
	if a.kv["archive"] != "x.tgz" || a.kv["force"] != "true" ||
		a.kv["dest"] != "" || len(a.pos) != 1 || a.pos[0] != "pos1" {
		t.Fatalf("parse=%+v", a)
	}
}

// --- cmdExtract/cmdInspect error paths via the wire ---

func TestExtractAndInspectErrors(t *testing.T) {
	if _, rc := runTool(t, "extract", "--archive", "/nope.tgz", "--dest", t.TempDir()); rc != 1 {
		t.Fatal("missing archive extracted")
	}
	if _, rc := runTool(t, "inspect", "--archive", "/nope.tgz"); rc != 1 {
		t.Fatal("missing archive inspected")
	}
	if _, rc := runTool(t, "recover", "--dest", "/nope-dest-dir"); rc != 1 {
		t.Fatal("missing dest recovered")
	}
	// Generic extract preserves the full member tree including dirs.
	arc := makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"d", []byte(""), tar.TypeDir},
		{"d/f", []byte("y"), 0},
		{"top", []byte("t"), 0},
	})
	dest := filepath.Join(t.TempDir(), "out")
	out, rc := runTool(t, "extract", "--archive", arc, "--dest", dest)
	if rc != 0 {
		t.Fatalf("extract rc=%d %s", rc, out)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "d", "f")); err != nil || string(b) != "y" {
		t.Fatalf("nested member missing: %v", err)
	}
}
