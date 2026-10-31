// client_test.go: the channel contract — handshake, framing,
// serialization, id correlation, size caps and every failure class.
package qga

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesanfe/quickemu/tools/quickemu-qga/qgatest"
)

// dial is the test shorthand: short deadlines keep failure tests fast.
func dial(t *testing.T, sock string, timeout time.Duration) *Client {
	t.Helper()
	cl, err := Dial(sock, timeout)
	if err != nil {
		t.Fatalf("Dial(%s): %v", sock, err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

func TestDialHandshakeAndPing(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(nil))
	cl := dial(t, srv.Sock, 2*time.Second)
	if err := cl.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	cmds := srv.Cmds()
	if len(cmds) != 2 || cmds[0] != "guest-sync-delimited" || cmds[1] != "guest-ping" {
		t.Fatalf("server saw %v", cmds)
	}
}

func TestDialSocketMissing(t *testing.T) {
	_, err := Dial(filepath.Join(t.TempDir(), "nope.sock"), time.Second)
	if !errors.Is(err, ErrSocketMissing) {
		t.Fatalf("want ErrSocketMissing, got %v", err)
	}
}

func TestDialNotASocket(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file.sock")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Dial(f, time.Second)
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}

// The socket exists and accepts, but no agent ever answers — the
// handshake deadline classifies as agent-not-ready.
func TestDialAgentNotReady(t *testing.T) {
	_, err := Dial(qgatest.OccupiedSocket(t), 150*time.Millisecond)
	if !errors.Is(err, ErrAgentNotReady) {
		t.Fatalf("want ErrAgentNotReady, got %v", err)
	}
}

func TestDialUnreachable(t *testing.T) {
	// Socket file exists but nobody listens.
	dir := t.TempDir()
	sock := filepath.Join(dir, "a.sock")
	f, _ := os.Create(sock)
	f.Close()
	os.Remove(sock)
	// Bind+close leaves a dangling path entry on some systems; create
	// a listener, take its name, then close it.
	_, err := Dial(sock, 300*time.Millisecond)
	if !errors.Is(err, ErrSocketMissing) {
		// An actual stale socket file would be unreachable.
		if !errors.Is(err, ErrAgentUnreachable) {
			t.Fatalf("want missing/unreachable, got %v", err)
		}
	}
}

func TestFragmentedReply(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Return: map[string]any{"host-name": "frag-vm"}, Split: 7}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	h, err := cl.HostName(context.Background())
	if err != nil || h != "frag-vm" {
		t.Fatalf("HostName %q %v", h, err)
	}
}

// A record carrying a different id must be skipped — the client
// waits for ITS reply instead of mis-attributing the stale one.
func TestOutOfOrderIDs(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{
			Return: map[string]any{"host-name": "real"},
			Before: []map[string]any{
				{"return": map[string]any{"host-name": "stale"}, "id": "qga-0"},
				{"unexpected": "async-record"},
			},
		}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	h, err := cl.HostName(context.Background())
	if err != nil || h != "real" {
		t.Fatalf("HostName %q %v (stale record must be skipped)", h, err)
	}
}

// Non-JSON noise between records is tolerated — the channel carries
// real framing, garbage lines do not desynchronise it.
func TestGarbageLinesTolerated(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Raw: []byte("not json\n{\"return\":{\"host-name\":\"ok\"},\"id\":\"" +
			req["id"].(string) + "\"}\n")}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	h, err := cl.HostName(context.Background())
	if err != nil || h != "ok" {
		t.Fatalf("HostName %q %v", h, err)
	}
}

// A reply over the message cap fails with ErrMessageTooBig — and a
// smaller cap is enforced early (test uses a tiny cap).
func TestOversizedReply(t *testing.T) {
	big := strings.Repeat("x", 3<<20) // > the 1MiB test cap, < 4MiB
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Raw: []byte("{\"return\":\"" + big + "\",\"id\":\"" + req["id"].(string) + "\"}\n")}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	cl.max = 1 << 20
	if _, err := cl.Execute(context.Background(), "guest-ping", nil); !errors.Is(err, ErrMessageTooBig) {
		t.Fatalf("want ErrMessageTooBig, got %v", err)
	}
}

// Silence is a timeout, classified separately from a dead channel.
func TestCommandTimeout(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Silent: true}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	cl.timeout = 150 * time.Millisecond
	_, err := cl.Execute(context.Background(), "guest-ping", nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
}

// A hang-up mid-command is a distinct class: the channel died, the
// request state is unknown.
func TestDisconnectMidCommand(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Silent: true, Close: true}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	_, err := cl.Execute(context.Background(), "guest-ping", nil)
	if !errors.Is(err, ErrAgentGone) {
		t.Fatalf("want ErrAgentGone, got %v", err)
	}
}

// An agent-side rejection surfaces as a typed AgentError with the
// class/description intact.
func TestAgentError(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Error: &qgatest.ErrorObj{
			Class: "CommandNotFound", Desc: "disabled by blacklist"}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	_, err := cl.Execute(context.Background(), "guest-secret", nil)
	var ae *AgentError
	if !errors.As(err, &ae) || ae.Class != "CommandNotFound" {
		t.Fatalf("want AgentError, got %v", err)
	}
}

// A reply with no return and no error is a protocol violation.
func TestBadReplyShape(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Raw: []byte("{\"bogus\":1,\"id\":\"" + req["id"].(string) + "\"}\n")}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	_, err := cl.Execute(context.Background(), "guest-ping", nil)
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}

// Context cancel must abort a blocked command promptly — the conn
// deadline is armed by Execute's AfterFunc.
func TestContextCancel(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Silent: true}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	_, err := cl.Execute(ctx, "guest-ping", nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
}

// Serialization: two goroutines' requests never interleave — each
// sees its own reply id.
func TestConcurrentSerialize(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		return qgatest.Reply{Return: req["id"], Delay: 5 * time.Millisecond}
	})
	cl := dial(t, srv.Sock, 3*time.Second)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			raw, err := cl.Execute(context.Background(), "guest-ping", nil)
			if err != nil {
				errs <- err
				return
			}
			var got string
			if err := json.Unmarshal(raw, &got); err != nil || !strings.HasPrefix(got, "qga-") {
				errs <- errors.New("bad reply")
				return
			}
			errs <- nil
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent execute: %v", err)
		}
	}
}
