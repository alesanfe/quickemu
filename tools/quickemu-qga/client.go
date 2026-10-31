// Package qga is the typed QEMU Guest Agent client shared by the
// quickemu guest-* verbs: JSON-framed requests over the virtio-serial
// unix socket the launcher emits as <vm>-agent.sock.
//
// Protocol (QGA is QMP-shaped without the capabilities handshake):
// each request is one line {"execute":..,"arguments":{..},"id":N} and
// each reply is one JSON object carrying "return" or "error". Channel
// sync is established once at dial time via guest-sync-delimited,
// whose reply is prefixed with a 0xFF sentinel byte; stray 0xFF bytes
// are tolerated on every subsequent message.
package qga

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// Classification sentinels — callers (and the CLI error table) use
// errors.Is to tell WHY the agent channel failed instead of matching
// message text:
//
//	ErrSocketMissing   the <vm>-agent.sock file does not exist
//	ErrAgentUnreachable the socket exists but connect/write failed
//	ErrAgentNotReady   the channel is up but the agent never answered
//	ErrBadResponse     malformed framing/JSON/oversized message
//	ErrAgentGone       the connection died mid-command
//	ErrTimeout         the caller's deadline/cancel fired
//	ErrMessageTooBig   a single reply exceeded MaxMessage bytes
var (
	ErrSocketMissing    = errors.New("qga: agent socket does not exist")
	ErrAgentUnreachable = errors.New("qga: agent channel unreachable")
	ErrAgentNotReady    = errors.New("qga: agent not responding")
	ErrBadResponse      = errors.New("qga: invalid response")
	ErrAgentGone        = errors.New("qga: agent connection lost")
	ErrTimeout          = errors.New("qga: command timed out")
	ErrMessageTooBig    = errors.New("qga: message exceeds size limit")
)

// AgentError is a failure reply from the agent itself — the guest
// answered but rejected the command ({"error":{"class","desc"}}).
type AgentError struct {
	Class string `json:"class"`
	Desc  string `json:"desc"`
}

func (e *AgentError) Error() string {
	return fmt.Sprintf("qga: %s: %s", e.Class, e.Desc)
}

// MaxMessage bounds a single reply (default 4MiB, matching the
// protocol's practical output ceiling for guest-exec/file reads).
const MaxMessage = 4 << 20

// Client is a serialized QGA channel: Execute holds a mutex for the
// whole request/reply pair, so a response can never be attributed to
// a different caller — the channel itself serialises anyway (QGA
// processes commands in order), and the id correlation below guards
// against stale/stray replies from earlier exchanges.
type Client struct {
	c       net.Conn
	r       *bufio.Reader
	mu      sync.Mutex
	seq     uint64
	max     int
	timeout time.Duration
}

// Dial opens the agent socket and performs the guest-sync-delimited
// handshake: a random token round-trips, flushing any garbage the
// agent buffered before we connected and proving the far end is a
// live qemu-ga (not a bare socket that accepts and stalls).
func Dial(socket string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	st, err := os.Stat(socket)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSocketMissing, socket)
	}
	if st.Mode()&os.ModeSocket == 0 {
		return nil, fmt.Errorf("%w: %s is not a socket", ErrBadResponse, socket)
	}
	d := net.Dialer{Timeout: timeout}
	c, err := d.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgentUnreachable, err)
	}
	cl := &Client{
		c:       c,
		r:       bufio.NewReaderSize(c, 64<<10),
		max:     MaxMessage,
		timeout: timeout,
	}
	if err := cl.sync(); err != nil {
		c.Close()
		return nil, err
	}
	return cl, nil
}

// sync sends guest-sync-delimited and waits for the 0xFF-prefixed
// reply carrying our token back. The agent emits 0xFF before this one
// reply so the client can resynchronise after partial writes.
func (cl *Client) sync() error {
	// The token round-trips as a JSON number — keep it under 2^53 so
	// the agent's float64 reply compares exactly.
	token := uint64(time.Now().UnixNano()%(1<<50)) | 1
	if err := cl.writeReq("guest-sync-delimited", map[string]any{"id": token}, nil); err != nil {
		return fmt.Errorf("%w: %v", ErrAgentUnreachable, err)
	}
	cl.c.SetDeadline(time.Now().Add(cl.timeout))
	m, err := cl.readMsg()
	if err != nil {
		return cl.dialErr(err)
	}
	ret, ok := m["return"]
	if !ok {
		if ae := agentErr(m); ae != nil {
			return ae
		}
		return fmt.Errorf("%w: sync reply has no return value", ErrBadResponse)
	}
	var got uint64
	switch v := ret.(type) {
	case float64:
		got = uint64(v)
	case json.Number:
		n, _ := v.Int64()
		got = uint64(n)
	}
	if got != token {
		return fmt.Errorf("%w: sync token mismatch", ErrBadResponse)
	}
	return nil
}

