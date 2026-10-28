package qespec

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMain self-bootstraps: the suite exercises real repo binaries, so
// it ensures entries (bin/) and tools are built before running.
func TestMain(m *testing.M) {
	root := Root()
	need := false
	for _, p := range []string{
		"bin/quickemu", "bin/quickget", "bin/quickgetc", "bin/quickreport",
		"tools/quickemu-vm/quickemu-vm", "tools/quickemu-qmp/quickemu-qmp",
		"tools/quickemu-ops/quickemu-ops", "tools/quickget-catalog/quickget-catalog",
		"tools/quickget-vm/quickget-vm",
	} {
		if st, err := os.Stat(filepath.Join(root, p)); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			need = true
			break
		}
	}
	if need {
		fmt.Println("qespec: building tools (go run ./tools/dev build)")
		cmd := exec.Command("go", "run", "./tools/dev", "build")
		cmd.Dir = root
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "qespec: build failed: %v\n", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
