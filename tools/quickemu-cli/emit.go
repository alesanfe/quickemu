package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func emitEnvelope(cmd string, success bool, data any, errors any) {
	qelib.Emit(cmd, "quickemu-cli/"+qeVersion, success, data, errors)
}

func emitOK(cmd string, data any) {
	emitEnvelope(cmd, true, data, []any{})
}

// ---------- action implementations ----------

func rc(err error) int {
	if err != nil {
		return 1
	}
	return 0
}

// jsonStr marshals a small object to a JSON string — for event
// payloads that travel inside the envelope rather than standalone.
func jsonStr(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// emitFail: failure envelope matching the Bash qe_fail contract.
func emitFail(cmd, code, msg string) {
	emitEnvelope(cmd, false, nil, []any{map[string]any{
		"code": code, "message": msg, "severity": "error", "retryable": false}})
}

// jsonRequested: true when argv asks for the v1 envelope — covers the
// early parse exits that run before --format has been consumed (so
// outFmt may still be the default).
func jsonRequested() bool {
	if outFmt == "json" {
		return true
	}
	for i, a := range os.Args {
		if a == "--format=json" ||
			(a == "--format" && i+1 < len(os.Args) && os.Args[i+1] == "json") {
			return true
		}
	}
	return false
}

// reqCmd: the v1 command name a fail envelope should carry when the
// run dies before dispatch — derived from the requested action so the
// caller's command-match still resolves (default: a start attempt).
func reqCmd() string {
	if len(actions) == 0 {
		return "vm.started"
	}
	return "vm." + strings.ReplaceAll(
		strings.TrimSuffix(actions[0], "_vm"), "_", "-")
}
