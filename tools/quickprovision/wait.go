// wait.go: condition waits. Everything here is a poll loop with a
// deadline and a heartbeat emitter — a hung wait must leave enough
// evidence (serial tail, last error) to diagnose without a rerun.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// waitSSH polls the forwarded port until an SSH handshake with the
// run key succeeds or the deadline hits. The installer environment
// never runs sshd, so the first successful login implies the target
// system is up.
func waitSSH(ctx context.Context, e *emitter, port int, user string, k *keypair, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		c, err := sshDial(port, user, k.signer)
		if err == nil {
			c.Close()
			return nil
		}
		if msg := err.Error(); msg != last {
			last = msg
			e.progress("wait-ssh", msg)
		}
		time.Sleep(4 * time.Second)
	}
	return fmt.Errorf("ssh not reachable on 127.0.0.1:%d within %s (last: %s)", port, timeout, last)
}

// waitPIDGone blocks until the recorded QEMU pid exits — the clean
// end of a `shutdown: poweroff` autoinstall or a guest poweroff.
func waitPIDGone(ctx context.Context, e *emitter, vmDir, vm string, timeout time.Duration) error {
	pidFile := filepath.Join(vmDir, vm+".pid")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		b, err := os.ReadFile(pidFile)
		if err != nil {
			return nil // pid file gone → VM is gone
		}
		var pid int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid); err == nil && !pidAlive(pid) {
			return nil
		}
		e.progress("wait-poweroff", "qemu still running")
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("VM still running after %s", timeout)
}

// tailSerial attaches to the VM's serial socket and streams output to
// the artifact log until the socket closes or ctx ends. Serial is
// diagnostics only — install completion is signalled by the guest
// powering off, not by parsing this stream. Matching on it anyway
// gives a belt-and-braces early-out for installers that reboot
// instead of powering down.
func tailSerial(ctx context.Context, sockPath, logPath string, e *emitter) {
	fh, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		buf := make([]byte, 8192)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				fh.Write(buf[:n])
			}
			if err != nil {
				conn.Close()
				if err != io.EOF {
					e.progress("serial", "serial stream ended: "+err.Error())
				}
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
}

// freePort asks the kernel for an unused TCP port on localhost.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
