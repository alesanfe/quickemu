// exec.go: the subprocess seam (ADR-0009 pattern shared with the
// other tools) — sibling binaries resolve through qelib.FindTool
// (sibling-first, PATH fallback) and tests point PATH at a stub dir.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// execCommand and findTool are the test seams — restore via
// t.Cleanup, never stub under t.Parallel.
var execCommand = exec.Command
var findTool = qelib.FindTool

func qelibAlive(pid int) bool { return qelib.Alive(pid) }

// runTool executes a sibling tool, capturing stdout (stderr flows
// through so the child's own diagnostics reach the user). dir is the
// working directory — "" keeps ours. It is a var so tests can stub
// the whole subprocess layer without scripting fake binaries.
var runTool = func(name, dir string, env []string, args ...string) (string, error) {
	bin, err := findTool(name)
	if err != nil {
		return "", err
	}
	cmd := execCommand(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	return buf.String(), err
}

// emitOK/emitFail wrap qelib's v1 envelope for --format json.
func emitOK(cmd string, data any) {
	qelib.EmitOK(cmd, "quickemu-env/"+version, data)
}

func emitErrs(cmd string, errs envErrs) {
	out := make([]qelib.QEError, 0, len(errs))
	for _, e := range errs {
		out = append(out, qelib.QEError{
			Code: "ENV_INVALID", Message: e.msg, Severity: "error"})
	}
	qelib.Emit(cmd, "quickemu-env/"+version, false, nil, out)
}

// errf prints a user-facing error on stdout (the historical quickemu
// contract — structured callers read the envelope instead) and
// returns rc 1 for command exit paths.
func errf(format string, args ...any) int {
	fmt.Printf("ERROR! "+format+"\n", args...)
	return 1
}
