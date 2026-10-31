package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// qgaConf writes <dir>/<name>.conf and prepares the <dir>/<name>/
// vmdir; the returned conf resolves agentSock under it.
func qgaConf(t *testing.T, dir, name string) string {
	t.Helper()
	conf := filepath.Join(dir, name+".conf")
	writeFile(t, conf, "")
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	return conf
}

func TestResolveTarget(t *testing.T) {
	dir := t.TempDir()
	conf := qgaConf(t, dir, "vm1")
	rt := vmRuntime{Ports: map[string]int{}}

	// Empty target derives the ssh forward — .ports first.
	rt.Ports["ssh"] = 2222
	h := &Health{VM: "vm1", Type: "tcp"}
	if got, err := resolveTarget(h, conf, rt); err != nil || got != "127.0.0.1:2222" {
		t.Fatalf("derive ports: %q %v", got, err)
	}
	// ...then the conf's ssh_port.
	delete(rt.Ports, "ssh")
	writeFile(t, conf, `ssh_port="2300"`+"\n")
	if got, err := resolveTarget(h, conf, rt); err != nil || got != "127.0.0.1:2300" {
		t.Fatalf("derive conf: %q %v", got, err)
	}
	// ...and nothing at all is an error.
	writeFile(t, conf, "")
	if _, err := resolveTarget(h, conf, rt); err == nil ||
		!strings.Contains(err.Error(), "no ssh forward known") {
		t.Fatalf("no forward: %v", err)
	}
	// Bare port becomes localhost:port.
	h.Target = "8080"
	if got, err := resolveTarget(h, conf, rt); err != nil || got != "127.0.0.1:8080" {
		t.Fatalf("bare port: %q %v", got, err)
	}
	// host:port passes through.
	h.Target = "10.0.0.2:22"
	if got, err := resolveTarget(h, conf, rt); err != nil || got != "10.0.0.2:22" {
		t.Fatalf("host:port: %q %v", got, err)
	}
	// Garbage is rejected.
	h.Target = "not:a:port:x"
	if _, err := resolveTarget(h, conf, rt); err == nil ||
		!strings.Contains(err.Error(), "invalid target") {
		t.Fatalf("bad target: %v", err)
	}
	h.Target = "a b:22"
	if _, err := resolveTarget(h, conf, rt); err == nil {
		t.Fatalf("spaced host should fail")
	}
}

func TestProbeQGAPing(t *testing.T) {
	dir := t.TempDir()
	conf := qgaConf(t, dir, "vm1")
	sock := agentSock(conf)
	h := &Health{VM: "vm1", Type: "qga-ping"}
	rt := vmRuntime{PID: os.Getpid()}

	// No socket at all.
	if err := h.probe(conf, rt); err == nil ||
		!strings.Contains(err.Error(), "not present") {
		t.Fatalf("missing sock: %v", err)
	}
	// A regular file is not a socket.
	writeFile(t, sock, "")
	if err := h.probe(conf, rt); err == nil ||
		!strings.Contains(err.Error(), "not present") {
		t.Fatalf("regular file: %v", err)
	}
	os.Remove(sock)
	// Real unix socket + live pid -> pass.
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("unix listen: %v", err)
	}
	defer ln.Close()
	if err := h.probe(conf, rt); err != nil {
		t.Fatalf("qga-ping ok: %v", err)
	}
	// Dead/missing pid fails.
	if err := h.probe(conf, vmRuntime{PID: 0}); err == nil ||
		!strings.Contains(err.Error(), "not running") {
		t.Fatalf("pid 0: %v", err)
	}
	stub(t, &alive, func(int) bool { return false })
	if err := h.probe(conf, vmRuntime{PID: 4242}); err == nil ||
		!strings.Contains(err.Error(), "not running") {
		t.Fatalf("dead pid: %v", err)
	}
}

func TestProbeTCP(t *testing.T) {
	port := listenTCP(t, nil)
	h := &Health{VM: "v", Type: "tcp", Target: "127.0.0.1:" + fmt.Sprint(port),
		Timeout: time.Second}
	if err := h.probe("", vmRuntime{}); err != nil {
		t.Fatalf("tcp probe: %v", err)
	}
	// Dial failure propagates.
	stub(t, &dialer, func(string, string, time.Duration) (net.Conn, error) {
		return nil, fmt.Errorf("refused")
	})
	if err := h.probe("", vmRuntime{}); err == nil ||
		!strings.Contains(err.Error(), "refused") {
		t.Fatalf("dial err: %v", err)
	}
}

