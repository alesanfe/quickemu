// guest_test.go: guest-verb coverage — intercept/verb parsing,
// iface filtering, the ip cache, error classification, and the act
// functions against a scripted fake agent at the real socket path.
package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
	"github.com/alesanfe/quickemu/tools/quickemu-qga"
	"github.com/alesanfe/quickemu/tools/quickemu-qga/qgatest"
)

// guestVM: a running-VM fixture — the pidfile holds a live (renamed
// sleep) pid inside vmdir so instanceVerify reports the VM running.
func guestVM(t *testing.T) (dir, name string) {
	t.Helper()
	dir = withConf(t, "")
	name = vmname()
	livePID(t, dir)
	return dir, name
}

// agentSock: bind the scripted fake agent and link it into the
// VM's real channel path (the .s/agent socket the launcher binds).
func agentSock(t *testing.T, h qgatest.Handler) *qgatest.Server {
	t.Helper()
	s := qgatest.Serve(t, h)
	link := qgaSock()
	// A dead listener's socket file can linger at the channel path —
	// the agent link always wins.
	os.MkdirAll(filepath.Dir(link), 0o700)
	os.Remove(link)
	if err := os.Symlink(s.Sock, link); err != nil {
		t.Fatalf("agent link: %v", err)
	}
	return s
}

func guestReset() {
	actions = nil
	guestVerbState.exec = guestExecOpts{}
	guestVerbState.xfer = guestXferOpts{}
	guestVerbState.power = guestPowerOpts{}
	guestVerbState.freeze = guestFreezeOpts{}
	guestVerbState.ip = guestIPOpts{}
}

// ---------- parse coverage ----------

func TestGuestInterceptVerbs(t *testing.T) {
	guestReset()
	if !guestIntercept([]string{"guest", "ping", "--vm", "x.conf"}) {
		t.Fatal("guest ping must intercept")
	}
	if actions[0] != "guest_ping" || vmConf != "x.conf" {
		t.Fatalf("ping parse: %v vm=%q", actions, vmConf)
	}
	guestReset()
	guestIntercept([]string{"guest", "exec", "--shell", "--env", "A=1",
		"--timeout", "5s", "--max-output", "2K", "--vm", "v.conf",
		"--", "ls", "-la"})
	if actions[0] != "guest_exec" || !guestVerbState.exec.shell ||
		guestVerbState.exec.argv[0] != "ls" ||
		guestVerbState.exec.argv[1] != "-la" ||
		guestVerbState.exec.env[0] != "A=1" ||
		guestVerbState.exec.timeout != 5*time.Second ||
		guestVerbState.exec.maxOutput != 2048 {
		t.Fatalf("exec parse: %+v", guestVerbState.exec)
	}
	guestReset()
	guestIntercept([]string{"guest", "exec", "uname", "-a"})
	if guestVerbState.exec.argv[0] != "uname" || guestVerbState.exec.argv[1] != "-a" {
		t.Fatalf("bare argv: %v", guestVerbState.exec.argv)
	}
	guestReset()
	guestIntercept([]string{"guest", "push", "/tmp/a", "/tmp/b", "--verify"})
	if actions[0] != "guest_push" || guestVerbState.xfer.src != "/tmp/a" ||
		guestVerbState.xfer.dst != "/tmp/b" || !guestVerbState.xfer.verify ||
		!guestVerbState.xfer.restricted {
		t.Fatalf("push parse: %+v", guestVerbState.xfer)
	}
	guestReset()
	guestIntercept([]string{"guest", "pull", "/g", "/l", "--force", "--restricted"})
	if actions[0] != "guest_pull" || !guestVerbState.xfer.force ||
		!guestVerbState.xfer.restricted || guestVerbState.xfer.maxSize <= 0 {
		t.Fatalf("pull parse: %+v", guestVerbState.xfer)
	}
	guestReset()
	guestIntercept([]string{"guest", "shutdown", "--timeout", "10", "--force"})
	if actions[0] != "guest_shutdown" || guestVerbState.power.timeout != 10*time.Second ||
		!guestVerbState.power.force {
		t.Fatalf("shutdown parse: %+v", guestVerbState.power)
	}
	guestReset()
	guestIntercept([]string{"guest", "reboot"})
	if actions[0] != "guest_reboot" {
		t.Fatalf("reboot: %v", actions)
	}
	guestReset()
	guestIntercept([]string{"guest", "fsfreeze", "freeze", "--hold", "30s"})
	if actions[0] != "guest_fsfreeze" || guestVerbState.freeze.sub != "freeze" ||
		guestVerbState.freeze.hold != 30*time.Second {
		t.Fatalf("freeze parse: %+v", guestVerbState.freeze)
	}
	guestReset()
	guestIntercept([]string{"ip", "--iface", "eth0", "-6"})
	if actions[0] != "guest_ip" || guestVerbState.ip.iface != "eth0" ||
		!guestVerbState.ip.v6 {
		t.Fatalf("ip parse: %+v", guestVerbState.ip)
	}
}

func TestGuestInterceptSkipsForeign(t *testing.T) {
	guestReset()
	if guestIntercept([]string{"--status"}) {
		t.Fatal("non-guest argv must not intercept")
	}
	// "guest"/"ip" as flag VALUES are left for the normal scan.
	if guestIntercept([]string{"--type", "guest", "--vm", "x"}) {
		t.Fatal("--type guest is a value, not the subcommand")
	}
	if guestIntercept([]string{"--monitor-cmd", "ip link"}) {
		t.Fatal("--monitor-cmd value must not intercept")
	}
	if len(actions) != 0 {
		t.Fatalf("foreign args queued actions: %v", actions)
	}
}

