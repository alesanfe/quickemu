package main

import (
	"encoding/json"

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
