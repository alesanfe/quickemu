package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNetListener(t *testing.T) {
	if (&Net{}).listener() != "" {
		t.Fatal("no attach -> no listener")
	}
	n := &Net{Attach: []string{"a", "b"}}
	if n.listener() != "a" {
		t.Fatalf("listener = %q", n.listener())
	}
}

func TestMcastAddr(t *testing.T) {
	if got := (&Net{Mcast: "239.1.2.3:4000"}).mcastAddr(9999); got != "239.1.2.3:4000" {
		t.Fatalf("explicit mcast = %q", got)
	}
	if got := (&Net{}).mcastAddr(7777); got != "230.0.0.1:7777" {
		t.Fatalf("default mcast = %q", got)
	}
}

func TestPortsBin(t *testing.T) {
	stub(t, &findTool, func(string) (string, error) {
		return "", fmt.Errorf("absent")
	})
	if portsBin() != "" {
		t.Fatal("portsBin should be empty without sibling")
	}
	stub(t, &findTool, func(string) (string, error) {
		return "/x/quickemu-ports", nil
	})
	if portsBin() != "/x/quickemu-ports" {
		t.Fatalf("portsBin = %q", portsBin())
	}
}

func TestPortsFindViaRegistry(t *testing.T) {
	e, _ := testEnv(t)
	calls := stubToolsSibling(t, func(c toolCall) (string, error) {
		if c.name == "quickemu-ports" && c.args[0] == "find" {
			return "7123\n", nil
		}
		return "", nil
	})
	if p := e.portsFind(); p != 7123 {
		t.Fatalf("portsFind = %d", p)
	}
	got := (*calls)[0].args
	if got[0] != "find" || got[1] != "--registry" ||
		got[3] != "--lo" || got[4] != "7100" ||
		got[5] != "--hi" || got[6] != "7999" {
		t.Fatalf("find args = %v", got)
	}
}

func TestPortsFindFallbackOnToolError(t *testing.T) {
	e, _ := testEnv(t)
	stubToolsSibling(t, func(c toolCall) (string, error) {
		return "", fmt.Errorf("boom")
	})
	if p := e.portsFind(); p < portLo || p > portHi {
		t.Fatalf("fallback port = %d", p)
	}
}

func TestPortsFindFallbackOnGarbage(t *testing.T) {
	e, _ := testEnv(t)
	stubToolsSibling(t, func(c toolCall) (string, error) {
		return "not-a-port\n", nil
	})
	if p := e.portsFind(); p < portLo || p > portHi {
		t.Fatalf("fallback port = %d", p)
	}
}

func TestPortsFindNoSibling(t *testing.T) {
	e, _ := testEnv(t)
	stubTools(t, nil)
	if p := e.portsFind(); p < portLo || p > portHi {
		t.Fatalf("bind-probe port = %d", p)
	}
}

func TestPortsReserveReleaseNoSibling(t *testing.T) {
	e, _ := testEnv(t)
	stubTools(t, nil) // findTool fails -> portsBin == "" -> no-ops
	e.portsReserve(7100, os.Getpid(), "env:demo/lan")
	e.portsRelease(7100)
}

func TestPortsReserveReleaseWithSibling(t *testing.T) {
	e, _ := testEnv(t)
	calls := stubToolsSibling(t, nil)
	e.portsReserve(7100, 4242, "env:demo/lan")
	e.portsRelease(7100)
	if len(*calls) != 2 {
		t.Fatalf("calls = %v", *calls)
	}
	rsv := (*calls)[0].args
	if rsv[0] != "reserve" || rsv[3] != "--port" || rsv[4] != "7100" ||
		rsv[5] != "--pid" || rsv[6] != "4242" ||
		rsv[7] != "--instance" || rsv[8] != "env:demo/lan" {
		t.Fatalf("reserve args = %v", rsv)
	}
	rel := (*calls)[1].args
	if rel[0] != "release" || rel[3] != "--port" || rel[4] != "7100" {
		t.Fatalf("release args = %v", rel)
	}
}

func TestPortsFree(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	p := ln.Addr().(*net.TCPAddr).Port
	if portsFree(p) {
		t.Fatalf("bound port %d reported free", p)
	}
	ln.Close()
	if !portsFree(p) {
		t.Fatalf("released port %d reported busy", p)
	}
}

