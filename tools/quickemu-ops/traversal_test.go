package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression: --id is joined into ops/ unchecked — "../x" would read
// or overwrite files outside the ops dir. checkID must die() on any
// id containing separators or ..
func TestCheckIDRejectsTraversal(t *testing.T) {
	vm := t.TempDir()
	outside := filepath.Join(vm, "secret.json")
	os.WriteFile(outside, []byte(`{"operationId":"secret"}`), 0o600)

	for _, id := range []string{"../secret", "..\\secret", "a/b", "a\\b", "..", ""} {
		out, rc := runTool(t, "status", "--vmdir", vm, "--id", id)
		if rc != -1 { // -1 = died via die() (os.Exit), as INVALID_ARGS must
			t.Fatalf("id %q must be rejected (rc=%d out=%s)", id, rc, out)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside file must not be touched")
	}
}

func TestCheckIDAcceptsPlainUUID(t *testing.T) {
	vm := t.TempDir()
	dir := opsDir(vm)
	os.MkdirAll(dir, 0o755)
	id := "3f6f0f7a-9c1d-4b8e-aaaa-0123456789ab"
	os.WriteFile(filepath.Join(dir, id+".json"),
		[]byte(`{"operationId":"`+id+`"}`), 0o600)
	out, rc := runTool(t, "status", "--vmdir", vm, "--id", id)
	if rc != 0 {
		t.Fatalf("valid uuid rejected rc=%d %s", rc, out)
	}
}
