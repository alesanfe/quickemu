// state_machine.go: the formal VM lifecycle machine (QE-003) — the
// derived words state() reports today stay the contract; this layer
// records them as an explicit machine: a transition table, a
// persisted record (<vmdir>/<vm>.state.json holding state + last
// transition + cause + actor), a structured vm.transition event per
// accepted jump, and boot-time reconciliation of the record against
// pidfile + runtime.json fingerprint + monitor/QMP evidence.
//
// The record also carries the crash-vs-stop distinction Bash never
// had: a dead process recorded mid-"stopping" finished a requested
// stop (quickemu kill/stop or a guest ACPI shutdown), while one
// recorded live (running/paused/starting/…) is a genuine crash —
// SIGKILL, OOM or a host reboot all land as "crashed".
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Formal machine states (superset of the Bash derived words).
const (
	stDefined    = "defined"
	stPreparing  = "preparing"
	stStarting   = "starting"
	stRunning    = "running"
	stPaused     = "paused"
	stSaving     = "saving"
	stSuspended  = "suspended"
	stStopping   = "stopping"
	stStopped    = "stopped"
	stCrashed    = "crashed"
	stRecovering = "recovering"
	stImporting  = "importing"
	stExporting  = "exporting"
	stDeleting   = "deleting"
	stError      = "error"
)

// machineTransitions: the legal from→to arcs. "error" is an implicit
// target from every state (canTransition); "" (no record yet) may
// adopt any observed state.
var machineTransitions = map[string][]string{
	stDefined:    {stPreparing, stImporting, stDeleting, stStopped},
	stPreparing:  {stStarting, stStopped},
	stStarting:   {stRunning, stStopped, stCrashed},
	stRunning:    {stPaused, stSaving, stSuspended, stStopping, stStopped, stCrashed},
	stPaused:     {stRunning, stStopping, stSuspended, stSaving, stStopped, stCrashed},
	stSaving:     {stRunning, stPaused, stStopped, stCrashed},
	stSuspended:  {stRunning, stStopped},
	stStopping:   {stStopped, stRunning, stCrashed},
	stStopped:    {stPreparing, stDefined, stImporting, stExporting, stDeleting},
	stCrashed:    {stRecovering, stStopped, stPreparing, stDeleting},
	stRecovering: {stStopped, stRunning, stPaused, stCrashed},
	stImporting:  {stStopped, stDeleting},
	stExporting:  {stStopped},
	stDeleting:   {stStopped, stDefined},
	stError:      {stRecovering, stStopped, stRunning, stPaused, stPreparing, stDeleting},
}

// machineRec: <vm>.state.json — the machine memory: current state,
// who/what caused the last accepted jump and when.
type machineRec struct {
	SchemaVersion int    `json:"schemaVersion"`
	State         string `json:"state"`
	Pid           int    `json:"pid,omitempty"`
	LastFrom      string `json:"lastFrom,omitempty"`
	LastTo        string `json:"lastTo,omitempty"`
	Cause         string `json:"cause,omitempty"`
	Actor         string `json:"actor,omitempty"`
	At            string `json:"transitionedAt,omitempty"`
	UpdatedAt     string `json:"updatedAt"`
}

func statePath() string { return filepath.Join(vmdir(), vmname()+".state.json") }

// machineState reads the persisted record; nil when absent/corrupt —
// a corrupt record is no record, the reconciler re-derives it.
func machineState() *machineRec {
	b, err := os.ReadFile(statePath())
	if err != nil {
		return nil
	}
	var r machineRec
	if json.Unmarshal(b, &r) != nil || r.State == "" {
		return nil
	}
	return &r
}

