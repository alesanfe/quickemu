// flow_test.go — the pipeline paths that need siblings and SSH.
// Fake quickemu-*/quickget binaries are dropped next to the test
// executable (qelib.FindTool resolves siblings first), and a
// throwaway in-process SSH server stands in for the guest so
// waitSSH/provision/verify/shutdown/run() execute end-to-end.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// fakeTool installs an executable script named <name> next to the
// test binary so qelib.FindTool picks it as the sibling.
func fakeTool(t *testing.T, name, script string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no test exe")
	}
	p := filepath.Join(filepath.Dir(exe), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(p) })
}

func TestToolOutRun(t *testing.T) {
	fakeTool(t, "fakesib", `echo "line1"
echo "line2"
echo -n "tail-no-newline"`)
	out, err := toolOut("fakesib", "x")
	if err != nil || !strings.Contains(out, "line1") {
		t.Fatalf("toolOut %q %v", out, err)
	}
	var lines []string
	if err := toolRun(t.TempDir(), "fakesib", func(l string) {
		lines = append(lines, l)
	}); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || lines[0] != "line1" || lines[2] != "tail-no-newline" {
		t.Fatalf("streamed %v", lines)
	}
	// nil callback also drains.
	if err := toolRun(t.TempDir(), "fakesib", nil); err != nil {
		t.Fatal(err)
	}
	// missing sibling → FindTool error propagates.
	if _, err := toolOut("no-such-tool-zzz"); err == nil {
		t.Fatal("missing tool must fail")
	}
	if err := toolRun(t.TempDir(), "no-such-tool-zzz", nil); err == nil {
		t.Fatal("missing tool must fail")
	}
	// non-zero exit propagates.
	fakeTool(t, "failsib", "exit 3")
	if _, err := toolOut("failsib"); err == nil {
		t.Fatal("rc!=0 must propagate")
	}
}

// fakeOps is a canned quickemu-ops: new prints an id, status prints a
// journal record, everything else exits 0.
func fakeOps(t *testing.T) {
	t.Helper()
	fakeTool(t, "quickemu-ops", `case "$1" in
new) echo "op-fake-1" ;;
status) echo '{"id":"op-fake-1","state":"running","progress":40}' ;;
*) exit 0 ;;
esac`)
}

func TestOpsBridge(t *testing.T) {
	fakeOps(t)
	dir := t.TempDir()
	id, err := opNew(dir, "vm")
	if err != nil || id != "op-fake-1" {
		t.Fatalf("opNew %q %v", id, err)
	}
	opSet(dir, id, "running", 40)
	opDone(dir, id)
	opFail(dir, id, "E", "m")
	// failing sibling: opNew surfaces the error.
	fakeTool(t, "quickemu-ops", "exit 9")
	if _, err := opNew(dir, "vm"); err == nil {
		t.Fatal("ops failure must propagate")
	}
}

func TestCmdStatusCancel(t *testing.T) {
	fakeOps(t)
	dir := t.TempDir()
	if rc := cmdStatus([]string{"--vmdir", dir, "--id", "op-fake-1"}); rc != 0 {
		t.Fatalf("status rc=%d", rc)
	}
	if rc := cmdCancel([]string{"--vmdir", dir, "--id", "op-fake-1"}); rc != 0 {
		t.Fatalf("cancel rc=%d", rc)
	}
}

