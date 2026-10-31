// ops_test.go: typed command wrappers plus the higher-level
// exec/file/fsfreeze/shutdown flows against the fake agent.
package qga

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alesanfe/quickemu/tools/quickemu-qga/qgatest"
)

func TestTypedCommands(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(map[string]any{
		"guest-info": map[string]any{"version": "9.1.0",
			"supported_commands": []map[string]any{
				{"name": "guest-ping", "enabled": true, "success-response": true}}},
		"guest-get-osinfo": map[string]any{
			"id": "debian", "pretty-name": "Debian GNU/Linux 12",
			"kernel-release": "6.1.0", "machine": "x86_64"},
		"guest-get-host-name": map[string]any{"host-name": "vm1"},
		"guest-get-time":      1735689600000000000,
		"guest-network-get-interfaces": []map[string]any{{
			"name": "enp1s0", "hardware-address": "52:54:00:11:22:33",
			"ip-addresses": []map[string]any{
				{"ip-address": "10.0.2.15", "ip-address-type": "ipv4", "prefix": 24},
				{"ip-address": "fe80::1", "ip-address-type": "ipv6", "prefix": 64}}}},
		"guest-get-fsinfo": []map[string]any{{
			"name": "sda1", "mountpoint": "/", "type": "ext4",
			"used-bytes": 5 << 30, "total-bytes": 16 << 30}},
	}))
	cl := dial(t, srv.Sock, 2*time.Second)
	ctx := context.Background()
	if gi, err := cl.Info(ctx); err != nil || gi.Version != "9.1.0" ||
		len(gi.SupportedCommands) != 1 || !gi.SupportedCommands[0].Enabled {
		t.Fatalf("Info %+v %v", gi, err)
	}
	if oi, err := cl.OSInfo(ctx); err != nil || oi.PrettyName == "" || oi.Architecture != "x86_64" {
		t.Fatalf("OSInfo %+v %v", oi, err)
	}
	if h, err := cl.HostName(ctx); err != nil || h != "vm1" {
		t.Fatalf("HostName %q %v", h, err)
	}
	if gt, err := cl.GuestTime(ctx); err != nil || gt.Year() != 2025 {
		t.Fatalf("GuestTime %v %v", gt, err)
	}
	ifs, err := cl.NetInterfaces(ctx)
	if err != nil || len(ifs) != 1 || len(ifs[0].IPAddresses) != 2 ||
		ifs[0].IPAddresses[0].Address != "10.0.2.15" {
		t.Fatalf("NetInterfaces %+v %v", ifs, err)
	}
	fss, err := cl.FSInfo(ctx)
	if err != nil || len(fss) != 1 || fss[0].Mountpoint != "/" {
		t.Fatalf("FSInfo %+v %v", fss, err)
	}
}

// guest-exec polls until "exited" — the fake serves a two-step
// transcript (running, then exited) and output arrives incrementally.
func TestExecLifecycle(t *testing.T) {
	var statusCalls atomic.Int32
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-exec":
			return qgatest.Reply{Return: map[string]any{"pid": 4242}}
		case "guest-exec-status":
			if statusCalls.Add(1) == 1 {
				return qgatest.Reply{Return: map[string]any{
					"exited": false, "out-data": base64.StdEncoding.EncodeToString([]byte("hel"))}}
			}
			code := 7
			return qgatest.Reply{Return: map[string]any{
				"exited": true, "exitcode": code,
				"out-data": base64.StdEncoding.EncodeToString([]byte("hello")),
				"err-data": base64.StdEncoding.EncodeToString([]byte("oops"))}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 3*time.Second)
	res, err := cl.Exec(context.Background(), ExecSpec{
		Path: "/bin/echo", Arg: []string{"hello"}}, 0)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 7 || string(res.Out) != "hello" || string(res.Err) != "oops" {
		t.Fatalf("Exec result %+v", res)
	}
	// argv reached the agent uninterpreted (arg array, no shell).
	for _, r := range srv.Reqs() {
		if r["execute"] == "guest-exec" {
			args := r["arguments"].(map[string]any)
			if args["path"] != "/bin/echo" || args["capture-output"] != true {
				t.Fatalf("guest-exec args %v", args)
			}
			if argv, ok := args["arg"].([]any); !ok || argv[0] != "hello" {
				t.Fatalf("guest-exec argv %v", args)
			}
		}
	}
}