// canTransition: from→to per the table; "" (no record) and error are
// universal; same-state is an idempotent no-op.
func canTransition(from, to string) bool {
	if from == "" || from == to || to == stError {
		return true
	}
	for _, t := range machineTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// machineTransition validates cur→to against the table (the record
// is the source of truth for cur), persists the new record, and
// emits a structured vm.transition event. force is the reconciler's
// privilege — actor-driven transitions never bypass the table.
func machineTransitionF(to, cause, actor string, force bool) bool {
	cur := ""
	if r := machineState(); r != nil {
		cur = r.State
	}
	if cur == to {
		return true // already there — no re-record, no event
	}
	if cur != "" && !force && !canTransition(cur, to) {
		fmt.Fprintf(os.Stderr,
			"WARNING: invalid VM state transition %s -> %s refused\n", cur, to)
		return false
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rec := &machineRec{SchemaVersion: 1, State: to, Pid: readPID(),
		LastFrom: cur, LastTo: to, Cause: cause, Actor: actor,
		At: now, UpdatedAt: now}
	b, err := json.Marshal(rec)
	if err != nil {
		return false
	}
	tmp := statePath() + ".tmp"
	if os.WriteFile(tmp, append(b, '\n'), 0644) != nil ||
		os.Rename(tmp, statePath()) != nil {
		os.Remove(tmp)
		return false
	}
	qeEvent("vm.transition", jsonStr(map[string]any{
		"from": cur, "to": to, "cause": cause, "actor": actor}))
	return true
}

func machineTransition(to, cause, actor string) bool {
	return machineTransitionF(to, cause, actor, false)
}

// machineOpWrap runs a delegated op inside a machine state (importing,
// exporting, …): the pre-state is captured, fn runs, and the machine
// lands back on the pre-state on success — or "error" on failure —
// so an interrupted op never masquerades as a clean stopped VM.
func machineOpWrap(state, op string, fn func() int) int {
	prev := ""
	if r := machineState(); r != nil {
		prev = r.State
	}
	machineTransition(state, op, "quickemu-cli")
	r := fn()
	if r != 0 {
		machineTransitionF(stError, op+"-failed", "quickemu-cli", true)
	} else if prev != "" && prev != state {
		machineTransitionF(prev, op+"-done", "quickemu-cli", true)
	}
	return r
}

// machineReconcile: boot-time truth sync — the persisted machine
// record vs the derived word (pidfile, runtime.json fingerprint,
// monitor/QMP sockets, .paused marker). Disagreements resolve to the
// observed truth one legal transition at a time; each jump is a
// vm.transition event with cause "reconcile:<evidence>".
func machineReconcile() string {
	if vmConf == "" || vmname() == "." {
		// No resolvable VM identity (bare host-level invocation) —
		// a record anchored at "." would pollute the caller's cwd.
		return "stopped"
	}
	rec := machineState()
	recorded := ""
	if rec != nil {
		recorded = rec.State
	}
	word, _ := state() // the derived truth
	target := ""
	switch word {
	case "running":
		target = stRunning
	case "paused":
		target = stPaused
	case "stopping":
		target = stStopping
	case "stopped":
		target = stStopped
	case "stale":
		// pidfile present but the process is gone — an unclean end.
		// Recorded "stopping" means a REQUESTED stop completed (kill,
		// monitor-quit or the guest's own powerdown); a live-state
		// record means QEMU died under us (SIGKILL/OOM/host reboot).
		switch recorded {
		case stStopping, stStopped, stDeleting, stCrashed:
			cause := "stop-completed"
			if rec != nil && rec.Cause != "" {
				cause = "stop-completed:" + rec.Cause
			}
			machineTransitionF(stStopped, cause, "reconcile", true)
		case "", stError:
			// No machine memory: a bare stale pidfile is a crash
			// remnant by definition.
			machineTransitionF(stCrashed, "stale-pidfile", "reconcile", true)
		default:
			machineTransitionF(stRecovering, "unclean-shutdown-detected", "reconcile", true)
			machineTransitionF(stCrashed, "process-died", "reconcile", true)
		}
	case "unknown":
		// Live but unverifiable (fingerprint/uid/pid-reuse/foreign) —
		// the record must not keep claiming a live state we can't
		// prove. error records the refusal; recovery re-derives.
		if recorded != "" && recorded != stError {
			vw, _ := instanceVerify()
			machineTransitionF(stError, "verify:"+vw, "reconcile", true)
		}
	case "archived":
		// Trash/delete outcome — outside the power machine; the
		// record is left as-is for post-mortem.
	}
	if target != "" && recorded != target {
		machineTransitionF(target, "reconcile:"+word, "reconcile", true)
	}
	if r := machineState(); r != nil {
		return r.State
	}
	return word
}
