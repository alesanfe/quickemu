package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlogVlog(t *testing.T) {
	out := captureOut(t, func() {
		plog("env %s: %d vm(s)", "demo", 2)
		vlog("web", "starting %s", "now")
	})
	if !strings.Contains(out, "env demo: 2 vm(s)") ||
		!strings.Contains(out, "[web] starting now") {
		t.Fatalf("out = %q", out)
	}
}

func TestSpecConf(t *testing.T) {
	e, dir := testEnv(t)
	if got := specConf(e, &VM{Conf: "/abs/x.conf"}); got != "/abs/x.conf" {
		t.Fatalf("abs conf = %q", got)
	}
	if got := specConf(e, &VM{Conf: "rel/x.conf"}); got != filepath.Join(dir, "rel/x.conf") {
		t.Fatalf("rel conf = %q", got)
	}
	if got := specConf(e, &VM{Name: "v1"}); got != filepath.Join(dir, "v1.conf") {
		t.Fatalf("spec conf = %q", got)
	}
}

func TestQuickgetConfName(t *testing.T) {
	if got := quickgetConfName(&VM{OS: "ubuntu", Release: "24.04"}); got != "ubuntu-24.04" {
		t.Fatalf("name = %q", got)
	}
	if got := quickgetConfName(&VM{OS: "fedora", Release: "40", Edition: "kde"}); got != "fedora-40-kde" {
		t.Fatalf("name = %q", got)
	}
}

func TestMergeVS(t *testing.T) {
	vs := mergeVS(nil, "/x.conf")
	if vs == nil || vs.Conf != "/x.conf" {
		t.Fatalf("nil merge = %+v", vs)
	}
	vs.Created = true
	vs2 := mergeVS(vs, "/y.conf")
	if vs2.Conf != "/y.conf" || !vs2.Created {
		t.Fatalf("merge = %+v", vs2)
	}
}

// orcEnv builds the standard two-VM env: a (level 0) <- b (level 1),
// both on the "lan" socket net. Conf files exist so no quickget/
// mkconf runs unless the test removes them.
func orcEnv(t *testing.T, dir string) *Env {
	t.Helper()
	e := loadGood(t, dir, `
[env]
name = "demo"

[[vm]]
name = "a"
conf = "a.conf"

[[vm]]
name = "b"
conf = "b.conf"
depends_on = ["a"]

[[net]]
name = "lan"
kind = "socket"
vms = ["a", "b"]
`)
	confAt(t, dir, "a.conf", `guest_os="linux"`+"\n")
	confAt(t, dir, "b.conf", `guest_os="linux"`+"\n")
	return e
}

// quickemuFake returns a runTool handler that simulates quickemu:
// --start drops a pidfile (live pid), --stop/--kill removes it.
// failOn, when non-empty, makes calls whose args contain it error.
func quickemuFake(t *testing.T, failOn string) func(c toolCall) (string, error) {
	t.Helper()
	return func(c toolCall) (string, error) {
		if c.name != "quickemu" {
			return "", nil
		}
		conf := ""
		start, stop := false, false
		for i, a := range c.args {
			if a == "--vm" && i+1 < len(c.args) {
				conf = c.args[i+1]
			}
			if a == "--start" {
				start = true
			}
			if a == "--stop" || a == "--kill" {
				stop = true
			}
		}
		if failOn != "" && strings.Contains(conf, failOn) {
			return "", fmt.Errorf("simulated failure on %s", conf)
		}
		if conf == "" {
			return "", nil
		}
		dir, name := vmDir(conf), confName(conf)
		if start {
			if err := os.MkdirAll(dir, 0o755); err == nil {
				os.WriteFile(filepath.Join(dir, name+".pid"),
					[]byte(fmt.Sprint(os.Getpid())), 0o644)
			}
		}
		if stop {
			os.Remove(filepath.Join(dir, name+".pid"))
		}
		return "", nil
	}
}

func TestEnsureConfExisting(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	vm := e.vm("a")
	conf, err := ensureConf(e, st, vm)
	if err != nil || conf != filepath.Join(dir, "a.conf") {
		t.Fatalf("ensureConf = %q %v", conf, err)
	}
	if st.VMs["a"].Conf != conf {
		t.Fatalf("state not updated: %+v", st.VMs["a"])
	}
	// A persisted conf path wins over the spec.
	st.VMs["a"] = &vmState{Conf: conf}
	if got, err := ensureConf(e, st, vm); err != nil || got != conf {
		t.Fatalf("persisted conf = %q %v", got, err)
	}
}