func TestGuestParseErrors(t *testing.T) {
	stubTools(t)
	dir := t.TempDir()
	conf := filepath.Join(dir, "g.conf")
	os.WriteFile(conf, nil, 0o644)
	for _, tc := range []struct {
		name string
		args []string
		rc   int
	}{
		{"no-verb", []string{"guest"}, 2},
		{"bad-verb", []string{"guest", "frobnicate"}, 1},
		{"misplaced", []string{"--vm", conf, "guest", "ping"}, 1},
		{"exec-empty", []string{"guest", "exec", "--vm", conf}, 1},
		{"xfer-arity", []string{"guest", "push", "/a", "--vm", conf}, 1},
		{"bad-dur", []string{"guest", "exec", "--timeout", "abc", "x"}, 1},
		{"bad-size", []string{"guest", "exec", "--max-output", "z", "x"}, 1},
		{"bad-flag", []string{"guest", "ping", "--bogus"}, 1},
		{"bad-format", []string{"guest", "ping", "--format", "xml"}, 1},
		{"fsfreeze-bad", []string{"guest", "fsfreeze", "melt"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, rc := runMain(t, tc.args...)
			if rc != tc.rc {
				t.Fatalf("rc=%d want %d out=%s", rc, tc.rc, out)
			}
		})
	}
}

func TestGuestHelpers(t *testing.T) {
	if d := guestDuration("90"); d != 90*time.Second {
		t.Fatalf("bare seconds: %v", d)
	}
	if s := guestSize("3M"); s != 3<<20 {
		t.Fatalf("size: %d", s)
	}
	for _, bad := range []string{"lo", "lo:1", "br0", "virbr0", "veth12",
		"docker0", "cni0", "flannel.1", "vnet3", "bridgeX"} {
		if !unusableGuestIface(bad) {
			t.Fatalf("%s must be unusable", bad)
		}
	}
	for _, good := range []string{"eth0", "ens3", "enp1s0", "wlan0", "bond0"} {
		if unusableGuestIface(good) {
			t.Fatalf("%s must be usable", good)
		}
	}
}

// ---------- ip cache ----------

func TestIPCacheLifecycle(t *testing.T) {
	dir, _ := guestVM(t)
	if ipCacheRead() != nil {
		t.Fatal("no cache initially")
	}
	ipCacheWrite([]string{"10.0.0.5"}, []string{"fd00::1"})
	c := ipCacheRead()
	if c == nil || len(c.V4) != 1 || c.V4[0] != "10.0.0.5" {
		t.Fatalf("read-back: %+v", c)
	}
	// State transition: rewrite the pidfile (new mtime) → key rolls,
	// the cached entry is stale.
	time.Sleep(20 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, vmname()+".pid"),
		[]byte(itoa(readPID())), 0o600)
	// Force a different mtime in case the fs granularity hid it.
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dir, vmname()+".pid"), old, old)
	if ipCacheRead() != nil {
		t.Fatal("state transition must invalidate the cache")
	}
	// Corrupt file → miss, never fatal.
	os.WriteFile(ipCacheFile(), []byte("{"), 0o600)
	if ipCacheRead() != nil {
		t.Fatal("corrupt cache must miss")
	}
}

// ---------- act functions against the fake agent ----------

func TestGuestPingInfo(t *testing.T) {
	guestVM(t)
	agentSock(t, qgatest.Static(map[string]any{
		"guest-info": map[string]any{"version": "9.1.0",
			"supported_commands": []any{
				map[string]any{"name": "guest-ping", "enabled": true}}},
		"guest-get-host-name": map[string]any{"host-name": "testvm"},
		"guest-get-osinfo":    map[string]any{"pretty-name": "TestOS 1", "kernel-release": "6.1", "machine": "x86_64"},
		"guest-get-time":      float64(1_700_000_000_000_000_000),
		"guest-get-fsinfo":    []any{map[string]any{"name": "sda1"}},
	}))
	if rc := actGuestPing(); rc != 0 {
		t.Fatalf("ping rc=%d", rc)
	}
	out := captureOut(t, func() {
		if rc := actGuestInfo(); rc != 0 {
			t.Fatalf("info rc=%d", rc)
		}
	})
	if !strings.Contains(out, "testvm") || !strings.Contains(out, "TestOS 1") {
		t.Fatalf("info output: %s", out)
	}
	old := outFmt
	outFmt = "json"
	out = captureOut(t, func() {
		if rc := actGuestInfo(); rc != 0 {
			t.Fatalf("info json rc=%d", rc)
		}
	})
	outFmt = old
	if !strings.Contains(out, `"command":"guest.info"`) ||
		!strings.Contains(out, `"hostname":"testvm"`) {
		t.Fatalf("info envelope: %s", out)
	}
}

func TestGuestIPOutput(t *testing.T) {
	dir, name := guestVM(t)
	_ = name
	ifaces := []any{
		map[string]any{"name": "lo", "ip-addresses": []any{
			map[string]any{"ip-address": "127.0.0.1", "ip-address-type": "ipv4"}}},
		map[string]any{"name": "eth0", "ip-addresses": []any{
			map[string]any{"ip-address": "192.168.1.10", "ip-address-type": "ipv4"},
			map[string]any{"ip-address": "fe80::1", "ip-address-type": "ipv6"},
			map[string]any{"ip-address": "fd00::9", "ip-address-type": "ipv6"}}},
		map[string]any{"name": "br0", "ip-addresses": []any{
			map[string]any{"ip-address": "10.9.9.9", "ip-address-type": "ipv4"}}},
		map[string]any{"name": "veth5", "ip-addresses": []any{
			map[string]any{"ip-address": "10.8.8.8", "ip-address-type": "ipv4"}}},
	}
	agentSock(t, qgatest.Static(map[string]any{
		"guest-network-get-interfaces": ifaces}))
	out := captureOut(t, func() {
		if rc := actGuestIP(); rc != 0 {
			t.Fatalf("ip rc=%d", rc)
		}
	})
	if !strings.Contains(out, "192.168.1.10") ||
		strings.Contains(out, "127.0.0.1") || strings.Contains(out, "10.9.9.9") ||
		strings.Contains(out, "10.8.8.8") || strings.Contains(out, "fe80::1") {
		t.Fatalf("filtered ip output: %s", out)
	}
	// The reply memoized: second call must not hit the agent again
	// (fixture would still answer, but the cache short-circuits).
	if b, err := os.ReadFile(filepath.Join(dir, ".ip-cache.json")); err != nil ||
		!strings.Contains(string(b), "192.168.1.10") {
		t.Fatal("ip cache not written")
	}
	// -6 lists the v6 family.
	guestVerbState.ip.v6 = true
	out = captureOut(t, func() {
		if rc := actGuestIP(); rc != 0 {
			t.Fatalf("ip -6 rc=%d", rc)
		}
	})
	if !strings.Contains(out, "fd00::9") {
		t.Fatalf("v6 output: %s", out)
	}
	// --iface picks one interface.
	guestVerbState.ip = guestIPOpts{iface: "eth0"}
	old := outFmt
	outFmt = "json"
	out = captureOut(t, func() {
		if rc := actGuestIP(); rc != 0 {
			t.Fatalf("ip --iface rc=%d", rc)
		}
	})
	outFmt = old
	guestVerbState.ip = guestIPOpts{}
	if !strings.Contains(out, "192.168.1.10") {
		t.Fatalf("iface output: %s", out)
	}
}

