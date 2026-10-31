// qgatest_test.go: the fake agent's own contract — handshake bytes,
// every Reply knob on the wire, request logging and lifecycle.
// These tests keep `go test -coverprofile ./...` honest: without a
// test file this package would count 0% toward combined coverage.
package qgatest

import (
	"bufio"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// wire dials the fake socket and returns a conn + reader with a
// test-friendly deadline.
func wire(t *testing.T, sock string) (net.Conn, *bufio.Reader) {
	t.Helper()
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial %s: %v", sock, err)
	}
	if err := c.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}
	return c, bufio.NewReader(c)
}

func send(t *testing.T, c net.Conn, line string) {
	t.Helper()
	if _, err := c.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return line
}

func handshake(t *testing.T, c net.Conn, r *bufio.Reader) {
	t.Helper()
	send(t, c, `{"execute":"guest-sync-delimited","arguments":{"id":4242}}`)
	line := readLine(t, r)
	if line[0] != 0xFF {
		t.Fatalf("missing 0xFF sync sentinel: %q", line)
	}
	if !strings.Contains(line, `"return":4242`) {
		t.Fatalf("token not echoed: %s", line)
	}
}

// The handshake echoes the caller's token behind a 0xFF sentinel,
// Static answers scripted commands and falls back to an empty
// return, and request logging sees everything.
func TestServeHandshakeAndStatic(t *testing.T) {
	srv := Serve(t, Static(map[string]any{"guest-ping": "pong"}))
	c, r := wire(t, srv.Sock)
	defer c.Close()
	handshake(t, c, r)

	send(t, c, `{"execute":"guest-ping","id":"a"}`)
	if line := readLine(t, r); !strings.Contains(line, `"return":"pong"`) ||
		!strings.Contains(line, `"id":"a"`) {
		t.Fatalf("scripted reply wrong: %s", line)
	}
	send(t, c, `{"execute":"unscripted","id":"b"}`)
	if line := readLine(t, r); !strings.Contains(line, `"return":{}`) {
		t.Fatalf("unscripted fallback wrong: %s", line)
	}
	// Non-JSON noise is skipped, the next real request still parses.
	send(t, c, "this is not json")
	send(t, c, `{"execute":"guest-ping","id":"c"}`)
	if line := readLine(t, r); !strings.Contains(line, `"id":"c"`) {
		t.Fatalf("request after garbage lost: %s", line)
	}

	want := []string{"guest-sync-delimited", "guest-ping", "unscripted", "guest-ping"}
	if cmds := srv.Cmds(); !reflect.DeepEqual(cmds, want) {
		t.Fatalf("Cmds %v want %v", cmds, want)
	}
	if reqs := srv.Reqs(); len(reqs) != 4 || reqs[1]["id"] != "a" {
		t.Fatalf("Reqs %+v", reqs)
	}
}

// Every Reply field lands on the wire as documented.
func TestServeReplyShapes(t *testing.T) {
	srv := Serve(t, func(req map[string]any) Reply {
		switch req["execute"] {
		case "err":
			return Reply{Error: &ErrorObj{Class: "Klass", Desc: "why"}}
		case "raw":
			return Reply{Raw: []byte("RAW-LINE\n")}
		case "before":
			return Reply{Return: 1, Before: []map[string]any{
				{"event": "x"}, {"id": "stale", "return": 0}}}
		case "split":
			return Reply{Return: map[string]any{
				"k": strings.Repeat("v", 300)}, Split: 4}
		case "delay":
			return Reply{Return: 2, Delay: 60 * time.Millisecond}
		case "silent":
			return Reply{Silent: true}
		case "bye":
			return Reply{Return: 3, Close: true}
		}
		return Reply{}
	})
	c, r := wire(t, srv.Sock)
	defer c.Close()
	handshake(t, c, r)

	send(t, c, `{"execute":"err","id":"e1"}`)
	if line := readLine(t, r); !strings.Contains(line, `"class":"Klass"`) ||
		!strings.Contains(line, `"id":"e1"`) {
		t.Fatalf("error reply: %s", line)
	}
	send(t, c, `{"execute":"raw","id":"e2"}`)
	if line := readLine(t, r); line != "RAW-LINE\n" {
		t.Fatalf("raw verbatim: %q", line)
	}
	send(t, c, `{"execute":"before","id":"e3"}`)
	if l1, l2, l3 := readLine(t, r), readLine(t, r), readLine(t, r); !strings.Contains(l1, `"event"`) ||
		!strings.Contains(l2, `"stale"`) || !strings.Contains(l3, `"id":"e3"`) {
		t.Fatalf("before records: %q %q %q", l1, l2, l3)
	}
	send(t, c, `{"execute":"split","id":"e4"}`)
	if line := readLine(t, r); !strings.Contains(line, strings.Repeat("v", 100)) {
		t.Fatalf("split reply not reassembled: %q", line[:80])
	}
	start := time.Now()
	send(t, c, `{"execute":"delay","id":"e5"}`)
	readLine(t, r)
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("delayed reply arrived early")
	}
	// Silent drops the request entirely; the channel stays usable.
	send(t, c, `{"execute":"silent","id":"e6"}`)
	send(t, c, `{"execute":"err","id":"e7"}`)
	if line := readLine(t, r); !strings.Contains(line, `"id":"e7"`) {
		t.Fatalf("post-silent reply: %s", line)
	}
	// Close replies then hangs up.
	send(t, c, `{"execute":"bye","id":"e8"}`)
	if line := readLine(t, r); !strings.Contains(line, `"return":3`) {
		t.Fatalf("close reply: %s", line)
	}
	if _, err := r.ReadString('\n'); err == nil {
		t.Fatal("connection still open after Close reply")
	}
}

