package main

import (
	"fmt"
	"os"
)

// actSnapshotCmd: `--snapshot <action> [tag]` — the compound snapshot
// command from options.sh (apply/create/tree/verify/consolidate/
// delete/info|list), with --external for create.
func actSnapshotCmd() int {
	switch snapshotAct {
	case "apply":
		r := vmOp("snapshot-apply", vmdir(), snapshotTag)()
		vmOp("snapshot-info", vmdir())()
		return r
	case "create":
		args := []string{"snapshot-create", vmdir(), snapshotTag}
		if externalFlag {
			args = append(args, "--external")
		}
		r := vmOp(args[0], args[1:]...)()
		vmOp("snapshot-info", vmdir())()
		return r
	case "tree":
		return vmOp("snapshot-tree", vmdir())()
	case "verify":
		return vmOp("snapshot-verify", vmdir())()
	case "consolidate":
		r := vmOp("snapshot-consolidate", vmdir(), snapshotTag)()
		vmOp("snapshot-tree", vmdir())()
		return r
	case "delete":
		r := vmOp("snapshot-delete", vmdir(), snapshotTag)()
		vmOp("snapshot-info", vmdir())()
		return r
	case "info", "list":
		if outFmt == "json" {
			return vmOp("snapshot-list-json", vmdir())()
		}
		return vmOp("snapshot-info", vmdir())()
	default:
		fmt.Fprintf(os.Stderr, "ERROR! %q is not a supported snapshot action.\n", snapshotAct)
		usage()
		return 1
	}
}
