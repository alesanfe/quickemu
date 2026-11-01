// events.go: the run event stream. Every state change is one JSONL
// record on stdout — the same record shape qelib.AppendEvent writes
// to <vmdir>/<vm>.events.jsonl, plus a run id, so a Quickgui adapter
// (or jq) can consume either source interchangeably.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
	"github.com/google/uuid"
)

// emitter is a run-scoped JSONL event stream. seq is per-run; ts is
// UTC. Events are also mirrored into the run's events.jsonl artifact
// and the VM event file so `quickemu --events` can read them.
type emitter struct {
	mu    sync.Mutex
	seq   int
	runID string
	vm    string
	vmDir string
	out   *os.File
	art   *os.File
}

func newEmitter(runID, vm, vmDir, artDir string, quiet bool) *emitter {
	e := &emitter{runID: runID, vm: vm, vmDir: vmDir, out: os.Stdout}
	if quiet {
		e.out = nil
	}
	if artDir != "" {
		if f, err := os.OpenFile(filepath.Join(artDir, "events.jsonl"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			e.art = f
		}
	}
	return e
}

// emit writes one event record to stdout + the artifact file + the
// per-VM event log. data marshals as-is (nil → null).
func (e *emitter) emit(typ string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seq++
	rec := map[string]any{
		"seq":   e.seq,
		"ts":    time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"event": typ,
		"run":   e.runID,
		"vm":    e.vm,
	}
	if data == nil {
		rec["data"] = nil
	} else {
		rec["data"] = data
	}
	b, _ := json.Marshal(rec)
	if e.out != nil {
		fmt.Fprintln(e.out, string(b))
	}
	if e.art != nil {
		fmt.Fprintln(e.art, string(b))
	}
	if e.vmDir != "" && e.vm != "" {
		raw, _ := json.Marshal(data)
		qelib.AppendEvent(e.vmDir, e.vm, os.Getpid(), typ, string(raw))
	}
}

// setVM points the stream at the real VM once quickget creates the
// conf — events before resolve carry vm:"" because the VM does not
// exist yet.
func (e *emitter) setVM(vm, vmDir string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vm, e.vmDir = vm, vmDir
}

func (e *emitter) close() {
	if e.art != nil {
		e.art.Close()
	}
}

// step wraps a pipeline stage: step.started → body → step.completed
// or step.failed. Progress is a 0-100 fraction of the whole run for
// UIs that only want a bar.
func (e *emitter) step(id string, idx, total int, fn func() error) error {
	e.emit("step.started", map[string]any{
		"step": id, "index": idx, "total": total})
	err := fn()
	if err != nil {
		e.emit("step.failed", map[string]any{
			"step": id, "index": idx, "total": total,
			"error": err.Error()})
		return err
	}
	e.emit("step.completed", map[string]any{
		"step": id, "index": idx, "total": total,
		"progress": float64(idx) / float64(total)})
	return nil
}

// progress emits a mid-step tick (long steps like downloads/installs
// report heartbeats so watchers can distinguish slow from stuck).
func (e *emitter) progress(id, msg string) {
	e.emit("step.progress", map[string]any{"step": id, "message": msg})
}

// --- ops journal bridge ------------------------------------------------

// opNew registers the run in the VM's ops journal — the same store
// quickemu --status and Quickgui's operationStatus() already read, so
// a UI gets list/cancel/watch of automation runs for free.
func opNew(vmDir, vm string) (string, error) {
	id, err := toolOut("quickemu-ops", "new", "--vmdir", vmDir,
		"--type", "quickprovision", "--vm", vm)
	if err != nil {
		return "", err
	}
	return trimLine(id), nil
}

func opSet(vmDir, id, state string, progress int) {
	_, _ = toolOut("quickemu-ops", "set", "--vmdir", vmDir, "--id", id,
		"--state", state, "--progress", fmt.Sprint(progress),
		"--worker-pid", fmt.Sprint(os.Getpid()))
}

func opDone(vmDir, id string) {
	_, _ = toolOut("quickemu-ops", "done", "--vmdir", vmDir, "--id", id)
}

func opFail(vmDir, id, code, msg string) {
	_, _ = toolOut("quickemu-ops", "fail", "--vmdir", vmDir, "--id", id,
		"--code", code, "--message", msg)
}

// opCancelled reports whether the journal entry has been asked to
// stop — checked at step boundaries so a cancel is a clean abort,
// not a mid-write kill.
func opCancelled(vmDir, id string) bool {
	b, err := os.ReadFile(filepath.Join(vmDir, "ops", id+".json"))
	if err != nil {
		return false
	}
	var o struct {
		State string `json:"state"`
	}
	if json.Unmarshal(b, &o) != nil {
		return false
	}
	return o.State == "cancelRequested" || o.State == "cancelled"
}

func newRunID() string {
	u := uuid.NewString()
	return time.Now().UTC().Format("20060102-150405") + "-" + u[:8]
}
