// Package qespec is the project test harness: the former ShellSpec
// suite ported to Go. Helpers here replace spec/spec_helper.sh,
// spec/e2e/support/e2e_lib.sh and the spec/fake-bin stubs — a single
// multicall binary (fakebin/) provides qemu-img/qemu-system/socat/
// xdg-open doubles dispatched on argv[0].
//
// Tests execute the real repo binaries: bin/<entry> and
// tools/<name>/<name>, self-bootstrapped by TestMain via `go build`.
package qespec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// ---------- repo paths ----------

var rootOnce sync.Once
var rootDir string

// Root returns the repo root (walks up to the dir holding go.work).
func Root() string {
	rootOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			dir = "."
		}
		for {
			if st, err := os.Stat(filepath.Join(dir, "go.work")); err == nil && !st.IsDir() {
				rootDir = dir
				return
			}
			up := filepath.Dir(dir)
			if up == dir {
				rootDir, _ = os.Getwd()
				return
			}
			dir = up
		}
	})
	return rootDir
}

// Bin resolves a repo binary: public entries live in bin/<name>,
// internal tools in tools/<name>/<name>.
func Bin(t *testing.T, name string) string {
	t.Helper()
	for _, p := range []string{
		filepath.Join(Root(), "bin", name),
		filepath.Join(Root(), "tools", name, name),
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	t.Fatalf("binary %q not built — run `go run ./tools/dev build`", name)
	return ""
}

// ---------- running tools ----------

// Result captures one tool invocation.
type Result struct {
	Stdout string
	Stderr string
	Code   int
}

func (r Result) OK() bool         { return r.Code == 0 }
func (r Result) Failed() bool     { return r.Code != 0 }
func (r Result) Out() string      { return r.Stdout }
func (r Result) Combined() string { return r.Stdout + r.Stderr }

// Contains asserts substring presence in stdout.
func (r Result) Contains(t *testing.T, subs ...string) Result {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(r.Stdout, s) {
			t.Errorf("stdout missing %q\ngot: %.400s", s, r.Stdout)
		}
	}
	return r
}

// ErrContains asserts substring presence in stderr.
func (r Result) ErrContains(t *testing.T, subs ...string) Result {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(r.Stderr, s) {
			t.Errorf("stderr missing %q\ngot: %.400s", s, r.Stderr)
		}
	}
	return r
}

// JSON decodes stdout (or the first {...} line when a human preamble
// precedes the payload) as a JSON value.
func (r Result) JSON(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(r.Stdout), &m); err == nil {
		return m
	}
	for _, l := range strings.Split(r.Stdout, "\n") {
		if strings.HasPrefix(l, "{") && json.Unmarshal([]byte(l), &m) == nil {
			return m
		}
	}
	t.Fatalf("stdout is not JSON: %.400s", r.Stdout)
	return nil
}

// Envelope asserts the v1 envelope keys and returns the parsed object.
func (r Result) Envelope(t *testing.T) map[string]any {
	t.Helper()
	m := r.JSON(t)
	for _, k := range []string{"schemaVersion", "command", "success", "data", "warnings", "errors", "meta"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("envelope missing key %q in %.300s", k, r.Stdout)
		}
	}
	if v, _ := m["schemaVersion"].(float64); v != 1 {
		t.Fatalf("schemaVersion %v != 1", m["schemaVersion"])
	}
	return m
}

type runCfg struct {
	dir   string
	env   []string
	stdin string
	tmo   time.Duration
}

// RunOpt tweaks one invocation.
type RunOpt func(*runCfg)

func InDir(d string) RunOpt          { return func(c *runCfg) { c.dir = d } }
func Env(kv ...string) RunOpt        { return func(c *runCfg) { c.env = kv } }
func Stdin(s string) RunOpt          { return func(c *runCfg) { c.stdin = s } }
func Timeout(d time.Duration) RunOpt { return func(c *runCfg) { c.tmo = d } }

// Run executes bin-or-tool `name` and captures the result.
func Run(t *testing.T, name string, opts []RunOpt, args ...string) Result {
	t.Helper()
	cfg := runCfg{tmo: 60 * time.Second}
	for _, o := range opts {
		o(&cfg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.tmo)
	defer cancel()
	cmd := exec.CommandContext(ctx, Bin(t, name), args...)
	if cfg.dir != "" {
		cmd.Dir = cfg.dir
	}
	if cfg.env != nil {
		cmd.Env = append(os.Environ(), cfg.env...)
	}
	if cfg.stdin != "" {
		cmd.Stdin = strings.NewReader(cfg.stdin)
	}
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	rc := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		} else if ctx.Err() != nil {
			t.Fatalf("%s %v: timed out after %s", name, args, cfg.tmo)
		} else {
			t.Fatalf("%s %v: %v", name, args, err)
		}
	}
	return Result{Stdout: so.String(), Stderr: se.String(), Code: rc}
}

// RunArgv executes an arbitrary binary path (for isolated copies).
func RunArgv(t *testing.T, bin string, opts []RunOpt, args ...string) Result {
	t.Helper()
	cfg := runCfg{tmo: 60 * time.Second}
	for _, o := range opts {
		o(&cfg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.tmo)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	if cfg.dir != "" {
		cmd.Dir = cfg.dir
	}
	if cfg.env != nil {
		cmd.Env = append(os.Environ(), cfg.env...)
	}
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	} else if err != nil && ctx.Err() != nil {
		t.Fatalf("%s: timed out", bin)
	} else if err != nil {
		t.Fatalf("%s: %v", bin, err)
	}
	return Result{Stdout: so.String(), Stderr: se.String(), Code: rc}
}

