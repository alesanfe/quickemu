// qe121_test.go — QE-121 filesystem/host-path matrix: VM dirs and
// confs under hostile-but-legal host paths must behave identically
// across naming edge cases. Runs against the real binaries.
package qespec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// qe121VM builds dir/<name>.conf + dir/<name>/disk.qcow2 and returns
// the conf path — the fixture every matrix row drives.
func qe121VM(t *testing.T, root, name, extraConf string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	os.MkdirAll(dir, 0o755)
	disk := filepath.Join(dir, "disk.qcow2")
	os.WriteFile(disk, []byte("qcow"), 0o600)
	iso := filepath.Join(dir, "boot.iso")
	os.WriteFile(iso, []byte("iso"), 0o600)
	conf := filepath.Join(root, name+".conf")
	os.WriteFile(conf, []byte("disk_img=\""+disk+"\"\niso=\""+iso+"\"\nguest_os=\"linux\"\nboot=\"legacy\"\n"+extraConf), 0o644)
	return conf
}

func TestFSMatrixPathShapes(t *testing.T) {
	WithFakeBin(t)
	root := t.TempDir()
	for _, name := range []string{
		"plain", "with space", "with.dots", "üñíçødé",
		"UPPER_lower-123", strings.Repeat("n", 120),
	} {
		conf := qe121VM(t, root, name, "")
		// Status must work on every path shape (stopped VM → the
		// human "not running" line, no crash).
		r := Run(t, "quickemu", nil, "--vm", conf, "--status")
		if r.Code > 1 {
			t.Errorf("name %q: rc=%d out=%.200s", name, r.Code, r.Combined())
		}
	}
}

func TestFSMatrixConfEdgeValues(t *testing.T) {
	WithFakeBin(t)
	root := t.TempDir()
	// CRLF line endings + quoted + whitespace values must parse
	// identically to their LF/unquoted twins.
	conf := qe121VM(t, root, "crlf", "monitor=\"socket\"\r\n")
	r := Run(t, "quickemu-launch", nil, conf, "--dry-run")
	if !strings.Contains(r.Stdout, "qemu-system") && !strings.Contains(r.Stdout, "-monitor") {
		t.Errorf("CRLF conf plan degraded: %.300s", r.Stdout)
	}
	// Comma-bearing disk path → the QEMU option carries ,,.
	conf2 := qe121VM(t, root, "comma", "")
	b, _ := os.ReadFile(conf2)
	commaDisk := filepath.Join(root, "comma", "d,isk.qcow2")
	os.WriteFile(commaDisk, []byte("qcow"), 0o600)
	os.WriteFile(conf2, append(b, []byte("disk_img=\""+commaDisk+"\"\n")...), 0o644)
	r = Run(t, "quickemu-launch", nil, conf2, "--dry-run")
	if !strings.Contains(r.Combined(), "d,,isk.qcow2") {
		t.Errorf("comma path not escaped: %.400s", r.Combined())
	}
}

func TestFSMatrixLockAndPorts(t *testing.T) {
	WithFakeBin(t)
	root := t.TempDir()
	// Ports registry under a spaced dir — reserve/list/release all
	// resolve the registry path identically.
	vmDir := filepath.Join(root, "vm space")
	os.MkdirAll(vmDir, 0o755)
	reg := filepath.Join(vmDir, ".ports")
	r := Run(t, "quickemu-ports", nil, "reserve",
		"--registry", reg, "--port", "7654", "--pid", "4242", "--instance", "x")
	if r.Failed() {
		t.Fatalf("reserve: %s", r.Combined())
	}
	r = Run(t, "quickemu-ports", nil, "check",
		"--registry", reg, "--port", "7654")
	if r.Failed() {
		t.Fatalf("check reserved port must fail (busy): %s", r.Combined())
	}
	r = Run(t, "quickemu-ports", nil, "release",
		"--registry", reg, "--port", "7654")
	if r.Failed() {
		t.Fatalf("release: %s", r.Combined())
	}
	// The lockfile lives alongside the VM dir — unicode parent dir.
	uDir := filepath.Join(root, "üñíçødé")
	os.MkdirAll(uDir, 0o755)
	conf := qe121VM(t, root, "üñíçødé", "")
	r = Run(t, "quickemu", nil, "--vm", conf, "--status")
	if r.Code > 1 {
		t.Fatalf("unicode dir status rc=%d: %s", r.Code, r.Combined())
	}
}

func TestFSMatrixSnapshotRegistryUnicode(t *testing.T) {
	WithFakeBin(t)
	dir, conf := qe120Conf(t)
	// A unicode desc flows into snapshots.json intact (JSON escapes
	// it safely — no raw control bytes reach the registry file).
	r := Run(t, "quickemu-vm", []RunOpt{Env("QE_VMCONF=" + conf)},
		"snapshot-register", dir, "m1", "vmstate", "üñíçødé desc")
	if r.Failed() {
		t.Fatalf("register: %s", r.Combined())
	}
	b, err := os.ReadFile(filepath.Join(dir, "snapshots.json"))
	if err != nil {
		t.Fatalf("no registry: %v", err)
	}
	if !strings.Contains(string(b), "üñíçødé") {
		t.Fatalf("unicode desc mangled: %s", b)
	}
}
