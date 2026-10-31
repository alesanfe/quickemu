package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Regression: genSeedISO appended `seed=` to the conf via
// `f, _ := os.OpenFile(...)` + unchecked Fprintf/Close — a nil f or a
// failed write still printed "seed.iso created" and returned 0 while
// the conf silently lacked the seed line (NoCloud never applied).
// A symlink to /dev/full accepts the open but fails the write.
func TestGenSeedISOConfAppendFailure(t *testing.T) {
	vmPath := t.TempDir()
	conf := filepath.Join(vmPath, "vm.conf")
	if err := os.Symlink("/dev/full", conf); err != nil {
		t.Skip("no symlink support")
	}
	if rc := genSeedISO(vmPath, conf); rc != 1 {
		t.Fatalf("genSeedISO rc=%d, want 1 on unwritable conf", rc)
	}
}

// Regression: extractZip ignored io.Copy/w.Close errors — a member
// whose stored bytes are corrupt (CRC mismatch at read end) produced a
// partial file while extractZip returned true.
func TestExtractZipCorruptMember(t *testing.T) {
	payload := []byte("vmlinuz-payload-that-must-arrive-complete")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// Store (no compression) so the member bytes sit verbatim in the
	// archive and can be corrupted deterministically.
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "kernel.img",
		Method: zip.Store,
	})
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	raw := buf.Bytes()
	// Corrupt the first byte of the stored member data — the zip
	// reader detects the CRC mismatch when the member is drained.
	i := bytes.Index(raw, payload)
	if i < 0 {
		t.Fatalf("payload not found in zip (compressed differently?)")
	}
	raw[i] ^= 0xFF
	zf := filepath.Join(t.TempDir(), "img.zip")
	if err := os.WriteFile(zf, raw, 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	dest := t.TempDir()
	if extractZip(zf, dest) {
		t.Fatalf("extractZip accepted a corrupt member")
	}
	if _, err := os.Stat(filepath.Join(dest, "kernel.img")); !os.IsNotExist(err) {
		t.Fatalf("partial member left behind: %v", err)
	}
}

// Regression: extractZip had no bound on a member's claimed size —
// a remote bomb could fill the disk before any truncation check ran.
func TestExtractZipOversizeMember(t *testing.T) {
	old := extractMaxMember
	extractMaxMember = 8
	defer func() { extractMaxMember = old }()

	payload := []byte("bigger-than-cap-member-payload")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "big.img",
		Method: zip.Store,
	})
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	zf := filepath.Join(t.TempDir(), "big.zip")
	if err := os.WriteFile(zf, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	dest := t.TempDir()
	extractZip(zf, dest)
	if _, err := os.Stat(filepath.Join(dest, "big.img")); !os.IsNotExist(err) {
		t.Fatalf("oversize member was extracted: %v", err)
	}
}
