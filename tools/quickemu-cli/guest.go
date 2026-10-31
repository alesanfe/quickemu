// guest.go: the guest-* verb family — QEMU Guest Agent operations
// over the universal agent channel (<vmdir>/<vm>-agent.sock, emitted
// for every display mode by quickemu-launch). The verbs are thin
// orchestration over tools/quickemu-qga: dial, classify the typed
// errors for the user, and shape the envelope/text output.
//
// Error text in this file is the user-facing contract: normal errors
// go to stdout (echo parity); only trust/warning lines hit stderr.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/quickemu-qga"
)

// qgaSock: the agent channel endpoint — the same <vm>-agent.sock
// name the launcher binds and runtimeCleanup reaps.
func qgaSock() string {
	return filepath.Join(vmdir(), vmname()+"-agent.sock")
}

// guestVerbState carries the parse output for one queued guest
// action (parse fills it, dispatchGuest's act functions consume it).
var guestVerbState struct {
	exec   guestExecOpts
	xfer   guestXferOpts
	power  guestPowerOpts
	freeze guestFreezeOpts
	ip     guestIPOpts
}

type guestExecOpts struct {
	argv      []string
	shell     bool
	env       []string
	timeout   time.Duration
	maxOutput int
}

type guestXferOpts struct {
	src, dst   string
	verify     bool
	restricted bool
	force      bool
	maxSize    int64
}

type guestPowerOpts struct {
	timeout time.Duration
	force   bool
	noWait  bool
}

type guestFreezeOpts struct {
	sub  string // status|freeze|thaw
	hold time.Duration
}

type guestIPOpts struct {
	iface string
	v4    bool
	v6    bool
}

// dispatchGuest: the guest-* action family — one slot in the
// dispatch table (registered beside the other families).
func dispatchGuest(act string) func() int {
	switch act {
	case "guest_ping":
		return actGuestPing
	case "guest_info":
		return actGuestInfo
	case "guest_exec":
		return actGuestExec
	case "guest_push":
		return func() int { return actGuestXfer(true) }
	case "guest_pull":
		return func() int { return actGuestXfer(false) }
	case "guest_shutdown":
		return func() int { return actGuestPower(qga.ShutdownPowerdown) }
	case "guest_reboot":
		return func() int { return actGuestPower(qga.ShutdownReboot) }
	case "guest_halt":
		return func() int { return actGuestPower(qga.ShutdownHalt) }
	case "guest_fsfreeze":
		return actGuestFSFreeze
	case "guest_ip":
		return actGuestIP
	}
	return nil
}

// guestFail maps the client's typed errors onto user-facing lines —
// the socket-missing / agent-absent / not-ready / timeout / bad-reply
// distinction is the actionable part of the error contract.
func guestFail(verb string, err error) int {
	var ae *qga.AgentError
	switch {
	case errors.Is(err, qga.ErrSocketMissing):
		fmt.Printf(" - %s: no guest-agent channel — the VM is not running or was launched without one (qga=off).\n", verb)
	case errors.Is(err, qga.ErrAgentUnreachable):
		fmt.Printf(" - %s: guest-agent channel unreachable: %v\n", verb, err)
	case errors.Is(err, qga.ErrAgentNotReady):
		fmt.Printf(" - %s: guest-agent not responding — is qemu-guest-agent installed and running in the guest?\n", verb)
	case errors.Is(err, qga.ErrTimeout):
		fmt.Printf(" - %s: timed out waiting for the guest.\n", verb)
	case errors.Is(err, qga.ErrAgentGone):
		fmt.Printf(" - %s: guest-agent connection dropped mid-command.\n", verb)
	case errors.As(err, &ae):
		fmt.Printf(" - %s: guest refused the command (%s: %s) — install/enable the feature in qemu-guest-agent.\n", verb, ae.Class, ae.Desc)
	case errors.Is(err, qga.ErrBadResponse), errors.Is(err, qga.ErrMessageTooBig):
		fmt.Printf(" - %s: invalid guest-agent reply.\n", verb)
	default:
		fmt.Printf(" - %s: %v\n", verb, err)
	}
	return 1
}

