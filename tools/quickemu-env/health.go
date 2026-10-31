// health.go: post-start health checks (QE-071). Three probe kinds,
// none of which needs the (separately-owned) qga module:
//
//	tcp:      net.Dial succeeds on the target
//	ssh:      dial + read an "SSH-" banner
//	qga-ping: <vmdir>/<vm>-agent.sock exists AND the VM pid lives —
//	          the guest agent may still be booting, so the check is
//	          socket-presence, not a QGA handshake
//
// Retrying happens attempts=retries times, one probe per timeout,
// interval apart. An empty target derives the SSH host forward from
// the live .ports record / conf ssh_port.
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// dialer is the probe seam — tests point it at in-process listeners
// without needing real VMs.
var dialer = func(network, addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

// resolveTarget turns a healthcheck target into host:port. Accepted
// forms: "" (derive the VM's ssh forward), "port" (localhost) and
// "host:port".
func resolveTarget(h *Health, conf string, rt vmRuntime) (string, error) {
	t := strings.TrimSpace(h.Target)
	if t == "" {
		p := sshPort(conf, rt)
		if p == 0 {
			return "", fmt.Errorf("no target and no ssh forward known for %q", h.VM)
		}
		return "127.0.0.1:" + strconv.Itoa(p), nil
	}
	if p, err := strconv.Atoi(t); err == nil {
		return "127.0.0.1:" + strconv.Itoa(p), nil
	}
	if _, _, err := net.SplitHostPort(t); err != nil {
		return "", fmt.Errorf("invalid target %q (want host:port or port)", h.Target)
	}
	return t, nil
}

// probe is one attempt. timeout bounds the dial AND the banner read.
func (h *Health) probe(conf string, rt vmRuntime) error {
	switch h.Type {
	case "qga-ping":
		sock := agentSock(conf)
		st, err := os.Stat(sock)
		if err != nil || st.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("agent socket %s not present", sock)
		}
		if rt.PID <= 0 || !alive(rt.PID) {
			return fmt.Errorf("vm %q not running", h.VM)
		}
		return nil
	case "tcp", "ssh":
		target, err := resolveTarget(h, conf, rt)
		if err != nil {
			return err
		}
		c, err := dialer("tcp", target, h.Timeout)
		if err != nil {
			return err
		}
		defer c.Close()
		if h.Type == "ssh" {
			c.SetReadDeadline(time.Now().Add(h.Timeout))
			line, err := bufio.NewReader(c).ReadString('\n')
			if err != nil {
				return fmt.Errorf("no ssh banner on %s: %v", target, err)
			}
			if !strings.HasPrefix(line, "SSH-") {
				return fmt.Errorf("%s answered %q — not an ssh banner", target, strings.TrimSpace(line))
			}
		}
		return nil
	}
	return fmt.Errorf("unknown healthcheck type %q", h.Type)
}

// wait retries probe until success or the retry budget is spent; the
// vmRuntime is re-inspected each attempt so .ports records written by
// the booting VM get picked up.
func (h *Health) wait(conf func() string, insp func() vmRuntime, logf func(string, ...any)) error {
	attempts := h.Retries
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for i := 1; i <= attempts; i++ {
		rt := insp()
		last = h.probe(conf(), rt)
		if last == nil {
			return nil
		}
		if i < attempts {
			logf("health[%s] attempt %d/%d failed: %v", h.VM, i, attempts, last)
			sleeper(h.Interval)
		}
	}
	return fmt.Errorf("healthcheck %s (%s): %v", h.VM, h.Type, last)
}

// sleeper is the retry-delay seam so tests run fast.
var sleeper = time.Sleep