// dispatch exercises every step id against a fully-staged runCtx —
// everything that can complete offline completes, the VM-dependent
// ones take their error branches (closed ssh port, missing qemu).
func TestDispatchSteps(t *testing.T) {
	fakeOps(t)
	// Never let dispatch reach a real quickemu on PATH.
	fakeTool(t, "quickemu", "exit 1")
	r, dir := testRunCtx(t)
	r.artDir = filepath.Join(dir, "art")
	os.MkdirAll(r.artDir, 0o755)
	r.e = newEmitter(r.runID, r.vmName, r.vmDir, r.artDir, true)
	defer r.e.close()
	ctx := context.Background()

	steps := map[string]bool{ // id → expect success offline
		"resolve": true, "acquire-image": true, "prepare": true,
		"await-install": true, "reconfigure": true, "provision": false,
		"verify": true, "shutdown": true, "seal": true,
		"launch": false, "launch-install": false,
	}
	for id, ok := range steps {
		err := r.dispatch(ctx, id)
		if ok && err != nil {
			t.Fatalf("step %s: %v", id, err)
		}
		if !ok && err == nil {
			t.Fatalf("step %s should fail without a guest", id)
		}
	}
	// wait-ssh honours ctx cancellation (a live ctx would poll for the
	// whole ssh_timeout window).
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := r.dispatch(cctx, "wait-ssh"); err == nil {
		t.Fatal("wait-ssh must fail on cancelled ctx")
	}
	// verify with declared checks → sshDial fails without a guest.
	r.p.Verify = append(r.p.Verify, struct {
		Name   string `toml:"name"`
		Run    string `toml:"run"`
		Expect string `toml:"expect"`
	}{Name: "v", Run: "true"})
	if err := r.dispatch(ctx, "verify"); err == nil {
		t.Fatal("verify must fail without ssh")
	}
}

// acquire with no conf runs quickget — the fake sibling drops a conf
// in workdir so findVMConf resolves it.
func TestAcquireDownload(t *testing.T) {
	fakeOps(t)
	r, dir := testRunCtx(t)
	r.artDir = filepath.Join(dir, "art")
	os.MkdirAll(r.artDir, 0o755)
	r.e = newEmitter(r.runID, r.vmName, r.vmDir, r.artDir, true)
	defer r.e.close()
	workdir := t.TempDir()
	r.workdir = workdir
	r.confPath = ""
	r.vmDir = ""
	conf := filepath.Join(workdir, "cirros-0.6.conf")
	t.Setenv("QE_FAKE_CONF", conf)
	fakeTool(t, "quickget", `echo "Downloading 50%"
echo "100%"
cat > "$QE_FAKE_CONF" <<'EOF'
guest_os="linux"
disk_img="disk.qcow2"
EOF`)
	r.p.Machine.DiskSize = "10G"
	if err := r.acquire(); err != nil {
		t.Fatal(err)
	}
	if r.confPath != conf || r.vmName != "cirros-0.6" {
		t.Fatalf("acquire %s %s", r.confPath, r.vmName)
	}
	if confGet(conf, "disk_size") != "10G" {
		t.Fatal("disk_size override not applied")
	}
	// quickget failure → error.
	t.Setenv("QE_FAKE_CONF", "")
	fakeTool(t, "quickget", "exit 1")
	r2, _ := testRunCtx(t)
	r2.workdir = t.TempDir()
	r2.confPath = ""
	r2.e = newEmitter("r2", "", "", "", true)
	if err := r2.acquire(); err == nil || !strings.Contains(err.Error(), "quickget") {
		t.Fatalf("acquire %v", err)
	}
}

func TestLaunch(t *testing.T) {
	fakeOps(t)
	r, dir := testRunCtx(t)
	r.artDir = filepath.Join(dir, "art")
	os.MkdirAll(r.artDir, 0o755)
	r.e = newEmitter(r.runID, r.vmName, r.vmDir, r.artDir, true)
	defer r.e.close()
	// Fake quickemu "starts" the VM: it just drops the pidfile.
	pidf := filepath.Join(dir, "vm.pid")
	t.Setenv("QE_FAKE_PIDFILE", pidf)
	fakeTool(t, "quickemu", `echo 99999999 > "$QE_FAKE_PIDFILE"`)
	if err := r.launch(context.Background()); err != nil {
		t.Fatal(err)
	}
	// quickemu itself fails → wrapped error.
	fakeTool(t, "quickemu", "exit 1")
	if err := r.launch(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "quickemu --vm") {
		t.Fatalf("launch %v", err)
	}
}

// --- in-process SSH guest ------------------------------------------------

// sshTestServer is a minimal sshd: accepts any public key, answers
// "session"+"exec" requests through handler(cmd)→(stdout,rc) and
// drains stdin so `tee`-style writes complete.
type sshTestServer struct {
	l      net.Listener
	signer ssh.Signer
}

