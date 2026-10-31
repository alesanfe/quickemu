// edit_test.go: QE-112 — transactional edit coverage: validation,
// selective patching, permission preservation, concurrent-modification
// guard and .hist history.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string, perm os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), perm); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConfSetPatchesOnlyOneKey(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vm.conf",
		"# header\nguest_os=\"linux\" # guest\nssh_port=\"2222\"\nram=\"4G\"\n", 0o644)
	if err := ConfSet(p, "ssh_port", "2223", nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	got := string(b)
	if !strings.Contains(got, "ssh_port=\"2223\"") ||
		!strings.Contains(got, "# header") ||
		!strings.Contains(got, `guest_os="linux" # guest`) ||
		!strings.Contains(got, `ram="4G"`) {
		t.Fatalf("patch was not selective:\n%s", got)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o644 {
		t.Fatalf("perm changed: %v", st.Mode().Perm())
	}
}

func TestConfSetAppendsMissingKey(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vm.conf", "guest_os=\"linux\"", 0o600)
	if err := ConfSet(p, "tpm", "on", nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.HasSuffix(string(b), "guest_os=\"linux\"\ntpm=\"on\"\n") {
		t.Fatalf("append failed:\n%s", b)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("0600 lost: %v", st.Mode().Perm())
	}
}

func TestConfSetListAndValidation(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vm.conf", "port_forwards=(\"1:1\")\n", 0o644)
	if err := ConfSet(p, "port_forwards", "2222:22,8080:80", nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `port_forwards=("2222:22" "8080:80")`) {
		t.Fatalf("array not emitted:\n%s", b)
	}
	// Invalid values and keys must be refused.
	for _, bad := range [][2]string{
		{"ssh_port", "99999"}, {"ssh_port", "abc"},
		{"macaddr", "zz:zz"}, {"boot", "sideways"},
		{"tpm", "maybe"}, {"bad key!", "x"},
	} {
		if err := ConfSet(p, bad[0], bad[1], nil); err == nil {
			t.Fatalf("ConfSet(%s=%s) accepted", bad[0], bad[1])
		}
	}
	// Unknown keys pass through (they are preserved verbatim anyway).
	if err := ConfSet(p, "my_flag", "anything", nil); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	if !strings.Contains(string(b), `my_flag="anything"`) {
		t.Fatalf("unknown key not appended:\n%s", b)
	}
}

func TestConfSetGuardDetectsRace(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vm.conf", "guest_os=\"linux\"\n", 0o644)
	g, err := NewEditGuard(p)
	if err != nil {
		t.Fatal(err)
	}
	// A concurrent writer lands between snapshot and commit.
	if err := os.WriteFile(p, []byte("guest_os=\"windows\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = ConfSet(p, "ram", "8G", &SetOptions{Guard: g})
	if !errors.Is(err, ErrConcurrent) {
		t.Fatalf("guard missed concurrent write: %v", err)
	}
	// Unchanged file + guard -> commit succeeds.
	g2, _ := NewEditGuard(p)
	if err := ConfSet(p, "ram", "8G", &SetOptions{Guard: g2}); err != nil {
		t.Fatal(err)
	}
}

func TestConfSetHistory(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vm.conf", "ram=\"4G\"\n", 0o644)
	for i, v := range []string{"8G", "16G", "32G"} {
		if err := ConfSet(p, "ram", v, &SetOptions{History: 2}); err != nil {
			t.Fatalf("set %d: %v", i, err)
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".hist", "vm.conf.*"))
	if len(matches) != 2 {
		t.Fatalf("history kept %d generations, want 2", len(matches))
	}
}

func TestTomlSetPatchesAndPreserves(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vm.toml",
		"schema_version = 1\n\n# display block\ndisplay = \"sdl\" # ui\nssh_port = 2222\n\n[extra]\nfoo = \"bar\"\n",
		0o600)
	if err := TomlSet(p, "ssh_port", "2223", nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	got := string(b)
	for _, want := range []string{
		"ssh_port = 2223", "# display block", "display = \"sdl\" # ui", "schema_version = 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("perm lost: %v", st.Mode().Perm())
	}
}

func TestTomlSetInsertsRootBeforeSections(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vm.toml",
		"schema_version = 1\nguest_os = \"linux\"\n\n[extra]\nfoo = \"bar\"\n", 0o644)
	if err := TomlSet(p, "ram", "8G", nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	lines := strings.Split(string(b), "\n")
	ramAt, extraAt := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "ram =") {
			ramAt = i
		}
		if l == "[extra]" {
			extraAt = i
		}
	}
	if ramAt < 0 || extraAt < 0 || ramAt > extraAt {
		t.Fatalf("root key must land before [extra]:\n%s", b)
	}
}

func TestTomlSetDottedAndNewSection(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vm.toml", "schema_version = 1\nguest_os = \"linux\"\n", 0o644)
	if err := TomlSet(p, "extra.hook", "on", nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "[extra]") || !strings.Contains(string(b), `hook = "on"`) {
		t.Fatalf("section not created:\n%s", b)
	}
	if err := TomlSet(p, "extra.hook", "off", nil); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	if !strings.Contains(string(b), `hook = "off"`) || strings.Count(string(b), "[extra]") != 1 {
		t.Fatalf("in-place section patch failed:\n%s", b)
	}
	// Foreign sections are refused.
	if err := TomlSet(p, "other.x", "1", nil); err == nil {
		t.Fatal("foreign section accepted")
	}
}

func TestTomlSetValueTypes(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vm.toml", "schema_version = 1\n", 0o644)
	if err := TomlSet(p, "cpu_cores", "4", nil); err != nil {
		t.Fatal(err)
	}
	if err := TomlSet(p, "port_forwards", "2222:22,8080:80", nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "cpu_cores = 4") {
		t.Fatalf("int not emitted bare:\n%s", b)
	}
	if !strings.Contains(string(b), `port_forwards = ["2222:22", "8080:80"]`) {
		t.Fatalf("array not emitted:\n%s", b)
	}
	// Bad value -> refused before touching the file.
	if err := TomlSet(p, "cpu_cores", "four", nil); err == nil {
		t.Fatal("invalid int accepted")
	}
	b, _ = os.ReadFile(p)
	if strings.Contains(string(b), "four") {
		t.Fatal("file modified after refused set")
	}
}