// A guest process that never exits must hit the ctx deadline — the
// caller gets ErrTimeout plus whatever output was captured so far.
func TestExecTimeout(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-exec":
			return qgatest.Reply{Return: map[string]any{"pid": 9}}
		case "guest-exec-status":
			return qgatest.Reply{Return: map[string]any{"exited": false,
				"out-data": base64.StdEncoding.EncodeToString([]byte("partial"))}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 3*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	res, err := cl.Exec(ctx, ExecSpec{Path: "/bin/sleep", Arg: []string{"99"}}, 0)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	if string(res.Out) != "partial" {
		t.Fatalf("partial output lost: %+v", res)
	}
}

// Output over the caller's cap is marked truncated, never fatal.
func TestExecOutputCap(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-exec" {
			return qgatest.Reply{Return: map[string]any{"pid": 3}}
		}
		code := 0
		return qgatest.Reply{Return: map[string]any{"exited": true, "exitcode": code,
			"out-data": base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	res, err := cl.Exec(context.Background(), ExecSpec{Path: "/bin/x"}, 8)
	if err != nil || !res.Truncated || len(res.Out) != 8 {
		t.Fatalf("cap: %+v %v", res, err)
	}
}

// guest-file-* transcript: open returns a handle, write echoes the
// decoded byte count, close acknowledges. Verifies content + sha256.
func TestPush(t *testing.T) {
	var got strings.Builder
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		args, _ := req["arguments"].(map[string]any)
		switch req["execute"] {
		case "guest-file-open":
			if args["path"] != "/tmp/x" || args["mode"] != "w+" {
				return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "GenericError", Desc: "bad open"}}
			}
			return qgatest.Reply{Return: 1001}
		case "guest-file-write":
			b, _ := base64.StdEncoding.DecodeString(args["buf-b64"].(string))
			got.Write(b)
			return qgatest.Reply{Return: map[string]any{"count": len(b), "eof": false}}
		case "guest-file-close":
			return qgatest.Reply{Return: map[string]any{}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 3*time.Second)
	local := filepath.Join(t.TempDir(), "in.bin")
	payload := strings.Repeat("data-block-", 12000) // > one 48KiB block
	if err := os.WriteFile(local, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := cl.Push(context.Background(), local, "/tmp/x", TransferOpts{Block: 4096})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if got.String() != payload || res.Bytes != int64(len(payload)) {
		t.Fatalf("payload mismatch: got %d want %d", got.Len(), res.Bytes)
	}
	if res.SHA256 == "" {
		t.Fatal("sha256 missing")
	}
	if len(srv.Reqs()) < 4 {
		t.Fatalf("multi-block write expected, reqs %v", srv.Cmds())
	}
}

func TestPull(t *testing.T) {
	payload := "line1\nline2\n" + strings.Repeat("x", 70000)
	sent := 0
	chunks := []string{
		base64.StdEncoding.EncodeToString([]byte(payload[:50000])),
		base64.StdEncoding.EncodeToString([]byte(payload[50000:])),
	}
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			return qgatest.Reply{Return: 77}
		case "guest-file-read":
			r := qgatest.Reply{Return: map[string]any{"count": 0, "buf-b64": "", "eof": true}}
			if sent < len(chunks) {
				b, _ := base64.StdEncoding.DecodeString(chunks[sent])
				r = qgatest.Reply{Return: map[string]any{
					"count": len(b), "buf-b64": chunks[sent], "eof": sent == len(chunks)-1}}
				sent++
			}
			return r
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 3*time.Second)
	dst := filepath.Join(t.TempDir(), "out.bin")
	res, err := cl.Pull(context.Background(), "/var/log/x", dst, TransferOpts{Block: 50000})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != payload {
		t.Fatalf("pulled content mismatch: %v", err)
	}
	if res.Bytes != int64(len(payload)) {
		t.Fatalf("bytes %d want %d", res.Bytes, len(payload))
	}
	// No temp debris after a clean atomic rename.
	if entries, _ := os.ReadDir(filepath.Dir(dst)); len(entries) != 1 {
		t.Fatalf("temp debris left: %v", entries)
	}
}

// Pull hitting the byte cap marks Trunc and still writes atomically.
func TestPullCap(t *testing.T) {
	payload := strings.Repeat("z", 100)
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-file-open" {
			return qgatest.Reply{Return: 1}
		}
		if req["execute"] == "guest-file-read" {
			return qgatest.Reply{Return: map[string]any{
				"count": len(payload), "buf-b64": base64.StdEncoding.EncodeToString([]byte(payload)), "eof": true}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	dst := filepath.Join(t.TempDir(), "o")
	res, err := cl.Pull(context.Background(), "/big", dst, TransferOpts{MaxBytes: 40})
	if err != nil || !res.Trunc || res.Bytes != 40 {
		t.Fatalf("cap: %+v %v", res, err)
	}
	b, _ := os.ReadFile(dst)
	if len(b) != 40 {
		t.Fatalf("truncated write: %d", len(b))
	}
}

// A mid-pull disconnect removes the temp file instead of leaving a
// truncated artifact at the destination name.
func TestPullDisconnectCleans(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-file-open" {
			return qgatest.Reply{Return: 5}
		}
		return qgatest.Reply{Silent: true, Close: true}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	dir := t.TempDir()
	dst := filepath.Join(dir, "o")
	if _, err := cl.Pull(context.Background(), "/x", dst, TransferOpts{}); err == nil {
		t.Fatal("expected error")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("debris: %v", entries)
	}
}

func TestSafeGuestPath(t *testing.T) {
	ok := []string{"/tmp/x", "/home/u/f", "/var/tmp/d", "C:\\Users\\a\\f.txt", "rel/file"}
	for _, p := range ok {
		if !SafeGuestPath(p) {
			t.Errorf("safe path rejected: %q", p)
		}
	}
	bad := []string{"", "/etc/passwd", "/boot/vmlinuz", "/dev/sda",
		"/proc/1/environ", "../x", "/tmp/../etc/x", "C:\\Windows\\system32\\x",
		"/usr/bin/x", "/root/.ssh/id", "/tmp/a\x00b"}
	for _, p := range bad {
		if SafeGuestPath(p) {
			t.Errorf("dangerous path accepted: %q", p)
		}
	}
	// Restricted-mode transfers refuse before touching the agent.
	srv := qgatest.Serve(t, qgatest.Static(nil))
	cl := dial(t, srv.Sock, 2*time.Second)
	if _, err := cl.Push(context.Background(), "/etc/hostname", "/etc/evil",
		TransferOpts{Restricted: true}); err == nil {
		t.Fatal("restricted push accepted /etc")
	}
	if _, err := cl.Pull(context.Background(), "/etc/shadow", filepath.Join(t.TempDir(), "s"),
		TransferOpts{Restricted: true}); err == nil {
		t.Fatal("restricted pull accepted /etc/shadow")
	}
	if len(srv.Reqs()) != 1 { // only the handshake
		t.Fatalf("agent touched despite refusal: %v", srv.Cmds())
	}
}

func TestFSFreezeFlow(t *testing.T) {
	var frozen atomic.Bool
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-fsfreeze-status":
			if frozen.Load() {
				return qgatest.Reply{Return: "frozen"}
			}
			return qgatest.Reply{Return: "thawed"}
		case "guest-fsfreeze-freeze":
			frozen.Store(true)
			return qgatest.Reply{Return: 2}
		case "guest-fsfreeze-thaw":
			frozen.Store(false)
			return qgatest.Reply{Return: 2}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	ctx := context.Background()
	if st, err := cl.FSFreezeStatus(ctx); err != nil || st != FSFThawed {
		t.Fatalf("status %q %v", st, err)
	}
	if n, err := cl.FSFreeze(ctx); err != nil || n != 2 {
		t.Fatalf("freeze %d %v", n, err)
	}
	if st, _ := cl.FSFreezeStatus(ctx); st != FSFFrozen {
		t.Fatalf("status %q", st)
	}
	if n, err := cl.FSThaw(ctx); err != nil || n != 2 {
		t.Fatalf("thaw %d %v", n, err)
	}
}

// WithFreeze must thaw even when fn fails — and fn must run while
// the guest is frozen.
func TestWithFreezeThawsOnError(t *testing.T) {
	var frozen atomic.Bool
	var sawFrozen atomic.Bool
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-fsfreeze-freeze":
			frozen.Store(true)
			return qgatest.Reply{Return: 1}
		case "guest-fsfreeze-thaw":
			frozen.Store(false)
			return qgatest.Reply{Return: 1}
		case "guest-ping":
			if !frozen.Load() {
				sawFrozen.Store(false)
			} else {
				sawFrozen.Store(true)
			}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	want := errors.New("fn failed")
	err := cl.WithFreeze(context.Background(), time.Minute, func(ctx context.Context) error {
		if err := cl.Ping(ctx); err != nil {
			t.Fatalf("ping inside freeze: %v", err)
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("want fn error, got %v", err)
	}
	if !sawFrozen.Load() {
		t.Fatal("fn ran unfrozen")
	}
	if frozen.Load() {
		t.Fatal("guest left frozen after fn error")
	}
}

// A fn that outlives the watchdog triggers a watchdog thaw — the
// guest is not left frozen while fn is still blocked.
func TestWithFreezeWatchdog(t *testing.T) {
	var frozen atomic.Bool
	var thaws atomic.Int32
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-fsfreeze-freeze":
			frozen.Store(true)
			return qgatest.Reply{Return: 1}
		case "guest-fsfreeze-thaw":
			frozen.Store(false)
			thaws.Add(1)
			return qgatest.Reply{Return: 1}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	err := cl.WithFreeze(context.Background(), 120*time.Millisecond,
		func(ctx context.Context) error {
			// Outlive the watchdog (it thaws at ~120ms), then finish.
			time.Sleep(400 * time.Millisecond)
			return nil
		})
	if err == nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("watchdog should surface an error: %v", err)
	}
	if frozen.Load() {
		t.Fatal("watchdog left the guest frozen")
	}
	if thaws.Load() < 1 {
		t.Fatal("watchdog never thawed")
	}
}

// A thaw that fails once is retried (thawBestEffort).
func TestThawRetry(t *testing.T) {
	var attempts atomic.Int32
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-fsfreeze-thaw" && attempts.Add(1) == 1 {
			return qgatest.Reply{Error: &qgatest.ErrorObj{Class: "GenericError", Desc: "busy"}}
		}
		return qgatest.Reply{Return: 1}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	n, err := cl.thawBestEffort(context.Background())
	if err != nil || n != 1 || attempts.Load() < 2 {
		t.Fatalf("retry: n=%d attempts=%d err=%v", n, attempts.Load(), err)
	}
}

func TestShutdownModes(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(nil))
	cl := dial(t, srv.Sock, 2*time.Second)
	if err := cl.Shutdown(context.Background(), ShutdownReboot); err != nil {
		t.Fatalf("reboot: %v", err)
	}
	if err := cl.Shutdown(context.Background(), "bogus"); err == nil {
		t.Fatal("bad mode accepted")
	}
	var last map[string]any
	for _, r := range srv.Reqs() {
		last = r
	}
	if last["execute"] != "guest-shutdown" {
		t.Fatalf("reqs %v", srv.Cmds())
	}
}