// guestEmitErr mirrors guestFail through the v1 envelope — JSON
// callers get the machine-readable error, text callers the line.
func guestEmitErr(cmd, verb string, err error) int {
	if outFmt == "json" {
		code := "guest_error"
		var ae *qga.AgentError
		switch {
		case errors.Is(err, qga.ErrSocketMissing):
			code = "channel_missing"
		case errors.Is(err, qga.ErrAgentUnreachable):
			code = "channel_unreachable"
		case errors.Is(err, qga.ErrAgentNotReady):
			code = "agent_not_ready"
		case errors.Is(err, qga.ErrTimeout):
			code = "timeout"
		case errors.Is(err, qga.ErrAgentGone):
			code = "connection_lost"
		case errors.As(err, &ae):
			code = "agent_error:" + ae.Class
		case errors.Is(err, qga.ErrBadResponse), errors.Is(err, qga.ErrMessageTooBig):
			code = "invalid_response"
		}
		emitFail(cmd, code, err.Error())
		return 1
	}
	return guestFail(verb, err)
}

// guestDial opens the agent channel for the current --vm: the guest
// must be alive — probing first keeps "not running" a clearer story
// than a raw dial error.
func guestDial(timeout time.Duration) (*qga.Client, error) {
	if !vmRunning() {
		return nil, fmt.Errorf("%w (VM is not running)", qga.ErrSocketMissing)
	}
	return qga.Dial(qgaSock(), timeout)
}

// guestCtx derives the command context — verb timeouts land here so
// a hung agent cannot park the CLI forever.
func guestCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return context.WithTimeout(context.Background(), timeout)
}

// ---------- guest ping / info ----------

func actGuestPing() int {
	cl, err := guestDial(5 * time.Second)
	if err != nil {
		return guestEmitErr("guest.ping", "guest ping", err)
	}
	defer cl.Close()
	ctx, cancel := guestCtx(10 * time.Second)
	defer cancel()
	if err := cl.Ping(ctx); err != nil {
		return guestEmitErr("guest.ping", "guest ping", err)
	}
	if outFmt == "json" {
		emitOK("guest.ping", map[string]any{"vm": vmname(), "alive": true})
		return 0
	}
	fmt.Printf(" - %s: guest agent is alive.\n", vmname())
	return 0
}

func actGuestInfo() int {
	cl, err := guestDial(8 * time.Second)
	if err != nil {
		return guestEmitErr("guest.info", "guest info", err)
	}
	defer cl.Close()
	ctx, cancel := guestCtx(20 * time.Second)
	defer cancel()
	out := map[string]any{"vm": vmname()}
	if gi, err := cl.Info(ctx); err == nil {
		out["agentVersion"] = gi.Version
		n := 0
		for _, sc := range gi.SupportedCommands {
			if sc.Enabled {
				n++
			}
		}
		out["supportedCommands"] = n
	}
	host, _ := cl.HostName(ctx)
	out["hostname"] = host
	if osi, err := cl.OSInfo(ctx); err == nil {
		out["os"] = osi
	}
	if t, err := cl.GuestTime(ctx); err == nil {
		out["guestTime"] = t.Format(time.RFC3339)
	}
	if fs, err := cl.FSInfo(ctx); err == nil {
		out["filesystems"] = len(fs)
	}
	if outFmt == "json" {
		emitOK("guest.info", out)
		return 0
	}
	fmt.Printf(" - %s guest agent report\n", vmname())
	if v, ok := out["agentVersion"]; ok {
		fmt.Printf("   Agent:    %v (%v commands enabled)\n", v, out["supportedCommands"])
	}
	fmt.Printf("   Hostname: %s\n", host)
	if osi, ok := out["os"].(*qga.OSInfo); ok && osi != nil {
		fmt.Printf("   OS:       %s\n", osi.PrettyName)
		fmt.Printf("   Kernel:   %s %s\n", osi.KernelRelease, osi.Architecture)
	}
	if t, ok := out["guestTime"]; ok {
		fmt.Printf("   Time:     %v\n", t)
	}
	if n, ok := out["filesystems"]; ok {
		fmt.Printf("   Filesystems: %v\n", n)
	}
	return 0
}

