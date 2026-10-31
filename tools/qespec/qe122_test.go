// qe122_test.go — QE-122: extended fake QMP/QGA wire coverage.
// In-process unix-socket fakes answer real protocol so the shipped
// binaries (quickemu-qmp execute, quickemu guest) run end-to-end.
package qespec

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// specFakeQMP: greeting + caps, then scripted replies keyed on
// "execute"; an async WATCHDOG event precedes every reply (the client
// must skip it).
func specFakeQMP(t *testing.T, replies map[string]map[string]any, log *os.File) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "t.qmp")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				w := bufio.NewWriter(c)
				w.WriteString(`{"QMP":{"version":{"qemu":{"major":8}},"capabilities":[]}}` + "\n")
				w.Flush()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					var req map[string]any
					if json.Unmarshal([]byte(line), &req) != nil {
						continue
					}
					ex, _ := req["execute"].(string)
					if log != nil && ex != "qmp_capabilities" {
						log.WriteString(line)
					}
					w.WriteString(`{"event":"WATCHDOG","timestamp":{"seconds":1,"microseconds":0}}` + "\n")
					if rep, ok := replies[ex]; ok {
						b, _ := json.Marshal(rep)
						w.Write(b)
						w.WriteString("\n")
					} else {
						w.WriteString(`{"return":{}}` + "\n")
					}
					w.Flush()
				}
			}(c)
		}
	}()
	return sock
}

func TestFakeQMPWire(t *testing.T) {
	logF, _ := os.CreateTemp("", "qmp-calls-*.log")
	defer os.Remove(logF.Name())
	defer logF.Close()
	sock := specFakeQMP(t, map[string]map[string]any{
		"query-status":           {"return": map[string]any{"status": "running"}},
		"human-monitor-command":  {"return": "VM m1 saved"},
		"block-dirty-bitmap-add": {"return": map[string]any{}},
	}, logF)
	// allowlisted: query-status surfaces the reply.
	r := Run(t, "quickemu-qmp", nil, "execute", "--socket", sock,
		"--command", `{"execute":"query-status"}`)
	if r.Failed() || !strings.Contains(r.Stdout, "running") {
		t.Fatalf("query-status: %d %s", r.Code, r.Combined())
	}
	// QE-021 path: human-monitor-command is allowlisted — savevm
	// text comes back.
	r = Run(t, "quickemu-qmp", nil, "execute", "--socket", sock,
		"--command", `{"execute":"human-monitor-command","arguments":{"command-line":"savevm m1"}}`)
	if r.Failed() || !strings.Contains(r.Stdout, "VM m1 saved") {
		t.Fatalf("savevm: %d %s", r.Code, r.Combined())
	}
	// QE-102 path: block-dirty-bitmap-* prefix allowed.
	r = Run(t, "quickemu-qmp", nil, "execute", "--socket", sock,
		"--command", `{"execute":"block-dirty-bitmap-add","arguments":{"node":"SystemDisk","name":"qe-incr"}}`)
	if r.Failed() {
		t.Fatalf("bitmap-add: %d %s", r.Code, r.Combined())
	}
	// Non-allowlisted refused client-side (never reaches the fake).
	r = Run(t, "quickemu-qmp", nil, "execute", "--socket", sock,
		"--command", `{"execute":"migrate-incoming"}`)
	if r.OK() {
		t.Fatal("non-allowlisted command passed")
	}
	time.Sleep(100 * time.Millisecond)
	b, _ := os.ReadFile(logF.Name())
	if strings.Contains(string(b), "migrate-incoming") {
		t.Fatal("refused command reached the server")
	}
	if !strings.Contains(string(b), "savevm m1") ||
		!strings.Contains(string(b), "qe-incr") {
		t.Fatalf("server missed commands: %s", b)
	}
}

// specFakeQGA: guest-sync-delimited handshake (0xFF sentinel + token
// echo), then per-command replies.
func specFakeQGA(t *testing.T, replies map[string]any) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "t-agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadBytes('\n')
					if err != nil {
						return
					}
					var req map[string]any
					if json.Unmarshal(bytes2(line), &req) != nil {
						continue
					}
					ex, _ := req["execute"].(string)
					if ex == "guest-sync-delimited" {
						var id any
						if a, ok := req["arguments"].(map[string]any); ok {
							id = a["id"]
						}
						resp, _ := json.Marshal(map[string]any{"return": id})
						c.Write(append([]byte{0xFF}, append(resp, '\n')...))
						continue
					}
					ret, ok := replies[ex]
					if !ok {
						ret = map[string]any{}
					}
					resp, _ := json.Marshal(map[string]any{"return": ret, "id": req["id"]})
					c.Write(append(resp, '\n'))
				}
			}(c)
		}
	}()
	return sock
}

func bytes2(b []byte) []byte {
	// Strip stray 0xFF sync sentinels an agent may emit mid-stream.
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c != 0xFF {
			out = append(out, c)
		}
	}
	return out
}

func TestFakeQGAWireGuestPing(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "vm"), 0o755)
	conf := filepath.Join(dir, "vm.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o644)
	// guest ping requires a running VM — fake one (live sleep named
	// after the VM + pidfile in the vm dir).
	SpawnFakeVM(t, filepath.Join(dir, "vm"), "vm")
	// The agent socket must live at <vmdir>/<vm>-agent.sock.
	sock := filepath.Join(dir, "vm", "vm-agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadBytes('\n')
					if err != nil {
						return
					}
					var req map[string]any
					if json.Unmarshal(bytes2(line), &req) != nil {
						continue
					}
					ex, _ := req["execute"].(string)
					var resp []byte
					switch ex {
					case "guest-sync-delimited":
						var id any
						if a, ok := req["arguments"].(map[string]any); ok {
							id = a["id"]
						}
						resp, _ = json.Marshal(map[string]any{"return": id})
						c.Write(append([]byte{0xFF}, append(resp, '\n')...))
						continue
					case "guest-ping":
						resp, _ = json.Marshal(map[string]any{"return": map[string]any{}, "id": req["id"]})
					case "guest-get-osinfo":
						resp, _ = json.Marshal(map[string]any{"return": map[string]any{
							"pretty-name": "FakeGuestOS 1.0", "name": "fake"}, "id": req["id"]})
					default:
						resp, _ = json.Marshal(map[string]any{"return": map[string]any{}, "id": req["id"]})
					}
					c.Write(append(resp, '\n'))
				}
			}(c)
		}
	}()
	r := Run(t, "quickemu", nil, "guest", "ping", "--vm", conf)
	if r.Failed() || !strings.Contains(r.Stdout, "alive") && !strings.Contains(r.Combined(), "alive") {
		t.Fatalf("guest ping: %d %s", r.Code, r.Combined())
	}
}