func startSSHServer(t *testing.T, handler func(string) (string, int)) *sshTestServer {
	t.Helper()
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(sk)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(signer)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &sshTestServer{l: l, signer: signer}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go s.serve(cfg, conn, handler)
		}
	}()
	t.Cleanup(func() { l.Close() })
	return s
}

func (s *sshTestServer) serve(cfg *ssh.ServerConfig, conn net.Conn,
	handler func(string) (string, int)) {
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)
	for ch := range chans {
		if ch.ChannelType() != "session" {
			ch.Reject(ssh.UnknownChannelType, "only session")
			continue
		}
		c, rqs, err := ch.Accept()
		if err != nil {
			continue
		}
		go func(c ssh.Channel, rqs <-chan *ssh.Request) {
			defer c.Close()
			for rq := range rqs {
				switch rq.Type {
				case "exec":
					var p struct{ Value string }
					ssh.Unmarshal(rq.Payload, &p)
					rq.Reply(true, nil)
					// The client always ends stdin with a channel
					// eof (empty reader included) — drain it fully
					// before replying or Run reports a mid-write EOF.
					io.Copy(io.Discard, c)
					out, rc := handler(p.Value)
					fmt.Fprint(c, out)
					c.SendRequest("exit-status", false,
						ssh.Marshal(struct{ Status uint32 }{uint32(rc)}))
					return
				case "pty-req", "shell":
					rq.Reply(rq.Type == "shell", nil)
				default:
					rq.Reply(false, nil)
				}
			}
		}(c, rqs)
	}
}

func (s *sshTestServer) port() int {
	return s.l.Addr().(*net.TCPAddr).Port
}

func TestSSHExecWrite(t *testing.T) {
	srv := startSSHServer(t, func(cmd string) (string, int) {
		if strings.Contains(cmd, "fails") {
			return "bad\n", 4
		}
		return "ok:" + cmd + "\n", 0
	})
	k, _ := genKeypair("")
	c, err := sshDial(srv.port(), "u", k.signer)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	out, rc, err := sshExec(c, "echo hi")
	if err != nil || rc != 0 || !strings.Contains(out, "ok:") {
		t.Fatalf("exec %q %d %v", out, rc, err)
	}
	_, rc, err = sshExec(c, "it fails")
	if err != nil || rc != 4 {
		t.Fatalf("rc %d %v", rc, err)
	}
	if err := sshWrite(c, "/tmp/f", "0644", []byte("data")); err != nil {
		t.Fatal(err)
	}
}