func TestGuestExec(t *testing.T) {
	guestVM(t)
	// Scripted exec: pid 42, one status poll reports exited with
	// base64 stdout/stderr.
	srv := agentSock(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-exec":
			args := req["arguments"].(map[string]any)
			if args["path"] != "/usr/bin/foo" {
				return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "GenericError", Desc: "bad path"}}
			}
			return qgatest.Reply{Return: map[string]any{"pid": 42}}
		case "guest-exec-status":
			return qgatest.Reply{Return: map[string]any{
				"exited": true, "exitcode": 3,
				"out-data": base64.StdEncoding.EncodeToString([]byte("out!")),
				"err-data": base64.StdEncoding.EncodeToString([]byte("err!"))}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	_ = srv
	guestVerbState.exec = guestExecOpts{argv: []string{"/usr/bin/foo", "bar"},
		timeout: 10 * time.Second}
	out := captureOut(t, func() {
		if rc := actGuestExec(); rc != 3 {
			t.Fatalf("exec rc=%d want 3", rc)
		}
	})
	if !strings.Contains(out, "out!") {
		t.Fatalf("exec stdout: %s", out)
	}
	// Agent-side error surfaces through the classifier.
	srv.Close()
	guestVerbState.exec.argv = []string{"/usr/bin/foo"}
	if rc := actGuestExec(); rc == 0 {
		t.Fatal("dead agent exec must fail")
	}
}

