// exec.go: guest-exec orchestration — spawn with safe argv (no
// implicit shell), poll guest-exec-status until the process exits or
// the caller's deadline hits, and decode the captured output.
package qga

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// ExecSpec describes one guest-exec invocation. Arg is passed
// element-by-element — never joined through a shell — so arguments
// cannot be reinterpreted. Use ShellSpec() to opt into a shell.
type ExecSpec struct {
	Path      string   `json:"path"`
	Arg       []string `json:"arg,omitempty"`
	Env       []string `json:"env,omitempty"` // "NAME=value" entries
	InputData string   `json:"input-data,omitempty"`
	// CaptureOutput asks the agent to buffer stdout/stderr for
	// guest-exec-status; required for this client (no output otherwise).
	CaptureOutput bool `json:"capture-output"`
}

// ShellSpec wraps a command line for `sh -c` — an explicit opt-in to
// shell interpretation. env may add "NAME=value" entries.
func ShellSpec(commandLine string, env []string) ExecSpec {
	return ExecSpec{Path: "/bin/sh", Arg: []string{"-c", commandLine},
		Env: env, CaptureOutput: true}
}

// ExecResult is the finished state of one guest process.
type ExecResult struct {
	PID       int
	Exited    bool
	ExitCode  int // -1 when the agent never reported one
	Signal    int // exit-signal when the process died by signal
	Out       []byte
	Err       []byte
	Truncated bool // output hit the caller's MaxOutput cap
	OutTrunc  bool // agent-side out-truncated flag
	ErrTrunc  bool // agent-side err-truncated flag
	Trapped   bool // guest process hit a trap
}

// execStatus mirrors the guest-exec-status reply.
type execStatus struct {
	Exited   bool   `json:"exited"`
	ExitCode *int   `json:"exitcode"`
	Signal   int    `json:"signal"`
	OutData  string `json:"out-data"`
	ErrData  string `json:"err-data"`
	OutTrunc bool   `json:"out-truncated"`
	ErrTrunc bool   `json:"err-truncated"`
	Trapped  bool   `json:"trap"`
}

// Exec runs a process in the guest and waits for it. ctx carries the
// deadline: when it fires the command returns ErrTimeout with whatever
// output was already captured (the guest process keeps running — QGA
// cannot kill it). maxOutput bounds each of Out/Err; 0 defaults to
// 1MiB. Polling is incremental: each guest-exec-status re-reads the
// captured buffers, and the loop backs off so a long-running command
// does not hammer the agent.
func (cl *Client) Exec(ctx context.Context, spec ExecSpec, maxOutput int) (*ExecResult, error) {
	spec.CaptureOutput = true
	if maxOutput <= 0 {
		maxOutput = 1 << 20
	}
	raw, err := cl.Execute(ctx, "guest-exec", execArgs(spec))
	if err != nil {
		return nil, err
	}
	var started struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(raw, &started); err != nil || started.PID <= 0 {
		return nil, fmt.Errorf("%w: guest-exec returned no pid", ErrBadResponse)
	}
	res := &ExecResult{PID: started.PID, ExitCode: -1}
	args := map[string]any{"pid": started.PID}
	wait := 50 * time.Millisecond
	for {
		var st execStatus
		if err := cl.typed(ctx, "guest-exec-status", args, &st); err != nil {
			return res, err
		}
		res.Out = appendOutput(res.Out, st.OutData, maxOutput, &res.Truncated)
		res.Err = appendOutput(res.Err, st.ErrData, maxOutput, &res.Truncated)
		res.OutTrunc, res.ErrTrunc = st.OutTrunc, st.ErrTrunc
		res.Trapped = st.Trapped
		if st.Exited {
			res.Exited = true
			res.Signal = st.Signal
			if st.ExitCode != nil {
				res.ExitCode = *st.ExitCode
			}
			return res, nil
		}
		select {
		case <-ctx.Done():
			res.OutTrunc = res.OutTrunc || res.Truncated
			return res, fmt.Errorf("%w: guest-exec pid %d", ErrTimeout, started.PID)
		case <-time.After(wait):
		}
		if wait < 500*time.Millisecond {
			wait *= 2
		}
	}
}

// execArgs renders the guest-exec argument object, dropping fields
// the agent would reject as empty.
func execArgs(spec ExecSpec) map[string]any {
	args := map[string]any{"path": spec.Path, "capture-output": true}
	if len(spec.Arg) > 0 {
		args["arg"] = spec.Arg
	}
	if len(spec.Env) > 0 {
		args["env"] = spec.Env
	}
	if spec.InputData != "" {
		args["input-data"] = spec.InputData
	}
	return args
}

// appendOutput decodes the agent's base64 output snapshot and keeps
// only the newest bytes up to the cap (the agent resends the whole
// buffer each poll — incremental read means taking the delta).
func appendOutput(dst []byte, b64 string, cap int, truncated *bool) []byte {
	if b64 == "" {
		return dst
	}
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		*truncated = true
		return dst
	}
	// The agent returns the full snapshot each poll — keep it while it
	// keeps growing (the delta is implicit), capped at the caller's
	// output limit.
	if len(b) <= len(dst) {
		return dst
	}
	if len(b) > cap {
		b = b[:cap]
		*truncated = true
	}
	return b
}
