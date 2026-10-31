// backup_incr_test.go: QE-102 — incremental backup flow through a
// scripted QMP stub (bitmap add/clear, drive-backup, block-jobs).
package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// incrStubs: quickemu-qmp answers by QMP command name; drive-backup
// touches the target file so the registry records a real artifact.
func incrStubs(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("posix stub")
	}
	d := t.TempDir()
	qmp := `#!/bin/sh
case "$*" in
  *query-block-jobs*) echo '{"data":{"return":[]}}' ;;
  *block-dirty-bitmap-add*)
    echo "$*" >> "$QE_QMP_CALLS"
    echo '{"data":{"return":{}}}' ;;
  *block-dirty-bitmap-clear*)
    echo "$*" >> "$QE_QMP_CALLS"
    echo '{"data":{"return":{}}}' ;;
  *drive-backup*)
    echo "$*" >> "$QE_QMP_CALLS"
    tgt=$(printf '%s' "$*" | sed -n 's/.*"target"[ :"]*\([^",}]*\).*/\1/p')
    [ -n "$tgt" ] && echo x > "$tgt"
    echo '{"data":{"return":{}}}' ;;
  *) echo '{"data":{"return":{}}}' ;;
esac
exit 0`
	if err := os.WriteFile(filepath.Join(d, "quickemu-qmp"), []byte(qmp), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestBackupIncrNotRunning(t *testing.T) {
	withConf(t, "")
	if rc := actBackupIncr(); rc != 1 {
		t.Fatalf("stopped VM rc=%d", rc)
	}
}

func TestBackupIncrBaselineThenIncremental(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("no /bin/sleep")
	}
	incrStubs(t)
	vd := withConf(t, "")
	livePID(t, vd)
	monSock(t, "-qmp.socket")
	machineTransition(stRunning, "test", "t")
	calls := filepath.Join(vd, "qmp-calls.log")
	t.Setenv("QE_QMP_CALLS", calls)

	// First run: bitmap add + full baseline.
	if rc := actBackupIncr(); rc != 0 {
		t.Fatalf("baseline rc=%d", rc)
	}
	b, _ := os.ReadFile(calls)
	if !strings.Contains(string(b), "block-dirty-bitmap-add") ||
		!strings.Contains(string(b), `"sync":"full"`) ||
		!strings.Contains(string(b), "block-dirty-bitmap-clear") {
		t.Fatalf("baseline calls: %s", b)
	}
	reg := incrRegistryRead()
	if len(reg) != 1 || reg[0].Mode != "full" || reg[0].Size <= 0 {
		t.Fatalf("registry %+v", reg)
	}

	// Second run: incremental carrying the bitmap.
	if rc := actBackupIncr(); rc != 0 {
		t.Fatalf("incr rc=%d", rc)
	}
	b, _ = os.ReadFile(calls)
	if !strings.Contains(string(b), `"sync":"incremental"`) ||
		!strings.Contains(string(b), `"bitmap":"qe-incr"`) {
		t.Fatalf("incr calls: %s", b)
	}
	reg = incrRegistryRead()
	if len(reg) != 2 || reg[1].Mode != "incremental" {
		t.Fatalf("registry %+v", reg)
	}
}

func TestBackupIncrList(t *testing.T) {
	withConf(t, "")
	os.MkdirAll(incrDir(), 0o755)
	incrRegistryAppend(incrRec{At: "t", File: "/x/1.qcow2", Mode: "full", Size: 10})
	incrRegistryAppend(incrRec{At: "t2", File: "/x/2.qcow2", Mode: "incremental", Size: 5})
	if rc := actBackupIncrList(); rc != 0 {
		t.Fatalf("list rc=%d", rc)
	}
	old := outFmt
	outFmt = "json"
	defer func() { outFmt = old }()
	if rc := actBackupIncrList(); rc != 0 {
		t.Fatalf("list json rc=%d", rc)
	}
}

func TestBackupIncrParse(t *testing.T) {
	old := actions
	actions = nil
	t.Cleanup(func() { actions = old })
	parseArgs([]string{"--vm", "x.conf", "--backup-incr", "--backup-incr-list"})
	if len(actions) != 2 || actions[0] != "backup_incr" || actions[1] != "backup_incr_list" {
		t.Fatalf("actions %v", actions)
	}
}
