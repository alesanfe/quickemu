package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// actEvents: qe_events_read parity — JSONL events with seq > cursor.
func actEvents() int {
	cur := 0
	fmt.Sscanf(extra["events"], "%d", &cur)
	f := filepath.Join(vmdir(), vmname()+".events.jsonl")
	b, err := os.ReadFile(f)
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.Contains(l, `"seq":`) {
			continue
		}
		var e struct {
			Seq int `json:"seq"`
		}
		if json.Unmarshal([]byte(l), &e) == nil && e.Seq > cur {
			fmt.Println(l)
		}
	}
	return 0
}

// qeEvent: append a JSONL event to <vmdir>/<vm>.events.jsonl
// (qe_event parity: seq, ts, event, vm, pid, data).
func qeEvent(typ, data string) {
	f := filepath.Join(vmdir(), vmname()+".events.jsonl")
	seq := 1
	if b, err := os.ReadFile(f); err == nil {
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		var last struct {
			Seq int `json:"seq"`
		}
		if json.Unmarshal([]byte(lines[len(lines)-1]), &last) == nil {
			seq = last.Seq + 1
		}
	}
	var pid any
	if p := readPID(); p > 0 {
		pid = p
	}
	if data == "" {
		data = "null"
	}
	b, _ := json.Marshal(map[string]any{
		"seq": seq, "ts": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"event": typ, "vm": vmname(), "pid": pid,
		"data": json.RawMessage(data)})
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer fh.Close()
	fmt.Fprintln(fh, string(b))
}
