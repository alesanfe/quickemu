// migrate_test.go: QE-111 migrate flow — backup, diff-before-write,
// dry-run purity, warning emission.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateWritesBackupAndToml(t *testing.T) {
	dir := t.TempDir()
	conf := writeFile(t, dir, "vm.conf",
		"guest_os=\"windows\"\ndisk_img=\"w/d.qcow2\"\ntpm=\"on\"\ncustom_thing=\"yes\"\n", 0o644)
	res, err := migrateConf(conf, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.Backup); err != nil {
		t.Fatalf("no .conf.bak: %v", err)
	}
	bb, _ := os.ReadFile(res.Backup)
	if string(bb) != "guest_os=\"windows\"\ndisk_img=\"w/d.qcow2\"\ntpm=\"on\"\ncustom_thing=\"yes\"\n" {
		t.Fatal("backup content differs from source conf")
	}
	tb, err := os.ReadFile(res.Out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tb), "schema_version = 1") ||
		!strings.Contains(string(tb), "[extra]") {
		t.Fatalf("bad toml:\n%s", tb)
	}
	if !strings.Contains(res.Diff, "+schema_version = 1") ||
		!strings.Contains(res.Diff, "--- a/vm.toml") {
		t.Fatalf("diff missing:\n%s", res.Diff)
	}
}

func TestMigrateDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	conf := writeFile(t, dir, "vm.conf", "guest_os=\"linux\"\n", 0o644)
	res, err := migrateConf(conf, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.Out); err == nil {
		t.Fatal("dry-run wrote the .toml")
	}
	if _, err := os.Stat(res.Backup); err == nil {
		t.Fatal("dry-run wrote the .bak")
	}
	if res.Diff == "" {
		t.Fatal("dry-run must still show the diff")
	}
}

func TestMigrateDiffsExistingToml(t *testing.T) {
	dir := t.TempDir()
	conf := writeFile(t, dir, "vm.conf", "guest_os=\"linux\"\nram=\"8G\"\n", 0o644)
	writeFile(t, dir, "vm.toml", "schema_version = 1\nguest_os = \"linux\"\nram = \"4G\"\n", 0o644)
	res, err := migrateConf(conf, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Diff, `-ram = "4G"`) || !strings.Contains(res.Diff, `+ram = "8G"`) {
		t.Fatalf("diff lacks old->new:\n%s", res.Diff)
	}
}

func TestMigrateWarnsNonMigratable(t *testing.T) {
	dir := t.TempDir()
	conf := writeFile(t, dir, "vm.conf",
		"guest_os=\"linux\"\n$(evil)\nexport side_key=\"1\"\n", 0o644)
	res, err := migrateConf(conf, true)
	if err != nil {
		t.Fatal(err)
	}
	var joined = strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "line 2") || !strings.Contains(joined, "export") {
		t.Fatalf("warnings missing: %v", res.Warnings)
	}
	// export-prefixed keys still migrate (prefix dropped, warned).
	if !strings.Contains(res.Diff, `side_key`) {
		t.Fatalf("exported key lost:\n%s", res.Diff)
	}
}

func TestMigrateCLIEndToEnd(t *testing.T) {
	dir := t.TempDir()
	conf := writeFile(t, dir, "cli.conf", "guest_os=\"linux\"\n", 0o644)
	out, se, rc := runTool(t, "migrate", "--file", conf, "--dry-run")
	if rc != 0 || !strings.Contains(out, "Dry run") || se != "" {
		t.Fatalf("rc=%d se=%q out=%q", rc, se, out)
	}
	out, _, rc = runTool(t, "migrate", "--file", conf)
	if rc != 0 || !strings.Contains(out, "Migrated") {
		t.Fatalf("rc=%d out=%q", rc, out)
	}
	for _, p := range []string{conf + ".bak", filepath.Join(dir, "cli.toml")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s", p)
		}
	}
	// Second migrate shows the up-to-date path.
	out, _, _ = runTool(t, "migrate", "--file", conf)
	if !strings.Contains(out, "up to date") {
		t.Fatalf("expected no-change path: %q", out)
	}
	if _, _, rc := runTool(t, "migrate", "--file", "/nonexistent.conf"); rc != 1 {
		t.Fatal("missing conf must fail")
	}
}

func TestSetCLI(t *testing.T) {
	dir := t.TempDir()
	conf := writeFile(t, dir, "s.conf", "guest_os=\"linux\"\n", 0o644)
	out, _, rc := runTool(t, "set", "--file", conf, "--key", "ram", "--value", "8G")
	if rc != 0 || !strings.Contains(out, `"set":true`) {
		t.Fatalf("rc=%d out=%q", rc, out)
	}
	b, _ := os.ReadFile(conf)
	if !strings.Contains(string(b), `ram="8G"`) {
		t.Fatalf("not patched:\n%s", b)
	}
	if _, _, rc := runTool(t, "set", "--file", conf); rc != 1 {
		t.Fatal("set without --key must fail")
	}
}

func TestToConfCLI(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "v.toml", "schema_version = 1\nguest_os = \"linux\"\n", 0o644)
	out, _, rc := runTool(t, "to-conf", "--file", p)
	if rc != 0 || !strings.Contains(out, `guest_os="linux"`) {
		t.Fatalf("rc=%d out=%q", rc, out)
	}
	if _, _, rc := runTool(t, "to-conf", "--file", "/nonexistent"); rc != 1 {
		t.Fatal("missing file must fail")
	}
}
