package main

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Build a synthetic CNKL chunklist (signature_method=2) + dmg and verify.
func TestCheckChunklist(t *testing.T) {
	dir := t.TempDir()
	dmgData := []byte("fake dmg payload bytes - chunk0")
	dmgData = append(dmgData, []byte("chunk one body")...)
	sizes := []uint32{31, 14}
	h := sha256.New()
	hdr := make([]byte, 36)
	copy(hdr[:4], "CNKL")
	binary.LittleEndian.PutUint32(hdr[4:8], 36)
	hdr[8], hdr[9], hdr[10] = 1, 1, 2
	binary.LittleEndian.PutUint64(hdr[12:20], uint64(len(sizes)))
	binary.LittleEndian.PutUint64(hdr[20:28], 36)
	binary.LittleEndian.PutUint64(hdr[28:36], 36+36*uint64(len(sizes)))
	h.Write(hdr)
	pos := 0
	for _, s := range sizes {
		var rec [36]byte
		binary.LittleEndian.PutUint32(rec[:4], s)
		sum := sha256.Sum256(dmgData[pos : pos+int(s)])
		copy(rec[4:36], sum[:])
		h.Write(rec[:])
		pos += int(s)
	}
	cl := append(hdr, []byte{}...)
	pos = 0
	for _, s := range sizes {
		var rec [36]byte
		binary.LittleEndian.PutUint32(rec[:4], s)
		sum := sha256.Sum256(dmgData[pos : pos+int(s)])
		copy(rec[4:36], sum[:])
		cl = append(cl, rec[:]...)
		pos += int(s)
	}
	cl = append(cl, h.Sum(nil)...)

	dmgPath := filepath.Join(dir, "RecoveryImage.dmg")
	clPath := filepath.Join(dir, "RecoveryImage.chunklist")
	if err := os.WriteFile(dmgPath, dmgData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clPath, cl, 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkChunklist(dmgPath, clPath); err != nil {
		t.Fatalf("valid chunklist rejected: %v", err)
	}
	// Corrupt one dmg byte -> must fail.
	corrupt := append([]byte(nil), dmgData...)
	corrupt[3] ^= 0xFF
	if err := os.WriteFile(dmgPath, corrupt, 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkChunklist(dmgPath, clPath); err == nil {
		t.Fatal("corrupted dmg passed verification")
	}
	// Trailing byte in dmg -> must fail.
	if err := os.WriteFile(dmgPath, append(corrupt, 0), 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkChunklist(dmgPath, clPath); err == nil {
		t.Fatal("dmg with trailing bytes passed verification")
	}
}

// A hostile mirror can claim a chunkCount far beyond the file's real
// contents while keeping sigOffset arithmetically consistent — the
// record slice is capacity-allocated from that count BEFORE any
// signature check (make([]chunk, 0, 1<<32) == ~150GiB, instant OOM).
// The count is only trustworthy once sigOffset+sigLen == file size.
func TestCheckChunklistInflatedCount(t *testing.T) {
	dir := t.TempDir()
	dmgPath := filepath.Join(dir, "RecoveryImage.dmg")
	clPath := filepath.Join(dir, "RecoveryImage.chunklist")
	os.WriteFile(dmgPath, []byte("x"), 0o644)
	hdr := make([]byte, 36)
	copy(hdr[:4], "CNKL")
	binary.LittleEndian.PutUint32(hdr[4:8], 36)
	hdr[8], hdr[9], hdr[10] = 1, 1, 2
	binary.LittleEndian.PutUint64(hdr[12:20], 1<<22) // 4M records claimed
	binary.LittleEndian.PutUint64(hdr[20:28], 36)
	binary.LittleEndian.PutUint64(hdr[28:36], 36+36*(1<<22))
	os.WriteFile(clPath, hdr, 0o644) // real file: header only, 36B
	err := checkChunklist(dmgPath, clPath)
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("inflated chunkCount not rejected by geometry: %v", err)
	}
}
