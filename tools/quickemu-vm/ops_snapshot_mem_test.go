// ops_snapshot_mem_test.go — QE-021 registry ops + QE-022 policy
// engine: parse, status, plan, create/prune through a stub qemu-img.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSnapAutoParse(t *testing.T) {
	// off/empty → no periods, no error.
	for _, spec := range []string{"", "off", "  off  "} {
		ps, err := snapAutoParse(spec)
		if err != nil || len(ps) != 0 {
			t.Fatalf("spec %q: ps=%v err=%v", spec, ps, err)
		}
	}
	// Bare names keep the default retention.
	ps, err := snapAutoParse("hourly,daily,weekly")
	if err != nil || len(ps) != 3 {
		t.Fatalf("bare: %v %v", ps, err)
	}
	if ps[0].Name != "hourly" || ps[0].Keep != 4 {
		t.Fatalf("hourly default %+v", ps[0])
	}
	// Custom retention wins.
	ps, err = snapAutoParse("daily:3")
	if err != nil || len(ps) != 1 || ps[0].Keep != 3 {
		t.Fatalf("custom keep: %v %v", ps, err)
	}
	// Bad retention / unknown period / out-of-range → error.
	for _, spec := range []string{"daily:x", "daily:0", "daily:100",
		"minutely", "daily:2:3"} {
		if _, err := snapAutoParse(spec); err == nil {
			t.Fatalf("spec %q should fail", spec)
		}
	}
}

func TestSnapshotRegisterUnregister(t *testing.T) {
	dir := t.TempDir()
	if rc := snapshotRegister(dir, "boot-ok", "vmstate", "post-install"); rc != 0 {
		t.Fatalf("register rc=%d", rc)
	}
	arr := snapMetaRead(dir)
	if len(arr) != 1 || arr[0].Tag != "boot-ok" || arr[0].Type != "vmstate" ||
		arr[0].Desc != "post-install" {
		t.Fatalf("meta %+v", arr)
	}
	// Unregister: removes the entry; unknown tag is a no-op.
	if rc := snapshotUnregister(dir, "boot-ok"); rc != 0 {
		t.Fatalf("unregister rc=%d", rc)
	}
	if len(snapMetaRead(dir)) != 0 {
		t.Fatal("entry not removed")
	}
	if rc := snapshotUnregister(dir, "never-existed"); rc != 0 {
		t.Fatalf("unregister unknown rc=%d", rc)
	}
}

// fakeQemuImgPolicy: stub logging `snapshot -q -X <tag> <disk>` calls
// to $QIMG_LOG and answering `snapshot -l --output=json` with
// $QIMG_SNAPLIST.
func fakeQemuImgPolicy(t *testing.T, list string) (logFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("posix stub")
	}
	dir := t.TempDir()
	logFile = filepath.Join(dir, "calls.log")
	sh := filepath.Join(dir, "qemu-img")
	os.WriteFile(sh, []byte(`#!/bin/sh
if [ "$1" = snapshot ]; then
  shift
  if [ "$1" = "-l" ]; then
    printf '%s' "$QIMG_SNAPLIST"
    exit 0
  fi
  echo "$@" >> "$QIMG_LOG"
  exit 0
fi
if [ "$1" = info ]; then
  printf '%s' "$QIMG_INFO"
  exit 0
fi
exit 0
`), 0o755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("QIMG_SNAPLIST", list)
	t.Setenv("QIMG_LOG", logFile)
	return logFile
}

// mkVMPolicy: mkVM3 plus a snapshot_auto conf line.
func mkVMPolicy(t *testing.T, spec string) (dir, disk string) {
	t.Helper()
	dir, conf, disk := mkVM3(t)
	b, _ := os.ReadFile(conf)
	os.WriteFile(conf, append(b, []byte("snapshot_auto=\""+spec+"\"\n")...), 0o644)
	return dir, disk
}

