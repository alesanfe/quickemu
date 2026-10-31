// log.go — QE-104: structured action logging. Every dispatched
// action records {ts, level, event, vm, pid, data} JSONL — the
// same record shape qe_event writes — into <vmdir>/<vm>.log.jsonl
// (or $QE_LOG_JSONL). Human output stays on stdout untouched; the
// log is the machine-parseable channel.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var logMu sync.Mutex

// logPath: $QE_LOG_JSONL wins; otherwise per-VM <vm>.log.jsonl.
func logPath() string {
	if p := os.Getenv("QE_LOG_JSONL"); p != "" {
		return p
	}
	if vmdir() == "" {
		return ""
	}
	return filepath.Join(vmdir(), vmname()+".log.jsonl")
}

// logCap: rotate at 8 MiB — a log that grows unboundedly becomes a
// disk problem, not a debugging aid. .log.jsonl.1 keeps one
// generation; older bytes drop.
const logMaxBytes = 8 << 20

// qlog writes one JSONL record. level is info|warn|error; data is a
// pre-marshalled JSON fragment ("" → omitted).
func qlog(level, event, data string) {
	p := logPath()
	if p == "" {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	if st, err := os.Stat(p); err == nil && st.Size() > logMaxBytes {
		os.Rename(p, p+".1") // single-generation rotation
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	rec := map[string]any{
		"ts":    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"level": level, "event": event,
		"vm": vmname(), "pid": os.Getpid(),
	}
	if data != "" {
		rec["data"] = json.RawMessage(data)
	}
	b, _ := json.Marshal(rec)
	f.Write(append(b, '\n'))
}

// qlogAction wraps one dispatched action: start/done/fail records.
func qlogActionStart(act string) {
	qlog("info", "action.start", fmt.Sprintf(`{"action":%q}`, act))
}

func qlogActionDone(act string, rc int) {
	ev, lvl := "action.done", "info"
	if rc != 0 {
		ev, lvl = "action.fail", "error"
	}
	qlog(lvl, ev, fmt.Sprintf(`{"action":%q,"rc":%d}`, act, rc))
}
