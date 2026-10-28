// guard_test.go — VM-running guards on disk-mutating ops. qemu-img
// resize/convert/commit against a disk QEMU is writing corrupts the
// image or silently swaps the file under the running guest; every op
// that reaches for qemu-img must refuse while the pidfile's process
// is alive (same VM_MUST_BE_STOPPED contract as delete).
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
)

// fakeRunningVM spawns a live process whose comm carries the "qemu"
// prefix vmRunningDir's identity check expects, then points the VM's
// pidfile at it — indistinguishable from a running guest.
func fakeRunningVM(t *testing.T, dir, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("posix only")
	}
	src, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep binary")
	}
	fake := filepath.Join(t.TempDir(), "qemu-fakevm")
	b, err := os.ReadFile(src)
	if err != nil || os.WriteFile(fake, b, 0o755) != nil {
		t.Skip("cannot copy sleep")
	}
	cmd := exec.Command(fake, "60")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot spawn fake qemu: " + err.Error())
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	if err := os.WriteFile(filepath.Join(dir, name+".pid"),
		[]byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
}

// markerQemuImg puts a qemu-img stub on PATH that records every
// invocation — a touch of the marker is proof the dangerous command
// executed — and, for `convert`, actually produces the output file so
// the compact swap completes exactly like the real tool.
func markerQemuImg(t *testing.T) (binDir, marker string) {
	t.Helper()
	binDir = t.TempDir()
	marker = filepath.Join(binDir, "called")
	os.WriteFile(filepath.Join(binDir, "qemu-img"), []byte(
		"#!/bin/sh\n"+
			"echo \"$@\" >> \""+marker+"\"\n"+
			"for last; do :; done\n"+
			"if [ \"$1\" = convert ]; then echo swapped > \"$last\"; fi\n"+
			"exit 0\n"), 0o755)
	return binDir, marker
}

// liveVM builds the dispatch fixture: <dir>/ with an absolute
// disk_img, a sibling <dir>.conf wired through QE_VMCONF, and a fake
// running guest — then returns env for runTool.
func liveVM(t *testing.T) (dir string, env []string) {
	t.Helper()
	dir = t.TempDir()
	name := filepath.Base(dir)
	conf := dir + ".conf"
	disk := filepath.Join(dir, "disk.qcow2")
	os.WriteFile(disk, []byte("img"), 0o600)
	os.WriteFile(conf, []byte("guest_os=\"linux\"\ndisk_img=\""+disk+"\"\n"), 0o644)
	fakeRunningVM(t, dir, name)
	return dir, []string{"QE_VMCONF=" + conf}
}

