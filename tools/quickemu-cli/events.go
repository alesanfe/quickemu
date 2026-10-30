package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
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
// (qe_event parity: seq, ts, event, vm, pid, data) — the shared
// qelib appender keeps quickemu-vm emitting the identical record.
func qeEvent(typ, data string) {
	qelib.AppendEvent(vmdir(), vmname(), readPID(), typ, data)
}
