package qespec

import (
	"os"
	"path/filepath"
	"testing"
)

// ---- spec/functional/quickreport_entry_spec.sh ----

func TestQuickreportComposition(t *testing.T) {
	WithFakeBin(t) // fake qemu-system-x86_64 + quickemu on PATH
	r := Run(t, "quickreport", nil)
	if r.Failed() {
		t.Fatalf("quickreport rc=%d\n%s", r.Code, r.Combined())
	}
	r.Contains(t, "Quickemu", "Memory", "QEMU", "CPU", "Quickemu 5.0.0")
}

func TestQuickreportMissingQuickemu(t *testing.T) {
	// Copy the real binary into an isolated dir: sibling lookups then
	// miss and PATH (fake qemu + coreutils only) has no quickemu.
	iso := t.TempDir()
	nb := t.TempDir()
	src, err := os.ReadFile(Bin(t, "quickreport"))
	if err != nil {
		t.Fatal(err)
	}
	isoBin := filepath.Join(iso, "quickreport")
	os.WriteFile(isoBin, src, 0o755)

	qemuStub := filepath.Join(nb, "qemu-system-x86_64")
	os.WriteFile(qemuStub,
		[]byte("#!/bin/sh\necho 'QEMU emulator version 8.2.2'\n"), 0o755)

	r := RunArgv(t, isoBin,
		[]RunOpt{Env("PATH=" + nb + string(os.PathListSeparator) + "/usr/bin:/bin")})
	if r.OK() {
		t.Error("quickreport succeeded without quickemu")
	}
	r.Contains(t, "Quickemu missing")
}
