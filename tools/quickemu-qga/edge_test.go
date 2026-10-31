// edge_test.go: the residual error branches — dial classification
// tails, typed-command error/decode paths, exec edge cases and the
// push/pull failure matrix that a hostile or broken agent can raise.
package qga

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesanfe/quickemu/tools/quickemu-qga/qgatest"
)

// The socket file outlives its binder: Dial reaches the connect
// stage and fails there — unreachable, not missing. A unixgram
// socket is used because Go stream listeners unlink on Close.
func TestDialConnRefused(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "dead.sock")
	c, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: sock, Net: "unixgram"})
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	c.Close() // datagram socket file stays on disk, refuses connects
	_, err = Dial(sock, 300*time.Millisecond)
	if !errors.Is(err, ErrAgentUnreachable) {
		t.Fatalf("want ErrAgentUnreachable, got %v", err)
	}
}

// The far end answers the connect then hangs up before replying to
// the sync request — the handshake read hits EOF, classified as
// unreachable (a broken channel, not a silent agent).
func TestDialHangupDuringSync(t *testing.T) {
	sock := rawServer(t, func(c net.Conn) {
		buf := make([]byte, 4096)
		c.SetDeadline(time.Now().Add(2 * time.Second))
		c.Read(buf) // consume the sync request, then drop the conn
	})
	_, err := Dial(sock, 2*time.Second)
	if !errors.Is(err, ErrAgentUnreachable) {
		t.Fatalf("want ErrAgentUnreachable, got %v", err)
	}
}

// A sync reply beyond the message cap is passed through dialErr
// unchanged — the caller sees ErrMessageTooBig, not a generic class.
func TestDialOversizedSyncReply(t *testing.T) {
	sock := rawServer(t, func(c net.Conn) {
		buf := make([]byte, 4096)
		c.SetDeadline(time.Now().Add(5 * time.Second))
		c.Read(buf)
		big := make([]byte, MaxMessage+1024)
		for i := range big {
			big[i] = 'x'
		}
		c.Write(append(big, '\n'))
	})
	_, err := Dial(sock, 5*time.Second)
	if !errors.Is(err, ErrMessageTooBig) {
		t.Fatalf("want ErrMessageTooBig, got %v", err)
	}
}

// An argument value json cannot marshal fails the write path before
// anything reaches the wire.
func TestExecuteArgMarshalError(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(nil))
	cl := dial(t, srv.Sock, 2*time.Second)
	_, err := cl.Execute(context.Background(), "guest-ping",
		map[string]any{"bad": make(chan int)})
	if !errors.Is(err, ErrAgentUnreachable) {
		t.Fatalf("want ErrAgentUnreachable, got %v", err)
	}
}

// Blank lines on the channel are skipped, not parsed — real agents
// emit stray whitespace around the 0xFF sync sentinel.
func TestBlankLinesSkipped(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Raw: []byte("\n \n{\"return\":{\"host-name\":\"ws\"},\"id\":\"" +
			req["id"].(string) + "\"}\n")}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	h, err := cl.HostName(context.Background())
	if err != nil || h != "ws" {
		t.Fatalf("HostName %q %v", h, err)
	}
}

// Every typed wrapper must surface an agent-side rejection — the
// shared table keeps per-command error plumbing honest.
func TestTypedCommandsAgentError(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{
			Class: "CommandNotFound", Desc: "disabled"}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	ctx := context.Background()
	if _, err := cl.Info(ctx); err == nil {
		t.Fatal("Info accepted agent error")
	}
	if _, err := cl.OSInfo(ctx); err == nil {
		t.Fatal("OSInfo accepted agent error")
	}
	if _, err := cl.HostName(ctx); err == nil {
		t.Fatal("HostName accepted agent error")
	}
	if _, err := cl.NetInterfaces(ctx); err == nil {
		t.Fatal("NetInterfaces accepted agent error")
	}
	if _, err := cl.GuestTime(ctx); err == nil {
		t.Fatal("GuestTime accepted agent error")
	}
	if _, err := cl.FSInfo(ctx); err == nil {
		t.Fatal("FSInfo accepted agent error")
	}
}