func TestGuestXfer(t *testing.T) {
	dir, _ := guestVM(t)
	local := filepath.Join(dir, "up.txt")
	os.WriteFile(local, []byte("payload-123"), 0o600)
	// The fake guest keeps a tiny virtual filesystem.
	store := map[string][]byte{}
	handles := map[float64]string{}
	next := 0.0
	agentSock(t, func(req map[string]any) qgatest.Reply {
		args, _ := req["arguments"].(map[string]any)
		switch req["execute"] {
		case "guest-file-open":
			next++
			handles[next] = args["path"].(string) + ":" + args["mode"].(string)
			return qgatest.Reply{Return: next}
		case "guest-file-write":
			b, _ := base64.StdEncoding.DecodeString(args["buf-b64"].(string))
			p := handles[args["handle"].(float64)]
			p = p[:len(p)-3]
			store[p] = append(store[p], b...)
			return qgatest.Reply{Return: map[string]any{"count": len(b)}}
		case "guest-file-read":
			h := args["handle"].(float64)
			p := handles[h]
			p = p[:len(p)-2]
			b := store[p]
			if len(b) == 0 {
				return qgatest.Reply{Return: map[string]any{"count": 0, "eof": true}}
			}
			return qgatest.Reply{Return: map[string]any{
				"count": len(b), "buf-b64": base64.StdEncoding.EncodeToString(b), "eof": true}}
		case "guest-file-close":
			return qgatest.Reply{Return: map[string]any{}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	// push restricted target → refused before touching the agent.
	guestVerbState.xfer = guestXferOpts{src: local, dst: "/etc/shadow",
		restricted: true, maxSize: 1 << 20}
	if rc := actGuestXfer(true); rc == 0 {
		t.Fatal("restricted push must refuse")
	}
	// Happy push into a guest-visible tmp path.
	guestVerbState.xfer = guestXferOpts{src: local, dst: "/tmp/up.txt",
		restricted: true, maxSize: 1 << 20}
	if rc := actGuestXfer(true); rc != 0 {
		t.Fatalf("push rc=%d", rc)
	}
	if string(store["/tmp/up.txt"]) != "payload-123" {
		t.Fatalf("guest store: %q", store["/tmp/up.txt"])
	}
	// Pull it back into a different local name.
	dst := filepath.Join(dir, "down.txt")
	guestVerbState.xfer = guestXferOpts{src: "/tmp/up.txt", dst: dst, maxSize: 1 << 20}
	if rc := actGuestXfer(false); rc != 0 {
		t.Fatalf("pull rc=%d", rc)
	}
	if b, _ := os.ReadFile(dst); string(b) != "payload-123" {
		t.Fatalf("pulled %q", b)
	}
	// Existing destination refuses without --force.
	guestVerbState.xfer = guestXferOpts{src: "/tmp/up.txt", dst: dst, maxSize: 1 << 20}
	if rc := actGuestXfer(false); rc == 0 {
		t.Fatal("pull over existing must refuse without --force")
	}
}

func TestGuestFSFreezeVerbs(t *testing.T) {
	guestVM(t)
	agentSock(t, qgatest.Static(map[string]any{
		"guest-fsfreeze-status": "thawed",
		"guest-fsfreeze-freeze": float64(4),
		"guest-fsfreeze-thaw":   float64(4),
	}))
	guestVerbState.freeze = guestFreezeOpts{sub: "status"}
	out := captureOut(t, func() {
		if rc := actGuestFSFreeze(); rc != 0 {
			t.Fatalf("status rc=%d", rc)
		}
	})
	if !strings.Contains(out, "thawed") {
		t.Fatalf("status out: %s", out)
	}
	guestVerbState.freeze = guestFreezeOpts{sub: "freeze"}
	if rc := actGuestFSFreeze(); rc != 0 {
		t.Fatalf("freeze rc=%d", rc)
	}
	guestVerbState.freeze = guestFreezeOpts{sub: "freeze", hold: 50 * time.Millisecond}
	if rc := actGuestFSFreeze(); rc != 0 {
		t.Fatalf("freeze --hold rc=%d", rc)
	}
	guestVerbState.freeze = guestFreezeOpts{sub: "thaw"}
	if rc := actGuestFSFreeze(); rc != 0 {
		t.Fatalf("thaw rc=%d", rc)
	}
	guestVerbState.freeze = guestFreezeOpts{sub: "bogus"}
	if rc := actGuestFSFreeze(); rc != 1 {
		t.Fatal("bogus sub must fail")
	}
}

func TestGuestPower(t *testing.T) {
	stubTools(t)
	guestVM(t)
	// No agent: the QMP channel (live socket + stubbed quickemu-qmp)
	// takes the ACPI fallback.
	monSock(t, "-qmp.socket")
	guestVerbState.power = guestPowerOpts{noWait: true, timeout: time.Second}
	if rc := actGuestPower(qga.ShutdownPowerdown); rc != 0 {
		t.Fatalf("monitor fallback rc=%d", rc)
	}
	// Agent path ACKs the shutdown — no-wait reports the request.
	agentSock(t, qgatest.Static(map[string]any{}))
	if rc := actGuestPower(qga.ShutdownPowerdown); rc != 0 {
		t.Fatalf("agent shutdown rc=%d", rc)
	}
	if rc := actGuestPower(qga.ShutdownReboot); rc != 0 {
		t.Fatalf("reboot rc=%d", rc)
	}
}

// ---------- error classification ----------

func TestGuestErrorClassification(t *testing.T) {
	// Stopped VM → socket-missing classification.
	dir, _ := mkLayout(t, "g1")
	_ = dir
	out := captureOut(t, func() {
		if rc := actGuestPing(); rc != 1 {
			t.Fatalf("stopped ping rc=%d", rc)
		}
	})
	if !strings.Contains(out, "no guest-agent channel") {
		t.Fatalf("missing-channel text: %s", out)
	}
	guestVM(t)
	// A socket file that accepts but never answers → not-ready.
	sock := qgaSock()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("no unix sockets")
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	old := outFmt
	outFmt = "json"
	out = captureOut(t, func() {
		if rc := actGuestPing(); rc != 1 {
			t.Fatalf("hung agent rc=%d", rc)
		}
	})
	outFmt = old
	if !strings.Contains(out, `"code":"agent_not_ready"`) {
		t.Fatalf("not-ready envelope: %s", out)
	}
	// AgentError (command-level refusal) classification.
	agentSock(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{
			Class: "CommandDisabled", Desc: "disabled by blacklist"}}
	})
	out = captureOut(t, func() {
		if rc := actGuestPing(); rc != 1 {
			t.Fatalf("refused rc=%d", rc)
		}
	})
	if !strings.Contains(out, "refused the command") {
		t.Fatalf("agent error text: %s", out)
	}
}

// captureOut wraps the shared stdout capture for act-function tests.
func captureOut(t *testing.T, fn func()) string {
	t.Helper()
	return qetest.CaptureStdout(t, fn)
}

// ---------- dispatch, classifier, ctx, status probe ----------

func TestDispatchGuest(t *testing.T) {
	for _, act := range []string{"guest_ping", "guest_info", "guest_exec",
		"guest_push", "guest_pull", "guest_shutdown", "guest_reboot",
		"guest_halt", "guest_fsfreeze", "guest_ip"} {
		if dispatchGuest(act) == nil {
			t.Fatalf("%s has no handler", act)
		}
	}
	if dispatchGuest("nope") != nil {
		t.Fatal("unknown action must not dispatch")
	}
}

func TestGuestFailClassify(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w", qga.ErrSocketMissing), "no guest-agent channel"},
		{fmt.Errorf("dial: %w", qga.ErrAgentUnreachable), "channel unreachable"},
		{fmt.Errorf("%w", qga.ErrAgentNotReady), "not responding"},
		{fmt.Errorf("%w", qga.ErrTimeout), "timed out"},
		{fmt.Errorf("%w", qga.ErrAgentGone), "dropped mid-command"},
		{&qga.AgentError{Class: "Blocked", Desc: "off"}, "refused the command"},
		{fmt.Errorf("%w", qga.ErrBadResponse), "invalid guest-agent reply"},
		{fmt.Errorf("%w", qga.ErrMessageTooBig), "invalid guest-agent reply"},
		{errors.New("odd"), "odd"},
	} {
		out := captureOut(t, func() {
			if rc := guestFail("guest x", tc.err); rc != 1 {
				t.Fatal("guestFail must report 1")
			}
		})
		if !strings.Contains(out, tc.want) {
			t.Fatalf("%v -> %q (want %q)", tc.err, out, tc.want)
		}
	}
	old := outFmt
	outFmt = "json"
	defer func() { outFmt = old }()
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w", qga.ErrSocketMissing), "channel_missing"},
		{fmt.Errorf("%w", qga.ErrAgentUnreachable), "channel_unreachable"},
		{fmt.Errorf("%w", qga.ErrAgentNotReady), "agent_not_ready"},
		{fmt.Errorf("%w", qga.ErrTimeout), "timeout"},
		{fmt.Errorf("%w", qga.ErrAgentGone), "connection_lost"},
		{&qga.AgentError{Class: "Blocked", Desc: "off"}, "agent_error:Blocked"},
		{fmt.Errorf("%w", qga.ErrBadResponse), "invalid_response"},
		{fmt.Errorf("%w", qga.ErrMessageTooBig), "invalid_response"},
		{errors.New("odd"), "guest_error"},
	} {
		out := captureOut(t, func() { guestEmitErr("guest.x", "guest x", tc.err) })
		if !strings.Contains(out, `"code":"`+tc.want+`"`) {
			t.Fatalf("%v -> %s (want code %s)", tc.err, out, tc.want)
		}
	}
}

