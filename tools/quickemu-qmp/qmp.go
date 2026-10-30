package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// QMP client: connect -> read greeting -> qmp_capabilities -> command.
// Each response may be interleaved with async {"event":...} records —
// execute echoes them to stderr AND appends them to the VM's
// events.jsonl (durable qe_event surface); watch streams them as JSONL.

var qmpRaw bool // --raw: emit the bare QMP reply line for Bash callers

var qmpSeq int // monotonic command ids for request/response correlation

type qmpConn struct {
	c net.Conn
	r *bufio.Reader
}

func qmpDial(socket string, timeout time.Duration) (*qmpConn, error) {
	d := net.Dialer{Timeout: timeout}
	c, err := d.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	return &qmpConn{c: c, r: bufio.NewReaderSize(c, 1<<20)}, nil
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

// qmpNameRe: QEMU command names are lowercase words joined by '-' or
// '_' — anything else isn't a command identifier and never reaches
// the allowlist.
var qmpNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// qmpAllowedExact are single commands; qmpAllowedPrefix are QEMU
// families (new query-*/block-dirty-bitmap-* members appear per
// release). Names that merely *contain* one of these as a substring
// are denied — matching is on the parsed name, never on the payload.
var qmpAllowedExact = map[string]bool{
	"screendump": true, "system_powerdown": true, "system_wakeup": true,
	"stop": true, "cont": true, "block-dirty-bitmap": true,
	"drive-backup": true, "human-monitor-command": true,
}
var qmpAllowedPrefix = []string{"query-", "block-dirty-bitmap-"}

// qmpCommandAllowed: qmp_command_allowed parity — the structured
// channel is fail-closed to read-only/graceful commands; raw QMP
// requires QUICKEMU_QMP_RAW=1 (same env var as Bash). The check is
// structural: only the QMP envelope keys (execute/arguments/id) may
// be present, and the name must match the allowlist exactly or by
// family prefix — substring inclusion is not authorization.
func qmpCommandAllowed(cmd map[string]any) bool {
	if os.Getenv("QUICKEMU_QMP_RAW") == "1" {
		return true
	}
	for k := range cmd {
		if k != "execute" && k != "arguments" && k != "id" {
			return false
		}
	}
	name, ok := cmd["execute"].(string)
	if !ok || !qmpNameRe.MatchString(name) {
		return false
	}
	if qmpAllowedExact[name] {
		return true
	}
	for _, p := range qmpAllowedPrefix {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func runExecute(socket, command string, timeout time.Duration) int {
	var cmd map[string]any
	if err := json.Unmarshal([]byte(command), &cmd); err != nil {
		return emit("qmp.execute", nil, "QMP_BAD_COMMAND", "command is not valid QMP JSON: "+err.Error())
	}
	if !qmpCommandAllowed(cmd) {
		name, _ := cmd["execute"].(string)
		return emit("qmp.execute", nil, "QMP_DENIED",
			"command '"+name+"' is not on the allowlist (set QUICKEMU_QMP_RAW=1 to send raw QMP)")
	}
	q, err := qmpDial(socket, timeout)
	if err != nil { // one reconnect: a socket briefly not-ready isn't fatal
		time.Sleep(200 * time.Millisecond)
		q, err = qmpDial(socket, timeout)
	}
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
	// find the capabilities reply; async events still land in the log
	for i := 0; i < 64; i++ {
		m, ev, err := q.readMsg()
		if err != nil {
			return emit("qmp.execute", nil, "QMP_CAPABILITIES", err.Error())
		}
		if ev != nil {
			qmpEventSink(socket, ev)
		}
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
			qmpEventSink(socket, ev)
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

// qmpEventSink: async events are durable, not just a stderr echo —
// appended to the VM's <vmdir>/<vm>.events.jsonl (qe_event parity) as
// qemu.<EVENT> with the raw record as data. The socket layout
// <vmdir>/<vm>-qmp.socket carries the identity; a non-VM socket (odd
// basename or unwritable dir) degrades to a silent no-op, matching
// AppendEvent's best-effort contract.
func qmpEventSink(sock string, ev map[string]any) {
	name := strings.TrimSuffix(filepath.Base(sock), "-qmp.socket")
	if name == "" || name == filepath.Base(sock) {
		return // socket not named <vm>-qmp.socket — nothing to attribute to
	}
	typ := "qemu.event"
	if n, ok := ev["event"].(string); ok && n != "" {
		typ = "qemu." + n
	}
	qelib.AppendEvent(filepath.Dir(sock), name, 0, typ, mustJSON(ev))
}
