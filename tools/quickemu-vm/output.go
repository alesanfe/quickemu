// tools/quickemu-vm/output.go: envelope/output helpers
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// die: qe_die parity — JSON mode emits the error envelope on stdout;
// human mode writes "ERROR! <msg>" to stderr. Always exits 1.
func die(code, msg string) {
	if !outputJSON {
		fmt.Fprintln(os.Stderr, "ERROR! "+msg)
		os.Exit(1)
	}
	qelib.Fail(opCmd, "quickemu-vm/"+vmVersion(), code, msg)
}

func ok(data string) {
	if !outputJSON {
		return
	}
	qelib.EmitOK(opCmd, "quickemu-vm/"+vmVersion(), json.RawMessage(data))
}

// okMap: typed variant of ok — data is marshalled by encoding/json
// instead of hand-built with fmt.Sprintf.
func okMap(data map[string]any) {
	if !outputJSON {
		return
	}
	qelib.EmitOK(opCmd, "quickemu-vm/"+vmVersion(), data)
}

// info/infof: qe_info parity — human progress lines print only in
// text mode; under --format json the envelope is the sole output.
func info(s string) {
	if !outputJSON {
		fmt.Println(s)
	}
}

func infof(f string, a ...any) {
	if !outputJSON {
		fmt.Printf(f, a...)
	}
}

// opCmd: the v1 "command" name for the running op (qe_emit parity —
// e.g. vm.delete-plan, vm.trash.list). Set in main() per dispatch.
var opCmd = ""

// Emission goes through qelib's canonical envelope
// {schemaVersion, command, success, data, warnings, errors, meta};
// qe_emit parity: nothing is printed unless OUTPUT_FORMAT=json — in
// text mode ops already printed their human lines.

func vmConfPath(dir string) string {
	// QE_VMCONF: the caller (quickemu-cli) knows exactly which conf the
	// user passed; a dir can contain several .conf files (clones).
	if c := os.Getenv("QE_VMCONF"); c != "" {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	ents, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
	if len(ents) > 0 {
		return ents[0]
	}
	return ""
}