// Silent+Close hangs up without answering.
func TestServeSilentClose(t *testing.T) {
	srv := Serve(t, func(req map[string]any) Reply {
		return Reply{Silent: true, Close: true}
	})
	c, r := wire(t, srv.Sock)
	defer c.Close()
	handshake(t, c, r)
	send(t, c, `{"execute":"guest-ping","id":"x"}`)
	if _, err := r.ReadString('\n'); err == nil {
		t.Fatal("silent-close answered or stayed open")
	}
}

// Server shutdown aborts a pending delayed reply — the connection
// dies instead of the handler leaking.
func TestServeDelayAbortedByClose(t *testing.T) {
	srv := Serve(t, func(req map[string]any) Reply {
		return Reply{Return: 1, Delay: 5 * time.Second}
	})
	c, r := wire(t, srv.Sock)
	defer c.Close()
	handshake(t, c, r)
	send(t, c, `{"execute":"slow","id":"s"}`)
	time.Sleep(60 * time.Millisecond) // let the handler reach its Delay
	srv.Close()                       // t.Cleanup's second Close hits the done-arm
	if _, err := r.ReadString('\n'); err == nil {
		t.Fatal("conn survived server Close")
	}
}

// A client that vanishes mid-exchange just ends the handler — no
// panic, no leak. Best-effort exercise of the write-error returns.
func TestServeClientHangup(t *testing.T) {
	srv := Serve(t, func(req map[string]any) Reply {
		return Reply{Return: strings.Repeat("v", 4096),
			Before: []map[string]any{{"a": 1}}, Split: 5}
	})
	c, _ := wire(t, srv.Sock)
	send(t, c, `{"execute":"guest-sync-delimited","arguments":{"id":1}}`)
	send(t, c, `{"execute":"big","id":"h"}`)
	c.Close() // do not read — writes race the hangup
	time.Sleep(30 * time.Millisecond)
	_ = srv // nothing to assert: surviving without panic is the test
}

// A listener nobody serves: connects succeed, everything stalls.
func TestOccupiedSocketStalls(t *testing.T) {
	sock := OccupiedSocket(t)
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("occupied socket refused connect: %v", err)
	}
	defer c.Close()
	if _, err := c.Write([]byte(`{"execute":"guest-ping"}` + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := c.SetReadDeadline(time.Now().Add(80 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(c).ReadString('\n'); err == nil {
		t.Fatal("occupied socket answered")
	}
}

// SocketPath hands out a free path inside a fresh tempdir.
func TestSocketPathUnused(t *testing.T) {
	p := SocketPath(t)
	if p == "" {
		t.Fatal("empty socket path")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("SocketPath already exists: %v", err)
	}
}

// Close is idempotent — cleanup calls it again after the test.
func TestServerCloseIdempotent(t *testing.T) {
	srv := Serve(t, Static(nil))
	srv.Close()
	srv.Close() // second close must not panic
}
