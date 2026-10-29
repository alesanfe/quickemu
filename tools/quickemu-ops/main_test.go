// main_test.go: run() in-process re-exec covering the whole op
// lifecycle — new→set→status→list→done/fail→cancel→prune→recover→
// rollback — against a temp vmdir, envelope and on-disk state asserted.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		a := os.Getenv("QE_HELPER_ARGS")
		os.Args = []string{"quickemu-ops"}
		if a != "" {
			os.Args = append(os.Args, strings.Split(a, "\x1f")...)
		}
		rc := run()
		_ = os.WriteFile(os.Getenv("QE_HELPER_RC"),
			[]byte(strconv.Itoa(rc)), 0o600)
		return
	}
	os.Exit(m.Run())
}

func runTool(t *testing.T, args ...string) (string, int) {
	t.Helper()
	rcf := filepath.Join(t.TempDir(), "rc")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QE_HELPER_ARGS="+strings.Join(args, "\x1f"),
		"QE_HELPER_RC="+rcf)
	var so bytes.Buffer
	cmd.Stdout = &so
	_ = cmd.Run() // helper exits non-zero on die() paths — not a test error
	rc := -1      // -1 = helper exited via die()/os.Exit mid-flow
	if b, err := os.ReadFile(rcf); err == nil {
		rc, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	return so.String(), rc
}

func newOp(t *testing.T, vm string) (vmdir, id string) {
	t.Helper()
	vmdir = t.TempDir()
	out, rc := runTool(t, "new", "--vmdir", vmdir, "--type", "export", "--vm", vm)
	if rc != 0 {
		t.Fatalf("new rc=%d", rc)
	}
	return vmdir, strings.TrimSpace(out)
}