func TestEnsureConfMissingExplicit(t *testing.T) {
	e, _ := testEnv(t)
	st := newState(e)
	vm := &VM{Name: "x", Conf: "gone.conf"}
	_, err := ensureConf(e, st, vm)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing conf err = %v", err)
	}
}

func TestEnsureConfInlineSpecMkconf(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stubTools(t, nil)
	vm := &VM{Name: "v1", RAM: "1G", CPU: 1}
	conf, err := ensureConf(e, st, vm)
	if err != nil {
		t.Fatalf("mkconf err = %v", err)
	}
	if conf != filepath.Join(dir, "v1.conf") {
		t.Fatalf("conf = %q", conf)
	}
	kv := confKV(conf)
	if kv["guest_os"] != "linux" || kv["ram"] != "1G" || kv["cpu_cores"] != "1" ||
		kv["disk_size"] != "16G" || kv["disk_img"] != "v1/disk.qcow2" {
		t.Fatalf("conf = %v", kv)
	}
	if st.VMs["v1"].Created != true {
		t.Fatalf("created flag = %+v", st.VMs["v1"])
	}
}

func TestQuickgetConfRename(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stubTools(t, func(c toolCall) (string, error) {
		if c.name != "quickget" {
			return "", nil
		}
		// Simulate quickget producing os-release.conf + matching dir.
		writeFile(t, filepath.Join(c.dir, "ubuntu-24.04.conf"),
			`disk_img="ubuntu-24.04/disk.qcow2"`+"\n")
		writeFile(t, filepath.Join(c.dir, "ubuntu-24.04", "disk.qcow2"), "")
		return "Downloading...\n", nil
	})
	vm := &VM{Name: "v1", OS: "ubuntu", Release: "24.04"}
	want := filepath.Join(dir, "v1.conf")
	conf, err := quickgetConf(e, st, vm, want)
	if err != nil || conf != want {
		t.Fatalf("quickgetConf = %q %v", conf, err)
	}
	kv := confKV(conf)
	if kv["disk_img"] != "v1/disk.qcow2" {
		t.Fatalf("conf not rewritten: %v", kv)
	}
	if _, err := os.Stat(filepath.Join(dir, "v1", "disk.qcow2")); err != nil {
		t.Fatalf("vmdir not renamed: %v", err)
	}
	vs := st.VMs["v1"]
	if vs == nil || !vs.Created || vs.Conf != want || vs.VMdir != filepath.Join(dir, "v1") {
		t.Fatalf("state = %+v", vs)
	}
}

func TestQuickgetConfSameName(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stubTools(t, func(c toolCall) (string, error) {
		writeFile(t, filepath.Join(c.dir, "ubuntu-24.04.conf"), "")
		return "", nil
	})
	vm := &VM{Name: "ubuntu-24.04", OS: "ubuntu", Release: "24.04"}
	want := filepath.Join(dir, "ubuntu-24.04.conf")
	conf, err := quickgetConf(e, st, vm, want)
	if err != nil || conf != want {
		t.Fatalf("same-name = %q %v", conf, err)
	}
	if !st.VMs["ubuntu-24.04"].Created {
		t.Fatal("created flag")
	}
}

func TestQuickgetConfToolError(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stubTools(t, func(c toolCall) (string, error) {
		return "partial output", fmt.Errorf("network down")
	})
	vm := &VM{Name: "v1", OS: "ubuntu", Release: "24.04"}
	_, err := quickgetConf(e, st, vm, filepath.Join(dir, "v1.conf"))
	if err == nil || !strings.Contains(err.Error(), "quickget ubuntu 24.04") {
		t.Fatalf("tool err = %v", err)
	}
}

func TestQuickgetConfNothingProduced(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stubTools(t, nil)
	vm := &VM{Name: "v1", OS: "ubuntu", Release: "24.04"}
	_, err := quickgetConf(e, st, vm, filepath.Join(dir, "v1.conf"))
	if err == nil || !strings.Contains(err.Error(), "produced no conf") {
		t.Fatalf("no conf err = %v", err)
	}
}

