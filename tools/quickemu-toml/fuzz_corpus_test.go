// Hostile-corpus test: the CLI must never hang, never crash (rc > 2)
// and never execute embedded payloads. Ported from tools/fuzz-toml.sh.
package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tomlBin(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "quickemu-toml")
	if testing.Short() {
		t.Skip("binary build skipped in -short")
	}
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func TestValidateHostileCorpus(t *testing.T) {
	bin := tomlBin(t)
	long := strings.Repeat("x", 200000)
	many := strings.Repeat("k=v\n", 5000)
	cases := map[string]string{
		"unicode":     "guest_os = \"linu\xc3\xa9\xe1\x88\xb4\"\n",
		"control":     "a\x01\x02\x03 = 1\n",
		"truncated":   "guest_os = \"unclosed",
		"longline":    "k = \"" + long + "\"",
		"deepsection": "[a.b.c.d.e.f.g.h.i.j.k.l.m]\nx=1\n",
		"dupkeys":     "a=1\na=1\na=1\n",
		"codeinject":  "guest_os = \"$(rm -rf /)\"",
		"backtick":    "guest_os = \"`id`\"",
		"crlf":        "guest_os=\"linux\"\r\nram=\"4G\"\r\n",
		"empty":       "",
		"nullbyte":    "a\x00b=1\n",
		"hugeval":     "ram = \"" + long + "\"",
		"manykeys":    many,
		"tablevals":   "x = {a=1, b=\"evil\"}",
		"nestarr":     "x = [[\"a\",\"b\"],[\"c\"]]",
	}
	dir := t.TempDir()
	for name, input := range cases {
		f := filepath.Join(dir, "f.toml")
		if err := os.WriteFile(f, []byte(input), 0o644); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, bin, "validate", "--file", f)
		err := cmd.Run()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			t.Errorf("HANG on %s", name)
			continue
		}
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() > 2 {
				t.Errorf("CRASH(%d) on %s", ee.ExitCode(), name)
			}
		}
	}
}

func TestValidateNeverExecutes(t *testing.T) {
	bin := tomlBin(t)
	dir := t.TempDir()
	pwned := filepath.Join(dir, "pwned")
	f := filepath.Join(dir, "f.toml")
	os.WriteFile(f, []byte("guest_os = \"$(touch "+pwned+")\"\n"), 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	exec.CommandContext(ctx, bin, "validate", "--file", f).Run()
	cancel()
	if _, err := os.Stat(pwned); err == nil {
		t.Fatal("EXEC-LEAK: parser executed payload")
	}
}