func TestGuestCtxAndStatusProbe(t *testing.T) {
	ctx, cancel := guestCtx(0)
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("guestCtx(0) must apply the default deadline")
	}
	cancel()
	// No VM → no probe; running VM without a socket → no probe.
	mkLayout(t, "si")
	if guestStatusInfo() != nil {
		t.Fatal("stopped VM must not probe the agent")
	}
	guestVM(t)
	if guestStatusInfo() != nil {
		t.Fatal("missing agent socket must not probe")
	}
	agentSock(t, qgatest.Static(map[string]any{
		"guest-info": map[string]any{"version": "1.2.3"},
		"guest-network-get-interfaces": []any{
			map[string]any{"name": "eth0", "ip-addresses": []any{
				map[string]any{"ip-address": "10.2.3.4", "ip-address-type": "ipv4"}}}},
	}))
	out := guestStatusInfo()
	if out == nil || out["agent"] != true || out["agentVersion"] != "1.2.3" {
		t.Fatalf("status probe: %+v", out)
	}
	v4, _ := out["ipv4"].([]string)
	if len(v4) != 1 || v4[0] != "10.2.3.4" {
		t.Fatalf("status ipv4: %v", out["ipv4"])
	}
	// Partial probe: net answers, info refused → agent:false row.
	agentSock(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-info" {
			return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "X", Desc: "y"}}
		}
		return qgatest.Reply{Return: []any{}}
	})
	out = guestStatusInfo()
	if out == nil || out["agent"] != false {
		t.Fatalf("partial probe: %+v", out)
	}
	// Both calls refused → nil (status must never stall on a dead agent).
	agentSock(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "X", Desc: "y"}}
	})
	if guestStatusInfo() != nil {
		t.Fatal("all-refused probe must be nil")
	}
}

// ---------- exec paths ----------

func TestGuestExecPaths(t *testing.T) {
	guestVM(t)
	// Empty argv never reaches the dial.
	guestVerbState.exec = guestExecOpts{}
	if rc := actGuestExec(); rc != 1 {
		t.Fatal("empty argv must fail")
	}
	srv := agentSock(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-exec":
			args := req["arguments"].(map[string]any)
			if args["path"] != "/bin/sh" {
				return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "E", Desc: "want /bin/sh"}}
			}
			env, _ := args["env"].([]any)
			var hasA, hasPath bool
			for _, e := range env {
				if e == "A=1" {
					hasA = true
				}
				if s, _ := e.(string); strings.HasPrefix(s, "PATH=") {
					hasPath = true
				}
			}
			if !hasA || !hasPath {
				return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "E", Desc: "env merge failed"}}
			}
			return qgatest.Reply{Return: map[string]any{"pid": 7}}
		case "guest-exec-status":
			return qgatest.Reply{Return: map[string]any{
				"exited": true, "exitcode": 0}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	// --shell wraps in /bin/sh -c; --env layers over the default PATH.
	guestVerbState.exec = guestExecOpts{argv: []string{"echo hi"}, shell: true,
		env: []string{"A=1"}, timeout: 5 * time.Second}
	if rc := actGuestExec(); rc != 0 {
		t.Fatalf("shell exec rc=%d", rc)
	}
	// JSON envelope reports the exited process.
	old := outFmt
	outFmt = "json"
	out := captureOut(t, func() {
		if rc := actGuestExec(); rc != 0 {
			t.Fatalf("json exec rc=%d", rc)
		}
	})
	outFmt = old
	if !strings.Contains(out, `"success":true`) || !strings.Contains(out, `"exitCode":0`) {
		t.Fatalf("exec envelope: %s", out)
	}
	srv.Close()
	// Signal death → 128+sig.
	agentSock(t, qgatest.Static(map[string]any{
		"guest-exec":        map[string]any{"pid": 9},
		"guest-exec-status": map[string]any{"exited": true, "signal": 9},
	}))
	guestVerbState.exec = guestExecOpts{argv: []string{"x"}, timeout: 5 * time.Second}
	if rc := actGuestExec(); rc != 128+9 {
		t.Fatalf("signal exec rc=%d", rc)
	}
	// Exited without an exitcode → generic failure.
	agentSock(t, qgatest.Static(map[string]any{
		"guest-exec":        map[string]any{"pid": 9},
		"guest-exec-status": map[string]any{"exited": true},
	}))
	if rc := actGuestExec(); rc != 1 {
		t.Fatalf("no-exitcode exec rc=%d", rc)
	}
	// Status poll refuses mid-wait: partial result + error — text
	// prints captured output then classifies; JSON emits exec_failed.
	agentSock(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-exec-status" {
			return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "Poll", Desc: "refused"}}
		}
		return qgatest.Reply{Return: map[string]any{"pid": 5}}
	})
	guestVerbState.exec.argv = []string{"x"}
	out = captureOut(t, func() {
		if rc := actGuestExec(); rc != 1 {
			t.Fatalf("poll-error exec rc=%d", rc)
		}
	})
	if !strings.Contains(out, "refused the command") {
		t.Fatalf("poll-error out: %s", out)
	}
	outFmt = "json"
	out = captureOut(t, func() {
		if rc := actGuestExec(); rc != 1 {
			t.Fatalf("poll-error json rc=%d", rc)
		}
	})
	outFmt = old
	if !strings.Contains(out, `"code":"exec_failed"`) {
		t.Fatalf("poll-error envelope: %s", out)
	}
	// A guest process that never exits hits the caller deadline:
	// partial output still lands, rc 1.
	agentSock(t, qgatest.Static(map[string]any{
		"guest-exec": map[string]any{"pid": 3},
		"guest-exec-status": map[string]any{
			"out-data": base64.StdEncoding.EncodeToString([]byte("tick"))},
	}))
	guestVerbState.exec = guestExecOpts{argv: []string{"x"}, timeout: 300 * time.Millisecond}
	out = captureOut(t, func() {
		if rc := actGuestExec(); rc != 1 {
			t.Fatalf("hung exec rc=%d", rc)
		}
	})
	if !strings.Contains(out, "tick") || !strings.Contains(out, "timed out") {
		t.Fatalf("hung exec out: %s", out)
	}
	// guest-exec itself refused → the res==nil emit path.
	agentSock(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "Exec", Desc: "disabled"}}
	})
	out = captureOut(t, func() {
		if rc := actGuestExec(); rc != 1 {
			t.Fatalf("exec-refused rc=%d", rc)
		}
	})
	if !strings.Contains(out, "refused the command") {
		t.Fatalf("exec-refused out: %s", out)
	}
}

// ---------- ip extras ----------

