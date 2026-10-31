// Package qgatest provides a scripted fake QEMU guest agent over a
// unix socket for the quickemu test suites — the same fixture feeds
// the qga client tests and the quickemu-cli guest-* verb tests.
//
// The server speaks real protocol: newline-delimited JSON requests,
// the guest-sync-delimited handshake (0xFF sentinel + echoed token),
// and per-command replies produced by the test's Handler. Reply
// knobs cover the failure modes the client must survive: fragmented
// writes, stale/extra records, oversized payloads, silence, and
// mid-command disconnects.
package qgatest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// ErrorObj is a QGA {"error":...} body.
type ErrorObj struct {
	Class string `json:"class"`
	Desc  string `json:"desc"`
}

// Reply describes what the server sends back for one request.
type Reply struct {
	// Return is marshalled into {"return":X,"id":<req id>}.
	Return any
	// Error, when non-nil, produces {"error":{class,desc}}.
	Error *ErrorObj
	// Raw is written verbatim instead of a marshalled record —
	// for malformed JSON, garbage, or an oversized payload.
	Raw []byte
	// Before are records written BEFORE the reply (stale ids,
	// out-of-order replies, async records the client must skip).
	Before []map[string]any
	// Split writes the reply in N fragments with a small delay —
	// the client must reassemble across reads.
	Split int
	// Delay stalls the reply.
	Delay time.Duration
	// Silent drops the request entirely (timeout testing).
	Silent bool
	// Close hangs up after replying (or immediately when Silent).
	Close bool
}

// Handler maps one decoded request to a Reply.
type Handler func(req map[string]any) Reply

// Server is a running fake agent.
type Server struct {
	Sock string
	ln   net.Listener
	mu   sync.Mutex
	reqs []map[string]any
	done chan struct{}
}

// Serve starts a fake agent on a fresh unix socket inside
// t.TempDir() and answers every request with h. The listener stays
// up for sequential reconnects (the client's Dial handshake runs per
// connection).
func Serve(t *testing.T, h Handler) *Server {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "qga.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	s := &Server{Sock: sock, ln: ln, done: make(chan struct{})}
	go s.acceptLoop(h)
	t.Cleanup(s.Close)
	return s
}

// Static returns a Handler that always answers replies[cmd] (or an
// empty return when the command is not scripted).
func Static(replies map[string]any) Handler {
	return func(req map[string]any) Reply {
		if v, ok := replies[req["execute"].(string)]; ok {
			return Reply{Return: v}
		}
		return Reply{Return: map[string]any{}}
	}
}

func (s *Server) acceptLoop(h Handler) {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.serve(c, h)
	}
}

// serve handles one connection: handshake replies are automatic,
// everything else goes through the handler.
func (s *Server) serve(c net.Conn, h Handler) {
	defer c.Close()
	r := bufio.NewReaderSize(c, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		var req map[string]any
		if json.Unmarshal(bytes.TrimSpace(line), &req) != nil {
			continue
		}
		s.mu.Lock()
		s.reqs = append(s.reqs, req)
		s.mu.Unlock()
		if req["execute"] == "guest-sync-delimited" {
			// 0xFF sentinel + echo of the client's token.
			var tok any
			if a, ok := req["arguments"].(map[string]any); ok {
				tok = a["id"]
			}
			resp, _ := json.Marshal(map[string]any{"return": tok})
			if _, err := c.Write(append([]byte{0xFF}, append(resp, '\n')...)); err != nil {
				return
			}
			continue
		}
		rep := h(req)
		if rep.Delay > 0 {
			select {
			case <-time.After(rep.Delay):
			case <-s.done:
				return
			}
		}
		if rep.Close && rep.Silent {
			return
		}
		if !rep.Silent {
			for _, b := range rep.Before {
				m, _ := json.Marshal(b)
				if _, err := c.Write(append(m, '\n')); err != nil {
					return
				}
			}
			out := rep.Raw
			if out == nil {
				var m map[string]any
				if rep.Error != nil {
					m = map[string]any{"error": rep.Error}
				} else {
					m = map[string]any{"return": rep.Return}
				}
				if id, ok := req["id"]; ok {
					m["id"] = id
				}
				out, _ = json.Marshal(m)
				out = append(out, '\n')
			}
			if rep.Split > 1 {
				n := rep.Split
				for i := 0; i < n; i++ {
					if writeChunked(c, out, i, n) {
						return
					}
					time.Sleep(5 * time.Millisecond)
				}
			} else if _, err := c.Write(out); err != nil {
				return
			}
		}
		if rep.Close {
			return
		}
	}
}

// writeChunked writes the i-th of n roughly-equal fragments.
// Returns true when the write failed (connection gone).
func writeChunked(c net.Conn, out []byte, i, n int) bool {
	start := len(out) * i / n
	end := len(out) * (i + 1) / n
	_, err := c.Write(out[start:end])
	return err != nil
}

// Reqs returns the decoded requests received so far, in order.
func (s *Server) Reqs() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, len(s.reqs))
	copy(out, s.reqs)
	return out
}

// Cmds returns just the execute names received, in order.
func (s *Server) Cmds() []string {
	var out []string
	for _, r := range s.Reqs() {
		if e, ok := r["execute"].(string); ok {
			out = append(out, e)
		}
	}
	return out
}

// Close stops the listener and any delayed handler replies.
func (s *Server) Close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	s.ln.Close()
}

// SocketPath returns a free socket path (no listener) — for tests
// that need a nonexistent or manually-controlled endpoint.
func SocketPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "agent.sock")
}

// OccupiedSocket creates a unix socket file no agent answers on —
// the channel exists but every command stalls (agent-not-ready).
func OccupiedSocket(t *testing.T) string {
	t.Helper()
	sock := SocketPath(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return sock
}
