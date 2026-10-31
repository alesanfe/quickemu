// diagnose_test.go: QE-110 — compiler-style line:col diagnostics.
package main

import (
	"strings"
	"testing"
)

func diagMsgs(ds []Diag) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.String() + "\n")
	}
	return b.String()
}

func TestDiagnosticsLineCol(t *testing.T) {
	src := "schema_version = 1\n" +
		"guest_os = \"linux\"\n" +
		"ssh_port = \"abc\"\n" +
		"macaddr = \"not-a-mac\"\n"
	ds := validateTOMLDoc("vm.toml", []byte(src))
	msg := diagMsgs(ds)
	// ssh_port error must point at line 3, at the value column (12).
	foundSSH := false
	for _, d := range ds {
		if strings.Contains(d.Message, "ssh_port") {
			foundSSH = true
			if d.Line != 3 || d.Col != 12 {
				t.Fatalf("ssh_port at %d:%d, want 3:12 (%s)", d.Line, d.Col, d)
			}
		}
	}
	if !foundSSH {
		t.Fatalf("no ssh_port diagnostic:\n%s", msg)
	}
	if !strings.Contains(msg, "vm.toml:4:") {
		t.Fatalf("macaddr diagnostic missing position:\n%s", msg)
	}
	if !hasErrors(ds) {
		t.Fatal("errors expected")
	}
}

func TestDiagnosticsMissingVersion(t *testing.T) {
	ds := validateTOMLDoc("x.toml", []byte("guest_os = \"linux\"\n"))
	found := false
	for _, d := range ds {
		if d.Line == 1 && d.Col == 1 && strings.Contains(d.Message, "schema_version") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing schema_version not reported:\n%s", diagMsgs(ds))
	}
}

func TestDiagnosticsDeprecatedFormatVersion(t *testing.T) {
	ds := validateTOMLDoc("x.toml", []byte("formatVersion = 1\nguest_os = \"linux\"\n"))
	// Deprecated alias satisfies the requirement but warns.
	if hasErrors(ds) {
		t.Fatalf("formatVersion should not error:\n%s", diagMsgs(ds))
	}
	found := false
	for _, d := range ds {
		if d.Severity == "warning" && strings.Contains(d.Message, "deprecated") {
			found = true
		}
	}
	if !found {
		t.Fatal("no deprecation warning")
	}
}

func TestDiagnosticsUnknownKeysAndSections(t *testing.T) {
	src := "schema_version = 1\n" +
		"weird_key = 1\n" +
		"[mystery]\nx = 1\n" +
		"[extra]\nanything_goes = \"yes\"\n" +
		"[runtime]\npid = 1\n"
	ds := validateTOMLDoc("x.toml", []byte(src))
	msg := diagMsgs(ds)
	if !strings.Contains(msg, "unknown key") || !strings.Contains(msg, "unknown section") {
		t.Fatalf("expected unknown key+section warnings:\n%s", msg)
	}
	if hasErrors(ds) {
		t.Fatalf("warnings must not be errors:\n%s", msg)
	}
}

func TestDiagnosticsSyntaxError(t *testing.T) {
	ds := validateTOMLDoc("x.toml", []byte("schema_version = 1\nk = [unclosed\n"))
	if len(ds) == 0 || ds[0].Severity != "error" || ds[0].Line != 2 {
		t.Fatalf("syntax error not positioned: %v", ds)
	}
	// Compiler-style: the string form must read file:line:col.
	if !strings.Contains(ds[0].String(), "x.toml:2:") {
		t.Fatalf("bad diag string: %s", ds[0])
	}
}

func TestDiagnosticsValueKinds(t *testing.T) {
	src := "schema_version = 1\n" +
		"arch = \"sparc\"\n" + // enum violation
		"ram = \"lots\"\n" + // size violation
		"tpm = \"perhaps\"\n" + // bool violation
		"port_forwards = [\"xx\"]\n" + // fwd shape violation
		"display = \"wayland\"\n" + // enum violation
		"disk_size = \"64G\"\n" // ok
	ds := validateTOMLDoc("x.toml", []byte(src))
	var bad []string
	for _, d := range ds {
		bad = append(bad, d.Message)
	}
	for _, k := range []string{"arch", "ram", "tpm", "port_forwards", "display"} {
		hit := false
		for _, m := range bad {
			if strings.Contains(m, k+":") {
				hit = true
			}
		}
		if !hit {
			t.Fatalf("no diagnostic for %s:\n%s", k, diagMsgs(ds))
		}
	}
}
