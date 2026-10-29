// main_test.go — helper-exec harness for main() flag/arg dispatch and
// actCreateConfig via a stub quickget-vm.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		os.Args = append([]string{"quickget"},
			strings.Split(os.Getenv("QG_ARGS"), "\x1f")...)
		main()
		return
	}
	os.Exit(m.Run())
}

func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QG_ARGS="+strings.Join(args, "\x1f"))
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	}
	return out.String() + errb.String(), rc
}

func TestMainDispatchFlags(t *testing.T) {
	setupCatalog(t)
	cases := []struct {
		name string
		args []string
		rc   int
		want string
	}{
		{"help", []string{"--help"}, 0, "Usage"},
		{"version", []string{"--version"}, 0, ""},
		{"list", []string{"--list"}, 0, "alpine"},
		{"list-csv", []string{"--list-csv"}, 0, "Display Name"},
		{"list-json", []string{"--list-json"}, 0, `"OS"`},
		{"show", []string{"--show", "alpine"}, 0, "Alpine Linux"},
		{"show-bad", []string{"--show", "nope"}, 1, ""},
		{"url", []string{"--url", "alpine", "3.19", "virt"}, 0, "a.iso"},
		{"check", []string{"--check", "alpine", "3.19", "virt"}, 0, "PASS"},
		{"check-all-arch", []string{"--check-all-arch", "alpine", "3.19", "virt"}, 0, ""},
		{"update", []string{"--update", "alpine"}, 0, "Latest"},
		{"bad-flag", []string{"--bogus"}, 1, "Not supported"},
		{"bad-arch", []string{"--arch", "mips"}, 1, "Unsupported"},
		{"arch-arm", []string{"--arch", "arm64", "--list"}, 0, "alpine"},
		{"bad-trust", []string{"--minimum-trust", "Z"}, 1, "invalid trust"},
		{"trust-ok", []string{"--minimum-trust", "C", "--list"}, 0, ""},
		{"unknown-os", []string{"zzz", "1"}, 1, "not a supported OS"},
		{"insecure", []string{"--insecure", "--list"}, 0, "alpine"},
		{"homepage", []string{"--open-homepage", "nope"}, 1, "operating system"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, rc := runCLI(t, c.args...)
			if rc != c.rc {
				t.Fatalf("rc=%d want %d out=%s", rc, c.rc, out)
			}
			if c.want != "" && !strings.Contains(out, c.want) {
				t.Fatalf("out missing %q: %s", c.want, out)
			}
		})
	}
}

func TestMainPositionalAndConfig(t *testing.T) {
	setupCatalog(t)
	// Stub quickget-vm for the config/download passthrough.
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-vm"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))

	// --create-config <vmpath> <input>: pos args are not an OS.
	out, rc := runCLI(t, "--create-config", "myvm", "x.iso")
	if rc != 0 {
		t.Fatalf("create-config rc=%d %s", rc, out)
	}
	out, rc = runCLI(t, "--create-config", "--unattended", "v2", "y.iso")
	if rc != 0 {
		t.Fatalf("create-config unattended rc=%d %s", rc, out)
	}
	// Failing passthrough → rc 1.
	os.WriteFile(filepath.Join(stub, "quickget-vm"),
		[]byte("#!/bin/sh\nexit 3\n"), 0o755)
	out, rc = runCLI(t, "--create-config", "v3", "z.iso")
	if rc != 1 {
		t.Fatalf("passthru fail rc=%d %s", rc, out)
	}
	// direct actCreateConfig.
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	if rc := actCreateConfig("v", "i"); rc != 1 {
		t.Fatalf("actCreateConfig rc=%d", rc)
	}
}

func TestMinimumTrustEnv(t *testing.T) {
	setupCatalog(t)
	t.Setenv("QC_TRUST", "D")
	t.Setenv("MINIMUM_TRUST", "A")
	out, rc := runCLI(t, "alpine", "3.19")
	if rc != 1 || !strings.Contains(out, "below --minimum-trust A") {
		t.Fatalf("env-seeded trust gate must reject D<A: rc=%d %s", rc, out)
	}
}
