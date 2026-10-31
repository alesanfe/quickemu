package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// invFixture builds an env with two VMs: "web" (running, ssh forward,
// on the lan socket net as listener) and "db" (stopped, nothing
// reachable). Returns the env and the persisted state.
func invFixture(t *testing.T) (*Env, *envState) {
	t.Helper()
	dir := t.TempDir()
	e := &Env{
		Dir:  dir,
		Name: "demo-env",
		File: filepath.Join(dir, "env.toml"),
		VMs: []*VM{
			{Name: "web", Deps: []string{"db"}},
			{Name: "db"},
		},
		Nets: []*Net{
			{Name: "lan", Kind: "socket", Attach: []string{"web", "db"}},
		},
	}
	webConf := confAt(t, dir, "web.conf", `ssh_port="2299"`+"\n")
	dbConf := confAt(t, dir, "db.conf", "")
	st := &envState{
		Version: 1, Env: e.Name, File: e.File,
		VMs: map[string]*vmState{
			"web": {Conf: webConf},
			"db":  {Conf: dbConf},
		},
		Nets: map[string]*netState{
			"lan": {Kind: "socket", Port: 7200, Listener: "web"},
		},
	}
	// web runs: pidfile (live) + .ports record.
	writeFile(t, filepath.Join(dir, "web", "web.pid"), fmt.Sprint(os.Getpid()))
	writeFile(t, filepath.Join(dir, "web", "web.ports"), "ssh,2222\n")
	return e, st
}

func TestQGAIPNoTool(t *testing.T) {
	stubTools(t, nil)
	if got := qgaIP("/any/conf"); got != "" {
		t.Fatalf("qgaIP = %q", got)
	}
}

func TestQGAIPNoSocket(t *testing.T) {
	qga := fakeScript(t, "quickemu-qga", `echo '{"ip":"10.0.0.1"}'`)
	stub(t, &findTool, func(string) (string, error) { return qga, nil })
	conf := filepath.Join(t.TempDir(), "vm.conf")
	if got := qgaIP(conf); got != "" {
		t.Fatalf("no socket qgaIP = %q", got)
	}
}

func TestQGAIPFromScript(t *testing.T) {
	dir := t.TempDir()
	conf := qgaConf(t, dir, "vm1")
	writeFile(t, agentSock(conf), "") // presence only — no handshake
	// Fields-splittable output: loopback and link-local are skipped.
	qga := fakeScript(t, "quickemu-qga",
		`echo '127.0.0.1 "169.254.1.1" 10.1.2.3 fe80::1'`)
	stub(t, &findTool, func(name string) (string, error) {
		if name == "quickemu-qga" {
			return qga, nil
		}
		return "", fmt.Errorf("no tool")
	})
	if got := qgaIP(conf); got != "10.1.2.3" {
		t.Fatalf("qgaIP = %q", got)
	}
}

func TestQGAIPScriptFails(t *testing.T) {
	dir := t.TempDir()
	conf := qgaConf(t, dir, "vm1")
	writeFile(t, agentSock(conf), "")
	qga := fakeScript(t, "quickemu-qga", "exit 3")
	stub(t, &findTool, func(string) (string, error) { return qga, nil })
	if got := qgaIP(conf); got != "" {
		t.Fatalf("failing qga script = %q", got)
	}
}

func TestQGAIPNoIPv4InOutput(t *testing.T) {
	dir := t.TempDir()
	conf := qgaConf(t, dir, "vm1")
	writeFile(t, agentSock(conf), "")
	qga := fakeScript(t, "quickemu-qga", `echo '["::1","localhost"]'`)
	stub(t, &findTool, func(string) (string, error) { return qga, nil })
	if got := qgaIP(conf); got != "" {
		t.Fatalf("no-ipv4 qgaIP = %q", got)
	}
}

func TestGuestIP(t *testing.T) {
	e, _ := invFixture(t)
	stubTools(t, nil) // findTool fails -> qgaIP ""
	if got := guestIP(e, "", vmRuntime{}); got != "" {
		t.Fatalf("empty conf guestIP = %q", got)
	}
	dir := e.Dir
	conf := confAt(t, dir, "x.conf", `ssh_port="2299"`)
	// ssh_port in conf -> host-reachable via the forward.
	if got := guestIP(e, conf, vmRuntime{Ports: map[string]int{}}); got != "127.0.0.1" {
		t.Fatalf("ssh_port guestIP = %q", got)
	}
	// .ports record works too.
	rt := vmRuntime{Ports: map[string]int{"http": 8080}}
	conf2 := confAt(t, dir, "y.conf", "")
	if got := guestIP(e, conf2, rt); got != "127.0.0.1" {
		t.Fatalf("ports guestIP = %q", got)
	}
	// Nothing known -> "".
	if got := guestIP(e, conf2, vmRuntime{Ports: map[string]int{}}); got != "" {
		t.Fatalf("unknowable guestIP = %q", got)
	}
}