func TestGuestIPMore(t *testing.T) {
	// Stopped VM refuses before dialing.
	mkLayout(t, "ip2")
	guestVerbState.ip = guestIPOpts{}
	if rc := actGuestIP(); rc != 1 {
		t.Fatal("stopped VM ip must fail")
	}
	guestVM(t)
	// Agent refusal classifies through guestEmitErr.
	agentSock(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "Net", Desc: "denied"}}
	})
	if rc := actGuestIP(); rc != 1 {
		t.Fatal("agent refusal must fail")
	}
	// Only unusable ifaces → the friendly empty state.
	agentSock(t, qgatest.Static(map[string]any{
		"guest-network-get-interfaces": []any{
			map[string]any{"name": "lo", "ip-addresses": []any{
				map[string]any{"ip-address": "127.0.0.1", "ip-address-type": "ipv4"}}},
			map[string]any{"name": "br0", "ip-addresses": []any{
				map[string]any{"ip-address": "10.1.1.1", "ip-address-type": "ipv4"}}}},
	}))
	out := captureOut(t, func() {
		if rc := actGuestIP(); rc != 0 {
			t.Fatalf("empty ip rc=%d", rc)
		}
	})
	if !strings.Contains(out, "no usable guest addresses") {
		t.Fatalf("empty ip out: %s", out)
	}
	// -4 -6 together prints both families (--iface pins eth0 so the
	// memoized empty reply from the previous call is bypassed).
	agentSock(t, qgatest.Static(map[string]any{
		"guest-network-get-interfaces": []any{
			map[string]any{"name": "eth0", "ip-addresses": []any{
				map[string]any{"ip-address": "10.5.5.5", "ip-address-type": "ipv4"},
				map[string]any{"ip-address": "fd00::55", "ip-address-type": "ipv6"}}}},
	}))
	guestVerbState.ip = guestIPOpts{iface: "eth0", v4: true, v6: true}
	out = captureOut(t, func() {
		if rc := actGuestIP(); rc != 0 {
			t.Fatalf("dual ip rc=%d", rc)
		}
	})
	guestVerbState.ip = guestIPOpts{}
	if !strings.Contains(out, "10.5.5.5") || !strings.Contains(out, "fd00::55") {
		t.Fatalf("dual out: %s", out)
	}
}

// ---------- xfer guards / verify / force ----------

// guestFSHandler is a tiny in-memory guest filesystem for xfer tests:
// file handles map to "<path>|<mode>" so read/write resolve the path.
func guestFSHandler(store map[string][]byte) qgatest.Handler {
	handles := map[float64]string{}
	next := 0.0
	return func(req map[string]any) qgatest.Reply {
		args, _ := req["arguments"].(map[string]any)
		switch req["execute"] {
		case "guest-file-open":
			next++
			handles[next] = args["path"].(string) + "|" + args["mode"].(string)
			return qgatest.Reply{Return: next}
		case "guest-file-write":
			b, _ := base64.StdEncoding.DecodeString(args["buf-b64"].(string))
			p := strings.SplitN(handles[args["handle"].(float64)], "|", 2)[0]
			store[p] = append(store[p], b...)
			return qgatest.Reply{Return: map[string]any{"count": len(b)}}
		case "guest-file-read":
			p := strings.SplitN(handles[args["handle"].(float64)], "|", 2)[0]
			b := store[p]
			if len(b) == 0 {
				return qgatest.Reply{Return: map[string]any{"count": 0, "eof": true}}
			}
			return qgatest.Reply{Return: map[string]any{
				"count": len(b), "buf-b64": base64.StdEncoding.EncodeToString(b), "eof": true}}
		case "guest-file-close":
			return qgatest.Reply{Return: map[string]any{}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	}
}

func TestGuestXferGuards(t *testing.T) {
	dir, _ := guestVM(t)
	// Local-side guards refuse before the agent is even dialled.
	guestVerbState.xfer = guestXferOpts{src: filepath.Join(dir, "nope"),
		dst: "/tmp/x", maxSize: 1 << 20}
	if rc := actGuestXfer(true); rc != 1 {
		t.Fatal("missing src must fail")
	}
	guestVerbState.xfer = guestXferOpts{src: dir, dst: "/tmp/x", maxSize: 1 << 20}
	if rc := actGuestXfer(true); rc != 1 {
		t.Fatal("dir src must fail")
	}
	big := filepath.Join(dir, "big.bin")
	os.WriteFile(big, make([]byte, 64), 0o600)
	guestVerbState.xfer = guestXferOpts{src: big, dst: "/tmp/x", maxSize: 8}
	if rc := actGuestXfer(true); rc != 1 {
		t.Fatal("oversize src must fail")
	}
	// Agent-side refusal classifies through guestEmitErr.
	local := filepath.Join(dir, "ok.bin")
	os.WriteFile(local, []byte("x"), 0o600)
	agentSock(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "IOError", Desc: "denied"}}
	})
	guestVerbState.xfer = guestXferOpts{src: local, dst: "/tmp/x", maxSize: 1 << 20}
	if rc := actGuestXfer(true); rc != 1 {
		t.Fatal("agent refusal must fail")
	}
}

func TestGuestXferVerifyForceJSON(t *testing.T) {
	dir, _ := guestVM(t)
	local := filepath.Join(dir, "v.bin")
	os.WriteFile(local, []byte("verify-me"), 0o600)
	store := map[string][]byte{}
	srv := agentSock(t, guestFSHandler(store))
	_ = srv
	// --verify re-reads the remote copy and compares the hash.
	guestVerbState.xfer = guestXferOpts{src: local, dst: "/tmp/v.bin",
		verify: true, maxSize: 1 << 20}
	out := captureOut(t, func() {
		if rc := actGuestXfer(true); rc != 0 {
			t.Fatalf("verify push rc=%d", rc)
		}
	})
	if !strings.Contains(out, "verified") {
		t.Fatalf("verify out: %s", out)
	}
	// A tampered remote copy refuses the transfer.
	store["/tmp/v.bin"] = []byte("tampered")
	if rc := actGuestXfer(true); rc != 1 {
		t.Fatal("tampered remote must fail verify")
	}
	// pull --force over an existing file + the JSON envelope.
	dst := filepath.Join(dir, "out.bin")
	os.WriteFile(dst, []byte("old"), 0o600)
	guestVerbState.xfer = guestXferOpts{src: "/tmp/v.bin", dst: dst,
		force: true, maxSize: 1 << 20}
	old := outFmt
	outFmt = "json"
	out = captureOut(t, func() {
		if rc := actGuestXfer(false); rc != 0 {
			t.Fatalf("force pull rc=%d", rc)
		}
	})
	outFmt = old
	if !strings.Contains(out, `"success":true`) || !strings.Contains(out, `"sha256"`) {
		t.Fatalf("pull envelope: %s", out)
	}
	if b, _ := os.ReadFile(dst); string(b) != "tamperedverify-me" {
		t.Fatalf("force-pull wrote %q", b)
	}
}

