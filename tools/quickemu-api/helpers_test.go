// helpers_test.go — unit coverage for the helpers.go plumbing:
// envelope writers, identity/dir resolution, conf/.ports parsing,
// the FSM-state readers, probeVM state derivation, the op journal
// readers and the bounded subprocess helpers.
package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// withDataDir swaps the package dataDir for a temp root for the
// duration of the test (same pattern the existing tests use).
func withDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := dataDir
	dataDir = dir
	t.Cleanup(func() { dataDir = old })
	return dir
}

func deadPID() int { return 1 << 30 } // beyond pid_max: ESRCH → dead

// ---------- v1 envelope ----------

func TestEnvelopeWriters(t *testing.T) {
	rec := httptest.NewRecorder()
	okData(rec, map[string]any{"x": 1})
	if rec.Code != 200 {
		t.Fatalf("okData code %d", rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != true || m["data"].(map[string]any)["x"].(float64) != 1 {
		t.Fatalf("okData body %v", m)
	}

	rec = httptest.NewRecorder()
	okDataCode(rec, 202, map[string]any{"id": "op-1"})
	if rec.Code != 202 {
		t.Fatalf("okDataCode code %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != true {
		t.Fatalf("okDataCode body %v", m)
	}

	rec = httptest.NewRecorder()
	errData(rec, 409, "OP_CONFLICT", "busy")
	if rec.Code != 409 {
		t.Fatalf("errData code %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != false {
		t.Fatalf("errData ok %v", m["ok"])
	}
	e := m["error"].(map[string]any)
	if e["code"] != "OP_CONFLICT" || e["message"] != "busy" {
		t.Fatalf("errData error %v", e)
	}
	// The legacy errors[] array rides along for pre-QE-080 clients.
	arr := m["errors"].([]any)
	if len(arr) != 1 || arr[0].(map[string]any)["code"] != "OP_CONFLICT" {
		t.Fatalf("errData errors %v", arr)
	}
}

// ---------- identity ----------

func TestBadVMName(t *testing.T) {
	for _, bad := range []string{
		"", "..", "a..b", "..x", "a/b", `a\b`, "/", `\\`,
		"a?b", "a*b", "a[b", "a]b", "*", "?", "[a]",
	} {
		if !badVMName(bad) {
			t.Errorf("badVMName(%q)=false, want true", bad)
		}
	}
	for _, ok := range []string{"vm1", "my-vm", "a.b", "a b", "vm_2", "a,b"} {
		if badVMName(ok) {
			t.Errorf("badVMName(%q)=true, want false", ok)
		}
	}
}

// ---------- inventory / dir resolution ----------

func TestVmsJSON(t *testing.T) {
	dir := withDataDir(t)
	if vmsJSON() != nil {
		t.Fatal("missing vms.json must give nil")
	}
	if err := os.WriteFile(filepath.Join(dir, "vms.json"), []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if vmsJSON() != nil {
		t.Fatal("corrupt vms.json must give nil")
	}
	if err := os.WriteFile(filepath.Join(dir, "vms.json"),
		[]byte(`{"k":{"vm":"a","state":"running"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := vmsJSON()
	if m == nil || m["k"]["vm"] != "a" {
		t.Fatalf("vmsJSON %v", m)
	}
	// A valid JSON non-object (array) must also give nil.
	if err := os.WriteFile(filepath.Join(dir, "vms.json"), []byte(`[1,2]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if vmsJSON() != nil {
		t.Fatal("array vms.json must give nil")
	}
}

func TestVMDirFor(t *testing.T) {
	dir := withDataDir(t)
	vmsRoot := filepath.Join(dir, "vms")

	// 1. On-disk <dataDir>/vms/<name> wins.
	d1 := filepath.Join(vmsRoot, "disk")
	if err := os.MkdirAll(d1, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := vmDirFor("disk"); got != d1 {
		t.Fatalf("disk dir: %q want %q", got, d1)
	}

	// 2. Inventory-recorded dir for a name with no on-disk dir.
	other := filepath.Join(dir, "elsewhere", "inv")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := fmt.Sprintf(`{"k":{"vm":"inv","dir":%q}}`, other)
	if err := os.WriteFile(filepath.Join(dir, "vms.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := vmDirFor("inv"); got != other {
		t.Fatalf("inventory dir: %q want %q", got, other)
	}
	// Inventory entry whose dir no longer exists falls through.
	reg = `{"k":{"vm":"gone","dir":"/nonexistent/qe-gone"}}`
	if err := os.WriteFile(filepath.Join(dir, "vms.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := vmDirFor("gone"); got != "" {
		t.Fatalf("dead inventory dir: %q want empty", got)
	}

	// 3. Loose <name>.conf in the vms root → the conventional dir guess.
	os.Remove(filepath.Join(dir, "vms.json"))
	if err := os.WriteFile(filepath.Join(vmsRoot, "loose.conf"), []byte("guest_os=\"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(vmsRoot, "loose")
	if got := vmDirFor("loose"); got != want {
		t.Fatalf("loose conf dir: %q want %q", got, want)
	}

	// 4. Nothing resolves → empty.
	if got := vmDirFor("nobody"); got != "" {
		t.Fatalf("unknown vm: %q want empty", got)
	}
}

func TestVMConfFor(t *testing.T) {
	dir := t.TempDir()
	// Preferred <name>.conf.
	os.WriteFile(filepath.Join(dir, "vm.conf"), []byte("x=1\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a-other.conf"), []byte("x=2\n"), 0o644)
	if got := vmConfFor(dir, "vm"); got != filepath.Join(dir, "vm.conf") {
		t.Fatalf("preferred conf: %q", got)
	}
	// No <name>.conf → first *.conf (sorted).
	if got := vmConfFor(dir, "other"); got != filepath.Join(dir, "a-other.conf") {
		t.Fatalf("fallback conf: %q", got)
	}
	// Empty dir → "".
	empty := t.TempDir()
	if got := vmConfFor(empty, "x"); got != "" {
		t.Fatalf("empty dir conf: %q want empty", got)
	}
}

func TestCollectVMs(t *testing.T) {
	dir := withDataDir(t)
	// On-disk dirs win the Dir field; inventory merges Recorded/LastSeen.
	for _, n := range []string{"bbb", "aaa"} {
		if err := os.MkdirAll(filepath.Join(dir, "vms", n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	reg := `{
		"k1": {"vm": "aaa", "state": "running", "lastSeen": "T1", "dir": "/ignored"},
		"k2": {"vm": "ccc", "state": "defined", "lastSeen": "T2"},
		"k3": {"state": "stopped"}
	}`
	if err := os.WriteFile(filepath.Join(dir, "vms.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	ents := collectVMs()
	if len(ents) != 4 {
		t.Fatalf("collectVMs len %d want 4: %+v", len(ents), ents)
	}
	// Sorted by Name: aaa, bbb, ccc, k3.
	names := []string{ents[0].Name, ents[1].Name, ents[2].Name, ents[3].Name}
	want := []string{"aaa", "bbb", "ccc", "k3"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("sorted names %v want %v", names, want)
		}
	}
	// Merged inventory metadata onto the disk entry.
	if ents[0].Recorded != "running" || ents[0].LastSeen != "T1" {
		t.Fatalf("merge: %+v", ents[0])
	}
	if ents[0].Dir != filepath.Join(dir, "vms", "aaa") {
		t.Fatalf("disk dir must win over inventory dir: %+v", ents[0])
	}
	// Inventory-only entry: recorded dir empty → conventional guess.
	if ents[2].Dir != filepath.Join(dir, "vms", "ccc") || ents[2].Key != "k2" {
		t.Fatalf("inventory-only: %+v", ents[2])
	}
	// Entry with no vm name → the key becomes the name.
	if ents[3].Name != "k3" || ents[3].Recorded != "stopped" {
		t.Fatalf("key-named: %+v", ents[3])
	}
	// Empty store → empty slice.
	dir2 := withDataDir(t)
	_ = dir2
	if got := collectVMs(); len(got) != 0 {
		t.Fatalf("empty collectVMs: %v", got)
	}
}

// ---------- conf / ports ----------

func TestConfMap(t *testing.T) {
	dir := t.TempDir()
	conf := `# comment line
guest_os="linux"
ram=4G
no-equals-line
empty=
password='s3cr3t'
  spaced =  "v al"  
`
	if err := os.WriteFile(filepath.Join(dir, "vm.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	m := confMap(dir, "vm")
	if m["guest_os"] != "linux" || m["ram"] != "4G" || m["password"] != "s3cr3t" {
		t.Fatalf("confMap %v", m)
	}
	if v, ok := m["empty"]; !ok || v != "" {
		t.Fatalf("empty value: %q", v)
	}
	if m["spaced"] != "v al" {
		t.Fatalf("spaced: %q", m["spaced"])
	}
	if _, ok := m["no-equals-line"]; ok {
		t.Fatal("line without = must be skipped")
	}
	if _, ok := m["# comment line"]; ok {
		t.Fatal("comment must be skipped")
	}
	// No conf at all → empty map.
	if got := confMap(t.TempDir(), "none"); len(got) != 0 {
		t.Fatalf("missing conf: %v", got)
	}
}

func TestConfSecretShape(t *testing.T) {
	for _, k := range []string{"password", "PassWord", "x_secret", "api_token",
		"ssh_key", "keyring", "cred_file"} {
		if !confSecretShape(k) {
			t.Errorf("confSecretShape(%q)=false", k)
		}
	}
	// "keyboard" trips the "key" substring guard — allowlisted but
	// still secret-shaped, so confSafe drops it.
	for _, k := range []string{"guest_os", "ram", "arch"} {
		if confSecretShape(k) {
			t.Errorf("confSecretShape(%q)=true", k)
		}
	}
}

func TestConfSafe(t *testing.T) {
	dir := t.TempDir()
	conf := `guest_os="linux"
ssh_port="2222"
password="hunter2"
extra_args="-enable-kvm"
keyboard="fr"
`
	if err := os.WriteFile(filepath.Join(dir, "vm.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	m := confSafe(dir, "vm")
	if m["guest_os"] != "linux" || m["ssh_port"] != "2222" {
		t.Fatalf("safe keys: %v", m)
	}
	for _, k := range []string{"password", "extra_args", "keyboard"} {
		if _, ok := m[k]; ok {
			t.Fatalf("key %q must not surface: %v", k, m)
		}
	}
}

func TestReadPorts(t *testing.T) {
	dir := t.TempDir()
	body := `ssh,2222,127.0.0.1
unix,/run/vm.sock
lonly
,2222,ignored

`
	if err := os.WriteFile(filepath.Join(dir, "vm.ports"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readPorts(dir, "vm")
	if len(got) != 3 {
		t.Fatalf("readPorts len %d: %v", len(got), got)
	}
	if got[0]["name"] != "ssh" || got[0]["port"] != "2222" || got[0]["host"] != "127.0.0.1" {
		t.Fatalf("full record: %v", got[0])
	}
	if got[1]["name"] != "unix" || got[1]["port"] != "/run/vm.sock" {
		t.Fatalf("unix record: %v", got[1])
	}
	if _, ok := got[1]["host"]; ok {
		t.Fatalf("unix record must not have host: %v", got[1])
	}
	if got[2]["name"] != "lonly" || len(got[2]) != 1 {
		t.Fatalf("name-only record: %v", got[2])
	}
	// Missing file → empty.
	if got := readPorts(dir, "none"); len(got) != 0 {
		t.Fatalf("missing ports: %v", got)
	}
}

// ---------- state ----------

func TestFSMRec(t *testing.T) {
	dir := t.TempDir()
	if fsmRec(dir, "vm") != nil {
		t.Fatal("missing file must give nil")
	}
	os.WriteFile(filepath.Join(dir, "vm.state.json"), []byte("{corrupt"), 0o644)
	if fsmRec(dir, "vm") != nil {
		t.Fatal("corrupt fsm must give nil")
	}
	// Valid JSON without a state word is no machine record.
	os.WriteFile(filepath.Join(dir, "vm.state.json"), []byte(`{"pid":1}`), 0o644)
	if fsmRec(dir, "vm") != nil {
		t.Fatal("state-less fsm must give nil")
	}
	os.WriteFile(filepath.Join(dir, "vm.state.json"), []byte(`{"state":"running","pid":42}`), 0o644)
	m := fsmRec(dir, "vm")
	if m == nil || m["state"] != "running" {
		t.Fatalf("fsmRec %v", m)
	}
}

func TestRuntimeRec(t *testing.T) {
	dir := t.TempDir()
	if runtimeRec(dir, "vm") != nil {
		t.Fatal("missing file must give nil")
	}
	os.WriteFile(filepath.Join(dir, "vm.runtime.json"), []byte("not json"), 0o644)
	if runtimeRec(dir, "vm") != nil {
		t.Fatal("corrupt runtime must give nil")
	}
	os.WriteFile(filepath.Join(dir, "vm.runtime.json"), []byte(`{"pid":42,"monitor":"x"}`), 0o644)
	m := runtimeRec(dir, "vm")
	if m == nil || m["pid"].(float64) != 42 {
		t.Fatalf("runtimeRec %v", m)
	}
}

func TestRecPID(t *testing.T) {
	if recPID(nil) != 0 {
		t.Fatal("nil map → 0")
	}
	if recPID(map[string]any{}) != 0 {
		t.Fatal("no pid → 0")
	}
	if recPID(map[string]any{"pid": float64(42)}) != 42 {
		t.Fatal("float pid")
	}
	if recPID(map[string]any{"pid": "123"}) != 123 {
		t.Fatal("string pid")
	}
	if recPID(map[string]any{"pid": "garbage"}) != 0 {
		t.Fatal("garbage string → 0")
	}
	if recPID(map[string]any{"pid": true}) != 0 {
		t.Fatal("bool pid → 0")
	}
}

func TestPIDFilePID(t *testing.T) {
	dir := t.TempDir()
	if pidFilePID(dir, "vm") != 0 {
		t.Fatal("missing file → 0")
	}
	os.WriteFile(filepath.Join(dir, "vm.pid"), []byte("  4242\n"), 0o644)
	if pidFilePID(dir, "vm") != 4242 {
		t.Fatal("trim+parse pid")
	}
	os.WriteFile(filepath.Join(dir, "vm.pid"), []byte("notanum"), 0o644)
	if pidFilePID(dir, "vm") != 0 {
		t.Fatal("garbage → 0")
	}
}

func TestLiveishFSM(t *testing.T) {
	for _, st := range []string{"running", "paused", "starting", "saving", "stopping", "suspended"} {
		if !liveishFSM(st) {
			t.Errorf("liveishFSM(%q)=false", st)
		}
	}
	for _, st := range []string{"stopped", "crashed", "error", "", "defined", "archived"} {
		if liveishFSM(st) {
			t.Errorf("liveishFSM(%q)=true", st)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestProbeVM walks the derived-state truth table: the FSM record
// when present plus process liveness decide the reported word.
func TestProbeVM(t *testing.T) {
	root := t.TempDir()
	live := os.Getpid()
	dead := deadPID()

	mk := func(name string) string {
		d := filepath.Join(root, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}

	cases := []struct {
		name     string
		setup    func(dir string)
		recorded string
		want     string
		wantLive bool
	}{
		{"fsm-stopped", func(d string) {
			writeFile(t, filepath.Join(d, "v.state.json"), `{"state":"stopped"}`)
		}, "", "stopped", false},
		{"runtime-live", func(d string) {
			b, _ := json.Marshal(map[string]any{"pid": live})
			writeFile(t, filepath.Join(d, "v.runtime.json"), string(b))
		}, "", "running", true},
		{"live-paused-marker", func(d string) {
			b, _ := json.Marshal(map[string]any{"pid": live})
			writeFile(t, filepath.Join(d, "v.runtime.json"), string(b))
			writeFile(t, filepath.Join(d, "v.paused"), "")
		}, "", "paused", true},
		{"fsm-live-dead-pid", func(d string) {
			b, _ := json.Marshal(map[string]any{"state": "running", "pid": dead})
			writeFile(t, filepath.Join(d, "v.state.json"), string(b))
		}, "", "crashed", false},
		{"fsm-stopping-dead-pid", func(d string) {
			b, _ := json.Marshal(map[string]any{"state": "stopping", "pid": dead})
			writeFile(t, filepath.Join(d, "v.state.json"), string(b))
		}, "", "stopped", false},
		{"fsm-paused-live", func(d string) {
			b, _ := json.Marshal(map[string]any{"state": "paused", "pid": live})
			writeFile(t, filepath.Join(d, "v.state.json"), string(b))
		}, "", "paused", true},
		{"pidfile-dead", func(d string) {
			writeFile(t, filepath.Join(d, "v.pid"), "1073741824\n")
		}, "", "stale", false},
		{"runtime-dead", func(d string) {
			writeFile(t, filepath.Join(d, "v.runtime.json"), `{"pid":1073741824}`)
		}, "", "stale", false},
		{"recorded-only", func(d string) {}, "defined", "defined", false},
		{"nothing", func(d string) {}, "", "stopped", false},
		{"corrupt-fsm-live-rt", func(d string) {
			writeFile(t, filepath.Join(d, "v.state.json"), "{corrupt")
			b, _ := json.Marshal(map[string]any{"pid": live})
			writeFile(t, filepath.Join(d, "v.runtime.json"), string(b))
		}, "", "running", true},
		{"runtime-string-pid", func(d string) {
			writeFile(t, filepath.Join(d, "v.runtime.json"), `{"pid":"1073741824"}`)
		}, "", "stale", false},
		{"fsm-pid-live", func(d string) {
			b, _ := json.Marshal(map[string]any{"state": "running", "pid": live})
			writeFile(t, filepath.Join(d, "v.state.json"), string(b))
		}, "", "running", true},
	}
	for _, c := range cases {
		dir := mk(c.name)
		c.setup(dir)
		ev := probeVM(dir, "v", c.recorded)
		if ev.State != c.want {
			t.Errorf("%s: state %q want %q", c.name, ev.State, c.want)
		}
		if ev.Alive != c.wantLive {
			t.Errorf("%s: alive %v want %v", c.name, ev.Alive, c.wantLive)
		}
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	if fileExists(filepath.Join(dir, "nope")) {
		t.Fatal("missing must be false")
	}
	p := filepath.Join(dir, "f")
	os.WriteFile(p, []byte("x"), 0o644)
	if !fileExists(p) {
		t.Fatal("existing must be true")
	}
}

func TestAllowedOpsFor(t *testing.T) {
	cases := map[string]string{
		"running":    "pause resume shutdown reset kill status monitor-cmd screenshot send-keys type snapshot-clone export",
		"paused":     "resume shutdown kill status monitor-cmd screenshot",
		"suspended":  "resume shutdown kill status monitor-cmd screenshot",
		"saving":     "resume shutdown kill status monitor-cmd screenshot",
		"stopped":    "start delete clone rename export import snapshot apply create delete info list inspect-conf delete-plan",
		"stale":      "start delete clone rename export import snapshot apply create delete info list inspect-conf delete-plan",
		"defined":    "start delete clone rename export import snapshot apply create delete info list inspect-conf delete-plan",
		"crashed":    "start delete clone rename export import snapshot apply create delete info list inspect-conf delete-plan",
		"error":      "start delete clone rename export import snapshot apply create delete info list inspect-conf delete-plan",
		"archived":   "restore trash-list status inspect-conf delete",
		"starting":   "status kill",
		"stopping":   "status kill",
		"recovering": "status kill",
		"bogus":      "status",
		"":           "status",
	}
	for st, want := range cases {
		got := allowedOpsFor(st)
		if strings.Join(got, " ") != want {
			t.Errorf("allowedOpsFor(%q)=%v want %v", st, got, want)
		}
	}
}

// ---------- op journal ----------

func TestOpActive(t *testing.T) {
	for _, st := range []string{"queued", "running", "cancelRequested"} {
		if !opActive(st) {
			t.Errorf("opActive(%q)=false", st)
		}
	}
	for _, st := range []string{"succeeded", "failed", "cancelled", ""} {
		if opActive(st) {
			t.Errorf("opActive(%q)=true", st)
		}
	}
}

func TestExclusiveOpType(t *testing.T) {
	for _, typ := range []string{"delete", "import", "export", "clone",
		"linked-clone", "rename", "move", "start", "stop", "kill",
		"snapshot-apply", "snapshot-create", "snapshot-delete",
		"backup", "backup-restore", "compact", "resize"} {
		if !exclusiveOpType(typ) {
			t.Errorf("exclusiveOpType(%q)=false", typ)
		}
	}
	for _, typ := range []string{"status", "download", "install", "", "monitor-cmd"} {
		if exclusiveOpType(typ) {
			t.Errorf("exclusiveOpType(%q)=true", typ)
		}
	}
}

func TestOpsDirs(t *testing.T) {
	dir := withDataDir(t)
	if err := os.MkdirAll(filepath.Join(dir, "vms", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := opsDirs()
	want := filepath.Join(dir, "vms", "x")
	if len(got) != 1 || got[want] != "x" {
		t.Fatalf("opsDirs %v", got)
	}
}

func TestScanOps(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	if err := os.MkdirAll(opsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(opsDir, "a.json"),
		`{"operationId":"op-a","type":"start","state":"succeeded","vm":"x","startedAt":"T1"}`)
	writeFile(t, filepath.Join(opsDir, "b.json"), "{corrupt")
	writeFile(t, filepath.Join(opsDir, "c.json"),
		`{"operationId":"../evil","type":"start","state":"succeeded"}`)
	writeFile(t, filepath.Join(opsDir, "d.json"),
		`{"operationId":"op-d","type":"stop","state":"queued","startedAt":"T2"}`)
	// A directory that glob-matches *.json must be skipped, not trusted.
	if err := os.MkdirAll(filepath.Join(opsDir, "dir.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := scanOps(dir)
	if len(got) != 2 {
		t.Fatalf("scanOps len %d: %+v", len(got), got)
	}
	if got[0].OperationID != "op-a" || got[1].OperationID != "op-d" {
		t.Fatalf("scanOps ids %v", got)
	}
	// Empty vm field inherits the dir basename.
	if got[1].VM != filepath.Base(dir) {
		t.Fatalf("vm fill: %q want %q", got[1].VM, filepath.Base(dir))
	}
	if got[0].File == "" {
		t.Fatal("File must record the source path")
	}
}

func TestAllOpsOrdering(t *testing.T) {
	dir := withDataDir(t)
	// Two VM dirs; ops sort by startedAt then operationId.
	for _, vm := range []string{"v1", "v2"} {
		od := filepath.Join(dir, "vms", vm, "ops")
		if err := os.MkdirAll(od, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, "vms", "v1", "ops", "b.json"),
		`{"operationId":"op-b","type":"stop","startedAt":"2024-01-02T00:00:00Z"}`)
	writeFile(t, filepath.Join(dir, "vms", "v2", "ops", "a.json"),
		`{"operationId":"op-a","type":"start","startedAt":"2024-01-01T00:00:00Z"}`)
	writeFile(t, filepath.Join(dir, "vms", "v2", "ops", "c.json"),
		`{"operationId":"op-c","type":"kill","startedAt":"2024-01-01T00:00:00Z"}`)
	got := allOps()
	if len(got) != 3 {
		t.Fatalf("allOps len %d", len(got))
	}
	ids := []string{got[0].OperationID, got[1].OperationID, got[2].OperationID}
	want := []string{"op-a", "op-c", "op-b"} // same ts → id order; later ts last
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order %v want %v", ids, want)
		}
	}
	// Empty store → empty, not nil-crash.
	withDataDir(t)
	if got := allOps(); len(got) != 0 {
		t.Fatalf("empty allOps: %v", got)
	}
}

// ---------- boot id / op id / envelope parsing ----------

func TestBootID(t *testing.T) {
	old := procBootID
	t.Cleanup(func() { procBootID = old })
	f := filepath.Join(t.TempDir(), "boot_id")
	os.WriteFile(f, []byte("  abc-123\n"), 0o644)
	procBootID = f
	if got := bootID(); got != "abc-123" {
		t.Fatalf("bootID %q", got)
	}
	procBootID = filepath.Join(t.TempDir(), "missing")
	if got := bootID(); got != "" {
		t.Fatalf("missing bootID %q want empty", got)
	}
}

func TestGenOpID(t *testing.T) {
	id := genOpID()
	re := regexp.MustCompile(`^op-\d{8}T\d{6}Z-[0-9a-f]{8}$`)
	if !re.MatchString(id) {
		t.Fatalf("genOpID shape %q", id)
	}
	if badOpID(id) {
		t.Fatalf("genOpID must pass badOpID: %q", id)
	}
	if genOpID() == genOpID() {
		t.Fatal("genOpID must be unique")
	}
}

func TestEnvelopeErrCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"errors":[{"code":"OP_CONFLICT","message":"busy"}]}`, "OP_CONFLICT: busy"},
		{`{"errors":[{"code":"OP_CONFLICT"}]}`, "OP_CONFLICT"},
		{"not json\nalso not", ""},
		{`{"errors":[]}`, ""},
		{"garbage\n" + `{"errors":[{"code":"X","message":"m"}]}`, "X: m"},
		{`{"ok":true,"data":{}}`, ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := envelopeErrCode(c.in); got != c.want {
			t.Errorf("envelopeErrCode(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestXDGEnv(t *testing.T) {
	dir := withDataDir(t)
	env := xdgEnv()
	if len(env) != 1 || env[0] != "XDG_DATA_HOME="+filepath.Dir(dir) {
		t.Fatalf("xdgEnv %v", env)
	}
}

// ---------- subprocess helpers ----------

// stubBin drops an executable shell script named <name> into a temp
// dir and prepends it to PATH — qelib.FindTool's last-resort lookup.
func stubBin(t *testing.T, name, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, name),
		[]byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestToolT(t *testing.T) {
	stubBin(t, "qe-echo", "#!/bin/sh\necho hello-$MYVAR\n")
	// Missing tool → FindTool error surfaces.
	if _, err := toolT("qe-not-a-tool", time.Second, nil); err == nil {
		t.Fatal("missing tool must error")
	}
	out, err := toolT("qe-echo", 5*time.Second, []string{"MYVAR=42"}, "a", "b")
	if err != nil || strings.TrimSpace(out) != "hello-42" {
		t.Fatalf("toolT out=%q err=%v", out, err)
	}
	// Non-zero exit → output + error both surface.
	stubBin(t, "qe-fail", "#!/bin/sh\necho oops\nexit 3\n")
	out, err = toolT("qe-fail", 5*time.Second, nil)
	if err == nil || !strings.Contains(out, "oops") {
		t.Fatalf("failing tool out=%q err=%v", out, err)
	}
	// Wall-clock bound: a stuck tool is killed and reported.
	stubBin(t, "qe-sleep", "#!/bin/sh\nexec sleep 30\n")
	if _, err := toolT("qe-sleep", 60*time.Millisecond, nil); err == nil ||
		!strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout err=%v", err)
	}
}

func TestCLITool(t *testing.T) {
	// quickemu preferred over quickemu-cli.
	stubBin(t, "quickemu", "#!/bin/sh\necho cli\n")
	stubBin(t, "quickemu-cli", "#!/bin/sh\necho cli2\n")
	if b, err := cliTool(); err != nil || filepath.Base(b) != "quickemu" {
		t.Fatalf("cliTool %q %v", b, err)
	}
	out, err := toolTCLI(5*time.Second, "status")
	if err != nil || strings.TrimSpace(out) != "cli" {
		t.Fatalf("toolTCLI %q %v", out, err)
	}
	// Only the dev-tree binary present → second choice resolves.
	bin2 := t.TempDir()
	os.WriteFile(filepath.Join(bin2, "quickemu-cli"),
		[]byte("#!/bin/sh\necho only-cli\n"), 0o755)
	t.Setenv("PATH", bin2+string(os.PathListSeparator)+"/usr/bin:/bin")
	if b, err := cliTool(); err != nil || filepath.Base(b) != "quickemu-cli" {
		t.Fatalf("cliTool fallback %q %v", b, err)
	}
	out, err = toolTCLI(5 * time.Second)
	if err != nil || strings.TrimSpace(out) != "only-cli" {
		t.Fatalf("toolTCLI fallback %q %v", out, err)
	}
	// Neither present → explicit error.
	t.Setenv("PATH", t.TempDir())
	if _, err := cliTool(); err == nil {
		t.Fatal("cliTool must error when no CLI resolves")
	}
	if _, err := toolTCLI(time.Second); err == nil {
		t.Fatal("toolTCLI must error when no CLI resolves")
	}
}

func TestToolTCLITimeout(t *testing.T) {
	stubBin(t, "quickemu", "#!/bin/sh\nexec sleep 30\n")
	if _, err := toolTCLI(60 * time.Millisecond); err == nil ||
		!strings.Contains(err.Error(), "timed out") {
		t.Fatalf("toolTCLI timeout err=%v", err)
	}
}

// ---------- enqueueOp ----------

func TestEnqueueOpViaTool(t *testing.T) {
	dir := t.TempDir()
	stubBin(t, "quickemu-ops", "#!/bin/sh\necho op-from-tool\n")
	id, err := enqueueOp(dir, "vm", "start", "phase1", "msg", "")
	if err != nil || id != "op-from-tool" {
		t.Fatalf("enqueueOp id=%q err=%v", id, err)
	}
	// Tool output that is a valid id shape is adopted verbatim —
	// even though no journal file exists for it.
}

func TestEnqueueOpFallbackDirectWrite(t *testing.T) {
	dir := t.TempDir()
	// No quickemu-ops on PATH → the API writes the journal record itself.
	// Non-exclusive types throughout: an active exclusive op would
	// conflict the next enqueue (covered by TestEnqueueOpConflict).
	t.Setenv("PATH", t.TempDir())
	id, err := enqueueOp(dir, "vm", "download", "", "", "")
	if err != nil || id == "" {
		t.Fatalf("fallback enqueue id=%q err=%v", id, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "ops", id+".json"))
	if err != nil {
		t.Fatalf("journal record not written: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatal(err)
	}
	if rec["operationId"] != id || rec["state"] != "queued" || rec["type"] != "download" {
		t.Fatalf("record %v", rec)
	}
	// A caller-supplied id is used when the tool is absent.
	id2, err := enqueueOp(dir, "vm", "status", "p", "m", "fixed-op")
	if err != nil || id2 != "fixed-op" {
		t.Fatalf("wantID enqueue id=%q err=%v", id2, err)
	}
	rec2 := map[string]any{}
	b2, _ := os.ReadFile(filepath.Join(dir, "ops", "fixed-op.json"))
	json.Unmarshal(b2, &rec2)
	if rec2["phase"] != "p" || rec2["message"] != "m" {
		t.Fatalf("phase/message: %v", rec2)
	}
	// Same id again → OP_EXISTS (non-exclusive type so the
	// conflict pass can't fire first).
	if _, err := enqueueOp(dir, "vm", "status", "", "", "fixed-op"); err == nil ||
		!strings.Contains(err.Error(), "OP_EXISTS") {
		t.Fatalf("dup id err=%v", err)
	}
	// Traversal-shaped wantID → INVALID_ARGS.
	if _, err := enqueueOp(dir, "vm", "status", "", "", "../x"); err == nil ||
		!strings.Contains(err.Error(), "INVALID_ARGS") {
		t.Fatalf("bad id err=%v", err)
	}
}

func TestEnqueueOpConflict(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", t.TempDir()) // force the direct-write fallback
	writeFile(t, filepath.Join(dir, "ops", "busy.json"),
		`{"operationId":"busy","type":"delete","state":"running","startedAt":"T"}`)
	// delete is exclusive → any other op conflicts.
	if _, err := enqueueOp(dir, "vm", "clone", "", "", ""); err == nil ||
		!strings.Contains(err.Error(), "OP_CONFLICT") {
		t.Fatalf("exclusive conflict err=%v", err)
	}
	// An active but non-exclusive op does not block a non-exclusive new one.
	dir2 := t.TempDir()
	writeFile(t, filepath.Join(dir2, "ops", "st.json"),
		`{"operationId":"st","type":"status","state":"running","startedAt":"T"}`)
	if id, err := enqueueOp(dir2, "vm", "download", "", "", ""); err != nil || id == "" {
		t.Fatalf("non-exclusive enqueue id=%q err=%v", id, err)
	}
	// A finished op never conflicts.
	dir3 := t.TempDir()
	writeFile(t, filepath.Join(dir3, "ops", "done.json"),
		`{"operationId":"done","type":"delete","state":"succeeded","startedAt":"T"}`)
	if id, err := enqueueOp(dir3, "vm", "start", "", "", ""); err != nil || id == "" {
		t.Fatalf("finished-op enqueue id=%q err=%v", id, err)
	}
}

func TestEnqueueOpToolErrorEnvelope(t *testing.T) {
	dir := t.TempDir()
	// A resolved-but-failing tool prints a v1 envelope: the fallback
	// writes the record, then surfaces the tool's error code.
	stubBin(t, "quickemu-ops",
		"#!/bin/sh\necho '{\"errors\":[{\"code\":\"OP_CONFLICT\",\"message\":\"busy\"}]}'\nexit 1\n")
	if _, err := enqueueOp(dir, "vm", "start", "", "", ""); err == nil ||
		err.Error() != "OP_CONFLICT: busy" {
		t.Fatalf("envelope err=%v", err)
	}
	// Silent failure (no envelope) → direct write still succeeds.
	dir2 := t.TempDir()
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "quickemu-ops"),
		[]byte("#!/bin/sh\nexit 1\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	id, err := enqueueOp(dir2, "vm", "start", "", "", "")
	if err != nil || id == "" {
		t.Fatalf("silent-fail enqueue id=%q err=%v", id, err)
	}
}
