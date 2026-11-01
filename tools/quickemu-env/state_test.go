package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testEnv(t *testing.T) (*Env, string) {
	t.Helper()
	dir := t.TempDir()
	return &Env{File: filepath.Join(dir, "env.toml"), Dir: dir, Name: "demo"}, dir
}

func TestStatePaths(t *testing.T) {
	e, dir := testEnv(t)
	if e.stateDir() != filepath.Join(dir, ".quickemu-env") {
		t.Fatalf("stateDir = %q", e.stateDir())
	}
	if e.stateFile() != filepath.Join(dir, ".quickemu-env", "state.json") {
		t.Fatalf("stateFile = %q", e.stateFile())
	}
	if e.portsReg() != filepath.Join(dir, ".quickemu-env", "ports") {
		t.Fatalf("portsReg = %q", e.portsReg())
	}
}

func TestNewState(t *testing.T) {
	e, _ := testEnv(t)
	st := newState(e)
	if st.Version != 1 || st.Env != "demo" || st.File != e.File ||
		st.VMs == nil || st.Nets == nil {
		t.Fatalf("newState = %+v", st)
	}
}

func TestSaveLoadStateRoundtrip(t *testing.T) {
	e, _ := testEnv(t)
	st := newState(e)
	st.VMs["a"] = &vmState{Conf: "/x/a.conf", Created: true, VMdir: "/x/a", PID: 42, Started: true}
	st.Nets["lan"] = &netState{Kind: "socket", Port: 7200, Listener: "a", Owner: 42}
	e.save(st)
	if _, err := os.Stat(e.stateFile()); err != nil {
		t.Fatalf("stateFile missing: %v", err)
	}
	// The ports registry dir is created alongside.
	if st2, err := os.Stat(e.portsReg()); err != nil || !st2.IsDir() {
		t.Fatalf("portsReg missing: %v", err)
	}
	got := e.loadState()
	vs := got.VMs["a"]
	if vs == nil || !vs.Created || vs.VMdir != "/x/a" || vs.PID != 42 || !vs.Started {
		t.Fatalf("vms = %+v", got.VMs["a"])
	}
	ns := got.Nets["lan"]
	if ns == nil || ns.Kind != "socket" || ns.Port != 7200 || ns.Listener != "a" || ns.Owner != 42 {
		t.Fatalf("nets = %+v", got.Nets["lan"])
	}
}

func TestLoadStateMissingAndCorrupt(t *testing.T) {
	e, _ := testEnv(t)
	st := e.loadState() // no file — fresh record
	if len(st.VMs) != 0 || len(st.Nets) != 0 || st.Env != "demo" {
		t.Fatalf("missing state = %+v", st)
	}
	writeFile(t, e.stateFile(), "{ not json")
	st = e.loadState() // corrupt — clean record rather than a wedge
	if len(st.VMs) != 0 || st.Env != "demo" {
		t.Fatalf("corrupt state = %+v", st)
	}
}

func TestLoadStateNullMapsRepaired(t *testing.T) {
	e, _ := testEnv(t)
	writeFile(t, e.stateFile(), `{"version":1,"env":"demo","vms":null,"nets":null}`)
	st := e.loadState()
	if st.VMs == nil || st.Nets == nil {
		t.Fatalf("nil maps not repaired: %+v", st)
	}
}

func TestSaveStateDirBlocked(t *testing.T) {
	e, dir := testEnv(t)
	// A regular file where .quickemu-env/ should be — MkdirAll fails
	// and save warns without writing.
	writeFile(t, filepath.Join(dir, ".quickemu-env"), "occupied")
	e.save(newState(e))
	if _, err := os.Stat(e.stateFile()); err == nil {
		t.Fatal("stateFile should not have been written")
	}
}

func TestConfKV(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "x.conf")
	writeFile(t, conf, `
# a comment
guest_os="linux"
ssh_port='2222'
bare=value
spaced = "v"
empty=
"weird"=x
noequalsign
unterminated="abc
`)
	kv := confKV(conf)
	if kv["guest_os"] != "linux" || kv["ssh_port"] != "2222" ||
		kv["bare"] != "value" || kv["spaced"] != "v" {
		t.Fatalf("confKV = %v", kv)
	}
	if _, ok := kv["empty"]; !ok || kv["empty"] != "" {
		t.Fatalf("empty value: %v", kv)
	}
	if kv["unterminated"] != `"abc` {
		t.Fatalf("unterminated quote kept raw: %q", kv["unterminated"])
	}
	if _, ok := kv["noequalsign"]; ok {
		t.Fatal("line without = parsed")
	}
	if got := confKV(filepath.Join(dir, "missing.conf")); len(got) != 0 {
		t.Fatalf("missing conf = %v", got)
	}
}

