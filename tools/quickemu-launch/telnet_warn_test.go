package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStderr: capture fmt.Fprint(os.Stderr) output produced by fn.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf strings.Builder
	io.Copy(&buf, r)
	r.Close()
	return buf.String()
}

// TD-003: a telnet monitor/serial endpoint on a non-loopback bind is an
// unauthenticated remote HMP channel — must warn on stderr. Loopback
// binds are local-control only and stay silent.
func TestWarnTelnetExposure(t *testing.T) {
	for _, host := range []string{"", "localhost", "127.0.0.1", "127.10.20.30", "::1"} {
		if out := captureStderr(t, func() { warnTelnetExposure("monitor", host) }); out != "" {
			t.Errorf("loopback host %q should not warn, got %q", host, out)
		}
	}
	for _, host := range []string{"0.0.0.0", "::", "192.168.1.10", "10.0.0.2", "example.com"} {
		out := captureStderr(t, func() { warnTelnetExposure("monitor", host) })
		if !strings.Contains(out, "unauthenticated") || !strings.Contains(out, host) {
			t.Errorf("non-loopback host %q should warn, got %q", host, out)
		}
	}
}

// Wiring: a real conf with a wildcard telnet bind surfaces the warning
// through the launch path (dry-run, stderr merged by launch()).
func TestTelnetExposureWarningDryRun(t *testing.T) {
	conf := richConf(t, `monitor="telnet"
monitor_telnet_host="0.0.0.0"
monitor_telnet_port="4440"
`)
	out, rc := launch(t, nil, conf, "--dry-run")
	if rc != 0 {
		t.Fatalf("rc=%d %s", rc, out)
	}
	if !strings.Contains(out, "unauthenticated") {
		t.Fatalf("wildcard monitor bind did not warn:\n%s", out)
	}
}
