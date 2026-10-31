// export_err_test.go: archive write/read failures must propagate.
// A silently truncated export would later import as a corrupt VM;
// a silently truncated member would install a partial disk file.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// truncArchive builds a *valid gzip* stream wrapping a tar whose last
// member's data is cut after keepLast real bytes — the gzip layer is
// intact so readers only discover the truncation when the member's
// declared size is not delivered (io.ErrUnexpectedEOF). keepLast must
// be < len(payload) and is honoured exactly: tar member data is padded
// to 512-byte blocks, so naive "cut half the member" math lands in the
// padding and delivers the member intact.
func truncArchive(t *testing.T, members [][2]string, payloadName string, payload []byte, keepLast int) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	put := func(name string, body []byte) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range members {
		put(m[0], []byte(m[1]))
	}
	put(payloadName, payload)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	data := raw.Bytes()
	// The last member's data region is padded to a 512 multiple right
	// before the two zero trailer blocks (1024).
	padded := (len(payload) + 511) / 512 * 512
	cut := len(data) - 1024 - (padded - keepLast)
	if keepLast >= len(payload) || cut <= 0 {
		t.Fatal("keepLast must leave a truncated member inside the stream")
	}
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	if _, err := gz.Write(data[:cut]); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// Regression: exportVm reported success even when the final tar/gzip
// flush failed (writes to manifest/checksums and the deferred Close
// errors were all dropped). /dev/full fails every write at flush.
func TestExportFlushFailsClosed(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("/dev/full unavailable")
	}
	_, dir := mkVM(t, "fullvm")
	o, rc := runTool(t, nil, "export", dir, "/dev/full")
	if rc == 0 {
		t.Fatalf("export into /dev/full must fail, got rc=0: %s", o)
	}
}

// Regression: importVm dropped the io.Copy error while extracting —
// a truncated member installed a partial file when it was not listed
// in the checksums (the only later integrity gate).
func TestImportTruncatedMemberAborts(t *testing.T) {
	blob := truncArchive(t, [][2]string{
		{"truncvm.conf", "guest_os=\"linux\"\n"},
		{"quickvm-manifest.json", `{"formatVersion":1,"vm":"truncvm"}`},
		{"quickvm-checksums.sha256", ""}, // present but lists nothing
	}, "truncvm/disk.qcow2", make([]byte, 4096), 1000)
	in := filepath.Join(t.TempDir(), "truncvm.quickvm.tar.gz")
	if err := os.WriteFile(in, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	o, rc := runToolIn(t, nil, dest, "import", t.TempDir(), in)
	if rc == 0 {
		t.Fatalf("import of truncated archive must fail, got rc=0: %s", o)
	}
	if _, err := os.Stat(filepath.Join(dest, "truncvm", "disk.qcow2")); err == nil {
		t.Fatal("partial member must not be installed")
	}
}

// Guard: backupVerify must still reject an archive whose checksummed
// member is truncated — whether the digest is partial (old) or absent
// (new), it cannot equal the recorded sum.
func TestBackupVerifyRejectsTruncatedMember(t *testing.T) {
	full := make([]byte, 4096)
	for i := range full {
		full[i] = byte(i)
	}
	sums := fmt.Sprintf("%x  %s\n", sha256.Sum256(full), "v/disk.qcow2")
	blob := truncArchive(t, [][2]string{
		{"v.conf", "guest_os=\"linux\"\n"},
		{"quickvm-manifest.json", `{"formatVersion":1,"vm":"v"}`},
		{"quickvm-checksums.sha256", sums},
	}, "v/disk.qcow2", full, 1000)
	in := filepath.Join(t.TempDir(), "v.quickvm.tar.gz")
	if err := os.WriteFile(in, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	o, rc := runTool(t, nil, "backup-verify", t.TempDir(), in)
	if rc == 0 {
		t.Fatalf("verify of truncated member must fail, got rc=0: %s", o)
	}
}

// Regression: a truncated *checksums* member was read with the error
// dropped — the partial sums left the payload member unlisted, so a
// corrupt archive verified OK. The kept half must contain a *real*
// sum (v.conf) so the only failure source is the unlisted member.
func TestBackupVerifyTruncatedChecksums(t *testing.T) {
	confBody := "guest_os=\"linux\"\n"
	disk := []byte("disk-bytes")
	sums := fmt.Sprintf("%x  v.conf\n%x  v/disk.qcow2\n",
		sha256.Sum256([]byte(confBody)), sha256.Sum256(disk))
	blob := truncArchive(t, [][2]string{
		{"v.conf", confBody},
		{"quickvm-manifest.json", `{"formatVersion":1,"vm":"v"}`},
		{"v/disk.qcow2", string(disk)},
	}, "quickvm-checksums.sha256", []byte(sums), len(sums)/2)
	in := filepath.Join(t.TempDir(), "v.quickvm.tar.gz")
	if err := os.WriteFile(in, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	o, rc := runTool(t, nil, "backup-verify", t.TempDir(), in)
	if rc == 0 {
		t.Fatalf("verify must not pass with a truncated checksums member: %s", o)
	}
}