func TestCmdHosts(t *testing.T) {
	e, _ := invFixture(t)
	stubTools(t, nil)
	out := captureOut(t, func() {
		if rc := cmdHosts(e, nil); rc != 0 {
			t.Fatalf("cmdHosts rc = %d", rc)
		}
	})
	if !strings.Contains(out, "# >>> quickemu-env demo-env — BEGIN") ||
		!strings.Contains(out, "127.0.0.1") ||
		!strings.Contains(out, "web") ||
		!strings.Contains(out, "no host-reachable address") ||
		!strings.Contains(out, "# >>> quickemu-env demo-env — END") {
		t.Fatalf("hosts block:\n%s", out)
	}
	if rc := cmdHosts(e, []string{"zzz"}); rc != 1 {
		t.Fatalf("bad selection rc = %d", rc)
	}
}

func TestInventory(t *testing.T) {
	e, st := invFixture(t)
	stubTools(t, nil)
	vms, _ := e.selectVMs(nil)
	inv := inventory(e, st, vms)
	if len(inv) != 2 {
		t.Fatalf("inv = %v", inv)
	}
	web := inv["web"].(map[string]any)
	if web["state"] != "running" || web["pid"] != os.Getpid() ||
		web["ip"] != "127.0.0.1" || web["conf"] == "" {
		t.Fatalf("web rec = %v", web)
	}
	if _, ok := web["ports"]; !ok {
		t.Fatalf("web ports missing: %v", web)
	}
	nets, ok := web["nets"].([]netInfo)
	if !ok || len(nets) != 1 || nets[0].Name != "lan" || nets[0].Role != "listen" || nets[0].Port != 7200 {
		t.Fatalf("web nets = %v", web["nets"])
	}
	db := inv["db"].(map[string]any)
	if db["state"] != "stopped" {
		t.Fatalf("db rec = %v", db)
	}
	if _, ok := db["ip"]; ok {
		t.Fatalf("db should have no ip: %v", db)
	}
	if _, ok := db["pid"]; ok {
		t.Fatalf("db should have no pid: %v", db)
	}
}

func TestCmdInventoryHuman(t *testing.T) {
	e, _ := invFixture(t)
	stubTools(t, nil)
	out := captureOut(t, func() {
		if rc := cmdInventory(e, nil, false, ""); rc != 0 {
			t.Fatalf("rc = %d", rc)
		}
	})
	if !strings.Contains(out, "VM") || !strings.Contains(out, "STATE") ||
		!strings.Contains(out, "web") || !strings.Contains(out, "running") ||
		!strings.Contains(out, "127.0.0.1") || !strings.Contains(out, "db") {
		t.Fatalf("human inventory:\n%s", out)
	}
}

func TestCmdInventoryJSON(t *testing.T) {
	e, _ := invFixture(t)
	stubTools(t, nil)
	out := captureOut(t, func() {
		if rc := cmdInventory(e, nil, true, ""); rc != 0 {
			t.Fatalf("rc = %d", rc)
		}
	})
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json out: %v\n%s", err, out)
	}
	if doc["env"] != "demo-env" {
		t.Fatalf("env = %v", doc["env"])
	}
	vms := doc["vms"].(map[string]any)
	if vms["web"].(map[string]any)["state"] != "running" {
		t.Fatalf("vms = %v", vms)
	}
	// -o json works like --json.
	out = captureOut(t, func() {
		if rc := cmdInventory(e, nil, false, "json"); rc != 0 {
			t.Fatalf("rc = %d", rc)
		}
	})
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("-o json: %v", err)
	}
}

func TestCmdInventoryAnsible(t *testing.T) {
	e, _ := invFixture(t)
	stubTools(t, nil)
	out := captureOut(t, func() {
		if rc := cmdInventory(e, nil, false, "ansible"); rc != 0 {
			t.Fatalf("rc = %d", rc)
		}
	})
	if !strings.Contains(out, "[env_demo_env]") {
		t.Fatalf("group header missing:\n%s", out)
	}
	if !strings.Contains(out, "web ansible_host=127.0.0.1 ansible_port=2222") {
		t.Fatalf("web host line missing:\n%s", out)
	}
	if !strings.Contains(out, "db ansible_host=db") ||
		!strings.Contains(out, "no host-reachable address") {
		t.Fatalf("db host line missing:\n%s", out)
	}
	if !strings.Contains(out, "[env_demo_env:vars]") ||
		!strings.Contains(out, "ansible_user=user") {
		t.Fatalf("vars section missing:\n%s", out)
	}
}

func TestCmdInventoryBadSelection(t *testing.T) {
	e, _ := invFixture(t)
	stubTools(t, nil)
	if rc := cmdInventory(e, []string{"zzz"}, false, ""); rc != 1 {
		t.Fatalf("rc = %d", rc)
	}
}

func TestGuestIPViaQGA(t *testing.T) {
	e, _ := invFixture(t)
	dir := t.TempDir()
	conf := qgaConf(t, dir, "vm1")
	writeFile(t, agentSock(conf), "")
	qga := fakeScript(t, "quickemu-qga", `echo '["10.9.9.9"]'`)
	stub(t, &findTool, func(name string) (string, error) {
		if name == "quickemu-qga" {
			return qga, nil
		}
		return "", fmt.Errorf("no tool")
	})
	if got := guestIP(e, conf, vmRuntime{Ports: map[string]int{}}); got != "10.9.9.9" {
		t.Fatalf("qga guestIP = %q", got)
	}
}
