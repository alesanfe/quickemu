// tools/quickemu-cli/ops_adapter.go: subcommand -> sibling-tool call adapters
package main

import (
	"fmt"
	"os"
)

func vmOp(sub string, rest ...string) func() int {
	return vmOpEmit(true, sub, rest...)
}

// vmOpSilent: vmOp without the stdout passthrough — internal plumbing
// ops (snapshot-register/unregister) whose envelope must never leak
// into the caller's single-envelope output contract.
func vmOpSilent(sub string, rest ...string) func() int {
	return vmOpEmit(false, sub, rest...)
}

func vmOpEmit(print bool, sub string, rest ...string) func() int {
	return func() int {
		// The vm tool locates the conf by glob; QE_VMCONF tells it which
		// one the user actually passed (dirs can hold several).
		if vmConf != "" {
			os.Setenv("QE_VMCONF", vmConf)
		}
		// bin/quickemu (the entry wrapper) pre-sets QE_LAUNCHER so
		// generated shortcuts/services invoke the canonical entry point
		// rather than the inner Go binary; keep it when present.
		if os.Getenv("QE_LAUNCHER") == "" {
			if self, err := os.Executable(); err == nil {
				os.Setenv("QE_LAUNCHER", self)
			}
		}
		// QE-002 lock split: mutating subs take the exclusive lock,
		// read-only subs the shared one — either way the lock error
		// names the holding operation.
		if vmLockedOps[sub] && !vmLockAcquireOp(sub) {
			return 1
		}
		if vmSharedOps[sub] && !vmLockAcquireShared(sub) {
			return 1
		}
		out, err := tool("quickemu-vm", append([]string{sub}, rest...)...)
		if vmLockedOps[sub] {
			vmLockRelease()
		}
		if vmSharedOps[sub] {
			vmLockReleaseShared()
		}
		if print {
			fmt.Print(out)
		}
		return rc(err)
	}
}

func opsOp(sub string, rest ...string) func() int {
	return func() int {
		out, err := tool("quickemu-ops", append([]string{sub, "--vmdir", vmdir()}, rest...)...)
		fmt.Print(out)
		return rc(err)
	}
}

func qmpOp(cmd, okMsg func() string) func() int {
	return func() int {
		out, err := qmp(cmd())
		fmt.Print(out)
		if err == nil && okMsg != nil {
			fmt.Println(okMsg())
		}
		return rc(err)
	}
}
