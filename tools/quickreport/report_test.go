// report_test.go — full report body via stub tools on PATH
// (quickemu --version, qemu-system-*, qemu-img, zsync, swtpm,
// virtiofsd) so every probe and both report formats execute.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubBin(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"quickemu", "qemu-img", "zsync",
		"swtpm", "virtiofsd", "qemu-system-x86_64"} {
		body := "#!/bin/sh\necho stub-1.0\n"
		if name == "qemu-system-x86_64" {
			body = "#!/bin/sh\nif [ \"$1\" = \"-cpu\" ]; then echo 'x86 cpu models'; else echo 'QEMU emulator version 8.2.0'; fi\n"
		}
		if name == "quickemu" {
			body = "#!/bin/sh\necho 5.0.0\n"
		}
		os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

func TestReportBody(t *testing.T) {
	stubBin(t)
	out, rc := runTool(t, t.TempDir())
	if rc != 0 {
		t.Fatalf("report rc=%d", rc)
	}
	for _, want := range []string{"Quickemu", "Distro:", "Kernel:",
		"Memory:", "CPU", "QEMU", "Companion"} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q\n%s", want, out)
		}
	}
	// JSON mode parses and carries the contract fields.
	out, rc = runTool(t, t.TempDir(), "--format", "json")
	if rc != 0 {
		t.Fatalf("json rc=%d", rc)
	}
	for _, want := range []string{`"schemaVersion":1`, `"command":"host.report"`,
		`"quickemu"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("json missing %q\n%s", want, out)
		}
	}
}

func TestReportMissingQuickemu(t *testing.T) {
	// Empty PATH → probeQuickemu fails → rc 1.
	t.Setenv("PATH", t.TempDir())
	out, rc := runTool(t, t.TempDir())
	if rc != 1 || !strings.Contains(out, "missing") {
		t.Fatalf("missing rc=%d %q", rc, out)
	}
}