// dialErr classifies a failure observed during the handshake: a read
// timeout means the agent is not answering (absent/hung qemu-ga),
// anything else is a broken channel.
func (cl *Client) dialErr(err error) error {
	if isTimeout(err) {
		return fmt.Errorf("%w (is qemu-guest-agent installed and running?)", ErrAgentNotReady)
	}
	var ae *AgentError
	if errors.As(err, &ae) {
		return ae
	}
	if errors.Is(err, ErrBadResponse) || errors.Is(err, ErrMessageTooBig) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrAgentUnreachable, err)
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// writeReq marshals and sends one request line. arguments may be nil.
func (cl *Client) writeReq(cmd string, args map[string]any, id any) error {
	req := map[string]any{"execute": cmd}
	if args != nil {
		req["arguments"] = args
	}
	if id != nil {
		req["id"] = id
	}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = cl.c.Write(append(b, '\n'))
	return err
}

// readMsg reads one JSON object, skipping leading 0xFF sync sentinels
// and non-JSON noise, and enforcing the per-message size cap. QGA
// messages are newline-delimited but a reply may arrive fragmented
// across reads — ReadSlice fragments are reassembled into a bounded
// accumulator so a peer that never sends '\n' cannot grow memory past
// the message cap.
func (cl *Client) readMsg() (map[string]any, error) {
	var acc []byte
	for {
		frag, err := cl.r.ReadSlice('\n')
		if len(frag) > 0 {
			if int64(len(acc))+int64(len(frag)) > int64(cl.max) {
				return nil, fmt.Errorf("%w (>%d bytes)", ErrMessageTooBig, cl.max)
			}
			acc = append(acc, frag...)
		}
		if err == bufio.ErrBufferFull {
			continue // fragment, not a whole line — accumulate
		}
		if err != nil {
			return nil, err
		}
		line := trimQGA(acc)
		acc = nil
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if json.Unmarshal(line, &m) != nil {
			continue // tolerate a stray non-JSON fragment between records
		}
		return m, nil
	}
}

// trimQGA strips the line terminator and any 0xFF sync sentinels an
// agent may emit before a record (qemu-ga sends 0xFF after a
// guest-sync-delimited; defensive readers tolerate it anywhere).
func trimQGA(b []byte) []byte {
	for len(b) > 0 {
		c := b[0]
		if c == '\n' || c == '\r' || c == 0xFF || c == ' ' || c == '\t' {
			b = b[1:]
			continue
		}
		break
	}
	for len(b) > 0 {
		c := b[len(b)-1]
		if c == '\n' || c == '\r' || c == 0xFF || c == ' ' || c == '\t' {
			b = b[:len(b)-1]
			continue
		}
		break
	}
	return b
}

// agentErr extracts an {"error":{class,desc}} record.
func agentErr(m map[string]any) *AgentError {
	e, ok := m["error"]
	if !ok {
		return nil
	}
	b, _ := json.Marshal(e)
	var ae AgentError
	if json.Unmarshal(b, &ae) != nil {
		return &AgentError{Class: "unknown", Desc: string(b)}
	}
	return &ae
}

// Execute sends one command and returns its "return" payload.
// Requests are serialized: the mutex is held from write until the
// matching reply id arrives, so concurrent callers queue rather than
// interleave — and a reply bearing a different id (stale, or an async
// record) is skipped instead of being handed to the wrong caller.
func (cl *Client) Execute(ctx context.Context, cmd string, args map[string]any) (json.RawMessage, error) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	cl.seq++
	id := fmt.Sprintf("qga-%d", cl.seq)
	if err := cl.writeReq(cmd, args, id); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgentUnreachable, err)
	}

	deadline := time.Now().Add(cl.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	cl.c.SetDeadline(deadline)
	if ctx.Done() != nil {
		stop := context.AfterFunc(ctx, func() {
			cl.c.SetDeadline(time.Now()) // force blocked Read to fail
		})
		defer stop()
	}
	for skipped := 0; skipped < 64; skipped++ {
		m, err := cl.readMsg()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w: %v", ErrTimeout, ctx.Err())
			default:
			}
			if isTimeout(err) {
				return nil, fmt.Errorf("%w: %s", ErrTimeout, cmd)
			}
			if errors.Is(err, ErrMessageTooBig) || errors.Is(err, ErrBadResponse) {
				return nil, err
			}
			return nil, fmt.Errorf("%w: %v", ErrAgentGone, err)
		}
		if rid, ok := m["id"]; ok {
			if rs, _ := rid.(string); rs != id {
				continue // reply for a different request — not ours
			}
		} else {
			continue // unsolicited record (event/async) — not a reply
		}
		if ae := agentErr(m); ae != nil {
			return nil, ae
		}
		ret, ok := m["return"]
		if !ok {
			return nil, fmt.Errorf("%w: reply to %s has no return value", ErrBadResponse, cmd)
		}
		b, err := json.Marshal(ret)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadResponse, err)
		}
		return json.RawMessage(b), nil
	}
	return nil, fmt.Errorf("%w: %s exhausted reply window", ErrBadResponse, cmd)
}

// Close shuts the channel.
func (cl *Client) Close() error { return cl.c.Close() }