// ---------- quickemu ip ----------

// unusableGuestIface: the ip verb filters interfaces that can never
// carry a guest-reachable address — loopback, Linux bridge/switch
// plumbing and container veth pairs (a guest's own bridges report
// peers, never a usable host-facing address).
func unusableGuestIface(name string) bool {
	n := strings.ToLower(name)
	switch {
	case n == "lo", strings.HasPrefix(n, "lo:"):
		return true
	case strings.HasPrefix(n, "br"), strings.HasPrefix(n, "bridge"),
		strings.HasPrefix(n, "virbr"), strings.HasPrefix(n, "veth"),
		strings.HasPrefix(n, "docker"), strings.HasPrefix(n, "cni"),
		strings.HasPrefix(n, "flannel"), strings.HasPrefix(n, "vnet"):
		return true
	}
	return false
}

// guestIPs: query the agent and split usable addresses by family —
// link-local v6 and loopback addresses never surface.
func guestIPs(ctx context.Context, cl *qga.Client) (v4, v6 []string, err error) {
	ifs, err := cl.NetInterfaces(ctx)
	if err != nil {
		return nil, nil, err
	}
	want := guestVerbState.ip.iface
	for _, inf := range ifs {
		if want != "" && inf.Name != want {
			continue
		}
		if unusableGuestIface(inf.Name) {
			continue
		}
		for _, a := range inf.IPAddresses {
			ip := net.ParseIP(a.Address)
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if a.Type == "ipv4" || ip.To4() != nil {
				v4 = append(v4, a.Address)
			} else {
				v6 = append(v6, a.Address)
			}
		}
	}
	return v4, v6, nil
}

// ipCache: the short-lived answer cache — guest-network-get-
// interfaces is not free, so `quickemu ip` memoizes the reply keyed
// on the VM's identity. Any state transition (stop/start relaunch,
// pid reuse) rewrites the pidfile, which changes the key and drops
// the stale entry.
type ipCache struct {
	Key string   `json:"key"`
	At  int64    `json:"at"`
	V4  []string `json:"ipv4"`
	V6  []string `json:"ipv6"`
}

func ipCacheFile() string {
	return filepath.Join(vmdir(), ".ip-cache.json")
}

// ipCacheKey: pid + pidfile mtime — a relaunch rewrites the pidfile
// so the key rolls with every state transition.
func ipCacheKey() string {
	pid := readPID()
	mt := int64(0)
	if st, err := os.Stat(pidPath()); err == nil {
		mt = st.ModTime().UnixNano()
	}
	return fmt.Sprintf("%d:%d", pid, mt)
}

func ipCacheRead() *ipCache {
	b, err := os.ReadFile(ipCacheFile())
	if err != nil {
		return nil
	}
	var c ipCache
	if json.Unmarshal(b, &c) != nil {
		return nil
	}
	// Bounded freshness on top of the state key: addresses can change
	// inside a single boot too (DHCP renewals) — 10s is the staleness
	// window the runtime-state cache is allowed to carry.
	if c.Key != ipCacheKey() || time.Since(time.Unix(0, c.At)) > 10*time.Second {
		return nil
	}
	return &c
}

func ipCacheWrite(v4, v6 []string) {
	b, _ := json.Marshal(ipCache{Key: ipCacheKey(),
		At: time.Now().UnixNano(), V4: v4, V6: v6})
	tmp := ipCacheFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		os.Rename(tmp, ipCacheFile())
	}
}