// ---------- fixtures ----------

// Write creates dir/name with the given contents.
func Write(t *testing.T, dir, name, contents string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// VMConf writes a minimal guest conf and returns its path.
func VMConf(t *testing.T, dir, extra string) string {
	t.Helper()
	return Write(t, dir, "vm.conf",
		"guest_os=\"linux\"\ndisk_img=\"disk.qcow2\"\niso=\"stub.iso\"\nram=\"256M\"\ndisplay=\"none\"\nboot=\"legacy\"\n"+extra)
}

// VMFixture builds the e2e_lib vm_fixture equivalent: conf + disk +
// stub iso under dir; returns the conf path. Uses real qemu-img when
// present, else writes a stand-in (non-boot tests only).
func VMFixture(t *testing.T, dir string) string {
	t.Helper()
	conf := VMConf(t, dir, "")
	disk := filepath.Join(dir, "disk.qcow2")
	if img, err := exec.LookPath("qemu-img"); err == nil {
		if out, err := exec.Command(img, "create", "-f", "qcow2", disk, "64M").CombinedOutput(); err != nil {
			t.Fatalf("qemu-img create: %v\n%s", err, out)
		}
	} else if err := os.WriteFile(disk, []byte{0}, 0o644); err != nil {
		t.Fatalf("fixture disk: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stub.iso"), []byte("x"), 0o644); err != nil {
		t.Fatalf("fixture iso: %v", err)
	}
	return conf
}

// HTTPServer serves dir's files over a random localhost port; closed
// at test cleanup. Replaces `python3 -m http.server` + fixed ports.
func HTTPServer(t *testing.T, dir string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)
	return srv
}

// WaitFor polls cond until true or timeout; fails the test with msg.
func WaitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("%s (after %s)", msg, timeout)
}

// SocketExists reports whether path is a unix socket.
func SocketExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode()&os.ModeSocket != 0
}

// PidAlive wraps qelib-style liveness (EPERM = alive).
func PidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// procByToken finds a live process whose /proc/<pid>/cmdline contains
// token — used by e2e cleanups to locate a QEMU whose pidfile is gone
// (it always carries `-name <vm>,process=<vm>` in argv).
func procByToken(t *testing.T, token string) int {
	t.Helper()
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if strings.Contains(string(b), token) {
			return pid
		}
	}
	return 0
}

// VMState runs `quickemu --vm conf --status` and returns the first word.
func VMState(t *testing.T, conf string) string {
	t.Helper()
	r := Run(t, "quickemu", nil, "--vm", conf, "--status")
	return strings.Fields(r.Stdout + " ")[0]
}

// SpawnFakeVM starts a fake QEMU process whose /proc comm matches the
// VM name (sleep copied under the name), writes dir/<name>.pid.
func SpawnFakeVM(t *testing.T, dir, name string) int {
	t.Helper()
	bin := filepath.Join(dir, name)
	src, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Skip("no /bin/sleep for fake VM")
	}
	if err := os.WriteFile(bin, src, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, name+".pid"), []byte(fmt.Sprint(pid)), 0o644)
	t.Cleanup(func() { cmd.Process.Kill() })
	return pid
}

// SourceFile reads a repo file — the former grep-based audit specs.
func SourceFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(Root(), rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// ---------- fakebin ----------

var fakeOnce sync.Once
var fakeDir string
var fakeErr error

// FakeBinDir compiles tools/qespec/fakebin once and returns a dir with
// it linked under every stub name.
func FakeBinDir(t *testing.T) string {
	t.Helper()
	fakeOnce.Do(func() {
		fakeDir, fakeErr = os.MkdirTemp("", "qe-fakebin-")
		if fakeErr != nil {
			return
		}
		out := filepath.Join(fakeDir, "fakebin")
		cmd := exec.Command("go", "build", "-o", out, "./fakebin")
		cmd.Dir = filepath.Join(Root(), "tools", "qespec")
		if b, err := cmd.CombinedOutput(); err != nil {
			fakeErr = fmt.Errorf("build fakebin: %v\n%s", err, b)
			return
		}
		for _, n := range []string{"qemu-img", "qemu-system-x86_64", "socat", "xdg-open", "quickemu"} {
			dst := filepath.Join(fakeDir, n)
			if err := os.Link(out, dst); err != nil {
				if b2, err2 := os.ReadFile(out); err2 == nil {
					os.WriteFile(dst, b2, 0o755)
				}
			}
		}
	})
	if fakeErr != nil {
		t.Fatal(fakeErr)
	}
	return fakeDir
}

// WithFakeBin prepends the fake-bin dir to PATH for the test.
func WithFakeBin(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", FakeBinDir(t)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// ---------- environment probes ----------

func QemuAvailable() bool {
	_, err := exec.LookPath("qemu-system-x86_64")
	return err == nil
}

func QemuImgAvailable() bool {
	_, err := exec.LookPath("qemu-img")
	return err == nil
}

// UnixDial opens a unix socket (replaces socat in QMP tests).
func UnixDial(t *testing.T, sock string, payload string, readFor time.Duration) string {
	t.Helper()
	c, err := net.DialTimeout("unix", sock, 3*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", sock, err)
	}
	defer c.Close()
	if _, err := c.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	done := time.Now().Add(readFor)
	for time.Now().Before(done) {
		c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		chunk := make([]byte, 4096)
		n, err := c.Read(chunk)
		buf.Write(chunk[:n])
		if err != nil {
			break
		}
	}
	return buf.String()
}