func TestQuickgetConfGlobFallback(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stubTools(t, func(c toolCall) (string, error) {
		// quickget names it something unexpected — glob finds it.
		writeFile(t, filepath.Join(c.dir, "zzz.conf"), "")
		return "", nil
	})
	vm := &VM{Name: "v1", OS: "ubuntu", Release: "24.04"}
	conf, err := quickgetConf(e, st, vm, filepath.Join(dir, "v1.conf"))
	if err != nil || conf != filepath.Join(dir, "v1.conf") {
		t.Fatalf("glob fallback = %q %v", conf, err)
	}
}

func TestQuickgetConfDirCollision(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stubTools(t, func(c toolCall) (string, error) {
		writeFile(t, filepath.Join(c.dir, "ubuntu-24.04.conf"), "")
		writeFile(t, filepath.Join(c.dir, "ubuntu-24.04", "x"), "")
		return "", nil
	})
	// The target vmdir already exists — the dir rename is refused.
	writeFile(t, filepath.Join(dir, "v1", "y"), "")
	vm := &VM{Name: "v1", OS: "ubuntu", Release: "24.04"}
	_, err := quickgetConf(e, st, vm, filepath.Join(dir, "v1.conf"))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("dir collision err = %v", err)
	}
}

func TestQuickgetConfRenameFails(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stubTools(t, func(c toolCall) (string, error) {
		writeFile(t, filepath.Join(c.dir, "ubuntu-24.04.conf"), "")
		return "", nil
	})
	// The wanted conf path is a directory — rename must fail.
	if err := os.MkdirAll(filepath.Join(dir, "v1.conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	vm := &VM{Name: "v1", OS: "ubuntu", Release: "24.04"}
	_, err := quickgetConf(e, st, vm, filepath.Join(dir, "v1.conf"))
	if err == nil || !strings.Contains(err.Error(), "rename") {
		t.Fatalf("rename err = %v", err)
	}
}

func TestMkconfProfiles(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stub(t, &profileDefaults, func(name string) (map[string]string, bool) {
		if name == "full" {
			return map[string]string{"guest_os": "windows", "ram": "8G", "cpu_cores": "6"}, true
		}
		if name == "empty" {
			return map[string]string{}, true
		}
		return nil, false
	})
	vm := &VM{Name: "w", Profile: "full"}
	conf, err := mkconf(e, st, vm, filepath.Join(dir, "w.conf"))
	if err != nil {
		t.Fatal(err)
	}
	kv := confKV(conf)
	if kv["guest_os"] != "windows" || kv["ram"] != "8G" || kv["cpu_cores"] != "6" ||
		kv["profile"] != "full" {
		t.Fatalf("profile conf = %v", kv)
	}
	// Empty defaults fall back to linux/2G/2.
	vm = &VM{Name: "l", Profile: "empty"}
	conf, err = mkconf(e, st, vm, filepath.Join(dir, "l.conf"))
	if err != nil {
		t.Fatal(err)
	}
	kv = confKV(conf)
	if kv["guest_os"] != "linux" || kv["ram"] != "2G" || kv["cpu_cores"] != "2" {
		t.Fatalf("empty profile conf = %v", kv)
	}
	// Unknown profile behaves like no profile.
	vm = &VM{Name: "n", Profile: "nope"}
	conf, err = mkconf(e, st, vm, filepath.Join(dir, "n.conf"))
	if err != nil {
		t.Fatal(err)
	}
	kv = confKV(conf)
	if kv["guest_os"] != "linux" || kv["ram"] != "2G" || kv["cpu_cores"] != "2" ||
		kv["profile"] != "nope" {
		t.Fatalf("unknown profile conf = %v", kv)
	}
	// Explicit ram/cpu/disk_size beat the profile table.
	vm = &VM{Name: "x", Profile: "full", RAM: "1G", CPU: 1, DiskSize: "4G"}
	conf, err = mkconf(e, st, vm, filepath.Join(dir, "x.conf"))
	if err != nil {
		t.Fatal(err)
	}
	kv = confKV(conf)
	if kv["ram"] != "1G" || kv["cpu_cores"] != "1" || kv["disk_size"] != "4G" {
		t.Fatalf("override conf = %v", kv)
	}
}

func TestMkconfDirAndWriteErrors(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, File: filepath.Join(dir, "env.toml"), Name: "demo"}
	st := newState(e)
	stub(t, &profileDefaults, func(string) (map[string]string, bool) { return nil, false })
	// A file where the vmdir should be — MkdirAll fails.
	writeFile(t, filepath.Join(dir, "v1"), "")
	_, err := mkconf(e, st, &VM{Name: "v1"}, filepath.Join(dir, "v1.conf"))
	if err == nil {
		t.Fatal("mkdir collision should error")
	}
	// A directory where the conf should be — WriteFile fails.
	if err := os.MkdirAll(filepath.Join(dir, "v2.conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = mkconf(e, st, &VM{Name: "v2"}, filepath.Join(dir, "v2.conf"))
	if err == nil {
		t.Fatal("conf dir collision should error")
	}
}

func TestStartVM(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	st.Nets["lan"] = &netState{Kind: "socket", Port: 7200, Listener: "a"}
	calls := stubTools(t, quickemuFake(t, ""))
	vm := e.vm("a")
	out := captureOut(t, func() {
		if err := startVM(e, st, vm); err != nil {
			t.Fatalf("startVM: %v", err)
		}
	})
	if !strings.Contains(out, "[a] starting") {
		t.Fatalf("out = %q", out)
	}
	// quickemu --vm <conf> --start --extra_args <socket args>
	var found *toolCall
	for i := range *calls {
		if (*calls)[i].name == "quickemu" {
			found = &(*calls)[i]
		}
	}
	if found == nil {
		t.Fatalf("quickemu never called: %v", *calls)
	}
	joined := strings.Join(found.args, " ")
	if !strings.Contains(joined, "--vm "+filepath.Join(dir, "a.conf")) ||
		!strings.Contains(joined, "--start") ||
		!strings.Contains(joined, "--extra_args") ||
		!strings.Contains(joined, "listen=:"+fmt.Sprint(st.Nets["lan"].Port)) {
		t.Fatalf("start args = %v", found.args)
	}
	if st.VMs["a"].PID != os.Getpid() || !st.VMs["a"].Started {
		t.Fatalf("vm state = %+v", st.VMs["a"])
	}
	if st.Nets["lan"].Owner != os.Getpid() {
		t.Fatalf("net owner = %+v", st.Nets["lan"])
	}
}

func TestStartVMEnsureConfError(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	os.Remove(filepath.Join(dir, "a.conf"))
	stubTools(t, nil)
	err := startVM(e, st, e.vm("a"))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("startVM err = %v", err)
	}
}

func TestStartVMToolError(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	stubTools(t, func(c toolCall) (string, error) {
		return "oops\n", fmt.Errorf("qemu died")
	})
	captureOut(t, func() {
		err := startVM(e, st, e.vm("a"))
		if err == nil || !strings.Contains(err.Error(), "start failed") {
			t.Fatalf("startVM err = %v", err)
		}
	})
}

func TestHealthFor(t *testing.T) {
	e := &Env{Health: []*Health{
		{VM: "a", Type: "tcp"}, {VM: "b", Type: "ssh"}, {VM: "a", Type: "qga-ping"},
	}}
	hs := e.healthFor("a")
	if len(hs) != 2 || hs[0].Type != "tcp" || hs[1].Type != "qga-ping" {
		t.Fatalf("healthFor = %v", hs)
	}
	if hs := e.healthFor("zz"); len(hs) != 0 {
		t.Fatalf("healthFor zz = %v", hs)
	}
}

func TestVMConf(t *testing.T) {
	e, dir := testEnv(t)
	vm := &VM{Name: "v", Conf: "v.conf"}
	if got := vmConf(e, nil, vm); got != filepath.Join(dir, "v.conf") {
		t.Fatalf("nil state vmConf = %q", got)
	}
	st := newState(e)
	if got := vmConf(e, st, vm); got != filepath.Join(dir, "v.conf") {
		t.Fatalf("no state entry vmConf = %q", got)
	}
	st.VMs["v"] = &vmState{Conf: "/elsewhere/v.conf"}
	if got := vmConf(e, st, vm); got != "/elsewhere/v.conf" {
		t.Fatalf("state conf = %q", got)
	}
}

func TestStopVMAlreadyStopped(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	stubTools(t, nil)
	out := captureOut(t, func() {
		if err := stopVM(e, st, e.vm("a"), time.Second, false); err != nil {
			t.Fatalf("stopVM: %v", err)
		}
	})
	if !strings.Contains(out, "already stopped") {
		t.Fatalf("out = %q", out)
	}
}

func TestStopVMGraceful(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	st.VMs["a"] = &vmState{Conf: filepath.Join(dir, "a.conf"), PID: os.Getpid(), Started: true}
	calls := stubTools(t, quickemuFake(t, ""))
	// Boot the fake first so the pidfile exists.
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	out := captureOut(t, func() {
		if err := stopVM(e, st, e.vm("a"), 5*time.Second, false); err != nil {
			t.Fatalf("stopVM: %v", err)
		}
	})
	if !strings.Contains(out, "stopping") || !strings.Contains(out, "stopped") {
		t.Fatalf("out = %q", out)
	}
	stopCalled := false
	for _, c := range *calls {
		if c.name == "quickemu" && strings.Contains(strings.Join(c.args, " "), "--stop") {
			stopCalled = true
		}
	}
	if !stopCalled {
		t.Fatalf("--stop never issued: %v", *calls)
	}
	if st.VMs["a"].Started || st.VMs["a"].PID != 0 {
		t.Fatalf("state = %+v", st.VMs["a"])
	}
}

func TestStopVMToolErrorStillStops(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	stubTools(t, func(c toolCall) (string, error) {
		if c.name == "quickemu" {
			for i, a := range c.args {
				if a == "--vm" && i+1 < len(c.args) {
					os.Remove(filepath.Join(vmDir(c.args[i+1]),
						confName(c.args[i+1])+".pid"))
				}
			}
			return "", fmt.Errorf("qmp timeout")
		}
		return "", nil
	})
	out := captureOut(t, func() {
		if err := stopVM(e, st, e.vm("a"), 5*time.Second, false); err != nil {
			t.Fatalf("stopVM: %v", err)
		}
	})
	if !strings.Contains(out, "stop reported") {
		t.Fatalf("out = %q", out)
	}
}

func TestStopVMGraceExpires(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	stubTools(t, nil)
	// grace=0 skips the wait loop entirely; without --force -> error.
	err := stopVM(e, st, e.vm("a"), 0, false)
	if err == nil || !strings.Contains(err.Error(), "did not stop") {
		t.Fatalf("grace err = %v", err)
	}
}

func TestStopVMForceKill(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	calls := stubTools(t, quickemuFake(t, ""))
	out := captureOut(t, func() {
		if err := stopVM(e, st, e.vm("a"), 0, true); err != nil {
			t.Fatalf("stopVM force: %v", err)
		}
	})
	if !strings.Contains(out, "force-killing") {
		t.Fatalf("out = %q", out)
	}
	killed := false
	for _, c := range *calls {
		if strings.Contains(strings.Join(c.args, " "), "--kill") {
			killed = true
		}
	}
	if !killed {
		t.Fatalf("--kill never issued: %v", *calls)
	}
}

func TestStopVMKillError(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	stubTools(t, func(c toolCall) (string, error) {
		return "", fmt.Errorf("kill refused")
	})
	err := stopVM(e, st, e.vm("a"), 0, true)
	if err == nil || !strings.Contains(err.Error(), "kill failed") {
		t.Fatalf("kill err = %v", err)
	}
}

func TestCmdUp(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	calls := stubTools(t, quickemuFake(t, ""))
	var rc int
	out := captureOut(t, func() {
		rc = cmdUp(e, nil, false)
	})
	if rc != 0 {
		t.Fatalf("cmdUp rc=%d out:\n%s", rc, out)
	}
	if !strings.Contains(out, "env demo: 2 vm(s), 2 level(s)") ||
		!strings.Contains(out, "== level 0: a") ||
		!strings.Contains(out, "== level 1: b") ||
		!strings.Contains(out, "env demo: up") {
		t.Fatalf("out:\n%s", out)
	}
	// a started before b (dependency order across levels).
	var ai, bi = -1, -1
	for i, c := range *calls {
		if c.name != "quickemu" {
			continue
		}
		if strings.Contains(strings.Join(c.args, " "), "a.conf") && ai < 0 {
			ai = i
		}
		if strings.Contains(strings.Join(c.args, " "), "b.conf") && bi < 0 {
			bi = i
		}
	}
	if ai < 0 || bi < 0 || ai > bi {
		t.Fatalf("start order a=%d b=%d: %v", ai, bi, *calls)
	}
	// State persisted: ports allocated, both VMs recorded.
	st := e.loadState()
	if st.Nets["lan"] == nil || st.Nets["lan"].Port == 0 {
		t.Fatalf("nets = %+v", st.Nets)
	}
	if st.VMs["a"] == nil || st.VMs["b"] == nil {
		t.Fatalf("vms = %+v", st.VMs)
	}
}

func TestCmdUpSelectionError(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	stubTools(t, nil)
	if rc := cmdUp(e, []string{"zzz"}, false); rc != 1 {
		t.Fatalf("rc = %d", rc)
	}
}

func TestCmdUpFailureAndRollback(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	calls := stubTools(t, quickemuFake(t, "b.conf"))
	var rc int
	out := captureOut(t, func() {
		rc = cmdUp(e, nil, true)
	})
	if rc != 1 {
		t.Fatalf("rc = %d", rc)
	}
	if !strings.Contains(out, "FAILED") || !strings.Contains(out, "rollback") {
		t.Fatalf("out:\n%s", out)
	}
	// Rollback stopped a: --stop was issued for a.conf.
	stopped := false
	for _, c := range *calls {
		j := strings.Join(c.args, " ")
		if c.name == "quickemu" && strings.Contains(j, "a.conf") && strings.Contains(j, "--stop") {
			stopped = true
		}
	}
	if !stopped {
		t.Fatalf("rollback never stopped a: %v", *calls)
	}
}

func TestCmdUpFailureNoRollback(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	calls := stubTools(t, quickemuFake(t, "b.conf"))
	var rc int
	captureOut(t, func() { rc = cmdUp(e, nil, false) })
	if rc != 1 {
		t.Fatalf("rc = %d", rc)
	}
	for _, c := range *calls {
		if strings.Contains(strings.Join(c.args, " "), "--stop") {
			t.Fatalf("unexpected stop without --rollback: %v", *calls)
		}
	}
}

func TestCmdUpHealthcheck(t *testing.T) {
	dir := t.TempDir()
	port := listenTCP(t, nil)
	body := fmt.Sprintf(`
[env]
name = "demo"

[[vm]]
name = "a"
conf = "a.conf"

[[healthcheck]]
vm = "a"
type = "tcp"
target = "127.0.0.1:%d"
timeout = "1s"
retries = 2
interval = "1ms"
`, port)
	e := loadGood(t, dir, body)
	confAt(t, dir, "a.conf", `guest_os="linux"`+"\n")
	stubTools(t, quickemuFake(t, ""))
	var rc int
	out := captureOut(t, func() { rc = cmdUp(e, nil, false) })
	if rc != 0 || !strings.Contains(out, "healthcheck tcp ok") {
		t.Fatalf("rc=%d out:\n%s", rc, out)
	}
}

func TestCmdUpHealthcheckFails(t *testing.T) {
	dir := t.TempDir()
	e := loadGood(t, dir, `
[env]
name = "demo"

[[vm]]
name = "a"
conf = "a.conf"

[[healthcheck]]
vm = "a"
type = "tcp"
target = "127.0.0.1:1"
timeout = "10ms"
retries = 2
interval = "1ms"
`)
	confAt(t, dir, "a.conf", `guest_os="linux"`+"\n")
	stubTools(t, quickemuFake(t, ""))
	var rc int
	out := captureOut(t, func() { rc = cmdUp(e, nil, false) })
	if rc != 1 || !strings.Contains(out, "FAILED healthcheck") {
		t.Fatalf("rc=%d out:\n%s", rc, out)
	}
}

func TestVMNamesAndReverse(t *testing.T) {
	l := []*VM{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	if got := vmNames(l); got != "a b c" {
		t.Fatalf("vmNames = %q", got)
	}
	rev := reverseVMs(l)
	if rev[0].Name != "c" || rev[2].Name != "a" || len(rev) != 3 {
		t.Fatalf("reverse = %v", rev)
	}
	if got := vmNames(nil); got != "" {
		t.Fatalf("empty vmNames = %q", got)
	}
}

func TestCmdDown(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	// Both VMs "running" — pidfiles present, the fake --stop removes them.
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	writeFile(t, filepath.Join(dir, "b", "b.pid"), fmt.Sprint(os.Getpid()))
	// A persisted net assignment is released once every member is down.
	st := newState(e)
	st.Nets["lan"] = &netState{Kind: "socket", Port: 7200, Listener: "a", Owner: os.Getpid()}
	e.save(st)
	calls := stubToolsSibling(t, quickemuFake(t, ""))
	var rc int
	out := captureOut(t, func() {
		rc = cmdDown(e, nil, 5*time.Second, false)
	})
	if rc != 0 {
		t.Fatalf("rc=%d out:\n%s", rc, out)
	}
	if !strings.Contains(out, "== level 1: b") || !strings.Contains(out, "== level 0: a") ||
		!strings.Contains(out, "env demo: down") {
		t.Fatalf("out:\n%s", out)
	}
	stops := 0
	released := false
	for _, c := range *calls {
		j := strings.Join(c.args, " ")
		if strings.Contains(j, "--stop") {
			stops++
		}
		if c.name == "quickemu-ports" && strings.Contains(j, "release") {
			released = true
		}
	}
	if stops != 2 {
		t.Fatalf("stops = %d: %v", stops, *calls)
	}
	if !released {
		t.Fatalf("lan port never released: %v", *calls)
	}
	if got := e.loadState().Nets["lan"]; got == nil || got.Owner != 0 {
		t.Fatalf("net state = %+v", got)
	}
}

func TestCmdDownSelectionError(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	stubTools(t, nil)
	if rc := cmdDown(e, []string{"zzz"}, time.Second, false); rc != 1 {
		t.Fatalf("rc = %d", rc)
	}
}

func TestSelAllDown(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	lan := e.net("lan")
	if !selAllDown(e, st, lan) {
		t.Fatal("all stopped should be down")
	}
	// One member still running.
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	if selAllDown(e, st, lan) {
		t.Fatal("running member should block")
	}
	// Unknown attach names are skipped.
	lan.Attach = append(lan.Attach, "ghost")
	if selAllDown(e, st, lan) {
		t.Fatal("ghost attach should be skipped, still blocked by a")
	}
}

func TestCmdDownKeepsPortWhenMembersRun(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	st.Nets["lan"] = &netState{Kind: "socket", Port: 7200, Listener: "a"}
	e.save(st)
	stub(t, &findTool, func(string) (string, error) {
		return "/x/quickemu-ports", nil
	})
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	// a "refuses" to die: the fake removes every pidfile except a's,
	// so stopVM's grace expires for it. A short grace keeps the test
	// fast — the expiry path still runs.
	calls := stubTools(t, func(c toolCall) (string, error) {
		if c.name == "quickemu" {
			for i, a := range c.args {
				if a == "--vm" && i+1 < len(c.args) {
					conf := c.args[i+1]
					if !strings.Contains(conf, "a.conf") {
						os.Remove(filepath.Join(vmDir(conf), confName(conf)+".pid"))
					}
				}
			}
		}
		return "", nil
	})
	var rc int
	captureOut(t, func() { rc = cmdDown(e, nil, 50*time.Millisecond, false) })
	if rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	// lan's port must NOT be released — a is still up.
	for _, c := range *calls {
		if c.name == "quickemu-ports" && strings.Contains(strings.Join(c.args, " "), "release") {
			t.Fatalf("port released while member alive: %v", *calls)
		}
	}
	st2 := e.loadState()
	if st2.Nets["lan"].Port != 7200 {
		t.Fatalf("port lost: %+v", st2.Nets["lan"])
	}
}

func TestCmdStatus(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	stubTools(t, nil)
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	out := captureOut(t, func() {
		if rc := cmdStatus(e, nil, false); rc != 0 {
			t.Fatalf("rc = %d", rc)
		}
	})
	if !strings.Contains(out, "VM") || !strings.Contains(out, "STATE") ||
		!strings.Contains(out, "running") || !strings.Contains(out, "stopped") {
		t.Fatalf("status:\n%s", out)
	}
}

func TestCmdStatusJSON(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	stubTools(t, nil)
	out := captureOut(t, func() {
		if rc := cmdStatus(e, nil, true); rc != 0 {
			t.Fatalf("rc = %d", rc)
		}
	})
	if !strings.Contains(out, `"command":"env.status"`) ||
		!strings.Contains(out, `"success":true`) ||
		!strings.Contains(out, `"name":"a"`) {
		t.Fatalf("status json:\n%s", out)
	}
}

func TestCmdStatusSelectionError(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	stubTools(t, nil)
	if rc := cmdStatus(e, []string{"zzz"}, false); rc != 1 {
		t.Fatalf("rc = %d", rc)
	}
}

func TestCmdRestart(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	// Running VMs get --stop, then the up pass --starts them again.
	writeFile(t, filepath.Join(dir, "a", "a.pid"), fmt.Sprint(os.Getpid()))
	writeFile(t, filepath.Join(dir, "b", "b.pid"), fmt.Sprint(os.Getpid()))
	calls := stubTools(t, quickemuFake(t, ""))
	var rc int
	captureOut(t, func() {
		rc = cmdRestart(e, nil, 5*time.Second, false, false)
	})
	if rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	stops, starts := 0, 0
	for _, c := range *calls {
		j := strings.Join(c.args, " ")
		if strings.Contains(j, "--stop") {
			stops++
		}
		if strings.Contains(j, "--start") {
			starts++
		}
	}
	if stops != 2 || starts != 2 {
		t.Fatalf("stops=%d starts=%d", stops, starts)
	}
}

func TestCmdDestroyYes(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	st.VMs["a"] = &vmState{Conf: filepath.Join(dir, "a.conf"), Created: true, VMdir: filepath.Join(dir, "a")}
	st.VMs["b"] = &vmState{Conf: filepath.Join(dir, "b.conf"), Created: false}
	st.Nets["lan"] = &netState{Port: 7200}
	e.save(st)
	writeFile(t, filepath.Join(dir, "a", "disk.qcow2"), "")
	calls := stubTools(t, quickemuFake(t, ""))
	stub(t, &findTool, func(string) (string, error) {
		return "/x/quickemu-ports", nil
	})
	var rc int
	out := captureOut(t, func() {
		rc = cmdDestroy(e, nil, true, 5*time.Second, false)
	})
	if rc != 0 || !strings.Contains(out, "destroyed") {
		t.Fatalf("rc=%d out:\n%s", rc, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.conf")); !os.IsNotExist(err) {
		t.Fatal("a.conf should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatal("vmdir should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "b.conf")); err != nil {
		t.Fatal("b.conf must survive — not env-created")
	}
	released := false
	for _, c := range *calls {
		if c.name == "quickemu-ports" && strings.Contains(strings.Join(c.args, " "), "release") {
			released = true
		}
	}
	if !released {
		t.Fatalf("port release never issued: %v", *calls)
	}
	st2 := e.loadState()
	if _, ok := st2.VMs["a"]; ok {
		t.Fatal("a still in state")
	}
	if _, ok := st2.VMs["b"]; !ok {
		t.Fatal("b must stay in state")
	}
	if len(st2.Nets) != 0 {
		t.Fatalf("nets = %+v", st2.Nets)
	}
}

func TestCmdDestroyPromptAbort(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	st.VMs["a"] = &vmState{Conf: filepath.Join(dir, "a.conf"), Created: true}
	e.save(st)
	stubTools(t, quickemuFake(t, ""))
	withStdin(t, "n\n")
	var rc int
	out := captureOut(t, func() {
		rc = cmdDestroy(e, nil, false, 5*time.Second, false)
	})
	if rc != 1 || !strings.Contains(out, "aborted") {
		t.Fatalf("rc=%d out:\n%s", rc, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.conf")); err != nil {
		t.Fatal("a.conf must survive abort")
	}
}

func TestCmdDestroyPromptConfirm(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	st.VMs["a"] = &vmState{Conf: filepath.Join(dir, "a.conf"), Created: true, VMdir: filepath.Join(dir, "a")}
	e.save(st)
	stubTools(t, quickemuFake(t, ""))
	withStdin(t, "y\n")
	var rc int
	captureOut(t, func() {
		rc = cmdDestroy(e, nil, false, 5*time.Second, false)
	})
	if rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.conf")); !os.IsNotExist(err) {
		t.Fatal("a.conf should be deleted after confirm")
	}
}

func TestCmdDestroySelectionError(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	stubTools(t, nil)
	if rc := cmdDestroy(e, []string{"zzz"}, true, time.Second, false); rc != 1 {
		t.Fatalf("rc = %d", rc)
	}
}

func TestCmdDestroySkipsEmptyConfState(t *testing.T) {
	dir := t.TempDir()
	e := orcEnv(t, dir)
	st := newState(e)
	st.VMs["a"] = &vmState{Conf: "", Created: true} // conf never recorded
	e.save(st)
	stubTools(t, quickemuFake(t, ""))
	var rc int
	captureOut(t, func() {
		rc = cmdDestroy(e, nil, true, 5*time.Second, false)
	})
	if rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.conf")); err != nil {
		t.Fatal("a.conf on disk must survive — state never recorded it")
	}
}