func actGuestIP() int {
	if !vmRunning() {
		fmt.Printf(" - %s is not running.\n", vmname())
		return 1
	}
	var v4, v6 []string
	if c := ipCacheRead(); c != nil && guestVerbState.ip.iface == "" {
		v4, v6 = c.V4, c.V6
	} else {
		cl, err := guestDial(5 * time.Second)
		if err != nil {
			return guestEmitErr("vm.ip", "ip", err)
		}
		defer cl.Close()
		ctx, cancel := guestCtx(10 * time.Second)
		defer cancel()
		v4, v6, err = guestIPs(ctx, cl)
		if err != nil {
			return guestEmitErr("vm.ip", "ip", err)
		}
		if guestVerbState.ip.iface == "" {
			ipCacheWrite(v4, v6)
		}
	}
	show4 := !guestVerbState.ip.v6 || guestVerbState.ip.v4
	show6 := guestVerbState.ip.v6
	if outFmt == "json" {
		emitOK("vm.ip", map[string]any{"vm": vmname(),
			"ipv4": v4, "ipv6": v6, "iface": guestVerbState.ip.iface})
		return 0
	}
	printed := false
	if show4 {
		for _, a := range v4 {
			fmt.Println(a)
			printed = true
		}
	}
	if show6 {
		for _, a := range v6 {
			fmt.Println(a)
			printed = true
		}
	}
	if !printed {
		fmt.Printf(" - %s: no usable guest addresses reported.\n", vmname())
	}
	return 0
}

