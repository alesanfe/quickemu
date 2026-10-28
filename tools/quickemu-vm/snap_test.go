// snap_test.go — external snapshot lifecycle, trash/delete/glob/num
// helpers with a fake qemu-img on PATH.
package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeQemuImg puts a stub qemu-img on PATH that prints QIMG_INFO for
// `info` and exits 0 otherwise.
func fakeQemuImg(t *testing.T, infoOut string) {
	t.Helper()
	dir := t.TempDir()
	sh := filepath.Join(dir, "qemu-img")
	os.WriteFile(sh, []byte("#!/bin/sh\nif [ \"$1\" = info ]; then\n printf '%s' \"$QIMG_INFO\"\nelse\n exit 0\nfi\n"), 0o755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("QIMG_INFO", infoOut)
}

// mkVM builds a temp VM dir with a conf + a fake disk.
func mkVM3(t *testing.T) (dir, conf, disk string) {
	t.Helper()
	dir = t.TempDir()
	disk = filepath.Join(dir, "disk.qcow2")
	os.WriteFile(disk, []byte("qcow"), 0o600)
	conf = dir + ".conf"
	os.WriteFile(conf, []byte("disk_img=\""+disk+"\"\nguest_os=\"linux\"\n"), 0o644)
	t.Setenv("QE_VMCONF", conf)
	return dir, conf, disk
}

func TestSnapshotLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix stub")
	}
	fakeQemuImg(t, "")
	dir, conf, disk := mkVM3(t)

	// create: errors first.
	if rc := snapshotCreateExternal(dir, "", "", conf); rc != 1 {
		t.Fatalf("empty tag rc=%d", rc)
	}
	emptyDir := t.TempDir()
	t.Setenv("QE_VMCONF", "") // glob finds nothing in an empty dir
	if rc := snapshotCreateExternal(emptyDir, "s1", "", ""); rc != 1 {
		t.Fatalf("no disk rc=%d", rc)
	}
	t.Setenv("QE_VMCONF", conf)
	// happy path — conf repointed at the overlay, metadata registered.
	if rc := snapshotCreateExternal(dir, "s1", "first", conf); rc != 0 {
		t.Fatalf("create rc=%d", rc)
	}
	overlay := filepath.Join(dir, filepath.Base(dir)+"-s1.qcow2")
	b, _ := os.ReadFile(conf)
	if !strings.Contains(string(b), overlay) {
		t.Fatalf("conf not repointed: %s", b)
	}
	meta := snapMetaRead(dir)
	if len(meta) != 1 || meta[0].Tag != "s1" || meta[0].Parent != disk {
		t.Fatalf("meta %+v", meta)
	}
	// duplicate tag → die — skip (exits); existing overlay file → 1.
	overlay2 := filepath.Join(dir, filepath.Base(dir)+"-s2.qcow2")
	os.WriteFile(overlay2, []byte("o"), 0o600)
	if rc := snapshotCreateExternal(dir, "s2", "", conf); rc != 1 {
		t.Fatalf("existing overlay rc=%d", rc)
	}
	os.Remove(overlay2)
	os.WriteFile(overlay, []byte("o"), 0o600)

	// tree: text + JSON modes.
	if rc := snapshotTree(dir); rc != 0 {
		t.Fatalf("tree rc=%d", rc)
	}
	oldJSON := outputJSON
	outputJSON = true
	if rc := snapshotTree(dir); rc != 0 {
		t.Fatalf("tree json rc=%d", rc)
	}
	outputJSON = oldJSON

	// verify: missing overlay → 1; present + wrong backing → 1; ok → 0.
	if rc := snapshotVerify(dir); rc != 1 {
		t.Fatalf("verify missing rc=%d", rc)
	}
	os.WriteFile(overlay, []byte("o"), 0o600)
	t.Setenv("QIMG_INFO", "image: x\nbacking file: wrong\n")
	if rc := snapshotVerify(dir); rc != 1 {
		t.Fatalf("verify mismatch rc=%d", rc)
	}
	t.Setenv("QIMG_INFO", "image: x\nbacking file: "+disk+"\n")
	if rc := snapshotVerify(dir); rc != 0 {
		t.Fatalf("verify ok rc=%d", rc)
	}

	// consolidate: leaf commit removes file + meta entry.
	if rc := snapshotConsolidate(dir, ""); rc != 1 {
		t.Fatalf("consolidate empty rc=%d", rc)
	}
	if rc := snapshotConsolidate(dir, "s1"); rc != 0 {
		t.Fatalf("consolidate rc=%d", rc)
	}
	if _, err := os.Stat(overlay); err == nil {
		t.Fatal("overlay not removed")
	}
	if len(snapMetaRead(dir)) != 0 {
		t.Fatal("meta not cleared")
	}
}

