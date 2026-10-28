package qespec

import (
	"strings"
	"testing"
)

// ---- spec/security/adversarial_download_spec.sh ----
// Source-level audits: the download layer must keep the defenses the
// Bash web.sh enforced.

func TestAdversarialDownload(t *testing.T) {
	vm := SourceFile(t, "tools/quickget-vm/main.go")
	http := SourceFile(t, "tools/qelib/http.go")
	cli := SourceFile(t, "tools/quickget-cli/main.go")

	checks := []struct {
		name string
		src  string
		pats []string
	}{
		{"scheme restricted to http(s)", vm,
			[]string{"unsupported URL scheme", `u.Scheme != "http" && u.Scheme != "https"`}},
		{"redirects cannot leave http(s)", http,
			[]string{"redirect to disallowed scheme"}},
		{"redirect guard applied", vm, []string{"qelib.DLClient"}},
		{"HEAD->GET range fallback", http,
			[]string{"bytes=0-0", "MethodHead"}},
		{"resume appends partials", vm,
			[]string{"Range", "bytes=", "O_APPEND"}},
		{"trust can refuse below minimum", cli,
			[]string{"minimum-trust", "below --minimum-trust"}},
		{"trust D/E warns", cli,
			[]string{"warning: %s source integrity not verifiable"}},
	}
	for _, c := range checks {
		for _, p := range c.pats {
			if !strings.Contains(c.src, p) {
				t.Errorf("%s: pattern %q absent", c.name, p)
			}
		}
	}
	if !strings.Contains(vm, "attempt <= 3") && !strings.Contains(vm, "attempt <=3") {
		t.Error("download retry bound absent")
	}
}