// guestStatusInfo: the status --json integration — when the VM runs
// and the agent answers quickly, embed the guest's addresses (QE-016).
// Best-effort: a missing/hung agent must never stall --status.
func guestStatusInfo() map[string]any {
	st, _ := state()
	if st != "running" && st != "paused" {
		return nil
	}
	if fi, err := os.Stat(qgaSock()); err != nil || fi.Mode()&os.ModeSocket == 0 {
		return nil
	}
	cl, err := qga.Dial(qgaSock(), 1500*time.Millisecond)
	if err != nil {
		return nil
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	gi, gerr := cl.Info(ctx)
	v4, v6, nerr := guestIPs(ctx, cl)
	if gerr != nil && nerr != nil {
		return nil
	}
	out := map[string]any{"agent": gerr == nil, "ipv4": v4, "ipv6": v6}
	if gerr == nil {
		out["agentVersion"] = gi.Version
	}
	return out
}

// ---------- guest-exec ----------

// defaultGuestEnv: controlled environment for guest-exec — a fixed
// PATH baseline rather than inheriting anything from the host (the
// agent substitutes its own defaults, which would silently vary the
// command's environment across guests).
func defaultGuestEnv() []string {
	return []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
}

func actGuestExec() int {
	e := guestVerbState.exec
	if len(e.argv) == 0 {
		fmt.Println(" - guest exec: nothing to run (append the command after --).")
		return 1
	}
	var spec qga.ExecSpec
	if e.shell {
		// --shell is the explicit opt-in to sh -c interpretation.
		spec = qga.ShellSpec(strings.Join(e.argv, " "), append(defaultGuestEnv(), e.env...))
	} else {
		spec = qga.ExecSpec{Path: e.argv[0], Arg: e.argv[1:],
			Env: append(defaultGuestEnv(), e.env...), CaptureOutput: true}
	}
	cl, err := guestDial(8 * time.Second)
	if err != nil {
		return guestEmitErr("guest.exec", "guest exec", err)
	}
	defer cl.Close()
	ctx, cancel := guestCtx(e.timeout)
	defer cancel()
	res, err := cl.Exec(ctx, spec, e.maxOutput)
	if err != nil && res == nil {
		return guestEmitErr("guest.exec", "guest exec", err)
	}
	if outFmt == "json" {
		d := map[string]any{
			"pid": res.PID, "exited": res.Exited, "exitCode": res.ExitCode,
			"stdout": string(res.Out), "stderr": string(res.Err),
			"truncated": res.Truncated || res.OutTrunc || res.ErrTrunc,
		}
		if err != nil {
			d["error"] = err.Error()
		}
		if res.Exited && err == nil {
			emitOK("guest.exec", d)
		} else {
			emitEnvelope("guest.exec", false, d, []any{map[string]any{
				"code": "exec_failed", "message": fmt.Sprint(err),
				"severity": "error", "retryable": true}})
		}
	} else {
		os.Stdout.Write(res.Out)
		os.Stderr.Write(res.Err)
		if err != nil {
			return guestFail("guest exec", err)
		}
	}
	if err != nil {
		return 1
	}
	if !res.Exited {
		return 1
	}
	if res.Signal != 0 {
		return 128 + res.Signal
	}
	if res.ExitCode < 0 {
		return 1
	}
	return res.ExitCode
}

// ---------- guest fsfreeze ----------

func actGuestFSFreeze() int {
	f := guestVerbState.freeze
	cl, err := guestDial(8 * time.Second)
	if err != nil {
		return guestEmitErr("guest.fsfreeze", "guest fsfreeze", err)
	}
	defer cl.Close()
	ctx, cancel := guestCtx(30 * time.Second)
	defer cancel()
	switch f.sub {
	case "status":
		st, err := cl.FSFreezeStatus(ctx)
		if err != nil {
			return guestEmitErr("guest.fsfreeze", "guest fsfreeze", err)
		}
		if outFmt == "json" {
			emitOK("guest.fsfreeze", map[string]any{"vm": vmname(), "state": string(st)})
		} else {
			fmt.Println(st)
		}
		return 0
	case "freeze":
		if f.hold > 0 {
			// --hold uses the watchdog-protected freeze: the guest is
			// guaranteed a thaw even if this process dies mid-wait.
			fmt.Printf(" - %s: freezing guest filesystems for %s.\n", vmname(), f.hold)
			ferr := cl.WithFreeze(ctx, f.hold+10*time.Second,
				func(context.Context) error {
					time.Sleep(f.hold)
					return nil
				})
			if ferr != nil {
				return guestEmitErr("guest.fsfreeze", "guest fsfreeze", ferr)
			}
			qeEvent("vm.fsfreeze", `{"hold":"`+f.hold.String()+`"}`)
			if outFmt == "json" {
				emitOK("guest.fsfreeze", map[string]any{"vm": vmname(),
					"state": "thawed", "hold": f.hold.String()})
			} else {
				fmt.Printf(" - %s: filesystems frozen and thawed (%s hold).\n", vmname(), f.hold)
			}
			return 0
		}
		n, err := cl.FSFreeze(ctx)
		if err != nil {
			return guestEmitErr("guest.fsfreeze", "guest fsfreeze", err)
		}
		qeEvent("vm.fsfreeze", `{"filesystems":`+strconv.Itoa(n)+`}`)
		if outFmt == "json" {
			emitOK("guest.fsfreeze", map[string]any{"vm": vmname(),
				"state": "frozen", "filesystems": n})
		} else {
			fmt.Printf(" - %s: %d filesystem(s) frozen — run 'guest fsfreeze thaw' to release.\n", vmname(), n)
		}
		return 0
	case "thaw":
		n, err := cl.FSThaw(ctx)
		if err != nil {
			return guestEmitErr("guest.fsfreeze", "guest fsfreeze", err)
		}
		qeEvent("vm.fsthaw", `{"filesystems":`+strconv.Itoa(n)+`}`)
		if outFmt == "json" {
			emitOK("guest.fsfreeze", map[string]any{"vm": vmname(),
				"state": "thawed", "filesystems": n})
		} else {
			fmt.Printf(" - %s: %d filesystem(s) thawed.\n", vmname(), n)
		}
		return 0
	}
	fmt.Println(" - guest fsfreeze: sub-command must be status|freeze|thaw.")
	return 1
}