func TestConfName(t *testing.T) {
	for in, want := range map[string]string{
		"/a/b/c.conf": "c",
		"noext":       "noext",
		"x.tar.gz":    "x.tar",
		"rel/x.conf":  "x",
	} {
		if got := confName(in); got != want {
			t.Fatalf("confName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVMDir(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "vm1.conf")
	writeFile(t, conf, `guest_os="linux"`+"\n")
	if got := vmDir(conf); got != filepath.Join(dir, "vm1") {
		t.Fatalf("default vmDir = %q", got)
	}
	// Relative disk_img stays inside the conf dir.
	writeFile(t, conf, `disk_img="sub/disk.qcow2"`+"\n")
	if got := vmDir(conf); got != filepath.Join(dir, "sub") {
		t.Fatalf("rel disk_img vmDir = %q", got)
	}
	// Absolute disk_img dirname wins.
	abs := filepath.Join(dir, "abs", "d.qcow2")
	writeFile(t, conf, `disk_img="`+abs+`"`+"\n")
	if got := vmDir(conf); got != filepath.Dir(abs) {
		t.Fatalf("abs disk_img vmDir = %q", got)
	}
	// A disk_img escaping the conf dir falls back to the default.
	writeFile(t, conf, `disk_img="../escape.qcow2"`+"\n")
	if got := vmDir(conf); got != filepath.Join(dir, "vm1") {
		t.Fatalf("escaping disk_img vmDir = %q", got)
	}
	// Missing conf file — confKV is empty, default applies.
	if got := vmDir(filepath.Join(dir, "ghost.conf")); got != filepath.Join(dir, "ghost") {
		t.Fatalf("missing conf vmDir = %q", got)
	}
}

func TestInspect(t *testing.T) {
	dir := t.TempDir()
	if r := inspect(""); r.State != "missing" {
		t.Fatalf("empty conf = %+v", r)
	}
	conf := filepath.Join(dir, "vm1.conf")
	writeFile(t, conf, `guest_os="linux"`+"\n")
	vmDir := filepath.Join(dir, "vm1")
	if r := inspect(conf); r.State != "stopped" || r.PID != 0 {
		t.Fatalf("no pidfile = %+v", r)
	}
	// Live pid -> running; .paused flips to paused.
	writeFile(t, filepath.Join(vmDir, "vm1.pid"), "4242")
	stub(t, &alive, func(pid int) bool { return pid == 4242 })
	if r := inspect(conf); r.State != "running" || r.PID != 4242 {
		t.Fatalf("running = %+v", r)
	}
	writeFile(t, filepath.Join(vmDir, "vm1.paused"), "")
	if r := inspect(conf); r.State != "paused" {
		t.Fatalf("paused = %+v", r)
	}
	// Dead pid -> stale.
	writeFile(t, filepath.Join(vmDir, "vm1.pid"), "99999")
	if r := inspect(conf); r.State != "stale" || r.PID != 0 {
		t.Fatalf("stale = %+v", r)
	}
	// Garbage pidfile parses as stopped.
	writeFile(t, filepath.Join(vmDir, "vm1.pid"), "not-a-pid")
	if r := inspect(conf); r.State != "stopped" {
		t.Fatalf("garbage pid = %+v", r)
	}
}

func TestInspectPortsFile(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "vm1.conf")
	writeFile(t, conf, "")
	writeFile(t, filepath.Join(dir, "vm1", "vm1.ports"),
		"ssh,2222\nhttp,8080\nmalformed\nbadport,xyz\n")
	r := inspect(conf)
	if r.Ports["ssh"] != 2222 || r.Ports["http"] != 8080 || len(r.Ports) != 2 {
		t.Fatalf("ports = %v", r.Ports)
	}
}

func TestSSHPort(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "vm1.conf")
	writeFile(t, conf, `ssh_port="2200"`+"\n")
	// Live .ports record wins.
	if p := sshPort(conf, vmRuntime{Ports: map[string]int{"ssh": 2222}}); p != 2222 {
		t.Fatalf("ports record = %d", p)
	}
	// Conf fallback.
	if p := sshPort(conf, vmRuntime{Ports: map[string]int{}}); p != 2200 {
		t.Fatalf("conf fallback = %d", p)
	}
	writeFile(t, conf, "")
	if p := sshPort(conf, vmRuntime{Ports: map[string]int{}}); p != 0 {
		t.Fatalf("no forward = %d", p)
	}
}

func TestAgentSock(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "vm1.conf")
	writeFile(t, conf, "")
	want := filepath.Join(dir, "vm1", ".s", "agent")
	if got := agentSock(conf); got != want {
		t.Fatalf("agentSock = %q, want %q", got, want)
	}
	// A live legacy socket (pre-.s launch) still resolves.
	legacy := filepath.Join(dir, "vm1", "vm1-agent.sock")
	os.MkdirAll(filepath.Dir(legacy), 0o755)
	ln, err := net.Listen("unix", legacy)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if got := agentSock(conf); got != legacy {
		t.Fatalf("agentSock legacy = %q, want %q", got, legacy)
	}
}

func TestSmallFSHelpers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	writeFile(t, p, "hi")
	if b, err := readFile(p); err != nil || string(b) != "hi" {
		t.Fatalf("readFile: %v", err)
	}
	if _, err := readFile(filepath.Join(dir, "none")); err == nil {
		t.Fatal("readFile missing should err")
	}
	if a, err := absPath("x"); err != nil || !filepath.IsAbs(a) {
		t.Fatalf("absPath: %q %v", a, err)
	}
	if got := dirOf("/a/b/c"); !strings.HasSuffix(got, filepath.Join("a", "b")) {
		t.Fatalf("dirOf = %q", got)
	}
}

func TestAliveSeam(t *testing.T) {
	if !alive(os.Getpid()) {
		t.Fatal("own pid should be alive")
	}
	if alive(0) {
		t.Fatal("pid 0 is not alive")
	}
	if !qelibAlive(os.Getpid()) {
		t.Fatal("qelibAlive(own pid)")
	}
}

func TestNowUTC(t *testing.T) {
	if ts := nowUTC(); ts.Location() != time.UTC {
		t.Fatalf("nowUTC location = %v", ts.Location())
	}
}