func TestOrphanSweep(t *testing.T) {
	e, _ := testEnv(t)
	stubTools(t, nil) // no ports sibling; releases are no-ops
	var logs []string
	logf := func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }

	// A genuinely dead owner: alive() is stubbed so only our pid lives.
	stub(t, &alive, func(pid int) bool { return pid == os.Getpid() })

	st := &envState{Nets: map[string]*netState{
		"skip-nil":   nil,
		"skip-zero":  {Port: 0},
		"live-owner": {Port: 7100, Owner: os.Getpid()},
		"dead-free":  {Port: 7101, Owner: 4242},
	}}
	e.orphanSweep(st, logf)
	if st.Nets["live-owner"].Port != 7100 {
		t.Fatal("live owner's port reaped")
	}
	if st.Nets["dead-free"].Owner != 0 {
		t.Fatal("dead owner not cleared")
	}
	if st.Nets["dead-free"].Port != 7101 {
		t.Fatalf("free port should be kept, got %d", st.Nets["dead-free"].Port)
	}
}

func TestOrphanSweepLiveRegistryOwner(t *testing.T) {
	e, _ := testEnv(t)
	stubTools(t, nil)
	stub(t, &alive, func(pid int) bool { return pid == os.Getpid() })
	if err := os.MkdirAll(e.portsReg(), 0o755); err != nil {
		t.Fatal(err)
	}
	// The registry file records our (live) pid as owner — the port is
	// taken, so the stale record is dropped for reallocation.
	writeFile(t, filepath.Join(e.portsReg(), "7100"), strconv.Itoa(os.Getpid()))
	st := &envState{Nets: map[string]*netState{
		"lan": {Port: 7100, Owner: 0},
	}}
	var logs []string
	e.orphanSweep(st, func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) })
	if st.Nets["lan"].Port != 0 || st.Nets["lan"].Owner != 0 {
		t.Fatalf("taken port kept: %+v", st.Nets["lan"])
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "taken by live pid") {
		t.Fatalf("logs = %v", logs)
	}
}

func TestOrphanSweepBoundByAnotherProcess(t *testing.T) {
	e, _ := testEnv(t)
	stubTools(t, nil)
	stub(t, &alive, func(pid int) bool { return false }) // every owner is dead
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	st := &envState{Nets: map[string]*netState{
		"lan": {Port: port, Owner: 4242},
	}}
	var logs []string
	e.orphanSweep(st, func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) })
	if st.Nets["lan"].Port != 0 {
		t.Fatalf("bound port kept: %+v", st.Nets["lan"])
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "bound by another process") {
		t.Fatalf("logs = %v", logs)
	}
}

func TestEnsurePorts(t *testing.T) {
	dir := t.TempDir()
	e := loadGood(t, dir, `
[env]
name = "demo"

[[vm]]
name = "a"
conf = "a.conf"

[[vm]]
name = "b"
conf = "b.conf"

[[net]]
name = "users"
kind = "user"
vms = ["a"]

[[net]]
name = "empty"
kind = "socket"

[[net]]
name = "lan"
kind = "socket"
vms = ["a", "b"]

[[net]]
name = "bus"
kind = "mcast"
vms = ["a", "b"]

[[net]]
name = "pinned"
kind = "socket"
vms = ["a", "b"]
port = 7450
`)
	stubTools(t, nil) // no ports sibling -> bind probe allocates
	st := newState(e)
	var logs []string
	logf := func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }
	if err := e.ensurePorts(st, logf); err != nil {
		t.Fatalf("ensurePorts: %v", err)
	}
	if _, ok := st.Nets["users"]; ok {
		t.Fatal("user net should be skipped")
	}
	if _, ok := st.Nets["empty"]; ok {
		t.Fatal("unattached socket net should be skipped")
	}
	for _, name := range []string{"lan", "bus"} {
		ns := st.Nets[name]
		if ns == nil || ns.Port < portLo || ns.Port > portHi {
			t.Fatalf("%s: %+v", name, ns)
		}
	}
	if st.Nets["lan"].Kind != "socket" || st.Nets["lan"].Listener != "a" {
		t.Fatalf("lan netState = %+v", st.Nets["lan"])
	}
	if st.Nets["bus"].Kind != "mcast" || st.Nets["bus"].Listener != "a" {
		t.Fatalf("bus netState = %+v", st.Nets["bus"])
	}
	if st.Nets["pinned"].Port != 7450 {
		t.Fatalf("pinned port = %d", st.Nets["pinned"].Port)
	}
	if len(logs) != 2 { // lan + bus allocated; pinned is silent
		t.Fatalf("alloc logs = %v", logs)
	}
	// Second run keeps existing assignments and stays quiet.
	logs = nil
	if err := e.ensurePorts(st, logf); err != nil {
		t.Fatalf("ensurePorts rerun: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("rerun reallocated: %v", logs)
	}
	lanPort := st.Nets["lan"].Port
	if lanPort == 0 {
		t.Fatal("lan port lost")
	}
}