// qemu-img resize on a live disk grows the file while the guest is
// mid-write — the metadata update races the guest's own writes and
// the image ends up inconsistent. The op must refuse outright.
func TestResizeOnRunningVMIsRefused(t *testing.T) {
	dir, env := liveVM(t)
	bin, mark := markerQemuImg(t)
	env = append(env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, rc := runTool(t, env, "resize", dir, "64G")
	if rc == 0 {
		t.Fatalf("resize succeeded on a live VM (rc=%d): %s", rc, out)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatalf("qemu-img resize executed against a running VM's disk: %s", out)
	}
}

// compact is strictly worse than resize: convert + os.Rename replaces
// the disk file under the running guest — QEMU keeps writing to the
// unlinked inode and every post-rename write is lost. The stub makes
// the damage observable: a "swapped" disk means the live file was
// replaced mid-run.
func TestCompactOnRunningVMDestroysDisk(t *testing.T) {
	dir, env := liveVM(t)
	bin, mark := markerQemuImg(t)
	env = append(env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, rc := runTool(t, env, "compact", dir)
	disk := filepath.Join(dir, "disk.qcow2")
	b, _ := os.ReadFile(disk)
	if string(b) != "img" || rc == 0 {
		t.Fatalf("compact mutated a live VM disk (rc=%d content=%q): %s", rc, b, out)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatalf("qemu-img convert executed against a running VM's disk")
	}
}

// qemu-img commit merges an overlay into its backing file — with the
// guest writing to the active layer, committing a frozen view races
// the live writes and corrupts the chain.
func TestConsolidateOnRunningVMIsRefused(t *testing.T) {
	dir, env := liveVM(t)
	bin, mark := markerQemuImg(t)
	env = append(env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	overlay := filepath.Join(dir, "vm-s1.qcow2")
	os.WriteFile(overlay, []byte("ovl"), 0o600)
	snapMetaWrite(dir, []snapMeta{
		{Tag: "s1", File: overlay, Parent: filepath.Join(dir, "disk.qcow2")},
	})
	out, rc := runTool(t, env, "snapshot-consolidate", dir, "s1")
	if rc == 0 {
		t.Fatalf("consolidate succeeded on a live VM (rc=%d): %s", rc, out)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatalf("qemu-img commit executed against a running VM's chain")
	}
	// The overlay and its registry entry must survive a refused op.
	if _, err := os.Stat(overlay); err != nil {
		t.Fatal("overlay removed by a refused consolidate")
	}
	if m := snapMetaRead(dir); len(m) != 1 || m[0].Tag != "s1" {
		t.Fatalf("snapshot registry corrupted by refused op: %+v", m)
	}
}

// Renaming a live VM moves the pidfile along with the dir — under the
// new name liveness can no longer be proven, so the VM reports
// stopped while still running and the next disk-mutating op passes
// its guard. The rename itself must be refused.
func TestRenameOnRunningVMIsRefused(t *testing.T) {
	dir, env := liveVM(t)
	out, rc := runTool(t, env, "rename", dir, "vm-renamed")
	if rc == 0 {
		t.Fatalf("rename succeeded on a live VM (rc=%d): %s", rc, out)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("live VM dir moved by refused rename")
	}
}

// Move copies the live dir — pidfile and sockets included — yielding
// a duplicate that claims to be running while the source keeps
// mutating it. Refuse like the other state-mutating ops.
func TestMoveOnRunningVMIsRefused(t *testing.T) {
	dir, env := liveVM(t)
	dst := filepath.Join(t.TempDir(), "moved")
	out, rc := runTool(t, env, "move", dir, dst)
	if rc == 0 {
		t.Fatalf("move succeeded on a live VM (rc=%d): %s", rc, out)
	}
	if _, err := os.Stat(dst); err == nil {
		t.Fatal("live VM copied by refused move")
	}
}

// The XDG trash and the VM dir can live on different mounts —
// rename(2) fails EXDEV there and the delete aborts with the VM left
// behind. mv semantics demand a copy+remove fallback.
func TestMoveAnyCrossDeviceFallback(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("aa"), 0o600)
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("bb"), 0o600)
	dst := filepath.Join(t.TempDir(), "dst")

	old := fsRename
	fsRename = func(o, n string) error {
		return &os.LinkError{Op: "rename", Old: o, New: n, Err: syscall.EXDEV}
	}
	t.Cleanup(func() { fsRename = old })

	if err := moveAny(src, dst); err != nil {
		t.Fatalf("cross-device move failed: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source left behind after fallback move")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "sub", "b.txt")); string(b) != "bb" {
		t.Fatal("nested content lost in fallback move")
	}

	// A plain file moves the same way (conf travels beside the dir).
	f := filepath.Join(t.TempDir(), "vm.conf")
	os.WriteFile(f, []byte("conf"), 0o600)
	fd := filepath.Join(t.TempDir(), "vm.conf")
	if err := moveAny(f, fd); err != nil {
		t.Fatalf("file fallback: %v", err)
	}
	if b, _ := os.ReadFile(fd); string(b) != "conf" {
		t.Fatal("file content lost in fallback move")
	}
}