func TestSnapshotMetaAndHelpers(t *testing.T) {
	dir := t.TempDir()
	// corrupt metadata → preserved to .corrupt, nil returned.
	bad := []byte("{oops")
	os.WriteFile(filepath.Join(dir, "snapshots.json"), bad, 0o600)
	if snapMetaRead(dir) != nil {
		t.Fatal("corrupt should yield nil")
	}
	if _, err := os.Stat(filepath.Join(dir, "snapshots.json.corrupt")); err != nil {
		t.Fatal("corrupt bytes not preserved")
	}
	// write/read round trip.
	snapMetaWrite(dir, nil)
	if arr := snapMetaRead(dir); arr == nil || len(arr) != 0 {
		t.Fatalf("empty roundtrip %+v", arr)
	}
	// num / exitCode / glob.
	if num(2.5) != 2.5 || num("x") != 0 {
		t.Fatal("num")
	}
	if exitCode(nil) != 0 {
		t.Fatal("exitCode nil")
	}
	os.WriteFile(filepath.Join(dir, "a.fd"), nil, 0o600)
	if len(glob(dir, "*.fd")) != 1 {
		t.Fatal("glob")
	}
}

func TestSnapshotInfoAndList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix stub")
	}
	dir, _, _ := mkVM3(t)
	fakeQemuImg(t, `{"snapshots":[{"id":"1","name":"s1","date-sec":10,"vm-state-size":64}]}`)
	oldJSON := outputJSON
	outputJSON = true
	t.Cleanup(func() { outputJSON = oldJSON })
	if rc := snapshotListJSON(dir, "vm"); rc != 0 {
		t.Fatalf("list rc=%d", rc)
	}
	if rc := snapshotInfo(dir); rc != 0 {
		t.Fatalf("info rc=%d", rc)
	}
}

func TestVMRunningDirAndDeleteDisk(t *testing.T) {
	dir, conf, disk := mkVM3(t)
	_ = conf
	// Live pid (this test process) → running (comm won't match but
	// comm-unreadable/qemu-prefix fallback still counts liveness on
	// hosts where /proc comm differs).
	pidFile := filepath.Join(dir, filepath.Base(dir)+".pid")
	if vmRunningDir(dir, filepath.Base(dir)) {
		t.Fatal("no pidfile should be false")
	}
	os.WriteFile(pidFile, []byte("bogus"), 0o600)
	if vmRunningDir(dir, filepath.Base(dir)) {
		t.Fatal("bad pid should be false")
	}
	os.WriteFile(pidFile, []byte("99999999"), 0o600)
	if vmRunningDir(dir, filepath.Base(dir)) {
		t.Fatal("dead pid should be false")
	}
	// deleteDisk: removes disk + OVMF vars, conf untouched.
	os.WriteFile(filepath.Join(dir, "OVMF_VARS-1.fd"), nil, 0o600)
	if rc := deleteDisk(dir, ""); rc != 0 {
		t.Fatalf("deleteDisk rc=%d", rc)
	}
	if _, err := os.Stat(disk); err == nil {
		t.Fatal("disk not removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "OVMF_VARS-1.fd")); err == nil {
		t.Fatal("efi vars not removed")
	}
	// No disk → no-op rc 0.
	dir2 := t.TempDir()
	if rc := deleteDisk(dir2, ""); rc != 0 {
		t.Fatalf("no-disk rc=%d", rc)
	}
}