// ---------- power wait / force ----------

func TestGuestPowerWaitForce(t *testing.T) {
	stubTools(t)
	// Not running at all → the early gate, no agent contact.
	mkLayout(t, "pw0")
	guestVerbState.power = guestPowerOpts{}
	if rc := actGuestPower(qga.ShutdownPowerdown); rc != 1 {
		t.Fatal("stopped VM power must fail")
	}
	guestVM(t)
	monSock(t, "-qmp.socket")
	// Agent refuses → warn + ACPI fallback through the live qmp sock
	// (halt rides the same system_powerdown path).
	agentSock(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-shutdown" {
			return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "P", Desc: "nope"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	guestVerbState.power = guestPowerOpts{noWait: true, timeout: time.Second}
	if rc := actGuestPower(qga.ShutdownHalt); rc != 0 {
		t.Fatalf("acpi halt rc=%d", rc)
	}
	// Graceful ACK, then the VM actually leaves running mid-wait —
	// the JSON envelope reports via:qga.
	agentSock(t, qgatest.Static(map[string]any{}))
	old := outFmt
	outFmt = "json"
	guestVerbState.power = guestPowerOpts{timeout: 5 * time.Second}
	out := captureOut(t, func() {
		done := make(chan int, 1)
		go func() { done <- actGuestPower(qga.ShutdownPowerdown) }()
		time.Sleep(300 * time.Millisecond)
		os.Remove(pidPath())
		select {
		case rc := <-done:
			if rc != 0 {
				t.Fatalf("waited shutdown rc=%d", rc)
			}
		case <-time.After(6 * time.Second):
			t.Fatal("waited shutdown hung")
		}
	})
	outFmt = old
	if !strings.Contains(out, `"via":"qga"`) {
		t.Fatalf("waited envelope: %s", out)
	}
}

func TestGuestPowerWaitTimeout(t *testing.T) {
	stubTools(t)
	guestVM(t)
	agentSock(t, qgatest.Static(map[string]any{}))
	// The request lands but the VM never leaves running → guidance.
	guestVerbState.power = guestPowerOpts{timeout: 400 * time.Millisecond}
	out := captureOut(t, func() {
		if rc := actGuestPower(qga.ShutdownPowerdown); rc != 1 {
			t.Fatalf("linger shutdown rc=%d", rc)
		}
	})
	if !strings.Contains(out, "still running") {
		t.Fatalf("linger out: %s", out)
	}
}

func TestGuestPowerForceKill(t *testing.T) {
	stubTools(t)
	guestVM(t)
	// No agent, no monitor channel: --force escalates straight to the
	// hard-stop path (monitor quit → SIGKILL as last resort).
	guestVerbState.power = guestPowerOpts{force: true, timeout: 400 * time.Millisecond}
	out := captureOut(t, func() {
		if rc := actGuestPower(qga.ShutdownPowerdown); rc != 0 {
			t.Fatalf("forced shutdown rc=%d", rc)
		}
	})
	if !strings.Contains(out, "forced") {
		t.Fatalf("force out: %s", out)
	}
}

// ---------- fsfreeze JSON + ping envelope ----------

func TestGuestFSFreezeJSON(t *testing.T) {
	guestVM(t)
	agentSock(t, qgatest.Static(map[string]any{
		"guest-fsfreeze-status": "frozen",
		"guest-fsfreeze-freeze": float64(2),
		"guest-fsfreeze-thaw":   float64(2),
	}))
	old := outFmt
	outFmt = "json"
	defer func() { outFmt = old }()
	guestVerbState.freeze = guestFreezeOpts{sub: "status"}
	out := captureOut(t, func() {
		if rc := actGuestFSFreeze(); rc != 0 {
			t.Fatalf("json status rc=%d", rc)
		}
	})
	if !strings.Contains(out, `"state":"frozen"`) {
		t.Fatalf("json status: %s", out)
	}
	guestVerbState.freeze = guestFreezeOpts{sub: "freeze"}
	out = captureOut(t, func() {
		if rc := actGuestFSFreeze(); rc != 0 {
			t.Fatalf("json freeze rc=%d", rc)
		}
	})
	if !strings.Contains(out, `"filesystems":2`) {
		t.Fatalf("json freeze: %s", out)
	}
	guestVerbState.freeze = guestFreezeOpts{sub: "freeze", hold: 20 * time.Millisecond}
	out = captureOut(t, func() {
		if rc := actGuestFSFreeze(); rc != 0 {
			t.Fatalf("json hold rc=%d", rc)
		}
	})
	if !strings.Contains(out, `"hold":"20ms"`) {
		t.Fatalf("json hold: %s", out)
	}
	guestVerbState.freeze = guestFreezeOpts{sub: "thaw"}
	out = captureOut(t, func() {
		if rc := actGuestFSFreeze(); rc != 0 {
			t.Fatalf("json thaw rc=%d", rc)
		}
	})
	if !strings.Contains(out, `"state":"thawed"`) {
		t.Fatalf("json thaw: %s", out)
	}
	// Agent-side refusal → classified envelope.
	agentSock(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "FS", Desc: "busy"}}
	})
	guestVerbState.freeze = guestFreezeOpts{sub: "status"}
	out = captureOut(t, func() {
		if rc := actGuestFSFreeze(); rc != 1 {
			t.Fatalf("fsfreeze err rc=%d", rc)
		}
	})
	if !strings.Contains(out, `"code":"agent_error:FS"`) {
		t.Fatalf("fsfreeze err envelope: %s", out)
	}
}

