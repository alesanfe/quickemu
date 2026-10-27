package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// QMP client: connect -> read greeting -> qmp_capabilities -> command.
// Each response may be interleaved with async {"event":...} records;
// execute skips them, watch emits them.

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

// qmpCommandAllowed: qmp_command_allowed parity — the structured
// channel is fail-closed to read-only/graceful commands; raw QMP
// requires QUICKEMU_QMP_RAW=1 (same env var as Bash).
func qmpCommandAllowed(name string) bool {
	if os.Getenv("QUICKEMU_QMP_RAW") == "1" {
		return true
	}
	for _, p := range []string{"query-", "screendump", "system_powerdown",
		"system_wakeup", "stop", "cont", "block-dirty-bitmap",
		"drive-backup", "human-monitor-command"} {
		if strings.Contains(name, p) {
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
	if name, _ := cmd["execute"].(string); name != "" && !qmpCommandAllowed(name) {
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

// SocketReady mirrors vm_monitor_socket_ready: socket exists, is a unix
// socket, is not a symlink, and is owned by the current user.
func SocketReady(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return false
	}
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		return stat.Uid == uint32(os.Getuid())
	}
	return true
}

// SendCmd mirrors monitor_send_cmd: writes a raw command to the monitor
// socket (not QMP) and returns the reply.
func SendCmd(path, cmd string) (string, error) {
	conn, err := net.DialTimeout("unix", path, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(conn, "%s\n", cmd)
	conn.(*net.UnixConn).CloseWrite()
	b, err := io.ReadAll(conn)
	return string(b), err
}

// HotplugUSB mirrors vm_hotplug_usb: QMP device_add usb-host with hex
// vendor:product parsing. Returns the device id on success.
func HotplugUSB(sock, vidpid string) (string, error) {
	parts := strings.SplitN(vidpid, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("need vendor:product hex (e.g. 05ac:12a8)")
	}
	v, err1 := strconv.ParseUint(parts[0], 16, 32)
	p, err2 := strconv.ParseUint(parts[1], 16, 32)
	if err1 != nil || err2 != nil || v == 0 || p == 0 {
		return "", fmt.Errorf("bad vendor:product %q", vidpid)
	}
	id := "hotusb" + parts[0] + parts[1]
	cmd := fmt.Sprintf(`{"execute":"device_add","arguments":{"driver":"usb-host","id":"%s","hostvendor":%d,"hostport":%d}}`, id, v, p)
	if rc := runExecute(sock, cmd, 5*time.Second); rc != 0 {
		return "", fmt.Errorf("device_add failed (rc=%d)", rc)
	}
	return id, nil
}

// HotplugDisk mirrors vm_hotplug_disk: blockdev-add the file then
// device_add virtio-blk-pci with the node name.
func HotplugDisk(sock, file string) (string, error) {
	if _, err := os.Stat(file); err != nil {
		return "", err
	}
	id := "hotdisk" + regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(filepath.Base(file), "")
	abs, _ := filepath.Abs(file)
	node := id + "node"
	// blockdev-add first (registers the file), then device_add references it
	q := fmt.Sprintf(`{"execute":"blockdev-add","arguments":{"node-name":"%s","driver":"qcow2","file":{"driver":"file","filename":%q}}}`, node, abs)
	if rc := runExecute(sock, q, 5*time.Second); rc != 0 {
		return "", fmt.Errorf("blockdev-add failed (rc=%d)", rc)
	}
	dev := fmt.Sprintf(`{"execute":"device_add","arguments":{"driver":"virtio-blk-pci","id":"%s","drive":"%s"}}`, id, node)
	if rc := runExecute(sock, dev, 5*time.Second); rc != 0 {
		return "", fmt.Errorf("device_add failed (rc=%d)", rc)
	}
	return id, nil
}

// Hotunplug mirrors vm_hotunplug: device_del by id.
func Hotunplug(sock, devid string) error {
	if rc := runExecute(sock, fmt.Sprintf(`{"execute":"device_del","arguments":{"id":%q}}`, devid), 5*time.Second); rc != 0 {
		return fmt.Errorf("device_del failed (rc=%d)", rc)
	}
	return nil
}

// SendKeys mirrors send_keys/vm_send_key_combo: sends a key combo via
// the human monitor ("sendkey ctrl-alt-f" style).
func SendKeys(monSock, combo string) error {
	if combo == "" {
		return fmt.Errorf("empty combo")
	}
	// normalize "ctrl+alt+f1" -> "ctrl-alt-f1" (monitor syntax)
	key := strings.ReplaceAll(combo, "+", "-")
	_, err := SendCmd(monSock, "sendkey "+key)
	return err
}

// TypeText mirrors type_text: sendkey each character (with shift for
// uppercase/punct) at ~30cps, matching the Bash pacing.
func TypeText(monSock, text string) error {
	for _, ch := range text {
		key := ""
		switch ch {
		case ' ':
			key = "spc"
		case '\n':
			key = "ret"
		case '\t':
			key = "tab"
		case '.':
			key = "dot"
		case ',':
			key = "comma"
		case '-':
			key = "minus"
		case '=':
			key = "equal"
		case '/':
			key = "slash"
		case ':':
			key = "shift-semicolon"
		default:
			if ch >= 'A' && ch <= 'Z' {
				key = "shift-" + string(ch+32)
			} else if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
				key = string(ch)
			}
		}
		if key == "" {
			continue
		}
		if _, err := SendCmd(monSock, "sendkey "+key); err != nil {
			return err
		}
		time.Sleep(33 * time.Millisecond) // ~30 chars/sec
	}
	return nil
}

// Screenshot mirrors screenshot_vm: QMP screendump to a PPM file.
func Screenshot(sock, out string) error {
	q := fmt.Sprintf(`{"execute":"screendump","arguments":{"filename":%q}}`, out)
	if rc := runExecute(sock, q, 5*time.Second); rc != 0 {
		return fmt.Errorf("screendump failed")
	}
	return nil
}

// HotplugPersist: hotplug_persist parity — appends a hotplugged device
// to the VM config so it survives reboots. .conf gets bash-array
// syntax, .toml gets TOML arrays; creates the key if absent.
func HotplugPersist(confPath, kind, val string) error {
	key := "extra_disks"
	if kind == "usb" {
		key = "usb_devices"
	}
	b, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}
	s := string(b)
	if strings.HasSuffix(confPath, ".toml") {
		re := regexp.MustCompile(`(?m)^` + key + `\s*=\s*\[(.*)\]`)
		if m := re.FindStringSubmatch(s); m != nil {
			inner := strings.TrimSpace(m[1])
			rep := `, "` + val + `"]`
			if inner == "" {
				rep = `"` + val + `"]`
			}
			s = re.ReplaceAllString(s, key+` = [`+inner+rep)
		} else {
			s += key + ` = ["` + val + `"]` + "\n"
		}
	} else {
		re := regexp.MustCompile(key + `=\((.*)\)`)
		if m := re.FindStringSubmatch(s); m != nil {
			inner := m[1]
			s = re.ReplaceAllString(s, key+`=(`+inner+` "`+val+`")`)
		} else {
			s += key + `=("` + val + `")` + "\n"
		}
	}
	return os.WriteFile(confPath, []byte(s), 0o644)
}
