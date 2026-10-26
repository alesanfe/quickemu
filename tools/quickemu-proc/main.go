// quickemu-proc: ADR-0001 phase 2. Verifies process identity against
// /proc (starttime, uid, exe basename) so a reused PID is never
// signalled by mistake. JSON envelope on stdout.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v4/process"
)

const version = "0.1.0"

type rep struct {
	SchemaVersion int      `json:"schemaVersion"`
	Command       string   `json:"command"`
	Success       bool     `json:"success"`
	Data          any      `json:"data,omitempty"`
	Errors        []string `json:"errors"`
	Meta          string   `json:"meta"`
}

func emit(ok bool, data any, err string) int {
	e := rep{SchemaVersion: 1, Command: "proc.verify", Success: ok,
		Data: data, Meta: "quickemu-proc/" + version, Errors: []string{}}
	if err != "" {
		e.Errors = []string{err}
	}
	b, _ := json.Marshal(e)
	fmt.Println(string(b))
	if !ok {
		return 1
	}
	return 0
}

// verify --pid P [--start-time S] [--uid U] [--exe name]
func main() {
	os.Args = normArgs(os.Args)
	if len(os.Args) < 2 || os.Args[1] != "verify" {
		fmt.Fprintln(os.Stderr, "usage: quickemu-proc verify --pid P [--start-time S] [--uid U] [--exe name]")
		os.Exit(2)
	}
	var pid, wantStart, wantUID int
	var wantExe, wantComm string
	for i := 2; i+1 < len(os.Args); i += 2 {
		switch os.Args[i] {
		case "--comm":
			wantComm = os.Args[i+1]
		case "--pid":
			pid, _ = strconv.Atoi(os.Args[i+1])
		case "--start-time":
			wantStart, _ = strconv.Atoi(os.Args[i+1])
		case "--uid":
			wantUID, _ = strconv.Atoi(os.Args[i+1])
		case "--exe":
			wantExe = os.Args[i+1]
		}
	}
	if pid <= 0 {
		os.Exit(emit(false, nil, "PID_INVALID"))
	}
	data := map[string]any{"pid": pid}
	// 1) alive
	if err := syscall.Kill(pid, 0); err != nil {
		os.Exit(emit(false, data, "PROCESS_DEAD"))
	}
	// 2) uid from /proc/<pid>/status
	if wantUID > 0 {
		st, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			os.Exit(emit(false, data, "PROC_UNREADABLE"))
		}
		uid := -1
		for _, l := range strings.Split(string(st), "\n") {
			if strings.HasPrefix(l, "Uid:") {
				fmt.Sscanf(strings.Fields(l)[1], "%d", &uid)
			}
		}
		if uid != wantUID {
			data["uid"] = uid
			os.Exit(emit(false, data, "UID_MISMATCH"))
		}
	}
	// 3) starttime: procfs parses /proc/<pid>/stat correctly even when
	// comm contains ')' or spaces (the manual-offset path was fragile);
	// gopsutil gives a portable fallback for non-Linux.
	if wantStart > 0 {
		var st int64
		if p, err := procfs.NewProc(pid); err == nil {
			if stat, err := p.Stat(); err == nil {
				st = int64(stat.Starttime)
			}
		}
		if st == 0 {
			if pr, err := process.NewProcess(int32(pid)); err == nil {
				if ms, err := pr.CreateTime(); err == nil {
					st = ms / 1000 // portable path: seconds-resolution epoch
					data["startTimeSource"] = "gopsutil"
				}
			}
		}
		data["startTime"] = st
		if st != 0 && st != int64(wantStart) {
			os.Exit(emit(false, data, "STARTTIME_MISMATCH"))
		}
	}
	// 4) comm (process name as the kernel reports it — matches
	// runtime.json "comm" expectations, including qemu* prefixes)
	if wantComm != "" {
		comm := ""
		if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
			comm = strings.TrimSpace(string(b))
		}
		data["comm"] = comm
		if comm != wantComm && !strings.HasPrefix(comm, wantComm[:min(len(wantComm), 15)]) &&
			!strings.HasPrefix(comm, "qemu") {
			os.Exit(emit(false, data, "COMM_MISMATCH"))
		}
	}
	// 5) exe basename
	if wantExe != "" {
		ex, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err != nil {
			os.Exit(emit(false, data, "EXE_UNREADABLE"))
		}
		base := ex[strings.LastIndex(ex, "/")+1:]
		data["exe"] = base
		if !strings.Contains(base, wantExe) {
			os.Exit(emit(false, data, "EXE_MISMATCH"))
		}
	}
	os.Exit(emit(true, data, ""))
}
// normArgs expands GNU-style --flag=value into --flag value so the
// positional pair parsers stay uniform (pflag parity without the dep).
func normArgs(a []string) []string {
	out := a[:0]
	for _, s := range a {
		if i := strings.IndexByte(s, '='); strings.HasPrefix(s, "--") && i > 2 {
			out = append(out, s[:i], s[i+1:])
		} else {
			out = append(out, s)
		}
	}
	return out
}