func TestProbeSSH(t *testing.T) {
	banner := listenTCP(t, func(c net.Conn) {
		fmt.Fprint(c, "SSH-2.0-OpenSSH_9.6\r\n")
		c.Close()
	})
	h := &Health{VM: "v", Type: "ssh",
		Target: "127.0.0.1:" + fmt.Sprint(banner), Timeout: time.Second}
	if err := h.probe("", vmRuntime{}); err != nil {
		t.Fatalf("ssh probe: %v", err)
	}

	wrong := listenTCP(t, func(c net.Conn) {
		fmt.Fprint(c, "HTTP/1.1 200 OK\r\n")
		c.Close()
	})
	h.Target = "127.0.0.1:" + fmt.Sprint(wrong)
	if err := h.probe("", vmRuntime{}); err == nil ||
		!strings.Contains(err.Error(), "not an ssh banner") {
		t.Fatalf("wrong banner: %v", err)
	}

	silent := listenTCP(t, func(c net.Conn) {
		time.Sleep(300 * time.Millisecond)
		c.Close()
	})
	h.Target = "127.0.0.1:" + fmt.Sprint(silent)
	h.Timeout = 50 * time.Millisecond
	if err := h.probe("", vmRuntime{}); err == nil ||
		!strings.Contains(err.Error(), "no ssh banner") {
		t.Fatalf("no banner: %v", err)
	}
}

func TestProbeUnknownType(t *testing.T) {
	h := &Health{VM: "v", Type: "carrier-pigeon"}
	if err := h.probe("", vmRuntime{}); err == nil ||
		!strings.Contains(err.Error(), "unknown healthcheck type") {
		t.Fatalf("unknown type: %v", err)
	}
}

func TestWaitSucceedsFirstTry(t *testing.T) {
	port := listenTCP(t, nil)
	h := &Health{VM: "v", Type: "tcp", Retries: 3,
		Target: "127.0.0.1:" + fmt.Sprint(port), Timeout: time.Second}
	stub(t, &sleeper, func(time.Duration) {})
	calls := 0
	err := h.wait(
		func() string { return "" },
		func() vmRuntime { calls++; return vmRuntime{} },
		func(string, ...any) {},
	)
	if err != nil || calls != 1 {
		t.Fatalf("wait: err=%v calls=%d", err, calls)
	}
}

func TestWaitExhaustsRetries(t *testing.T) {
	h := &Health{VM: "v", Type: "tcp", Retries: 3, Interval: time.Millisecond,
		Target: "127.0.0.1:1", Timeout: 10 * time.Millisecond}
	stub(t, &sleeper, func(time.Duration) {})
	var logs []string
	calls := 0
	err := h.wait(
		func() string { return "" },
		func() vmRuntime { calls++; return vmRuntime{} },
		func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) },
	)
	if err == nil || !strings.Contains(err.Error(), "healthcheck v (tcp)") {
		t.Fatalf("wait err: %v", err)
	}
	if calls != 3 {
		t.Fatalf("attempts = %d", calls)
	}
	if len(logs) != 2 || !strings.Contains(logs[0], "attempt 1/3 failed") {
		t.Fatalf("logs = %v", logs)
	}
}

func TestWaitRetriesLessThanOne(t *testing.T) {
	h := &Health{VM: "v", Type: "tcp", Retries: 0,
		Target: "127.0.0.1:1", Timeout: 10 * time.Millisecond}
	stub(t, &sleeper, func(time.Duration) {})
	calls := 0
	err := h.wait(
		func() string { return "" },
		func() vmRuntime { calls++; return vmRuntime{} },
		func(string, ...any) {},
	)
	if err == nil || calls != 1 {
		t.Fatalf("retries<1: err=%v calls=%d", err, calls)
	}
}

func TestWaitSucceedsAfterFailures(t *testing.T) {
	port := listenTCP(t, nil)
	h := &Health{VM: "v", Type: "tcp", Retries: 5, Interval: time.Millisecond,
		Target: "127.0.0.1:" + fmt.Sprint(port), Timeout: time.Second}
	stub(t, &sleeper, func(time.Duration) {})
	// Fail twice through the dialer seam, then let the real dial through.
	n := 0
	stub(t, &dialer, func(network, addr string, to time.Duration) (net.Conn, error) {
		n++
		if n <= 2 {
			return nil, fmt.Errorf("still booting")
		}
		return net.DialTimeout(network, addr, to)
	})
	err := h.wait(
		func() string { return "" },
		func() vmRuntime { return vmRuntime{} },
		func(string, ...any) {},
	)
	if err != nil || n != 3 {
		t.Fatalf("wait: err=%v dials=%d", err, n)
	}
}
