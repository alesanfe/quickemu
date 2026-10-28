package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// qmpRaw sends a privileged QMP command from a trusted internal flow
// (hotplug), bypassing the user-channel allowlist — qmp_query_raw
// parity.
func qmpRaw(payload string) (string, error) {
	return toolEnv("quickemu-qmp", []string{"QUICKEMU_QMP_RAW=1"},
		"execute", "--socket", qmpSock(), "--command", payload)
}

// qmp sends a QMP command via quickemu-qmp execute.
func qmp(cmd string) (string, error) {
	return tool("quickemu-qmp", "execute", "--socket", qmpSock(),
		"--command", qmpCmd(cmd, nil))
}

// hmp sends a human-monitor-command (sendkey/type/screendump).
func hmp(cmd string) (string, error) {
	return tool("quickemu-qmp", "execute", "--socket", qmpSock(),
		"--command", qmpCmd("human-monitor-command",
			map[string]any{"command-line": cmd}))
}

// actMonitorCmd: monitor_send_cmd parity — send an arbitrary monitor
// command. Order: raw HMP socket (monitor=socket) → telnet
// (monitor=telnet) → QMP human-monitor-command as last resort.
func actMonitorCmd() int {
	msg := extra["monitor-cmd"]
	if msg == "" {
		fmt.Fprintln(os.Stderr, "WARNING! Send to QEMU-Monitor: Message empty!")
		return 1
	}
	conf := confKV()
	mon := confOpts["monitor"]
	if mon == "" {
		mon = conf["monitor"]
	}
	switch mon {
	case "", "socket":
		sock := filepath.Join(vmdir(), vmname()+"-monitor.socket")
		if _, err := os.Stat(sock); err == nil {
			fmt.Println(" - Sending:  via socket " + msg)
			if out, err := monitorSend("unix", sock, msg); err == nil {
				fmt.Print(out)
				return 0
			}
			fmt.Fprintln(os.Stderr, "WARNING! No qemu-monitor socket at "+sock+" - Couldn't send message to monitor!")
			return 1
		}
		// QMP human-monitor-command covers the same surface.
		out, err := hmp(msg)
		fmt.Print(out)
		return rc(err)
	case "telnet":
		fmt.Println(" - Sending:  via telnet " + msg)
		addr := conf["monitor_telnet_host"] + ":" + conf["monitor_telnet_port"]
		out, err := monitorSend("tcp", addr, msg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "WARNING! No qemu-monitor channel available - Couldn't send message to monitor!")
			return 1
		}
		fmt.Print(out)
		return 0
	default:
		fmt.Fprintln(os.Stderr, "WARNING! No qemu-monitor channel available - Couldn't send message to monitor!")
		return 1
	}
}

// monitorSend: write a line to a QEMU human-monitor endpoint and read
// the reply until the (qemu) prompt. The greeting and the per-char
// readline echo are drained so only the reply body is returned (the
// Bash socat+sed pipeline leaves the echo in; the payload is the same).
func monitorSend(network, addr, msg string) (string, error) {
	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.Dial(network, addr)
	if err != nil {
		return "", err
	}
	defer c.Close()
	readUntil := func(token string, timeout time.Duration) string {
		c.SetDeadline(time.Now().Add(timeout))
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
				if strings.Contains(b.String(), token) {
					break
				}
			}
			if err != nil {
				break
			}
		}
		return b.String()
	}
	readUntil("(qemu)", 1500*time.Millisecond) // greeting
	if _, err := c.Write([]byte(msg + "\n")); err != nil {
		return "", err
	}
	out := readUntil("(qemu)", 1500*time.Millisecond)
	out = strings.ReplaceAll(out, "(qemu) ", "")
	// Drop the echo line: everything up to the first newline is the
	// command being re-typed by the monitor's readline.
	if i := strings.Index(out, "\n"); i >= 0 {
		out = out[i+1:]
	}
	out = ansiRe.ReplaceAllString(out, "")
	return strings.ReplaceAll(out, "\r", ""), nil
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]|[\x00-\x08\x0b\x0e-\x1f]")

var stateRe = regexp.MustCompile(`"state":"[^"]*"`)

func actSendKeys() int {
	out, err := hmp("sendkey " + extra["send-keys"])
	fmt.Print(out)
	return rc(err)
}

func actType() int {
	for _, ch := range extra["type"] {
		k := qelib.SendkeyName(ch)
		if k == "" {
			continue
		}
		if _, err := hmp("sendkey " + k); err != nil {
			return rc(err)
		}
		time.Sleep(60 * time.Millisecond)
	}
	return 0
}

func actScreenshot() int {
	dst := extra["screenshot"]
	if dst == "" {
		dst = filepath.Join(vmdir(), vmname()+".ppm")
	}
	out, err := hmp("screendump " + dst)
	fmt.Print(out)
	return rc(err)
}

// monitorSendBest: try the raw monitor socket first, then QMP hmp —
// used for post-boot commands where either channel may be up.
func monitorSendBest(msg string) {
	sock := filepath.Join(vmdir(), vmname()+"-monitor.socket")
	if _, err := os.Stat(sock); err == nil {
		if _, err := monitorSend("unix", sock, msg); err == nil {
			return
		}
	}
	hmp(msg)
}

// qmpCmd marshals a QMP execute envelope — shared helper so embedded
// ids/filenames are escaped by encoding/json, not hand-rolled Sprintf.
func qmpCmd(execute string, args map[string]any) string {
	return qelib.QMPCmd(execute, args)
}
