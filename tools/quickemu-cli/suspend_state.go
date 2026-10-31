// suspend_state.go — reconcile support for QE-020 suspend-to-disk.
// The suspend verb itself lands with the migrate machinery; the FSM
// only needs to know whether a recorded suspended state still has
// its state image on disk.
package main

import (
	"os"
	"path/filepath"
)

// suspendImageExists reports whether a state image survives in the
// VM dir — the saved vmstate a suspended record points at. Any of
// the candidate names (migrate exec output, savevm marker or the
// future tooling's canonical name) counts; a missing or empty image
// means the suspended record cannot resume and reconciles to
// stopped instead.
func suspendImageExists() bool {
	dir, name := vmdir(), vmname()
	for _, cand := range []string{
		name + ".state.gz",
		name + ".vmstate",
		name + ".suspend",
		"suspend.img",
	} {
		if st, err := os.Stat(filepath.Join(dir, cand)); err == nil && st.Size() > 0 {
			return true
		}
	}
	return false
}