func TestEnsurePortsExistingRecord(t *testing.T) {
	dir := t.TempDir()
	e := loadGood(t, dir, `
[[vm]]
name = "a"
conf = "a.conf"

[[vm]]
name = "b"
conf = "b.conf"

[[net]]
name = "lan"
kind = "socket"
vms = ["a", "b"]
`)
	stubTools(t, nil)
	st := newState(e)
	// Persisted assignment survives — no reallocation.
	st.Nets["lan"] = &netState{Kind: "user", Port: 7200, Listener: "old", Owner: 9}
	if err := e.ensurePorts(st, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	ns := st.Nets["lan"]
	if ns.Port != 7200 {
		t.Fatalf("kept port = %d", ns.Port)
	}
	// Kind/Listener are refreshed from the live env.
	if ns.Kind != "socket" || ns.Listener != "a" {
		t.Fatalf("refreshed fields = %+v", ns)
	}
}

func TestNetdevID(t *testing.T) {
	vm := &VM{Name: "web"}
	if got := netdevID(vm, 0); got != "qenvwebn0" {
		t.Fatalf("netdevID = %q", got)
	}
	if got := netdevID(vm, 3); got != "qenvwebn3" {
		t.Fatalf("netdevID = %q", got)
	}
}

func netEnv(t *testing.T) (*Env, *envState) {
	t.Helper()
	e := &Env{
		Dir:  t.TempDir(),
		Name: "demo",
		VMs: []*VM{
			{Name: "a"},
			{Name: "b"},
			{Name: "c"},
		},
		Nets: []*Net{
			{Name: "lan", Kind: "socket", Attach: []string{"a", "b"}},
			{Name: "bus", Kind: "mcast", Attach: []string{"a", "c"}},
			{Name: "edge", Kind: "user", Attach: []string{"a"}, Forwards: []Fwd{
				{VM: "", Host: 8080, Guest: 80},
				{VM: "b", Host: 2222, Guest: 22},
			}},
			{Name: "plain", Kind: "user", Attach: []string{"a"}},
			{Name: "dead", Kind: "socket", Attach: []string{"a", "b"}},
		},
		Shares: []*Share{
			{Name: "www", Path: "rel/path", Attach: []string{"a"}, Mode: "ro"},
			{Name: "abs", Path: "/abs/path", Attach: []string{"a"}, Mode: "rw"},
			{Name: "other", Path: "/x", Attach: []string{"b"}},
		},
	}
	st := newState(e)
	st.Nets["lan"] = &netState{Kind: "socket", Port: 7200, Listener: "a"}
	st.Nets["bus"] = &netState{Kind: "mcast", Port: 7300}
	// "dead" has no port assignment -> skipped.
	return e, st
}

func TestNetArgsSocket(t *testing.T) {
	e, st := netEnv(t)
	args := netArgs(e.vm("a"), e, st)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-netdev socket,id=qenvan0,listen=:7200") {
		t.Fatalf("listener args = %v", args)
	}
	if !strings.Contains(joined, "-device "+netDevice+",netdev=qenvan0") {
		t.Fatalf("device args = %v", args)
	}
	args = netArgs(e.vm("b"), e, st)
	joined = strings.Join(args, " ")
	if !strings.Contains(joined, "-netdev socket,id=qenvbn0,connect=127.0.0.1:7200") {
		t.Fatalf("connect args = %v", args)
	}
	// b is not attached to bus/edge/plain -> only the socket net.
	if strings.Contains(joined, "mcast") || strings.Contains(joined, "hostfwd") {
		t.Fatalf("b got foreign args = %v", args)
	}
}

func TestNetArgsMcast(t *testing.T) {
	e, st := netEnv(t)
	args := netArgs(e.vm("a"), e, st)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-netdev socket,id=qenvan1,mcast=230.0.0.1:7300") {
		t.Fatalf("mcast args = %v", args)
	}
	// Explicit mcast= overrides the derived group.
	e.net("bus").Mcast = "239.1.1.1:4000"
	args = netArgs(e.vm("a"), e, st)
	joined = strings.Join(args, " ")
	if !strings.Contains(joined, "mcast=239.1.1.1:4000") {
		t.Fatalf("explicit mcast args = %v", args)
	}
}