func TestSnapshotPolicyStatus(t *testing.T) {
	dir, _ := mkVMPolicy(t, "off")
	if rc := snapshotPolicy(dir, "status", false); rc != 0 {
		t.Fatalf("status off rc=%d", rc)
	}
	// Enabled policy: report counts per period.
	dir2, _ := mkVMPolicy(t, "hourly:2,daily:3")
	list := `[{"name":"auto-hourly-2026010100","date-sec":1},
	         {"name":"auto-hourly-2026010101","date-sec":2},
	         {"name":"auto-daily-20260101","date-sec":3},
	         {"name":"manual-tag","date-sec":4}]`
	fakeQemuImgPolicy(t, list)
	if rc := snapshotPolicy(dir2, "status", false); rc != 0 {
		t.Fatalf("status rc=%d", rc)
	}
	oldJSON := outputJSON
	outputJSON = true
	defer func() { outputJSON = oldJSON }()
	if rc := snapshotPolicy(dir2, "status", false); rc != 0 {
		t.Fatalf("status json rc=%d", rc)
	}
}

func TestSnapshotPolicyUnitsAndRun(t *testing.T) {
	dir, _ := mkVMPolicy(t, "hourly:2")
	if rc := snapshotPolicy(dir, "units", false); rc != 0 {
		t.Fatalf("units rc=%d", rc)
	}

	// Dry run: plan only — wants the current hourly stamp, prunes
	// the two oldest of three existing.
	list := `[{"name":"auto-hourly-2020010100","date-sec":1},
	         {"name":"auto-hourly-2020010101","date-sec":2},
	         {"name":"auto-hourly-2020010102","date-sec":3}]`
	logFile := fakeQemuImgPolicy(t, list)
	if rc := snapshotPolicy(dir, "run", true); rc != 0 {
		t.Fatalf("dry run rc=%d", rc)
	}
	if b, err := os.ReadFile(logFile); err == nil && len(b) > 0 {
		t.Fatalf("dry run must not call qemu-img: %s", b)
	}

	// Real run: creates the current stamp + prunes to keep=2.
	if rc := snapshotPolicy(dir, "run", false); rc != 0 {
		t.Fatalf("run rc=%d", rc)
	}
	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("no qemu-img calls logged: %v", err)
	}
	log := string(b)
	var creates, dels int
	for _, l := range strings.Split(strings.TrimSpace(log), "\n") {
		if strings.HasPrefix(l, "-q -c ") {
			creates++
			if !strings.Contains(l, "auto-hourly-") {
				t.Fatalf("bad create: %s", l)
			}
		}
		if strings.HasPrefix(l, "-q -d ") {
			dels++
		}
	}
	// 3 existing + 1 created = 4 > keep 2 → 2 prunes.
	if creates != 1 || dels != 2 {
		t.Fatalf("creates=%d dels=%d log=%s", creates, dels, log)
	}
}

func TestSnapshotPolicyOff(t *testing.T) {
	dir, _ := mkVMPolicy(t, "off")
	if rc := snapshotPolicy(dir, "run", false); rc != 0 {
		t.Fatalf("run off rc=%d", rc)
	}
	oldJSON := outputJSON
	outputJSON = true
	defer func() { outputJSON = oldJSON }()
	if rc := snapshotPolicy(dir, "run", false); rc != 0 {
		t.Fatalf("run off json rc=%d", rc)
	}
}

// snapInternalList: corrupt/absent qemu-img output → nil, not a crash.
func TestSnapInternalListBad(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix stub")
	}
	fakeQemuImgPolicy(t, "{not json")
	if got := snapInternalList("x"); got != nil {
		t.Fatalf("bad json → %v", got)
	}
	// Empty list parses to zero entries.
	fakeQemuImgPolicy(t, "[]")
	if got := snapInternalList("x"); len(got) != 0 {
		t.Fatalf("empty → %v", got)
	}
	// Round trip through the register: a registered vmstate entry
	// surfaces with its type for status consumers.
	dir := t.TempDir()
	if rc := snapshotRegister(dir, "m1", "vmstate", ""); rc != 0 {
		t.Fatalf("register rc=%d", rc)
	}
	arr := snapMetaRead(dir)
	b, _ := json.Marshal(arr)
	if !strings.Contains(string(b), "vmstate") {
		t.Fatalf("meta missing type: %s", b)
	}
}
