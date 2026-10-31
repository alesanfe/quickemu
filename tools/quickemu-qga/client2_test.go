// client2_test.go: handshake edge cases, path helpers and the
// secondary error branches that keep coverage honest.
package qga

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alesanfe/quickemu/tools/quickemu-qga/qgatest"
)

// rawServer speaks just enough protocol to control the handshake
// bytes exactly — needed for cases the scripted Reply cannot express.
func rawServer(t *testing.T, script func(c net.Conn)) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "qga.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		script(c)
	}()
	return sock
}

// A reply with the wrong sync token fails the handshake.
func TestHandshakeTokenMismatch(t *testing.T) {
	sock := rawServer(t, func(c net.Conn) {
		buf := make([]byte, 4096)
		c.SetDeadline(time.Now().Add(2 * time.Second))
		c.Read(buf)
		c.Write([]byte{0xFF})
		c.Write([]byte("{\"return\": 999}\n"))
	})
	_, err := Dial(sock, time.Second)
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}

// An agent that greets with an error record fails the handshake with
// the typed error, not a generic timeout.
func TestHandshakeAgentError(t *testing.T) {
	sock := rawServer(t, func(c net.Conn) {
		buf := make([]byte, 4096)
		c.SetDeadline(time.Now().Add(2 * time.Second))
		c.Read(buf)
		c.Write([]byte{0xFF})
		c.Write([]byte("{\"error\":{\"class\":\"GenericError\",\"desc\":\"sync refused\"}}\n"))
	})
	_, err := Dial(sock, time.Second)
	var ae *AgentError
	if !errors.As(err, &ae) {
		t.Fatalf("want AgentError, got %v", err)
	}
}

// Garbage instead of a handshake reply is a bad response.
func TestHandshakeGarbage(t *testing.T) {
	sock := rawServer(t, func(c net.Conn) {
		buf := make([]byte, 4096)
		c.SetDeadline(time.Now().Add(2 * time.Second))
		c.Read(buf)
		c.Write([]byte{0xFF})
		c.Write([]byte("{\"nope\":true}\n"))
	})
	_, err := Dial(sock, time.Second)
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}

// Dial timeout default: a zero/negative timeout is replaced.
func TestDialTimeoutDefault(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(nil))
	cl, err := Dial(srv.Sock, 0)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer cl.Close()
	if cl.timeout != 15*time.Second {
		t.Fatalf("default timeout %v", cl.timeout)
	}
}

func TestAgentErrorString(t *testing.T) {
	ae := &AgentError{Class: "X", Desc: "y"}
	if ae.Error() == "" {
		t.Fatal("empty Error()")
	}
	ae2 := agentErr(map[string]any{"error": 7})
	if ae2 == nil || ae2.Class != "unknown" {
		t.Fatalf("unmarshable error body: %+v", ae2)
	}
	if agentErr(map[string]any{}) != nil {
		t.Fatal("no error key must be nil")
	}
}

// A write failure on a dead connection classifies as unreachable.
func TestExecuteWriteFailure(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(nil))
	cl := dial(t, srv.Sock, 2*time.Second)
	cl.Close() // dead conn — the write fails before any read
	_, err := cl.Execute(context.Background(), "guest-ping", nil)
	if !errors.Is(err, ErrAgentUnreachable) {
		t.Fatalf("want ErrAgentUnreachable, got %v", err)
	}
}

// 64 mismatched replies exhaust the correlation window.
func TestReplyWindowExhausted(t *testing.T) {
	var n int
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		n++
		before := make([]map[string]any, 0, 64)
		for i := 0; i < 64; i++ {
			before = append(before, map[string]any{
				"return": map[string]any{}, "id": fmt.Sprintf("stale-%d", i)})
		}
		return qgatest.Reply{Return: map[string]any{}, Before: before}
	})
	cl := dial(t, srv.Sock, 3*time.Second)
	_, err := cl.Execute(context.Background(), "guest-ping", nil)
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("want ErrBadResponse, got %v", err)
	}
}

// VerifyPull re-reads the guest file and returns its digest.
func TestVerifyPull(t *testing.T) {
	payload := "verify-me"
	opened := false
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		switch req["execute"] {
		case "guest-file-open":
			opened = true
			return qgatest.Reply{Return: 3}
		case "guest-file-read":
			if opened {
				opened = false
				return qgatest.Reply{Return: map[string]any{
					"count":   len(payload),
					"buf-b64": b64(payload), "eof": true}}
			}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	sum, err := cl.VerifyPull(context.Background(), "/tmp/f", TransferOpts{})
	if err != nil || len(sum) != 64 {
		t.Fatalf("VerifyPull %q %v", sum, err)
	}
}

func TestPathHelpers(t *testing.T) {
	if !CleanGuestPath("/tmp/f") || !CleanGuestPath("C:\\Users\\a\\f") {
		t.Fatal("clean paths rejected")
	}
	for _, p := range []string{"rel/x", "/etc/x", "", "C:rel", "/tmp/../x"} {
		if CleanGuestPath(p) {
			t.Fatalf("CleanGuestPath accepted %q", p)
		}
	}
	if !PathIsUnder("/a/b", "/a/b/c") || PathIsUnder("/a/b", "/a/c") {
		t.Fatal("PathIsUnder")
	}
	if PathIsUnder("/a/b", "/a/b") != true {
		t.Fatal("equal dirs are under")
	}
}

// Push refuses oversized local files before opening the guest side.
func TestPushSizeLimit(t *testing.T) {
	srv := qgatest.Serve(t, qgatest.Static(nil))
	cl := dial(t, srv.Sock, 2*time.Second)
	f := filepath.Join(t.TempDir(), "big")
	if err := os.WriteFile(f, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := cl.Push(context.Background(), f, "/tmp/x", TransferOpts{MaxBytes: 4})
	if !errors.Is(err, ErrMessageTooBig) {
		t.Fatalf("want ErrMessageTooBig, got %v", err)
	}
}

// guest-file-open rejection surfaces as AgentError.
func TestPushOpenRefused(t *testing.T) {
	srv := qgatest.Serve(t, func(req map[string]any) qgatest.Reply {
		if req["execute"] == "guest-file-open" {
			return qgatest.Reply{Error: &qgatest.ErrorObj{
				Class: "FileOpenFailed", Desc: "denied"}}
		}
		return qgatest.Reply{Return: map[string]any{}}
	})
	cl := dial(t, srv.Sock, 2*time.Second)
	f := filepath.Join(t.TempDir(), "s")
	os.WriteFile(f, []byte("x"), 0o600)
	_, err := cl.Push(context.Background(), f, "/sys/firmware", TransferOpts{})
	var ae *AgentError
	if !errors.As(err, &ae) {
		t.Fatalf("want AgentError, got %v", err)
	}
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
