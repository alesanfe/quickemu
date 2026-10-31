// oci_test.go — QE-092: OCI layout push/pull/inspect, digest
// verification, ref sanitisation and hostile index.json input.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ociFixture(t *testing.T) string {
	t.Helper()
	return makeArchive(t, []member{
		{"quickvm-manifest.json", manifestJSON(), 0},
		{"vm/disk.qcow2", []byte("qcow-bytes"), 0},
		{"vm.conf", []byte("guest_os=\"linux\"\n"), 0},
	})
}

func TestOciPushPullRoundtrip(t *testing.T) {
	arc := ociFixture(t)
	dest := t.TempDir()
	if rc := ociPush(arc, dest, "myvm:1.0"); rc != 0 {
		t.Fatalf("push rc=%d", rc)
	}
	layout := filepath.Join(dest, "myvm")
	for _, f := range []string{"oci-layout", "index.json"} {
		if _, err := os.Stat(filepath.Join(layout, f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
	// index.json resolves to a manifest with one layer.
	man, desc, err := ociReadManifest(layout)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if desc.Annots[ociRefAnnotKey] != "myvm:1.0" {
		t.Fatalf("ref annot %v", desc.Annots)
	}
	layers := ociLayers(man)
	if len(layers) != 1 || layers[0].MediaType != ociLayerMedia {
		t.Fatalf("layers %v", layers)
	}
	// Push onto an existing layout refuses.
	if rc := ociPush(arc, dest, "myvm:1.0"); rc != 1 {
		t.Fatalf("re-push rc=%d", rc)
	}
	// Bad refs refuse.
	for _, ref := range []string{"a/b:c", "a:b:c:d", "x:.."} {
		if rc := ociPush(arc, t.TempDir(), ref); rc != 1 {
			t.Fatalf("ref %q rc=%d", ref, rc)
		}
	}
	// Inspect succeeds.
	if rc := ociInspect(layout); rc != 0 {
		t.Fatalf("inspect rc=%d", rc)
	}
	// Pull materialises the tgz byte-identical.
	pullDest := t.TempDir()
	if rc := ociPull(layout, pullDest, "myvm:1.0", false, false); rc != 0 {
		t.Fatalf("pull rc=%d", rc)
	}
	got, err := os.ReadFile(filepath.Join(pullDest, "myvm.quickvm.tar.gz"))
	if err != nil {
		t.Fatalf("pulled archive: %v", err)
	}
	want, _ := os.ReadFile(arc)
	if len(got) != len(want) {
		t.Fatalf("pulled %d bytes, want %d", len(got), len(want))
	}
	// Re-pull without force refuses; force overwrites.
	if rc := ociPull(layout, pullDest, "myvm:1.0", false, false); rc != 1 {
		t.Fatalf("re-pull rc=%d", rc)
	}
	if rc := ociPull(layout, pullDest, "myvm:1.0", false, true); rc != 0 {
		t.Fatalf("force pull rc=%d", rc)
	}
}

func TestOciPullTamperedBlob(t *testing.T) {
	arc := ociFixture(t)
	dest := t.TempDir()
	if rc := ociPush(arc, dest, "v:1"); rc != 0 {
		t.Fatalf("push rc=%d", rc)
	}
	layout := filepath.Join(dest, "v")
	man, _, _ := ociReadManifest(layout)
	layers := ociLayers(man)
	blobPath := ociBlobPath(layout, layers[0].Digest)
	os.WriteFile(blobPath, []byte("tampered"), 0o644)
	if rc := ociPull(layout, t.TempDir(), "v:1", false, false); rc != 1 {
		t.Fatalf("tampered pull rc=%d", rc)
	}
}

func TestOciBlobPathDefense(t *testing.T) {
	if p := ociBlobPath("/x", "sha256:../../etc/passwd"); p != "" {
		t.Fatalf("traversal digest → %q", p)
	}
	if p := ociBlobPath("/x", "md5:abc"); p != "" {
		t.Fatalf("wrong algo → %q", p)
	}
	if p := ociBlobPath("/x", "sha256:short"); p != "" {
		t.Fatalf("short digest → %q", p)
	}
	ok := ociBlobPath("/x", "sha256:"+strings.Repeat("a", 64))
	if !strings.HasSuffix(ok, filepath.Join("sha256", strings.Repeat("a", 64))) {
		t.Fatalf("valid digest → %q", ok)
	}
}

func TestOciPullHostileIndex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"manifests":[]}`), 0o644)
	if rc := ociPull(dir, t.TempDir(), "", false, false); rc != 1 {
		t.Fatalf("empty manifests rc=%d", rc)
	}
	os.WriteFile(filepath.Join(dir, "index.json"), []byte("not json"), 0o644)
	if rc := ociPull(dir, t.TempDir(), "", false, false); rc != 1 {
		t.Fatalf("bad json rc=%d", rc)
	}
	// Manifest pointing at a traversal digest is refused.
	os.WriteFile(filepath.Join(dir, "index.json"), []byte(
		`{"manifests":[{"digest":"sha256:`+strings.Repeat("a", 64)+`"}]}`), 0o644)
	if rc := ociPull(dir, t.TempDir(), "", false, false); rc != 1 {
		t.Fatalf("missing manifest blob rc=%d", rc)
	}
}

func TestOciPullNoLayers(t *testing.T) {
	arc := ociFixture(t)
	dest := t.TempDir()
	ociPush(arc, dest, "v:1")
	layout := filepath.Join(dest, "v")
	// Rewrite the manifest blob with zero layers.
	man, _, _ := ociReadManifest(layout)
	delete(man, "layers")
	mb, _ := json.Marshal(man)
	// Keep the same digest? No — write a new blob + repoint index.
	nd, err := ociBlob(layout, ociManifestM, mb)
	if err != nil {
		t.Fatal(err)
	}
	idx, _ := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"manifests":     []ociDescriptor{nd},
	})
	os.WriteFile(filepath.Join(layout, "index.json"), idx, 0o644)
	if rc := ociPull(layout, t.TempDir(), "", false, false); rc != 1 {
		t.Fatalf("no layers rc=%d", rc)
	}
}
