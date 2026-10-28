// quickemu-proc: ADR-0001 phase 2. Verifies process identity against
// /proc (starttime, uid, exe basename) so a reused PID is never
// signalled by mistake. JSON envelope on stdout.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const version = "0.1.0"

func emit(ok bool, data any, err string) int {
	errs := []qelib.QEError{}
	if err != "" {
		errs = []qelib.QEError{{Code: "VERIFY_FAILED", Message: err,
			Severity: "error", Retryable: false}}
	}
	qelib.Emit("proc.verify", "quickemu-proc/"+version, ok, data, errs)
	if !ok {
		return 1
	}
	return 0
}

// verify --pid P [--start-time S] [--uid U] [--exe name]
func main() { os.Exit(run()) }

func run() int {
	os.Args = qelib.NormArgs(os.Args)
	if len(os.Args) < 2 || os.Args[1] != "verify" {
		fmt.Fprintln(os.Stderr, "usage: quickemu-proc verify --pid P [--start-time S] [--uid U] [--exe name]")
		return 2
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
		return emit(false, nil, "PID_INVALID")
	}
	data := map[string]any{"pid": pid}
	// 1) alive — EPERM-aware: a foreign-owned process is alive, not dead.
	if !qelib.Alive(pid) {
		return emit(false, data, "PROCESS_DEAD")
	}
	// 2) uid from /proc/<pid>/status
	if wantUID > 0 {
		st, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			return emit(false, data, "PROC_UNREADABLE")
		}
		uid := -1
		for _, l := range strings.Split(string(st), "\n") {
			if f := strings.Fields(l); len(f) > 1 && f[0] == "Uid:" {
				fmt.Sscanf(f[1], "%d", &uid)
			}
		}
		if uid != wantUID {
			data["uid"] = uid
			return emit(false, data, "UID_MISMATCH")
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
			return emit(false, data, "STARTTIME_MISMATCH")
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
			return emit(false, data, "COMM_MISMATCH")
		}
	}
	// 5) exe basename
	if wantExe != "" {
		ex, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err != nil {
			return emit(false, data, "EXE_UNREADABLE")
		}
		base := ex[strings.LastIndex(ex, "/")+1:]
		data["exe"] = base
		if !strings.Contains(base, wantExe) {
			return emit(false, data, "EXE_MISMATCH")
		}
	}
	return emit(true, data, "")
}
