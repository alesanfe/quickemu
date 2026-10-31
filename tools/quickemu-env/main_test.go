package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsage(t *testing.T) {
	out := captureOut(t, usage)
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "quickemu-env <command>") {
		t.Fatalf("usage:\n%s", out)
	}
}

func TestParseArgsDefaults(t *testing.T) {
	o, err := parseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.file != "env.toml" || o.format != "human" || o.grace != 10*time.Second ||
		o.rollback || o.yes || o.force || o.jsonFlag || o.output != "" || len(o.pos) != 0 {
		t.Fatalf("defaults = %+v", o)
	}
}

func TestParseArgsFlags(t *testing.T) {
	o, err := parseArgs([]string{
		"-f", "x.toml", "--format", "json", "--rollback", "--force",
		"--grace", "5s", "-y", "--json", "-o", "ansible", "vm1", "vm2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if o.file != "x.toml" || o.format != "json" || !o.rollback || !o.force ||
		o.grace != 5*time.Second || !o.yes || !o.jsonFlag || o.output != "ansible" {
		t.Fatalf("opts = %+v", o)
	}
	if len(o.pos) != 2 || o.pos[0] != "vm1" || o.pos[1] != "vm2" {
		t.Fatalf("pos = %v", o.pos)
	}
}

func TestParseArgsLongForms(t *testing.T) {
	o, err := parseArgs([]string{"--file", "y.toml", "--yes", "--output", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if o.file != "y.toml" || !o.yes || o.output != "x" {
		t.Fatalf("opts = %+v", o)
	}
}

func TestParseArgsEqForms(t *testing.T) {
	o, err := parseArgs([]string{"--file=z.toml", "--format=text", "--grace=2s", "--output=json"})
	if err != nil {
		t.Fatal(err)
	}
	if o.file != "z.toml" || o.format != "text" || o.grace != 2*time.Second || o.output != "json" {
		t.Fatalf("opts = %+v", o)
	}
}

func TestParseArgsFormatValues(t *testing.T) {
	for _, f := range []string{"human", "json", "text"} {
		o, err := parseArgs([]string{"--format", f})
		if err != nil || o.format != f {
			t.Fatalf("format %s: %+v %v", f, o, err)
		}
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--format", "yaml"}, "--format must be human or json"},
		{[]string{"--grace", "bogus"}, "--grace"},
		{[]string{"--grace"}, "--grace"},
		{[]string{"--bogus"}, "unknown flag --bogus"},
		{[]string{"-z"}, "unknown flag -z"},
		{[]string{"-f", "x", "--"}, "unknown flag --"},
	} {
		_, err := parseArgs(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestParseArgsFlagWithoutValue(t *testing.T) {
	// A trailing value-flag swallows "".
	o, err := parseArgs([]string{"-f"})
	if err != nil || o.file != "" {
		t.Fatalf("opts = %+v err=%v", o, err)
	}
}

// TestParseArgsExitHelper runs inside a child process for the flags
// that os.Exit(0) — the parent asserts the clean exit.
func TestParseArgsExitHelper(t *testing.T) {
	flag := os.Getenv("QE_ENV_PARSE_EXIT")
	if flag == "" {
		t.Skip("helper process")
	}
	parseArgs([]string{flag})
	os.Exit(9) // reached only if parseArgs returned — a bug
}

func TestParseArgsExitFlags(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "--version"} {
		cmd := exec.Command(os.Args[0], "-test.run=TestParseArgsExitHelper")
		cmd.Env = append(os.Environ(), "QE_ENV_PARSE_EXIT="+flag)
		cmd.Stdout = nil
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: child did not exit 0: %v", flag, err)
		}
	}
}

func TestPrintErrs(t *testing.T) {
	var rc int
	out := captureOut(t, func() {
		rc = printErrs("env.toml", envErrs{
			{3, 5, "with position"},
			{0, 0, "file level"},
		})
	})
	if rc != 1 {
		t.Fatalf("rc = %d", rc)
	}
	if !strings.Contains(out, "env.toml:3:5: error: with position") ||
		!strings.Contains(out, "env.toml: error: file level") {
		t.Fatalf("out:\n%s", out)
	}
}

func TestLoadOrDie(t *testing.T) {
	dir := t.TempDir()
	// Bad file -> nil env + rc from printErrs.
	e, rc := loadOrDie(filepath.Join(dir, "nope.toml"))
	if e != nil || rc != 1 {
		t.Fatalf("bad loadOrDie = %v %d", e, rc)
	}
	writeEnvFile(t, dir, "[[vm]]\nname=\"a\"\nconf=\"a.conf\"\n")
	e, rc = loadOrDie(filepath.Join(dir, "env.toml"))
	if e == nil || rc != 0 || len(e.VMs) != 1 {
		t.Fatalf("good loadOrDie = %v %d", e, rc)
	}
}

// runEnvFile writes a minimal valid env with two conf-backed VMs.
func runEnvFile(t *testing.T, dir string) string {
	t.Helper()
	confAt(t, dir, "a.conf", `guest_os="linux"`+"\n")
	confAt(t, dir, "b.conf", `guest_os="linux"`+"\n")
	return writeEnvFile(t, dir, `
[env]
name = "demo"

[[vm]]
name = "a"
conf = "a.conf"

[[vm]]
name = "b"
conf = "b.conf"
depends_on = ["a"]
`)
}

func TestRunNoArgs(t *testing.T) {
	out := captureOut(t, func() {
		if rc := run(nil); rc != 2 {
			t.Fatalf("rc = %d", rc)
		}
	})
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("out:\n%s", out)
	}
}

func TestRunHelpVersion(t *testing.T) {
	for _, args := range [][]string{
		{"help"}, {"-h"}, {"--help"}, {"--version"},
	} {
		var rc int
		captureOut(t, func() { rc = run(args) })
		if rc != 0 {
			t.Fatalf("%v: rc = %d", args, rc)
		}
	}
}

func TestRunUnknownCommand(t *testing.T) {
	if rc := run([]string{"frobnicate"}); rc != 2 {
		t.Fatalf("rc = %d", rc)
	}
}

func TestRunParseArgsError(t *testing.T) {
	out := captureOut(t, func() {
		if rc := run([]string{"status", "--bogus"}); rc != 2 {
			t.Fatalf("rc = %d", rc)
		}
	})
	if !strings.Contains(out, "ERROR!") {
		t.Fatalf("out:\n%s", out)
	}
}

func TestRunValidate(t *testing.T) {
	dir := t.TempDir()
	f := runEnvFile(t, dir)
	var rc int
	out := captureOut(t, func() { rc = run([]string{"validate", "-f", f}) })
	if rc != 0 || !strings.Contains(out, "OK (2 vm(s), 0 net(s), 0 share(s), 0 healthcheck(s))") {
		t.Fatalf("rc=%d out:\n%s", rc, out)
	}
	out = captureOut(t, func() {
		rc = run([]string{"validate", "--format", "json", "-f", f})
	})
	if rc != 0 || !strings.Contains(out, `"command":"env.validate"`) ||
		!strings.Contains(out, `"success":true`) {
		t.Fatalf("json rc=%d out:\n%s", rc, out)
	}
	// Missing file -> diagnostics on stdout, rc 1.
	out = captureOut(t, func() {
		rc = run([]string{"validate", "-f", filepath.Join(dir, "gone.toml")})
	})
	if rc != 1 || !strings.Contains(out, "error:") {
		t.Fatalf("missing rc=%d out:\n%s", rc, out)
	}
	// Invalid file, json format -> envelope + diagnostics.
	bad := filepath.Join(dir, "bad.toml")
	writeFile(t, bad, "[[vm]]\nconf=\"x\"\n")
	out = captureOut(t, func() {
		rc = run([]string{"validate", "--format", "json", "-f", bad})
	})
	if rc != 1 || !strings.Contains(out, "ENV_INVALID") ||
		!strings.Contains(out, "error:") {
		t.Fatalf("bad json rc=%d out:\n%s", rc, out)
	}
	// Invalid file, human format -> diagnostics only.
	out = captureOut(t, func() {
		rc = run([]string{"validate", "-f", bad})
	})
	if rc != 1 || strings.Contains(out, "ENV_INVALID") {
		t.Fatalf("bad human rc=%d out:\n%s", rc, out)
	}
}

func TestRunLifecycleCommands(t *testing.T) {
	dir := t.TempDir()
	f := runEnvFile(t, dir)
	stubTools(t, quickemuFake(t, ""))
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"up", "-f", f}, 0},
		{[]string{"status", "-f", f}, 0},
		{[]string{"status", "--format", "json", "-f", f}, 0},
		{[]string{"down", "-f", f}, 0},
		{[]string{"restart", "-f", f}, 0},
		{[]string{"hosts", "-f", f}, 0},
		{[]string{"hosts", "-f", f, "a"}, 0},
		{[]string{"inventory", "-f", f}, 0},
		{[]string{"inventory", "--json", "-f", f}, 0},
		{[]string{"inventory", "-o", "ansible", "-f", f}, 0},
		{[]string{"destroy", "--yes", "-f", f}, 0},
	} {
		var rc int
		captureOut(t, func() { rc = run(tc.args) })
		if rc != tc.want {
			t.Fatalf("%v: rc = %d, want %d", tc.args, rc, tc.want)
		}
	}
}

func TestRunLifecycleLoadError(t *testing.T) {
	dir := t.TempDir()
	stubTools(t, nil)
	missing := filepath.Join(dir, "nope.toml")
	for _, cmd := range []string{"up", "down", "status", "restart", "destroy", "hosts", "inventory"} {
		var rc int
		captureOut(t, func() { rc = run([]string{cmd, "-f", missing}) })
		if rc != 1 {
			t.Fatalf("%s: rc = %d", cmd, rc)
		}
	}
}

func TestQELibProfileDefaults(t *testing.T) {
	m, ok := qelibProfileDefaults("minimal")
	if !ok || m["cpu_cores"] != "1" || m["ram"] != "512M" {
		t.Fatalf("minimal = %v %v", m, ok)
	}
	if _, ok := qelibProfileDefaults("no-such-profile"); ok {
		t.Fatal("unknown profile should miss")
	}
}
