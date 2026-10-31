// overlay_test.go — QE-042 base-image overlay coverage: installBase
// read-only install/reuse, makeOverlay qemu-img arg flow, backing-chain
// validation (missing/writable/corrupt base), explicit rebase and
// flatten. Uses real qemu-img when on PATH; the getOS --overlay
// integration runs against a PATH stub.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// qemuImgOrSkip returns the qemu-img binary or skips the test — the
// chain-correctness cases need a real qemu-img to trust their result.
func qemuImgOrSkip(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("qemu-img")
	if err != nil {
		t.Skip("qemu-img not available")
	}
	return bin
}

// fakeQcow writes a minimal qcow2 (magic + padding) — enough for
// qcowMagic; qemu-img calls are stubbed separately.
func fakeQcow(t *testing.T, path string) {
	t.Helper()
	b := append([]byte{'Q', 'F', 'I', 0xfb}, make([]byte, 1024)...)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// stubQemuImgJSON installs a PATH qemu-img that honours create/convert/
// rebase (touch the output file with QCOW magic) and info (emit a
// single-entry JSON descriptor).
func stubQemuImgJSON(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	stub := `#!/bin/sh
cmd="$1"; shift
dst=""; file=""
for a in "$@"; do case "$a" in -*) ;; *) last="$a";; esac; done
case "$cmd" in
create|convert) printf 'QFI\xfb' > "$last" ;;
rebase) printf 'QFI\xfb' > "$last" ;;
info) echo "[{\"filename\":\"$last\",\"format\":\"qcow2\",\"format-specific\":{\"type\":\"qcow2\",\"data\":{\"corrupt\":false}}}]" ;;
esac
exit 0
`
	p := filepath.Join(d, "qemu-img")
	if err := os.WriteFile(p, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	return p
}

func TestInstallBase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QE_BASE_CACHE", dir)
	src := filepath.Join(dir, "dl.qcow2")
	fakeQcow(t, src)
	base, err := installBase(src, "os-1-amd64.qcow2")
	if err != nil {
		t.Fatalf("installBase: %v", err)
	}
	if base != filepath.Join(dir, "os-1-amd64.qcow2") {
		t.Fatalf("base path %q", base)
	}
	st, _ := os.Stat(base)
	if st.Mode().Perm() != 0o444 {
		t.Fatalf("base must be read-only, mode=%o", st.Mode().Perm())
	}
	// Second install of the same name reuses the base and deletes the
	// duplicate download — never rewrites the shared image.
	src2 := filepath.Join(dir, "dl2.qcow2")
	fakeQcow(t, src2)
	base2, err := installBase(src2, "os-1-amd64.qcow2")
	if err != nil || base2 != base {
		t.Fatalf("reuse %v %q", err, base2)
	}
	if _, err := os.Stat(src2); err == nil {
		t.Fatal("duplicate download must be removed")
	}
	// A corrupt cached base is refused, not overlaid.
	os.WriteFile(filepath.Join(dir, "bad.qcow2"), []byte("notqcow"), 0o644)
	if _, err := installBase(src, "bad.qcow2"); err == nil {
		t.Fatal("corrupt base must fail")
	}
}

func TestMakeOverlayAndChain(t *testing.T) {
	qemuImgOrSkip(t)
	dir := t.TempDir()
	t.Setenv("QE_BASE_CACHE", filepath.Join(dir, "base"))
	src := filepath.Join(dir, "dl.qcow2")
	if err := exec.Command("qemu-img", "create", "-f", "qcow2", src, "8M").Run(); err != nil {
		t.Fatalf("qemu-img create: %v", err)
	}
	vm := filepath.Join(dir, "vm")
	os.MkdirAll(vm, 0o755)
	dst := filepath.Join(vm, "disk.qcow2")
	base, err := makeOverlay(src, dst, "os-1-amd64.qcow2")
	if err != nil {
		t.Fatalf("makeOverlay: %v", err)
	}
	// The overlay points at the cached read-only base.
	if err := checkBackingOverlay(dst); err != nil {
		t.Fatalf("fresh overlay chain must validate: %v", err)
	}
	info, err := qemuImgJSON(dst)
	if err != nil {
		t.Fatalf("qemuImgJSON: %v", err)
	}
	if bf, _ := info["full-backing-filename"].(string); !strings.HasSuffix(bf, "os-1-amd64.qcow2") {
		t.Fatalf("overlay backing %q want %q", bf, base)
	}
	if st, _ := os.Stat(base); st.Mode().Perm()&0o222 != 0 {
		t.Fatal("base must be read-only")
	}
	// A writable base breaks the contract — start-up validation fires.
	os.Chmod(base, 0o644)
	if err := checkBackingOverlay(dst); err == nil {
		t.Fatal("writable base must fail validation")
	}
	os.Chmod(base, 0o444)
	// Rebase onto another read-only base: explicit and validated.
	nb := filepath.Join(dir, "newbase.qcow2")
	if err := exec.Command("qemu-img", "create", "-f", "qcow2", nb, "8M").Run(); err != nil {
		t.Fatal(err)
	}
	os.Chmod(nb, 0o444)
	if err := rebaseOverlay(dst, nb); err != nil {
		t.Fatalf("rebaseOverlay: %v", err)
	}
	if err := checkBackingOverlay(dst); err != nil {
		t.Fatalf("rebased chain: %v", err)
	}
	// Flatten produces a standalone image with no backing.
	out := filepath.Join(dir, "flat.qcow2")
	if err := flattenOverlay(dst, out); err != nil {
		t.Fatalf("flattenOverlay: %v", err)
	}
	fi, err := qemuImgJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	if bf, _ := fi["full-backing-filename"].(string); bf != "" {
		t.Fatalf("flattened image keeps backing %q", bf)
	}
	// Deleting the base is detected on the next check.
	os.Remove(nb)
	if err := checkBackingOverlay(dst); err == nil {
		t.Fatal("missing base must fail validation")
	}
}

func TestRebaseRejectsNonQcowAndWritable(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.qcow2")
	os.WriteFile(bad, []byte("junk"), 0o644)
	if err := rebaseOverlay(filepath.Join(dir, "ov.qcow2"), bad); err == nil {
		t.Fatal("non-qcow2 base must fail")
	}
	ov := filepath.Join(dir, "ov.qcow2")
	fakeQcow(t, ov)
	wb := filepath.Join(dir, "wb.qcow2")
	fakeQcow(t, wb) // 0644: writable
	if err := rebaseOverlay(ov, wb); err == nil {
		t.Fatal("writable new base must fail")
	}
}

func TestOverlaySubcommands(t *testing.T) {
	stubQemuImgJSON(t)
	dir := t.TempDir()
	t.Setenv("QE_BASE_CACHE", filepath.Join(dir, "base"))
	src := filepath.Join(dir, "dl.qcow2")
	fakeQcow(t, src)
	vm := filepath.Join(dir, "vm")
	if rc := cmdOverlay([]string{vm, src}); rc != 0 {
		t.Fatalf("overlay rc=%d", rc)
	}
	dst := filepath.Join(vm, "disk.qcow2")
	if _, err := os.Stat(dst); err != nil {
		t.Fatal("overlay disk not created")
	}
	if rc := cmdCheckBase([]string{dst}); rc != 0 {
		t.Fatalf("checkbase rc=%d", rc)
	}
	if rc := cmdCheckBase([]string{}); rc != 2 {
		t.Fatalf("checkbase usage rc=%d", rc)
	}
	if rc := cmdCheckBase([]string{filepath.Join(dir, "nope.qcow2")}); rc != 1 {
		t.Fatalf("checkbase missing rc=%d", rc)
	}
	nb := filepath.Join(dir, "nb.qcow2")
	fakeQcow(t, nb)
	os.Chmod(nb, 0o444)
	if rc := cmdRebase([]string{dst, nb}); rc != 0 {
		t.Fatalf("rebase rc=%d", rc)
	}
	if rc := cmdRebase([]string{dst}); rc != 2 {
		t.Fatalf("rebase usage rc=%d", rc)
	}
	out := filepath.Join(dir, "flat.qcow2")
	if rc := cmdFlatten([]string{dst, out}); rc != 0 {
		t.Fatalf("flatten rc=%d", rc)
	}
	if rc := cmdFlatten([]string{dst}); rc != 2 {
		t.Fatalf("flatten usage rc=%d", rc)
	}
	// Non-qcow2 source refused before qemu-img runs.
	junk := filepath.Join(dir, "junk.qcow2")
	os.WriteFile(junk, []byte("x"), 0o644)
	if rc := cmdOverlay([]string{filepath.Join(dir, "v2"), junk}); rc != 1 {
		t.Fatalf("overlay non-qcow2 rc=%d", rc)
	}
	if rc := cmdOverlay([]string{}); rc != 2 {
		t.Fatalf("overlay usage rc=%d", rc)
	}
}

// getOS --overlay routes the downloaded qcow2 into the base cache and
// makes the VM disk an overlay; the conf points at disk.qcow2.
func TestGetOSOverlay(t *testing.T) {
	stubQemuImgJSON(t)
	payload := append([]byte{'Q', 'F', 'I', 0xfb}, make([]byte, 2048)...)
	srv := newTestServer(payload)
	defer srv.Close()
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-catalog"),
		[]byte("#!/bin/sh\necho '"+srv.URL+"/img.qcow2'\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("QE_BASE_CACHE", filepath.Join(t.TempDir(), "basecache"))

	dir := filepath.Join(t.TempDir(), "ovm")
	old := overlayFlag
	overlayFlag = true
	t.Cleanup(func() { overlayFlag = old })
	if rc := getOS("testos-cloud", "1.0", "", dir, "amd64"); rc != 0 {
		t.Fatal("getOS --overlay failed")
	}
	if _, err := os.Stat(filepath.Join(dir, "disk.qcow2")); err != nil {
		t.Fatal("overlay disk missing")
	}
	conf, _ := os.ReadFile(dir + ".conf")
	if !strings.Contains(string(conf), `disk_img="`) || !strings.Contains(string(conf), "disk.qcow2") {
		t.Fatalf("conf must point at the overlay: %s", conf)
	}
	// The base landed in the shared cache, read-only.
	b := filepath.Join(os.Getenv("QE_BASE_CACHE"), baseImageName("testos-cloud", "1.0", "amd64"))
	st, err := os.Stat(b)
	if err != nil || st.Mode().Perm() != 0o444 {
		t.Fatalf("cached base %v mode %v", err, st)
	}
}