func TestGuestPingJSON(t *testing.T) {
	guestVM(t)
	agentSock(t, qgatest.Static(map[string]any{}))
	old := outFmt
	outFmt = "json"
	out := captureOut(t, func() {
		if rc := actGuestPing(); rc != 0 {
			t.Fatalf("json ping rc=%d", rc)
		}
	})
	outFmt = old
	if !strings.Contains(out, `"alive":true`) {
		t.Fatalf("ping envelope: %s", out)
	}
	// A refused ping classifies like any other verb.
	agentSock(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "Ping", Desc: "no"}}
	})
	out = captureOut(t, func() {
		if rc := actGuestPing(); rc != 1 {
			t.Fatalf("refused ping rc=%d", rc)
		}
	})
	if !strings.Contains(out, "refused the command") {
		t.Fatalf("refused ping out: %s", out)
	}
}

// ---------- verb aliases / flag tails ----------

func TestGuestParseAliases(t *testing.T) {
	oldVM, oldFmt := vmConf, outFmt
	defer func() { vmConf, outFmt = oldVM, oldFmt }()
	guestReset()
	guestIntercept([]string{"guest", "status"})
	if actions[0] != "guest_info" {
		t.Fatalf("status alias: %v", actions)
	}
	guestReset()
	guestIntercept([]string{"guest", "run", "id"})
	if actions[0] != "guest_exec" || guestVerbState.exec.argv[0] != "id" {
		t.Fatalf("run alias: %+v", guestVerbState.exec)
	}
	guestReset()
	guestIntercept([]string{"guest", "get", "/g", "/l"})
	if actions[0] != "guest_pull" {
		t.Fatalf("get alias: %v", actions)
	}
	guestReset()
	guestIntercept([]string{"guest", "poweroff"})
	if actions[0] != "guest_shutdown" {
		t.Fatalf("poweroff alias: %v", actions)
	}
	guestReset()
	guestIntercept([]string{"guest", "halt"})
	if actions[0] != "guest_halt" {
		t.Fatalf("halt: %v", actions)
	}
	guestReset()
	guestIntercept([]string{"guest", "freeze", "thaw"})
	if actions[0] != "guest_fsfreeze" || guestVerbState.freeze.sub != "thaw" {
		t.Fatalf("freeze alias: %+v", guestVerbState.freeze)
	}
	guestReset()
	guestIntercept([]string{"guest", "fsfreeze"})
	if guestVerbState.freeze.sub != "status" {
		t.Fatalf("bare fsfreeze sub=%q", guestVerbState.freeze.sub)
	}
	guestReset()
	guestIntercept([]string{"ip", "--iface", "eth9", "-4", "--ipv6"})
	if guestVerbState.ip.iface != "eth9" || !guestVerbState.ip.v4 || !guestVerbState.ip.v6 {
		t.Fatalf("ip flags: %+v", guestVerbState.ip)
	}
	guestReset()
	// --format human normalises to text; json passes through.
	guestIntercept([]string{"guest", "ping", "--format", "human"})
	if outFmt != "text" {
		t.Fatalf("human normalisation: %q", outFmt)
	}
	guestReset()
	guestIntercept([]string{"guest", "ping", "--format", "json"})
	if outFmt != "json" {
		t.Fatalf("json fmt: %q", outFmt)
	}
	outFmt = oldFmt
	guestReset()
	// '--' keeps dashed argv literal; --unsafe drops the restricted
	// push default; --verify/--restricted set on pull.
	guestIntercept([]string{"guest", "exec", "--", "-x", "-y"})
	if len(guestVerbState.exec.argv) != 2 || guestVerbState.exec.argv[0] != "-x" {
		t.Fatalf("dash argv: %v", guestVerbState.exec.argv)
	}
	guestReset()
	guestIntercept([]string{"guest", "push", "a", "b", "--unsafe"})
	if guestVerbState.xfer.restricted {
		t.Fatal("--unsafe must clear restricted")
	}
	guestReset()
	guestIntercept([]string{"guest", "pull", "a", "b", "--unsafe", "--verify"})
	if guestVerbState.xfer.restricted || !guestVerbState.xfer.verify {
		t.Fatalf("pull unsafe+verify: %+v", guestVerbState.xfer)
	}
	guestReset()
	guestIntercept([]string{"guest", "shutdown", "--no-wait"})
	if !guestVerbState.power.noWait {
		t.Fatal("--no-wait")
	}
	guestReset()
}

func TestGuestParseErrors2(t *testing.T) {
	stubTools(t)
	dir := t.TempDir()
	conf := filepath.Join(dir, "g2.conf")
	os.WriteFile(conf, nil, 0o644)
	for _, tc := range []struct {
		name string
		args []string
		rc   int
	}{
		{"info-flag", []string{"guest", "info", "--bogus"}, 1},
		{"exec-noval", []string{"guest", "exec", "--env"}, 1},
		{"exec-badfmt", []string{"guest", "exec", "--format", "yaml", "x"}, 1},
		{"exec-badflag", []string{"guest", "exec", "--bogus", "x"}, 1},
		{"xfer-noval", []string{"guest", "push", "a", "b", "--max-size"}, 1},
		{"xfer-fmt", []string{"guest", "pull", "a", "b", "--format", "yaml"}, 1},
		{"xfer-flag", []string{"guest", "push", "a", "b", "--bogus"}, 1},
		{"power-badflag", []string{"guest", "shutdown", "--bogus"}, 1},
		{"power-noval", []string{"guest", "shutdown", "--timeout"}, 1},
		{"power-fmt", []string{"guest", "reboot", "--format", "yaml"}, 1},
		{"freeze-2pos", []string{"guest", "fsfreeze", "a", "b"}, 1},
		{"freeze-noval", []string{"guest", "fsfreeze", "freeze", "--hold"}, 1},
		{"freeze-badflag", []string{"guest", "fsfreeze", "--bogus"}, 1},
		{"freeze-fmt", []string{"guest", "fsfreeze", "status", "--format", "yaml"}, 1},
		{"ip-badflag", []string{"ip", "--bogus"}, 1},
		{"ip-noval", []string{"ip", "--iface"}, 1},
		{"ip-fmt", []string{"ip", "--format", "yaml"}, 1},
		{"shared-novm", []string{"guest", "ping", "--vm"}, 1},
		{"shared-nofmt", []string{"guest", "ping", "--format"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, rc := runMain(t, tc.args...)
			if rc != tc.rc {
				t.Fatalf("rc=%d want %d out=%s", rc, tc.rc, out)
			}
		})
	}
}