func TestWaitSSHSuccess(t *testing.T) {
	srv := startSSHServer(t, func(string) (string, int) { return "", 0 })
	k, _ := genKeypair("")
	e := newEmitter("r", "", "", "", true)
	if err := waitSSH(context.Background(), e, srv.port(), "u", k,
		5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestDetectPkgMgr(t *testing.T) {
	srv := startSSHServer(t, func(cmd string) (string, int) {
		if strings.Contains(cmd, "apk") {
			return "", 0
		}
		return "", 1
	})
	k, _ := genKeypair("")
	c, err := sshDial(srv.port(), "u", k.signer)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if pm := detectPkgMgr(c); pm != "apk" {
		t.Fatalf("pm %q", pm)
	}
}

func TestProvisionVerifyShutdown(t *testing.T) {
	var ran []string
	srv := startSSHServer(t, func(cmd string) (string, int) {
		ran = append(ran, cmd)
		switch {
		case strings.Contains(cmd, "it fails"):
			return "bad\n", 4
		case strings.Contains(cmd, "apt-get"), strings.Contains(cmd, "apk"),
			strings.Contains(cmd, "dnf"), strings.Contains(cmd, "pacman"):
			if strings.Contains(cmd, "command -v") {
				if strings.Contains(cmd, "apk") {
					return "", 0 // apk present
				}
				return "", 1
			}
			return "installed\n", 0
		case strings.Contains(cmd, "command -v rc-service"):
			return "", 1 // systemd guest — no OpenRC
		case strings.Contains(cmd, "command -v sudo"):
			return "", 0 // sudo present
		case strings.Contains(cmd, "cloud-init"):
			return "", 0
		case strings.Contains(cmd, "checkme"):
			return "expected-output\n", 0
		case strings.Contains(cmd, "poweroff"), strings.Contains(cmd, "halt"):
			return "", 0
		}
		return "", 0
	})
	r, dir := testRunCtx(t)
	r.artDir = filepath.Join(dir, "art")
	os.MkdirAll(r.artDir, 0o755)
	r.e = newEmitter(r.runID, r.vmName, r.vmDir, r.artDir, true)
	defer r.e.close()
	r.sshPort = srv.port()
	r.p.Provision.Packages = []string{"htop"}
	r.p.Provision.Services = []string{"svc1"}
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("payload"), 0o600)
	r.p.Provision.Files = append(r.p.Provision.Files, struct {
		Source      string `toml:"source"`
		Destination string `toml:"destination"`
		Mode        string `toml:"mode"`
		Owner       string `toml:"owner"`
	}{Source: src, Destination: "/etc/dst.txt", Mode: "0600", Owner: "root:root"})
	r.p.Provision.Commands = append(r.p.Provision.Commands, struct {
		Name string `toml:"name"`
		Run  string `toml:"run"`
		As   string `toml:"as"`
	}{Name: "c1", Run: "true"}, struct {
		Name string `toml:"name"`
		Run  string `toml:"run"`
		As   string `toml:"as"`
	}{Name: "c2", Run: "id", As: "nobody"})
	if err := r.provision(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(ran, "|")
	for _, want := range []string{"apk add", "systemctl enable --now 'svc1'",
		"tee '/etc/dst.txt'", "chown 'root:root'", "sudo -u 'nobody'"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("guest never saw %q in %s", want, joined)
		}
	}
	// a failing command aborts the run.
	r.p.Provision.Commands = append(r.p.Provision.Commands, struct {
		Name string `toml:"name"`
		Run  string `toml:"run"`
		As   string `toml:"as"`
	}{Name: "bad", Run: "it fails"})
	if err := r.provision(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "bad") {
		t.Fatalf("command failure %v", err)
	}

	// verify: pass on expect-substring, fail collects names.
	r2, dir2 := testRunCtx(t)
	r2.artDir = filepath.Join(dir2, "art")
	os.MkdirAll(r2.artDir, 0o755)
	r2.e = newEmitter("r2", r2.vmName, r2.vmDir, r2.artDir, true)
	defer r2.e.close()
	r2.sshPort = srv.port()
	r2.p.Verify = append(r2.p.Verify, struct {
		Name   string `toml:"name"`
		Run    string `toml:"run"`
		Expect string `toml:"expect"`
	}{Name: "ok", Run: "checkme", Expect: "expected"},
		struct {
			Name   string `toml:"name"`
			Run    string `toml:"run"`
			Expect string `toml:"expect"`
		}{Name: "bad", Run: "it fails"})
	if err := r2.verify(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "bad") {
		t.Fatalf("verify %v", err)
	}
	r2.p.Verify = r2.p.Verify[:1]
	if err := r2.verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	// empty verify → instant nil.
	r2.p.Verify = nil
	if err := r2.verify(context.Background()); err != nil {
		t.Fatal(err)
	}

	// shutdown over live ssh + dead pidfile → completes.
	r.sshPort = srv.port()
	if err := r.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Alpine guests carry OpenRC, not systemd — services must go through
// rc-update/rc-service, detected via `command -v rc-service`.
func TestProvisionOpenRC(t *testing.T) {
	var ran []string
	srv := startSSHServer(t, func(cmd string) (string, int) {
		ran = append(ran, cmd)
		switch {
		case strings.Contains(cmd, "command -v rc-service"):
			return "", 0 // OpenRC guest
		case strings.Contains(cmd, "command -v sudo"):
			return "", 0
		case strings.Contains(cmd, "command -v apk"):
			return "", 0 // apk = the detected package manager
		}
		return "", 1
	})
	r, dir := testRunCtx(t)
	r.artDir = filepath.Join(dir, "art")
	os.MkdirAll(r.artDir, 0o755)
	r.e = newEmitter(r.runID, r.vmName, r.vmDir, r.artDir, true)
	defer r.e.close()
	r.sshPort = srv.port()
	r.p.Provision.Services = []string{"qemu-guest-agent"}
	if err := r.provision(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(ran, "|")
	for _, want := range []string{"rc-update add 'qemu-guest-agent' default",
		"rc-service 'qemu-guest-agent' start"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("guest never saw %q in %s", want, joined)
		}
	}
	if strings.Contains(joined, "systemctl") {
		t.Fatalf("OpenRC guest must not see systemctl: %s", joined)
	}
}

// A guest where sudo never lands (cloud-init skipped the package
// stage) still proceeds — the wait warns instead of hanging forever.
func TestProvisionSudoAbsent(t *testing.T) {
	old := sudoPollInterval
	sudoPollInterval = time.Millisecond
	t.Cleanup(func() { sudoPollInterval = old })
	srv := startSSHServer(t, func(cmd string) (string, int) {
		if strings.Contains(cmd, "command -v") {
			return "", 1 // nothing installed — no sudo, no PM, no rc-service
		}
		return "", 0
	})
	r, dir := testRunCtx(t)
	r.artDir = filepath.Join(dir, "art")
	os.MkdirAll(r.artDir, 0o755)
	r.e = newEmitter(r.runID, r.vmName, r.vmDir, r.artDir, true)
	defer r.e.close()
	r.sshPort = srv.port()
	r.p.Provision.Packages = []string{"htop"}
	start := time.Now()
	if err := r.provision(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("sudo wait did not honour the shrunken interval")
	}
}

// --- run() end-to-end ----------------------------------------------------

// stagedWorkdir returns a workdir holding a reusable VM conf so
// run()'s acquire takes the fast path.
func stagedWorkdir(t *testing.T) (workdir, vmDir string) {
	t.Helper()
	workdir = t.TempDir()
	vmDir = filepath.Join(workdir, "fakeos-1")
	os.MkdirAll(vmDir, 0o755)
	os.WriteFile(filepath.Join(workdir, "fakeos-1.conf"),
		[]byte("guest_os=\"linux\"\ndisk_img=\"fakeos-1/disk.qcow2\"\n"), 0o600)
	return workdir, vmDir
}

func runProfile(t *testing.T, sshPort int, extra string) *Profile {
	p, err := loadProfile(writeProfile(t, fmt.Sprintf(`
[system]
os = "fakeos"
release = "1"
[machine]
ssh_port = %d
[automation]
provider = "nocloud-image"
ssh_timeout_min = 1
%s`, sshPort, extra)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunEndToEnd(t *testing.T) {
	fakeOps(t)
	workdir, vmDir := stagedWorkdir(t)
	// Fake quickemu "boots" the VM: drop the pidfile launch() polls for.
	t.Setenv("QE_FAKE_PIDFILE", filepath.Join(vmDir, "fakeos-1.pid"))
	fakeTool(t, "quickemu", `echo 99999999 > "$QE_FAKE_PIDFILE"`)
	srv := startSSHServer(t, func(cmd string) (string, int) {
		return "ok\n", 0
	})
	p := runProfile(t, srv.port(), "")
	if rc := run(p, t.TempDir(), workdir, false); rc != 0 {
		t.Fatalf("run rc=%d", rc)
	}
}

func TestRunCancelAtBoundary(t *testing.T) {
	fakeOps(t)
	workdir, vmDir := stagedWorkdir(t)
	srv := startSSHServer(t, func(string) (string, int) { return "", 0 })
	p := runProfile(t, srv.port(), "")
	// Pre-arm the cancel: ops new → op-fake-1; the journal says
	// cancelRequested so the second step boundary aborts.
	ops := filepath.Join(vmDir, "ops")
	os.MkdirAll(ops, 0o755)
	os.WriteFile(filepath.Join(ops, "op-fake-1.json"),
		[]byte(`{"state":"cancelRequested"}`), 0o600)
	if rc := run(p, t.TempDir(), workdir, true); rc != 1 {
		t.Fatalf("cancelled run rc=%d", rc)
	}
}
