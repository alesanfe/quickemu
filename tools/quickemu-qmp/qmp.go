package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// QMP client: connect -> read greeting -> qmp_capabilities -> command.
// Each response may be interleaved with async {"event":...} records;
// execute skips them, watch emits them.

var qmpRaw bool // --raw: emit the bare QMP reply line for Bash callers

var qmpSeq int // monotonic command ids for request/response correlation

type qmpConn struct {
	c   net.Conn
	r   *bufio.Reader
}

func qmpDial(socket string, timeout time.Duration) (*qmpConn, error) {
	d := net.Dialer{Timeout: timeout}
	c, err := d.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	return &qmpConn{c: c, r: bufio.NewReader(c)}, nil
}

// readMsg returns the next JSON object; async events are reported via
// the 'event' out-param and not returned.
func (q *qmpConn) readMsg() (map[string]any, map[string]any, error) {
	for {
		line, err := q.r.ReadString('\n')
		if err != nil {
			return nil, nil, err
		}
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue // tolerate blank/garbage lines
		}
		if _, isEvent := m["event"]; isEvent {
			return nil, m, nil // whole record is the async event
		}
		return m, nil, nil
	}
}

func (q *qmpConn) write(cmd map[string]any) error {
	b, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	_, err = q.c.Write(append(b, '\n'))
	return err
}

func runExecute(socket, command string, timeout time.Duration) int {
	var cmd map[string]any
	if err := json.Unmarshal([]byte(command), &cmd); err != nil {
		return emit("qmp.execute", nil, "QMP_BAD_COMMAND", "command is not valid QMP JSON: "+err.Error())
	}
	q, err := qmpDial(socket, timeout)
	if err != nil {
		return emit("qmp.execute", nil, "QMP_UNAVAILABLE", err.Error())
	}
	defer q.c.Close()
	q.c.SetDeadline(time.Now().Add(timeout))

	// greeting
	if _, _, err := q.readMsg(); err != nil {
		return emit("qmp.execute", nil, "QMP_GREETING", err.Error())
	}
	// capabilities negotiation
	if err := q.write(map[string]any{"execute": "qmp_capabilities"}); err != nil {
		return emit("qmp.execute", nil, "QMP_CAPABILITIES", err.Error())
	}
	// find the capabilities reply, skipping async events
	for i := 0; i < 64; i++ {
		m, ev, err := q.readMsg()
		if err != nil {
			return emit("qmp.execute", nil, "QMP_CAPABILITIES", err.Error())
		}
		_ = ev
		if m != nil {
			if e, ok := m["error"]; ok {
				b, _ := json.Marshal(e)
				return emit("qmp.execute", nil, "QMP_CAPABILITIES", string(b))
			}
			break
		}
	}
	// the actual command — attach a correlation id so an interleaved
	// async event or a stale reply can never be mistaken for ours.
	qmpSeq++
	wantID := fmt.Sprintf("qe-%d", qmpSeq)
	cmd["id"] = wantID
	if err := q.write(cmd); err != nil {
		return emit("qmp.execute", nil, "QMP_WRITE", err.Error())
	}
	for i := 0; i < 1024; i++ {
		m, ev, err := q.readMsg()
		if err != nil {
			return emit("qmp.execute", nil, "QMP_TIMEOUT", err.Error())
		}
		if ev != nil {
			fmt.Fprintf(os.Stderr, "qmp event: %s\n", mustJSON(ev))
			continue // strictly separated from the response stream
		}
		if m == nil {
			continue
		}
		// correlate: a reply bearing a different id is a stale/async
		// artifact — skip it instead of desynchronising the channel.
		if rid, ok := m["id"]; ok && rid != wantID {
			continue
		}
		if e, ok := m["error"]; ok {
			if qmpRaw {
				fmt.Println(mustJSON(map[string]any{"error": e}))
				return 1
			}
			return emit("qmp.execute", nil, "QMP_ERROR", mustJSON(e))
		}
		if qmpRaw {
			fmt.Println(mustJSON(map[string]any{"return": m["return"]}))
			return 0
		}
		return emit("qmp.execute", m["return"], "", "")
	}
	return emit("qmp.execute", nil, "QMP_TIMEOUT", "no reply within event window")
}

func runWatch(socket string) int {
	q, err := qmpDial(socket, 10*time.Second)
	if err != nil {
		return emit("qmp.watch", nil, "QMP_UNAVAILABLE", err.Error())
	}
	defer q.c.Close()
	if _, _, err := q.readMsg(); err != nil {
		return emit("qmp.watch", nil, "QMP_GREETING", err.Error())
	}
	if err := q.write(map[string]any{"execute": "qmp_capabilities"}); err != nil {
		return emit("qmp.watch", nil, "QMP_CAPABILITIES", err.Error())
	}
	// stream events as JSONL on stdout forever
	for {
		line, err := q.r.ReadBytes('\n')
		if err != nil {
			return emit("qmp.watch", nil, "QMP_EOF", err.Error())
		}
		var m map[string]any
		if json.Unmarshal(line, &m) == nil {
			if _, isEvent := m["event"]; isEvent {
				os.Stdout.Write(line)
			}
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
