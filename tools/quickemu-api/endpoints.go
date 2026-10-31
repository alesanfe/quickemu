// endpoints.go: the QE-080–083 surface — host/vm stats, the ops
// journal view, per-VM events and the capabilities record. Everything
// here is read-only; mutations stay behind /v1/vm-cmd (the CLI
// boundary enforces locks and the state machine).
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// handleStats: GET /v1/stats — host-side structured stats (QE-083).
func handleStats(w http.ResponseWriter, _ *http.Request) {
	write(w, 200, map[string]any{"data": hostStats()})
}

// handleCapabilities: GET /v1/capabilities — QE-082. Consumers
// negotiate on this, never on version-string comparisons.
func handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	write(w, 200, map[string]any{"data": map[string]any{
		"api":     "v1",
		"backend": "quickemu-api/" + qelib.QEVersion,
		"commands": []string{
			"start", "stop", "shutdown", "pause", "resume", "kill", "reset",
		},
		"endpoints": []string{
			"/v1/vms", "/v1/vm/{name}/state", "/v1/vm/{name}/ops",
			"/v1/vm/{name}/stats", "/v1/vm/{name}/events",
			"/v1/vm-cmd/{name}", "/v1/stats", "/v1/ops", "/v1/ops/{id}",
			"/v1/capabilities", "/v1/health",
		},
		"experimental": []string{"stats", "events"},
	}})
}

// handleOps: GET /v1/ops — the cross-VM operation journal (QE-080).
func handleOps(w http.ResponseWriter, _ *http.Request) {
	ops := allOps()
	out := make([]*opRec, 0, len(ops))
	out = append(out, ops...)
	write(w, 200, map[string]any{"data": map[string]any{"operations": out}})
}

// handleOpByID: GET /v1/ops/<id> — one operation's record.
func handleOpByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/ops/")
	if badOpID(id) {
		write(w, 400, map[string]any{"errors": []any{map[string]any{"code": "INVALID_ARGUMENT"}}})
		return
	}
	o := findOp(id)
	if o == nil {
		write(w, 404, map[string]any{"errors": []any{map[string]any{"code": "NOT_FOUND"}}})
		return
	}
	write(w, 200, map[string]any{"data": o})
}

// vmStatsHandler: /v1/vm/<n>/stats — the per-VM QE-083 shape. The
// sampler keeps no state across requests: a single sample reports
// rates as deltas-from-zero (consumers wanting rates poll and diff —
// the interval is theirs, not ours).
func vmStatsHandler(w http.ResponseWriter, vm string) {
	dir := vmDirFor(vm)
	ev := probeVM(dir, vm, "")
	write(w, 200, map[string]any{"data": vmStats(dir, vm, ev, nil, time.Now())})
}

// vmEventsHandler: /v1/vm/<n>/events?since=N — the JSONL event log
// with seq > cursor (QE-081's resume contract; polling, not
// streaming — the unix-socket stream is a later transport).
func vmEventsHandler(w http.ResponseWriter, r *http.Request, vm string) {
	since := int64(0)
	if s := r.URL.Query().Get("since"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			since = n
		}
	}
	dir := vmDirFor(vm)
	b, err := os.ReadFile(filepath.Join(dir, vm+".events.jsonl"))
	if err != nil {
		write(w, 200, map[string]any{"data": map[string]any{"events": []any{}, "latest": since}})
		return
	}
	evs := []any{}
	latest := since
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l == "" {
			continue
		}
		var v struct {
			Seq int64 `json:"seq"`
		}
		if json.Unmarshal([]byte(l), &v) != nil || v.Seq <= since {
			continue
		}
		var ev any
		if json.Unmarshal([]byte(l), &ev) == nil {
			evs = append(evs, ev)
			if v.Seq > latest {
				latest = v.Seq
			}
		}
	}
	write(w, 200, map[string]any{"data": map[string]any{"events": evs, "latest": latest}})
}