func TestNetArgsUserForwards(t *testing.T) {
	e, st := netEnv(t)
	args := netArgs(e.vm("a"), e, st)
	joined := strings.Join(args, " ")
	// a's forward fans out; b's is filtered for a's spec but the
	// vm-less one lands.
	if !strings.Contains(joined, "-netdev user,id=qenvan2,hostfwd=tcp::8080-:80,hostfwd=udp::8080-:80") {
		t.Fatalf("user fwd args = %v", args)
	}
	if strings.Contains(joined, "2222") {
		t.Fatalf("a's spec carried b's forward = %v", args)
	}
	// The plain user net without forwards emits nothing.
	if strings.Contains(joined, "plain") || strings.Count(joined, "-netdev") != 3 {
		t.Fatalf("plain net emitted args = %v", args)
	}
}

func TestNetArgsNoNets(t *testing.T) {
	e, st := netEnv(t)
	lonely := &VM{Name: "zzz"}
	if got := netArgs(lonely, e, st); len(got) != 0 {
		t.Fatalf("unattached vm args = %v", got)
	}
}

func TestShareArgs(t *testing.T) {
	e, _ := netEnv(t)
	args := shareArgs(e.vm("a"), e)
	if len(args) != 2 {
		t.Fatalf("shareArgs = %v", args)
	}
	if !strings.HasPrefix(args[0], "-virtfs local,path=") ||
		!strings.Contains(args[0], filepath.Join(e.Dir, "rel/path")) ||
		!strings.Contains(args[0], "mount_tag=www") ||
		!strings.Contains(args[0], "security_model=none") ||
		!strings.Contains(args[0], "readonly=on") {
		t.Fatalf("ro share arg = %q", args[0])
	}
	if !strings.Contains(args[1], "path=/abs/path") ||
		strings.Contains(args[1], "readonly") {
		t.Fatalf("rw share arg = %q", args[1])
	}
	args = shareArgs(e.vm("b"), e)
	if len(args) != 1 || !strings.Contains(args[0], "mount_tag=other") {
		t.Fatalf("b shareArgs = %v", args)
	}
	if got := shareArgs(&VM{Name: "zzz"}, e); len(got) != 0 {
		t.Fatalf("unattached shareArgs = %v", got)
	}
}

func TestExtraArgs(t *testing.T) {
	e, st := netEnv(t)
	got := extraArgs(e.vm("a"), e, st)
	for _, want := range []string{"-netdev socket", "-virtfs local"} {
		if !strings.Contains(got, want) {
			t.Fatalf("extraArgs %q missing %q", got, want)
		}
	}
	// Nothing for a detached VM.
	e2 := &Env{Dir: e.Dir, VMs: e.VMs}
	if got := extraArgs(e.vm("a"), e2, st); got != "" {
		t.Fatalf("empty extraArgs = %q", got)
	}
}

func TestVMNets(t *testing.T) {
	e, st := netEnv(t)
	nets := vmNets(e.vm("a"), e, st)
	if len(nets) != 5 {
		t.Fatalf("vmNets = %v", nets)
	}
	byName := map[string]netInfo{}
	for _, n := range nets {
		byName[n.Name] = n
	}
	if byName["lan"].Role != "listen" || byName["lan"].Port != 7200 {
		t.Fatalf("lan = %+v", byName["lan"])
	}
	if byName["bus"].Role != "member" || byName["bus"].Port != 7300 {
		t.Fatalf("bus = %+v", byName["bus"])
	}
	if byName["edge"].Role != "member" {
		t.Fatalf("edge = %+v", byName["edge"])
	}
	nets = vmNets(e.vm("b"), e, st)
	if len(nets) != 2 {
		t.Fatalf("b vmNets = %v", nets)
	}
	for _, n := range nets {
		if n.Name == "lan" && n.Role != "connect" {
			t.Fatalf("b lan role = %+v", n)
		}
	}
	if got := vmNets(&VM{Name: "zzz"}, e, st); len(got) != 0 {
		t.Fatalf("zzz vmNets = %v", got)
	}
}

func TestPortsList(t *testing.T) {
	e, _ := testEnv(t)
	stubTools(t, nil) // no sibling -> nil
	if got := e.portsList(); got != nil {
		t.Fatalf("no sibling portsList = %v", got)
	}
	stubToolsSibling(t, func(c toolCall) (string, error) {
		return "", fmt.Errorf("boom")
	})
	if got := e.portsList(); got != nil {
		t.Fatalf("err portsList = %v", got)
	}
	stubToolsSibling(t, func(c toolCall) (string, error) {
		return "not json", nil
	})
	if got := e.portsList(); got != nil {
		t.Fatalf("bad json portsList = %v", got)
	}
	stubToolsSibling(t, func(c toolCall) (string, error) {
		return `{"data":{"reservations":[{"port":7100,"pid":42}]}}`, nil
	})
	got := e.portsList()
	if len(got) != 1 || got[0]["port"] != float64(7100) {
		t.Fatalf("portsList = %v", got)
	}
}
