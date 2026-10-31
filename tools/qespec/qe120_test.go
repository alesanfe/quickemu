// qe120_test.go — QE-120 adversarial wire cases: hostile inputs
// thrown at the real binaries must be refused or sanitised before
// they reach monitor commands, QEMU options, blob paths or the conf.
package qespec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// qe120Conf writes a minimal VM conf + fake disk and returns (dir, conf).
func qe120Conf(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	disk := filepath.Join(dir, "disk.qcow2")
	os.WriteFile(disk, []byte("qcow"), 0o600)
	conf := dir + ".conf"
	os.WriteFile(conf, []byte("disk_img=\""+disk+"\"\nguest_os=\"linux\"\n"), 0o644)
	return dir, conf
}

func TestAdversarialSnapshotTags(t *testing.T) {
	WithFakeBin(t)
	dir, conf := qe120Conf(t)
	for _, tag := range []string{
		"x;rm -rf /", "$(id)", "a b", "a/../b", "-d",
		"tag'quote", "tag`tick`", "tag|pipe", "tag&gt",
	} {
		r := Run(t, "quickemu-vm", []RunOpt{Env("QE_VMCONF=" + conf)},
			"snapshot-create", dir, tag)
		if r.OK() {
			t.Errorf("tag %q accepted — got rc=0", tag)
		}
		if !strings.Contains(r.Combined(), "[A-Za-z0-9._-]") {
			t.Errorf("tag %q: expected charset refusal, got %.200s", tag, r.Combined())
		}
	}
	// A sane tag passes the gate (qemu-img stub accepts).
	r := Run(t, "quickemu-vm", []RunOpt{Env("QE_VMCONF=" + conf)},
		"snapshot-create", dir, "good.tag-1")
	if r.Failed() {
		t.Errorf("legit tag refused: %s", r.Combined())
	}
}

func TestAdversarialCliMemSnapshotTag(t *testing.T) {
	WithFakeBin(t)
	_, conf := qe120Conf(t)
	// Charset gate runs BEFORE the running check — a stopped VM with
	// a hostile tag must still fail at validation.
	r := Run(t, "quickemu", nil, "--vm", conf, "--snapshot", "create",
		"bad;tag", "--memory")
	if r.OK() {
		t.Fatal("hostile tag accepted")
	}
	r.Contains(t, "[A-Za-z0-9._-]+")
}

func TestAdversarialOCIRefAndIndex(t *testing.T) {
	dir := t.TempDir()
	// Push: hostile refs refuse before any blob writes.
	arc := filepath.Join(dir, "x.quickvm.tar.gz")
	os.WriteFile(arc, []byte("fake"), 0o600)
	for _, ref := range []string{"a/b:c", "a:b:c:d", "x:..", ".:1", "a/:1"} {
		r := Run(t, "quickemu-archive", nil, "oci-push", "--archive", arc,
			"--dest", dir, "--ref", ref)
		if r.OK() {
			t.Errorf("ref %q accepted", ref)
		}
	}
	// Pull: index.json with a traversal digest must not read outside
	// blobs/.
	layout := filepath.Join(dir, "evil")
	os.MkdirAll(filepath.Join(layout, "blobs", "sha256"), 0o755)
	os.WriteFile(filepath.Join(layout, "oci-layout"),
		[]byte(`{"imageLayoutVersion":"1.0.0"}`), 0o644)
	for _, dg := range []string{
		"sha256:" + strings.Repeat("a", 64) + "/../../victim",
		"md5:deadbeef",
		"sha256:short",
	} {
		idx := `{"manifests":[{"digest":"` + dg + `"}]}`
		os.WriteFile(filepath.Join(layout, "index.json"), []byte(idx), 0o644)
		r := Run(t, "quickemu-archive", nil, "oci-pull",
			"--oci", layout, "--dest", t.TempDir())
		if r.OK() {
			t.Errorf("digest %q accepted", dg)
		}
	}
}

func TestAdversarialConfInjection(t *testing.T) {
	WithFakeBin(t)
	dir, conf := qe120Conf(t)
	cases := []struct{ name, kv, mustNotContain string }{
		{"loadvm option injection", `loadvm="--evil-opt"`, "-loadvm"},
		{"loadvm second arg", `loadvm="x -machine none"`, "-loadvm x -machine"},
		{"bad share tag", "share_dirs=\"/tmp\"\nshare_tags=\"bad;tag\"", "mount_tag=bad"},
		{"bad sandbox", `sandbox="bogus"`, "-sandbox"},
	}
	for _, c := range cases {
		f := filepath.Join(t.TempDir(), "vm.conf")
		b, _ := os.ReadFile(conf)
		os.WriteFile(f, append(b, []byte(c.kv+"\n")...), 0o644)
		r := Run(t, "quickemu-launch", []RunOpt{InDir(dir), Env("QE_VMCONF=" + f)}, f)
		if strings.Contains(r.Stdout, c.mustNotContain) {
			t.Errorf("%s: emitted %q\n%s", c.name, c.mustNotContain, r.Stdout)
		}
	}
}

// Source-audit: the defenses must stay in the tree, not just in tests.
func TestAdversarialDefensesPresent(t *testing.T) {
	checks := []struct {
		mod  string
		pats []string
	}{
		{"tools/quickemu-cli", []string{
			"memSnapTagRe", "block-dirty-bitmap-add", `"node": "SystemDisk"`}},
		{"tools/quickemu-launch", []string{
			"snapTagChars", "shareTagRe", "qemuOptEsc", "shellQuote"}},
		{"tools/quickemu-archive", []string{
			"ociBlobPath", "CHECKSUM_MISMATCH", "refNameOk"}},
		{"tools/quickemu-vm", []string{
			"snapTagOk", "snapAutoParse", "vmStoppedOrDie"}},
	}
	for _, c := range checks {
		src := SourceTree(t, c.mod)
		for _, p := range c.pats {
			if !strings.Contains(src, p) {
				t.Errorf("%s: defense %q absent", c.mod, p)
			}
		}
	}
}
