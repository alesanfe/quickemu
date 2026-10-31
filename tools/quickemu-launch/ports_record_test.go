package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
)

// Regression: portsRecord wrote the assignment record via unchecked
// fmt.Fprintln — a dropped record means the next VM can be handed the
// same host port with no warning at all.
func TestPortsRecordWriteFailureWarns(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "vm.ports")
	if err := os.Symlink("/dev/full", rec); err != nil {
		t.Skip("no symlink support")
	}
	out := qetest.CaptureStderr(t, func() {
		portsRecord(dir, "vm", "ssh,22,host", true)
	})
	if !strings.Contains(out, "could not record port") {
		t.Fatalf("missing write-failure warning, got %q", out)
	}
}

// FuzzParseConf: the conf parser must never panic on arbitrary input —
// it processes user-owned files whose bytes are not trusted.
func FuzzParseConf(f *testing.F) {
	f.Add("ram=\"4G\"\ncpu_cores=\"4\"\n")
	f.Add("guest_os=\"windows\"\ndisk_img=\"x.qcow2\"\n")
	f.Add("=\n\n\n")
	f.Add("\x00\x01\x02")
	f.Add("a=\"\xed\xa0\x80\"\n")
	f.Add(strings.Repeat("k=\"v\"\n", 4000))
	f.Fuzz(func(t *testing.T, s string) {
		p := filepath.Join(t.TempDir(), "f.conf")
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Skip()
		}
		_ = parseConf(p)
	})
}