// A reply that cannot decode into the expected shape is a protocol
// violation (ErrBadResponse), not silently zero-valued data.
func TestTypedDecodeFailure(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(map[string]any{
		"guest-get-host-name": map[string]any{"host-name": 123},
		"guest-get-fsinfo":    map[string]any{"not": "an array"},
	}))
	cl := dial(t, srv.Sock, 2*time.Second)
	if _, err := cl.HostName(context.Background()); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
	if _, err := cl.FSInfo(context.Background()); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}

// ShellSpec is the explicit opt-in to shell interpretation; env and
// input-data must reach the wire intact when set.
func TestShellSpecAndExecArgs(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-exec":
			return qgatest.Reply{Return: map[string]any{"pid": 5}}
		case "guest-exec-status":
			code := 0
			return qgatest.Reply{Return: map[string]any{"exited": true, "exitcode": code}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	ctx := context.Background()

	spec := ShellSpec("echo hi", []string{"A=1"})
	if spec.Path != "/bin/sh" || !spec.CaptureOutput ||
		len(spec.Arg) != 2 || spec.Arg[0] != "-c" || spec.Arg[1] != "echo hi" {
		t.Fatalf("ShellSpec %+v", spec)
	}
	if _, err := cl.Exec(ctx, spec, 0); err != nil {
		t.Fatalf("Exec shell spec: %v", err)
	}
	if _, err := cl.Exec(ctx, ExecSpec{Path: "/bin/cat", InputData: "aGk="}, 0); err != nil {
		t.Fatalf("Exec input-data: %v", err)
	}
	var envOK, inputOK bool
	for _, r := range srv.Reqs() {
		if r["execute"] != "guest-exec" {
			continue
		}
		args := r["arguments"].(map[string]any)
		if env, ok := args["env"].([]any); ok && len(env) == 1 && env[0] == "A=1" {
			envOK = true
		}
		if args["input-data"] == "aGk=" {
			inputOK = true
		}
	}
	if !envOK || !inputOK {
		t.Fatalf("wire args missing env/input-data: %v", srv.Reqs())
	}
}

// guest-exec rejected outright (blacklisted command) — no pid, no
// polling, the agent error surfaces directly.
func TestExecStartRefused(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-exec" {
			return qgatest.Reply{Error: &qgatest.ErrorObj{
				Class: "CommandNotFound", Desc: "exec disabled"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	res, err := cl.Exec(context.Background(), ExecSpec{Path: "/bin/x"}, 0)
	var ae *AgentError
	if !errors.As(err, &ae) || res != nil {
		t.Fatalf("want AgentError/nil res, got %v %+v", err, res)
	}
}

// A guest-exec reply without a pid is a protocol violation.
func TestExecNoPID(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	_, err := cl.Exec(context.Background(), ExecSpec{Path: "/bin/x"}, 0)
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}

// A status poll that fails mid-wait returns the partial result with
// the error — the caller decides what to do with it.
func TestExecStatusRefused(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-exec":
			return qgatest.Reply{Return: map[string]any{"pid": 11}}
		case "guest-exec-status":
			return qgatest.Reply{Error: &qgatest.ErrorObj{
				Class: "GenericError", Desc: "status lost"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	res, err := cl.Exec(context.Background(), ExecSpec{Path: "/bin/x"}, 0)
	if err == nil || res == nil || res.PID != 11 {
		t.Fatalf("want partial res + err, got %+v %v", res, err)
	}
}

// Undecodable base64 output marks the result truncated rather than
// failing the command; a signal death leaves ExitCode at -1.
func TestExecBadBase64Signal(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-exec":
			return qgatest.Reply{Return: map[string]any{"pid": 6}}
		case "guest-exec-status":
			return qgatest.Reply{Return: map[string]any{
				"exited":   true,
				"signal":   9,
				"out-data": base64.StdEncoding.EncodeToString([]byte("x")),
				"err-data": "%%%not-base64%%%"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	res, err := cl.Exec(context.Background(), ExecSpec{Path: "/bin/x"}, 0)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !res.Exited || res.ExitCode != -1 || res.Signal != 9 || !res.Truncated {
		t.Fatalf("signal/bad-b64 result %+v", res)
	}
}

// A missing local file fails before the agent is touched.
func TestPushLocalMissing(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(nil))
	cl := dial(t, srv.Sock, 2*time.Second)
	_, err := cl.Push(context.Background(),
		filepath.Join(t.TempDir(), "nope"), "/tmp/x", TransferOpts{})
	if err == nil {
		t.Fatal("push of missing file succeeded")
	}
	if len(srv.Reqs()) != 1 { // handshake only
		t.Fatalf("agent touched despite local failure: %v", srv.Cmds())
	}
}

// A local read error mid-push (directory opened as a file) still
// closes the guest handle.
func TestPushReadError(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-file-open" {
			return qgatest.Reply{Return: 7}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	if _, err := cl.Push(context.Background(), t.TempDir(), "/tmp/x", TransferOpts{}); err == nil {
		t.Fatal("push of a directory succeeded")
	}
	for _, c := range srv.Cmds() {
		if c == "guest-file-close" {
			return
		}
	}
	t.Fatalf("guest handle not closed after read error: %v", srv.Cmds())
}

// guest-file-write rejected: the handle is closed and the agent
// error propagates.
func TestPushWriteRefused(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 8}
		case "guest-file-write":
			return qgatest.Reply{Error: &qgatest.ErrorObj{
				Class: "GenericError", Desc: "read-only fs"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	local := filepath.Join(t.TempDir(), "s")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := cl.Push(context.Background(), local, "/tmp/x", TransferOpts{})
	var ae *AgentError
	if !errors.As(err, &ae) {
		t.Fatalf("want AgentError, got %v", err)
	}
}

// The agent acknowledging fewer bytes than sent is a protocol
// violation — partial writes must not be treated as success.
func TestPushShortWrite(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 9}
		case "guest-file-write":
			args := req["arguments"].(map[string]any)
			b, _ := base64.StdEncoding.DecodeString(args["buf-b64"].(string))
			return qgatest.Reply{Return: map[string]any{"count": len(b) - 1}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	local := filepath.Join(t.TempDir(), "s")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := cl.Push(context.Background(), local, "/tmp/x", TransferOpts{})
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}

// guest-file-close failing after a successful push surfaces the
// error alongside the completed byte count.
func TestPushCloseRefused(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		args, _ := req["arguments"].(map[string]any)
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 10}
		case "guest-file-write":
			b, _ := base64.StdEncoding.DecodeString(args["buf-b64"].(string))
			return qgatest.Reply{Return: map[string]any{"count": len(b)}}
		case "guest-file-close":
			return qgatest.Reply{Error: &qgatest.ErrorObj{
				Class: "GenericError", Desc: "close failed"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	local := filepath.Join(t.TempDir(), "s")
	if err := os.WriteFile(local, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := cl.Push(context.Background(), local, "/tmp/x", TransferOpts{})
	var ae *AgentError
	if !errors.As(err, &ae) || res.Bytes != 4 {
		t.Fatalf("want AgentError + 4 bytes, got %+v %v", res, err)
	}
}

// guest-file-open rejection on pull returns before any local file
// is created.
func TestPullOpenRefused(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-file-open" {
			return qgatest.Reply{Error: &qgatest.ErrorObj{
				Class: "FileOpenFailed", Desc: "no such file"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	dir := t.TempDir()
	_, err := cl.Pull(context.Background(), "/nope", filepath.Join(dir, "o"), TransferOpts{})
	var ae *AgentError
	if !errors.As(err, &ae) {
		t.Fatalf("want AgentError, got %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("debris left after open refusal: %v", entries)
	}
}

// When the temp file cannot be created the guest handle is closed
// and the open error propagates — no dangling handle.
func TestPullTmpCreateFail(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-file-open" {
			return qgatest.Reply{Return: 12}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	dst := filepath.Join(t.TempDir(), "missing-dir", "out")
	if _, err := cl.Pull(context.Background(), "/x", dst, TransferOpts{}); err == nil {
		t.Fatal("pull into nonexistent dir succeeded")
	}
	for _, c := range srv.Cmds() {
		if c == "guest-file-close" {
			return
		}
	}
	t.Fatalf("guest handle not closed after tmp failure: %v", srv.Cmds())
}

// Undecodable buf-b64 is a protocol violation; the temp artifact is
// removed rather than left behind.
func TestPullBadB64(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 13}
		case "guest-file-read":
			return qgatest.Reply{Return: map[string]any{
				"count": 4, "buf-b64": "%%%bad%%%", "eof": false}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	dir := t.TempDir()
	_, err := cl.Pull(context.Background(), "/x", filepath.Join(dir, "o"), TransferOpts{})
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("temp debris left: %v", entries)
	}
}

// The agent may over-report: buf-b64 carrying more bytes than count
// is clamped to count, never trusted blindly.
func TestPullCountClamp(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 14}
		case "guest-file-read":
			return qgatest.Reply{Return: map[string]any{
				"count":   2,
				"buf-b64": base64.StdEncoding.EncodeToString([]byte("12345")),
				"eof":     true}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	dst := filepath.Join(t.TempDir(), "o")
	res, err := cl.Pull(context.Background(), "/x", dst, TransferOpts{})
	if err != nil || res.Bytes != 2 {
		t.Fatalf("clamp: %+v %v", res, err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "12" {
		t.Fatalf("clamped content %q", b)
	}
}

// A stalling agent (count 0, no EOF, forever) is bounded by ctx —
// ErrTimeout, not a spin.
func TestPullStallTimeout(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 15}
		case "guest-file-read":
			return qgatest.Reply{Return: map[string]any{
				"count": 0, "buf-b64": "", "eof": false}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := cl.Pull(ctx, "/x", filepath.Join(t.TempDir(), "o"), TransferOpts{})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
}

// A stall frame followed by real data is just a slow read — the
// transfer completes normally.
func TestPullStallThenData(t *testing.T) {
	reads := 0
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 16}
		case "guest-file-read":
			reads++
			if reads == 1 {
				return qgatest.Reply{Return: map[string]any{
					"count": 0, "buf-b64": "", "eof": false}}
			}
			return qgatest.Reply{Return: map[string]any{
				"count":   3,
				"buf-b64": base64.StdEncoding.EncodeToString([]byte("abc")),
				"eof":     true}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	dst := filepath.Join(t.TempDir(), "o")
	res, err := cl.Pull(context.Background(), "/x", dst, TransferOpts{})
	if err != nil || res.Bytes != 3 {
		t.Fatalf("stall-then-data: %+v %v", res, err)
	}
}

// guest-file-close failing after a completed pull removes the temp
// file and reports the error.
func TestPullCloseRefused(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 17}
		case "guest-file-read":
			return qgatest.Reply{Return: map[string]any{
				"count":   1,
				"buf-b64": base64.StdEncoding.EncodeToString([]byte("a")),
				"eof":     true}}
		case "guest-file-close":
			return qgatest.Reply{Error: &qgatest.ErrorObj{
				Class: "GenericError", Desc: "close failed"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	dir := t.TempDir()
	_, err := cl.Pull(context.Background(), "/x", filepath.Join(dir, "o"), TransferOpts{})
	if err == nil {
		t.Fatal("close refusal accepted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("temp debris left: %v", entries)
	}
}

// The atomic rename must fail cleanly when the destination cannot
// be replaced — an existing directory cannot become a file.
func TestPullRenameFails(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 18}
		case "guest-file-read":
			return qgatest.Reply{Return: map[string]any{
				"count":   1,
				"buf-b64": base64.StdEncoding.EncodeToString([]byte("a")),
				"eof":     true}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	dir := t.TempDir()
	dst := filepath.Join(dir, "destdir")
	if err := os.Mkdir(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Pull(context.Background(), "/x", dst, TransferOpts{}); err == nil {
		t.Fatal("rename onto a directory succeeded")
	}
	if _, err := os.Stat(dst + ".tmp-qga"); !os.IsNotExist(err) {
		t.Fatal("temp file left after rename failure")
	}
}

// VerifyPull propagates a pull failure instead of a digest.
func TestVerifyPullRefused(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-file-open" {
			return qgatest.Reply{Error: &qgatest.ErrorObj{
				Class: "FileOpenFailed", Desc: "denied"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	sum, err := cl.VerifyPull(context.Background(), "/x", TransferOpts{})
	if err == nil || sum != "" {
		t.Fatalf("VerifyPull %q %v", sum, err)
	}
}

// The remaining SafeGuestPath corners: bare root, the trailing-slash
// denylist entries and exact-match denies.
func TestSafeGuestPathEdges(t *testing.T) {
	for _, p := range []string{"/", "/run", "/run/systemd/x", "/var/lib", "c:/windows/x"} {
		if SafeGuestPath(p) {
			t.Errorf("dangerous path accepted: %q", p)
		}
	}
	if !SafeGuestPath("/var/run/x") { // /var/run is not /run/
		t.Error("/var/run/x wrongly rejected")
	}
}

// Agent refusals on the fsfreeze commands surface per-wrapper.
func TestFSFreezeCmdErrors(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{
			Class: "GenericError", Desc: "fsfreeze denied"}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	ctx := context.Background()
	if st, err := cl.FSFreezeStatus(ctx); err == nil || st != "" {
		t.Fatalf("status %q %v", st, err)
	}
	if n, err := cl.FSFreeze(ctx); err == nil || n != -1 {
		t.Fatalf("freeze %d %v", n, err)
	}
}

// A freeze refusal stops WithFreeze before fn runs.
func TestWithFreezeRefused(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-fsfreeze-freeze" {
			return qgatest.Reply{Error: &qgatest.ErrorObj{
				Class: "GenericError", Desc: "no freeze"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	ran := false
	err := cl.WithFreeze(context.Background(), time.Minute,
		func(ctx context.Context) error { ran = true; return nil })
	if err == nil || ran {
		t.Fatalf("WithFreeze ran/failed silently: ran=%v err=%v", ran, err)
	}
}

// The happy path: watchdog defaults when <= 0, fn succeeds, thaw
// runs and its (nil) result is what WithFreeze returns.
func TestWithFreezeSuccess(t *testing.T) {
	var thaws int
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-fsfreeze-freeze":
			return qgatest.Reply{Return: 2}
		case "guest-fsfreeze-thaw":
			thaws++
			return qgatest.Reply{Return: 2}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	ran := false
	err := cl.WithFreeze(context.Background(), 0, func(ctx context.Context) error {
		ran = true
		return nil
	})
	if err != nil || !ran || thaws != 1 {
		t.Fatalf("WithFreeze clean run: ran=%v thaws=%d err=%v", ran, thaws, err)
	}
}

// When the context is already done, the thaw retry does not wait
// for the backoff — it reports immediately.
func TestThawRetryCtxDone(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{
			Class: "GenericError", Desc: "thaw busy"}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	n, err := cl.thawBestEffort(ctx)
	if err == nil || n != -1 {
		t.Fatalf("want -1 + err, got %d %v", n, err)
	}
	if time.Since(start) > 400*time.Millisecond {
		t.Fatal("ctx-done retry waited for backoff")
	}
}

// Remaining shutdown modes take the same accept path.
func TestShutdownOtherModes(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(nil))
	cl := dial(t, srv.Sock, 2*time.Second)
	for _, m := range []ShutdownMode{ShutdownPowerdown, ShutdownHalt} {
		if err := cl.Shutdown(context.Background(), m); err != nil {
			t.Fatalf("shutdown %s: %v", m, err)
		}
	}
	var saw []string
	for _, r := range srv.Reqs() {
		if r["execute"] == "guest-shutdown" {
			saw = append(saw, r["arguments"].(map[string]any)["mode"].(string))
		}
	}
	if strings.Join(saw, ",") != "powerdown,halt" {
		t.Fatalf("shutdown modes on wire: %v", saw)
	}
}