func opState(t *testing.T, vmdir, id string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(vmdir, "ops", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var o map[string]any
	json.Unmarshal(b, &o)
	return o
}

func TestLifecycle(t *testing.T) {
	vmdir, id := newOp(t, "v")

	if out, rc := runTool(t, "set", "--vmdir", vmdir, "--id", id,
		"--state", "running", "--progress", "40"); rc != 0 {
		t.Fatalf("set rc=%d %s", rc, out)
	}
	if st := opState(t, vmdir, id); st["state"] != "running" ||
		st["progress"].(float64) != 40 {
		t.Fatalf("state=%v", st)
	}

	out, rc := runTool(t, "status", "--vmdir", vmdir, "--id", id)
	var so map[string]any
	if rc != 0 || json.Unmarshal([]byte(out), &so) != nil ||
		so["state"] != "running" {
		t.Fatalf("status rc=%d %s", rc, out)
	}

	if _, rc := runTool(t, "done", "--vmdir", vmdir, "--id", id); rc != 0 {
		t.Fatalf("done rc=%d", rc)
	}
	if st := opState(t, vmdir, id); st["state"] != "completed" ||
		st["progress"].(float64) != 100 {
		t.Fatalf("%v", st)
	}

	// Terminal guard: completed op refuses set (die → rc=-1, envelope out).
	out, rc = runTool(t, "set", "--vmdir", vmdir, "--id", id,
		"--state", "running", "--progress", "1")
	if rc != -1 || !strings.Contains(out, "OP_FINISHED") {
		t.Fatalf("terminal guard rc=%d %s", rc, out)
	}
}

func TestFailAndCancel(t *testing.T) {
	vmdir, id := newOp(t, "v")
	if _, rc := runTool(t, "fail", "--vmdir", vmdir, "--id", id,
		"--code", "E", "--message", "boom"); rc != 0 {
		t.Fatalf("fail rc=%d", rc)
	}
	st := opState(t, vmdir, id)
	if st["state"] != "failed" ||
		!strings.Contains(string(st["error"].(map[string]any)["message"].(string)), "boom") {
		t.Fatalf("%v", st)
	}

	// cancel with no live worker → cancelled immediately.
	v2, id2 := newOp(t, "v")
	if _, rc := runTool(t, "cancel", "--vmdir", v2, "--id", id2); rc != 0 {
		t.Fatalf("cancel rc=%d", rc)
	}
	if opState(t, v2, id2)["state"] != "cancelled" {
		t.Fatal("cancel must be final without worker")
	}
}

// A hostile op file can name any live workerPid — cancel must verify
// the pid is actually a quickemu worker (its cmdline carries the ops
// dir or a quickemu* binary) before SIGTERM, the same ownership proof
// .pid consumers already require. Otherwise a crafted ops/*.json
// points the signal at an unrelated user process.
func TestCancelForeignWorkerPid(t *testing.T) {
	vmdir, id := newOp(t, "v")
	sl := exec.Command("sleep", "60")
	if err := sl.Start(); err != nil {
		t.Skip("no sleep binary")
	}
	defer sl.Process.Kill()

	// Rewrite the op as running with the foreign pid as its worker.
	var o Op
	b, err := os.ReadFile(filepath.Join(vmdir, "ops", id+".json"))
	if err != nil || json.Unmarshal(b, &o) != nil {
		t.Fatal(err)
	}
	o.State, o.WorkerPid = "running", sl.Process.Pid
	nb, _ := json.Marshal(o)
	if err := os.WriteFile(filepath.Join(vmdir, "ops", id+".json"), nb, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, rc := runTool(t, "cancel", "--vmdir", vmdir, "--id", id); rc != 0 {
		t.Fatalf("cancel rc=%d", rc)
	}
	if err := sl.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("foreign workerPid was signalled by cancel")
	}
	if st := opState(t, vmdir, id); st["state"] != "cancelled" {
		t.Fatalf("unverified worker must not defer cancel, state=%v", st["state"])
	}
}

func TestListPruneRecoverRollback(t *testing.T) {
	vmdir, id := newOp(t, "v")
	out, rc := runTool(t, "list", "--vmdir", vmdir)
	if rc != 0 || !strings.Contains(out, id) {
		t.Fatalf("list rc=%d", rc)
	}

	// prune --keep 0 removes all.
	out, rc = runTool(t, "prune", "--vmdir", vmdir, "--keep", "0")
	if rc != 0 || !strings.Contains(out, `"pruned":1`) {
		t.Fatalf("prune rc=%d %s", rc, out)
	}

	// recover marks a stale running op failed.
	v2, id2 := newOp(t, "v")
	o := opState(t, v2, id2)
	o["state"] = "running"
	b, _ := json.Marshal(o)
	os.WriteFile(filepath.Join(v2, "ops", id2+".json"), b, 0o600)
	out, rc = runTool(t, "recover", "--vmdir", v2)
	if rc != 0 || !strings.Contains(out, `"reaped":1`) {
		t.Fatalf("recover rc=%d %s", rc, out)
	}
	if opState(t, v2, id2)["state"] != "failed" {
		t.Fatal("recover must fail stale ops")
	}

	// rollback cleans .part files for download-type ops.
	v3, id3 := newOp(t, "v")
	os.WriteFile(filepath.Join(v3, "x.part"), []byte("p"), 0o600)
	out, rc = runTool(t, "set", "--vmdir", v3, "--id", id3,
		"--state", "running", "--progress", "10")
	if rc != 0 {
		t.Fatalf("set %s", out)
	}
	o = opState(t, v3, id3)
	o["type"] = "download"
	b, _ = json.Marshal(o)
	os.WriteFile(filepath.Join(v3, "ops", id3+".json"), b, 0o600)
	out, rc = runTool(t, "rollback", "--vmdir", v3, "--id", id3)
	if rc != 0 || !strings.Contains(out, "rolledBack") {
		t.Fatalf("rollback rc=%d %s", rc, out)
	}
	if _, err := os.Stat(filepath.Join(v3, "x.part")); !os.IsNotExist(err) {
		t.Fatal("rollback must delete .part")
	}
}

// rollback interpolates o.Type — file content, not a trusted value —
// into a json.RawMessage literal. A quote in type escapes the message
// string and injects arbitrary fields into the rewritten op.
func TestRollbackTypeInjection(t *testing.T) {
	vmdir, id := newOp(t, "v")
	o := opState(t, vmdir, id)
	evil := `x","state":"pwned`
	o["type"] = evil
	b, _ := json.Marshal(o)
	os.WriteFile(filepath.Join(vmdir, "ops", id+".json"), b, 0o600)
	out, rc := runTool(t, "rollback", "--vmdir", vmdir, "--id", id)
	if rc != 0 {
		t.Fatalf("rollback rc=%d %s", rc, out)
	}
	got := opState(t, vmdir, id)
	er, ok := got["error"].(map[string]any)
	if !ok || len(er) != 2 || er["code"] != "OPERATION_ROLLBACK" {
		t.Fatalf("type injected extra keys into error: %v", got["error"])
	}
	if er["message"] != "rolled back "+evil+" effects" {
		t.Fatalf("type must stay literal in message: %v", er["message"])
	}
	if got["state"] != "failed" {
		t.Fatalf("state rewritten by injection: %v", got["state"])
	}
}

func TestBadArgs(t *testing.T) {
	if _, rc := runTool(t); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
	if _, rc := runTool(t, "list", "--foo", "x"); rc != -1 { // die(BAD_ARGS)
		t.Fatalf("no vmdir rc=%d", rc)
	}
	if _, rc := runTool(t, "bogus", "--vmdir", t.TempDir()); rc != 2 {
		t.Fatalf("unknown rc=%d", rc)
	}
}

// An op file whose embedded operationId disagrees with its filename —
// including traversal — must never reach o.save()'s filepath.Join.
// checkID guards the flag, but load() trusted file content.
func TestEmbeddedIDTraversal(t *testing.T) {
	vmdir := t.TempDir()
	ops := filepath.Join(vmdir, "ops")
	os.MkdirAll(ops, 0o755)
	escape := filepath.Join(filepath.Dir(vmdir), "escaped.json")
	plant := `{"schemaVersion":1,"operationId":"../../escaped","type":"export","state":"queued"}`
	os.WriteFile(filepath.Join(ops, "evil.json"), []byte(plant), 0o600)
	if _, rc := runTool(t, "set", "--vmdir", vmdir, "--id", "evil",
		"--state", "running"); rc == 0 {
		t.Fatal("set on foreign-id op must fail")
	}
	if _, err := os.Stat(escape); !os.IsNotExist(err) {
		t.Fatal("embedded operationId escaped the ops dir")
	}
	// recover/rollback unmarshal op files directly — the write path
	// itself must reject an escaping embedded id.
	escape2 := filepath.Join(filepath.Dir(vmdir), "escaped2.json")
	plant2 := `{"schemaVersion":1,"operationId":"../../escaped2","type":"export","state":"queued"}`
	os.WriteFile(filepath.Join(ops, "evil2.json"), []byte(plant2), 0o600)
	runTool(t, "recover", "--vmdir", vmdir)
	if _, err := os.Stat(escape2); !os.IsNotExist(err) {
		t.Fatal("recover wrote outside the ops dir")
	}
	escape3 := filepath.Join(filepath.Dir(vmdir), "escaped3.json")
	plant3 := `{"schemaVersion":1,"operationId":"../../escaped3","type":"export","state":"queued"}`
	os.WriteFile(filepath.Join(ops, "evil3.json"), []byte(plant3), 0o600)
	runTool(t, "rollback", "--vmdir", vmdir, "--id", "evil3")
	if _, err := os.Stat(escape3); !os.IsNotExist(err) {
		t.Fatal("rollback wrote outside the ops dir")
	}
}

func TestVerbosityRank(t *testing.T) {
	want := map[string]int{"quiet": 0, "normal": 1, "verbose": 2, "debug": 3, "bogus": 1, "": 1}
	for s, w := range want {
		if g := verbosityRank(s); g != w {
			t.Errorf("verbosityRank(%q)=%d want %d", s, g, w)
		}
	}
}
