package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
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

// actMonitorCmd: --monitor-cmd → monitor_send_cmd "${MONITOR_CMD}".
func actMonitorCmd() int {
	return monitorSendCmd(extra["monitor-cmd"])
}

// monitorSendCmd: monitor_send_cmd parity — the human-monitor channel
// (never QMP). monitor=socket requires a live monitor socket; telnet
// sends over tcp. Like the Bash socat|tr|sed pipeline, a failed send
// after the " - Sending:" line still returns 0 — the reply surface is
// best-effort, the gate is channel availability.
func monitorSendCmd(msg string) int {
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
		if st, err := os.Stat(sock); err != nil || st.Mode()&os.ModeSocket == 0 {
			fmt.Fprintln(os.Stderr, "WARNING! No qemu-monitor socket at "+sock+" - Couldn't send message to monitor!")
			return 1
		}
		fmt.Println(" - Sending:  via socket " + msg)
		out, _ := monitorSend("unix", sock, msg)
		fmt.Print(out)
		return 0
	case "telnet":
		fmt.Println(" - Sending:  via telnet " + msg)
		addr := conf["monitor_telnet_host"] + ":" + conf["monitor_telnet_port"]
		out, _ := monitorSend("tcp", addr, msg)
		fmt.Print(out)
		return 0
	default:
		fmt.Fprintln(os.Stderr, "WARNING! No qemu-monitor channel available - Couldn't send message to monitor!")
		return 1
	}
}

// vmControl: vm_control parity — prefer the structured QMP channel when
// the control socket is up (the qmp tool applies the allowlist; a
// refused or failed command is silent, exactly like Bash's qmp_query |
// grep), then fall back to the human monitor.
func vmControl(hmpCmd, qmpJSON string) int {
	if st, err := os.Stat(qmpSock()); err == nil && st.Mode()&os.ModeSocket != 0 {
		if _, err := qmp(qmpJSON); err == nil {
			return 0
		}
	}
	return monitorSendCmd(hmpCmd)
}

// sendKeyQMP: vm_send_key_combo's structured payload — a combo string
// ('ctrl-alt-delete') becomes a qcode array, splitting on '-'.
func sendKeyQMP(combo string) string {
	parts := strings.Split(combo, "-")
	keys := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		keys = append(keys, map[string]any{"type": "qcode", "data": p})
	}
	return qmpCmd("send-key", map[string]any{"keys": keys})
}

// monitorSend: write a line to a QEMU human-monitor endpoint and read
// the reply until the (qemu) prompt. The greeting and the per-char
// readline echo are drained so only the reply body is returned (the
// Bash socat+sed pipeline leaves the echo in; the payload is the same).
// qmpTimeout: QMP_TIMEOUT env parity (Bash's socat -t bound, default 1).
func qmpTimeout() time.Duration {
	t := 1.0
	if v := os.Getenv("QMP_TIMEOUT"); v != "" {
		fmt.Sscan(v, &t)
	}
	return time.Duration(t * float64(time.Second))
}

func monitorSend(network, addr, msg string) (string, error) {
	d := net.Dialer{Timeout: qmpTimeout()}
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
	readUntil("(qemu)", qmpTimeout()) // greeting
	if _, err := c.Write([]byte(msg + "\n")); err != nil {
		return "", err
	}
	out := readUntil("(qemu)", qmpTimeout())
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

// actSendKeys: send_keys → vm_send_key_combo — QMP send-key when the
// control socket is up, HMP sendkey otherwise; the combo's rc is the
// action's rc.
func actSendKeys() int {
	combo := extra["send-keys"]
	return vmControl("sendkey "+combo, sendKeyQMP(combo))
}

// actType: type_text — map each char to a sendkey name, skip unmapped
// chars with a notice, and send each key via vm_send_key_combo. Bash
// ignores per-key failures (the loop always completes), so the action
// reports 0 even when every send warned.
func actType() int {
	for _, ch := range extra["type"] {
		k := qelib.SendkeyName(ch)
		if k == "" {
			fmt.Printf(" - Skipping unmapped char '%c'\n", ch)
			continue
		}
		vmControl("sendkey "+k, sendKeyQMP(k))
		time.Sleep(50 * time.Millisecond)
	}
	return 0
}

// actScreenshot: screenshot_vm parity — gated on a running VM and a
// real monitor channel; screendump always lands as PPM and a .png
// destination converts via convert/pnmtopng when the host has one.
func actScreenshot() int {
	if !vmRunning() {
		fmt.Println(" - " + vmname() + " is not running.")
		return 1
	}
	conf := confKV()
	if conf["monitor"] == "none" {
		fmt.Println(" - ERROR! No QEMU monitor channel for " + vmname() + " (monitor is disabled).")
		return 1
	}
	if conf["monitor"] == "socket" {
		sock := filepath.Join(vmdir(), vmname()+"-monitor.socket")
		if _, err := os.Stat(sock); err != nil {
			fmt.Println(" - ERROR! No QEMU monitor socket for " + vmname() + ".")
			return 1
		}
	}
	out := extra["screenshot"]
	if out == "" {
		out = os.Getenv("SCREENSHOT_OUT")
	}
	if out == "" {
		out = filepath.Join(vmdir(), vmname()+"-"+time.Now().Format("20060102-150405")+".ppm")
	}
	ppm := out
	png := strings.HasSuffix(out, ".png")
	if png {
		ppm = strings.TrimSuffix(out, ".png") + ".ppm"
	}
	// vm_control: screendump is on the allowlist — QMP first, HMP
	// fallback; the file check below is the authoritative outcome.
	vmControl("screendump "+ppm,
		qmpCmd("screendump", map[string]any{"filename": ppm}))
	time.Sleep(500 * time.Millisecond)
	if st, err := os.Stat(ppm); err != nil || st.Size() == 0 {
		fmt.Println(" - ERROR! Screenshot was not created at " + ppm + ".")
		return 1
	}
	if png {
		if bin, err := exec.LookPath("convert"); err == nil {
			if exec.Command(bin, ppm, out).Run() == nil {
				os.Remove(ppm)
			}
		} else if bin, err := exec.LookPath("pnmtopng"); err == nil {
			if ob, err := exec.Command(bin, ppm).Output(); err == nil {
				if os.WriteFile(out, ob, 0644) == nil {
					os.Remove(ppm)
				}
			}
		} else {
			out = ppm
			fmt.Println(" - WARNING! 'convert'/'pnmtopng' not found; saved PPM instead.")
		}
	}
	fmt.Println(" - Screenshot saved to " + out)
	return 0
}

// monitorSockReady: vm_monitor_socket_ready — the -monitor unix socket
// must exist and actually be a socket (a stale regular file doesn't
// count).
func monitorSockReady() bool {
	st, err := os.Stat(filepath.Join(vmdir(), vmname()+"-monitor.socket"))
	return err == nil && st.Mode()&os.ModeSocket != 0
}

// qmpCmd marshals a QMP execute envelope — shared helper so embedded
// ids/filenames are escaped by encoding/json, not hand-rolled Sprintf.
func qmpCmd(execute string, args map[string]any) string {
	return qelib.QMPCmd(execute, args)
}
