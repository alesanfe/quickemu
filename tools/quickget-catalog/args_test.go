// args_test.go — operand-count contract: every subcommand that needs
// <os> must fail with usage(2), never panic on args[1].
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingOperandIsUsageNotPanic(t *testing.T) {
	dir := t.TempDir()
	cat := filepath.Join(dir, "catalog.json")
	os.WriteFile(cat, []byte(`{"entries":[{"os":"x","releases":["1"]}]}`), 0o644)
	inf := filepath.Join(dir, "info.json")
	os.WriteFile(inf, []byte(`{}`), 0o644)
	t.Setenv("QUICKEMU_CATALOG", cat)
	t.Setenv("QUICKEMU_INFO", inf)

	for _, sub := range []string{
		"releases", "editions", "arch", "url", "check",
		"info", "info-json", "trust",
	} {
		old := os.Args
		os.Args = []string{"quickget-catalog", sub}
		rc := func() (rc int) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s without operand panicked: %v", sub, r)
				}
			}()
			return run()
		}()
		os.Args = old
		if rc != 2 {
			t.Errorf("%s without operand rc=%d, want usage(2)", sub, rc)
		}
	}
}
