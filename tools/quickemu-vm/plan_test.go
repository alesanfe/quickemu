package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConfGet(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "vm.conf"),
		[]byte("guest_os=\"linux\"\ndisk_img='d.qcow2'\nempty=\n"), 0o600)
	if got := confGet(dir, "guest_os"); got != "linux" {
		t.Fatalf("confGet=%q", got)
	}
	if got := confGet(dir, "disk_img"); got != "d.qcow2" {
		t.Fatalf("confGet single-quote=%q", got)
	}
	if got := confGet(dir, "missing"); got != "" {
		t.Fatalf("missing key=%q", got)
	}
	if got := confGet(filepath.Join(dir, "nope"), "x"); got != "" {
		t.Fatal("no conf dir must give empty")
	}
}

func TestConfDisk(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "vm.conf"),
		[]byte("disk_img=\"sub/d.qcow2\"\n"), 0o600)
	got := confDisk(dir)
	want := filepath.Join(dir, "sub/d.qcow2")
	if got != want {
		t.Fatalf("relative disk must join dir: %q want %q", got, want)
	}
	abs := filepath.Join(dir, "abs.qcow2")
	os.WriteFile(filepath.Join(dir, "vm.conf"),
		[]byte("disk_img=\""+abs+"\"\n"), 0o600)
	if got := confDisk(dir); got != abs {
		t.Fatalf("absolute disk must pass through: %q want %q", got, abs)
	}
}

// A relative disk_img must stay inside the VM dir — compact, resize
// and delete-disk act on the resolved path, so ../ would let a
// crafted conf move or destroy files outside the VM.
func TestConfDiskRejectsEscapes(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"../out.qcow2", "sub/../../out.qcow2", ".."} {
		os.WriteFile(filepath.Join(dir, "vm.conf"),
			[]byte("disk_img=\""+v+"\"\n"), 0o600)
		if got := confDisk(dir); got != "" {
			t.Fatalf("disk_img %q escaped vmdir: %q", v, got)
		}
	}
	// A '..' that resolves back inside is still legal.
	os.WriteFile(filepath.Join(dir, "vm.conf"),
		[]byte("disk_img=\"sub/../d.qcow2\"\n"), 0o600)
	if got := confDisk(dir); got != filepath.Join(dir, "d.qcow2") {
		t.Fatalf("in-dir .. must resolve normally: %q", got)
	}
}

// deletePlanItems must exclude lock/runtime/socket sidecars and put
// the conf first.
func TestDeletePlanItemsExclusions(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{
		"vm.conf", "disk.qcow2",
		"vm.pid", "vm.ports", "vm.sock", "vm.paused",
		"vm.runtime.json", "vm.spice",
	} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600)
	}
	items := deletePlanItems(dir)
	base := func(p string) string { return filepath.Base(p) }
	for _, it := range items {
		if b := base(it); strings.HasSuffix(b, ".pid") ||
			strings.HasSuffix(b, ".sock") || strings.HasSuffix(b, ".ports") ||
			strings.HasSuffix(b, ".paused") || strings.HasSuffix(b, ".spice") ||
			strings.HasSuffix(b, ".runtime.json") {
			t.Fatalf("sidecar %s must be excluded from delete plan", it)
		}
	}
	if len(items) == 0 || base(items[0]) != "vm.conf" {
		t.Fatalf("conf must head the plan: %v", items)
	}
	if !slices.ContainsFunc(items, func(p string) bool { return base(p) == "disk.qcow2" }) {
		t.Fatal("disk must be in the plan")
	}
}

// The plan id is a content hash — same tree → same id, any change →
// different id (guards against deleting the wrong backup).
func TestDeletePlanIDDeterministic(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "d.qcow2")
	os.WriteFile(f, []byte("a"), 0o600)
	items := []string{f}
	id1 := deletePlanID("vm", items)
	id2 := deletePlanID("vm", items)
	if id1 != id2 || !strings.HasPrefix(id1, "sha256:") {
		t.Fatalf("plan id not deterministic: %q vs %q", id1, id2)
	}
	if deletePlanID("other", items) == id1 {
		t.Fatal("vm name must feed the hash")
	}
	os.WriteFile(f, []byte("changed"), 0o600)
	if deletePlanID("vm", items) == id1 {
		t.Fatal("content change must change the plan id")
	}
}
