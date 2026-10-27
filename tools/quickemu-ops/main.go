// quickemu-ops: ADR-0001 phase 3 — the async-operation registry moved
// to Go. Same on-disk schema as lib/quickemu/runtime/ops.sh
// (<vmdir>/ops/<operationId>.json) so files stay inter-operable.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/google/renameio/v2"
	"github.com/google/uuid"
)

const version = "0.1.0"

type Op struct {
	SchemaVersion int             `json:"schemaVersion"`
	OperationID   string          `json:"operationId"`
	Type          string          `json:"type"`
	VM            string          `json:"vm"`
	State         string          `json:"state"`
	Progress      int             `json:"progress"`
	StartedAt     string          `json:"startedAt"`
	WorkerPid     int             `json:"workerPid,omitempty"`
	Result        json.RawMessage `json:"result"`
	Error         json.RawMessage `json:"error"`
}

func die(code, msg string) {
	b, _ := json.Marshal(map[string]any{"schemaVersion": 1, "success": false,
		"errors": []map[string]string{{"code": code, "message": msg}}})
	fmt.Println(string(b))
	os.Exit(1)
}

func opsDir(vmdir string) string { return filepath.Join(vmdir, "ops") }

func load(dir, id string) *Op {
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		die("OPERATION_NOT_FOUND", "unknown operation "+id)
	}
	var o Op
	if json.Unmarshal(b, &o) != nil {
		die("OP_CORRUPT", id+" is not parseable")
	}
	return &o
}

// AtomicStore: durability requires fsync on the file AND its dir —
// renameio handles staging on the same filesystem, atomic replace and
// cleanup; Sync() gives the file-level durability guarantee.
func (o *Op) save(dir string) {
	b, _ := json.Marshal(o)
	f, err := renameio.TempFile(dir, filepath.Join(dir, o.OperationID+".json"))
	if err != nil {
		die("OP_PERSIST", err.Error())
	}
	defer f.Cleanup()
	if _, err := f.Write(append(b, '\n')); err != nil {
		die("OP_PERSIST", err.Error())
	}
	if err := f.Sync(); err != nil {
		die("OP_PERSIST", err.Error())
	}
	if err := f.CloseAtomicallyReplace(); err != nil {
		die("OP_PERSIST", err.Error())
	}
}

func writeOp(dir string, o *Op) {
	b, _ := json.Marshal(o)
	fmt.Println(string(b))
	o.save(dir)
}

func main() {
	os.Args = normArgs(os.Args)
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-ops <new|set|done|fail|status|list|cancel|prune> --vmdir <dir> [args]")
		os.Exit(2)
	}
	sub := os.Args[1]
	if os.Getenv("OPS_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "args=%q\n", os.Args)
	}
	var vmdir string
	args := os.Args[2:]
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--vmdir" {
			vmdir = args[i+1]
		}
	}
	if vmdir == "" {
		die("BAD_ARGS", "--vmdir required")
	}
	dir := opsDir(vmdir)
	os.MkdirAll(dir, 0o755)

	switch sub {
	case "new":
		var typ, vm string
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "--type":
				typ = args[i+1]
			case "--vm":
				vm = args[i+1]
			}
		}
		id := uuid.NewString()
		o := &Op{SchemaVersion: 1, OperationID: id, Type: typ, VM: vm,
			State: "queued", Progress: 0, StartedAt: time.Now().UTC().Format(time.RFC3339),
			Result: json.RawMessage("null"), Error: json.RawMessage("null")}
		o.save(dir)
		fmt.Println(id)
	case "set":
		var id, state string
		var progress, wpid int
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "--id":
				id = args[i+1]
			case "--state":
				state = args[i+1]
			case "--progress":
				fmt.Sscanf(args[i+1], "%d", &progress)
			case "--worker-pid":
				fmt.Sscanf(args[i+1], "%d", &wpid)
			}
		}
		o := load(dir, id)
		// terminal states are one-way: a finished op never re-arms.
		switch o.State {
		case "completed", "failed", "cancelled", "interrupted":
			die("OP_FINISHED", "operation "+id+" is terminal ("+o.State+")")
		}
		// requested->confirmed cancellation is an explicit transition,
		// not a silent flag flip.
		if state == "cancelled" && o.State != "cancelRequested" {
			o.State = "cancelRequested"
			writeOp(dir, o)
			return
		}
		o.State = state
		o.Progress = progress
		if wpid > 0 {
			o.WorkerPid = wpid
		}
		writeOp(dir, o)
	case "done":
		id := flag(args, "--id")
		o := load(dir, id)
		o.State, o.Progress = "completed", 100
		o.Result = json.RawMessage(`{"finishedAt":"` + time.Now().UTC().Format(time.RFC3339) + `"}`)
		writeOp(dir, o)
	case "fail":
		id, code, msg := flag(args, "--id"), flag(args, "--code"), flag(args, "--message")
		o := load(dir, id)
		o.State = "failed"
		o.Error = json.RawMessage(`{"code":"` + code + `","message":"` + msg + `"}`)
		writeOp(dir, o)
	case "status":
		fmt.Println(string(must(filepath.Join(dir, flag(args, "--id")+".json"))))
	case "watch":
		os.Exit(runWatch(dir))
	case "list":
		ents, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		sort.Strings(ents)
		fmt.Print(`{"operations":[`)
		for i, e := range ents {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Print(string(must(e)))
		}
		fmt.Println("]}")
	case "cancel":
		id := flag(args, "--id")
		o := load(dir, id)
		switch o.State {
		case "completed", "failed", "cancelled":
			die("OP_FINISHED", "operation already finished")
		}
		if o.WorkerPid > 0 && syscall.Kill(o.WorkerPid, 0) == nil {
			syscall.Kill(o.WorkerPid, syscall.SIGTERM)
			o.State = "cancelRequested" // worker confirms via set/cancelled
			writeOp(dir, o)
			return
		}
		o.State = "cancelled" // no live worker: cancel is final now
		writeOp(dir, o)
	case "prune":
		keep := 10
		fmt.Sscanf(flag(args, "--keep"), "%d", &keep)
		ents, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		sort.Strings(ents)
		for i := 0; i < len(ents)-keep; i++ {
			os.Remove(ents[i])
		}
		fmt.Printf("{\"pruned\":%d}\n", max(0, len(ents)-keep))
	case "recover":
		ents, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		reaped := 0
		for _, e := range ents {
			var o Op
			if json.Unmarshal(must(e), &o) != nil {
				continue
			}
			if o.State != "queued" && o.State != "running" {
				continue
			}
			if o.WorkerPid > 0 && syscall.Kill(o.WorkerPid, 0) == nil {
				continue
			}
			o.State = "failed"
			o.Error = json.RawMessage(`{"code":"OPERATION_INTERRUPTED","message":"worker dead"}`)
			writeOp(dir, &o)
			reaped++
		}
		fmt.Printf("{\"reaped\":%d}\n", reaped)
	case "rollback":
		id := flag(args, "--id")
		if id == "" {
			die("INVALID_ARGS", "--id required")
		}
		var o Op
		if json.Unmarshal(must(filepath.Join(dir, id+".json")), &o) != nil {
			die("OPERATION_NOT_FOUND", id)
		}
		rbCompensate(dir, o.Type)
		o.State = "failed"
		o.Error = json.RawMessage(`{"code":"OPERATION_ROLLBACK","message":"rolled back ` + o.Type + ` effects"}`)
		writeOp(dir, &o)
		fmt.Printf("{\"rolledBack\":\"%s\"}\n", o.Type)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
		os.Exit(2)
	}
}

// rbCompensate undoes the filesystem effects of an interrupted op by type.
// Mirrors op_rollback in lib/quickemu/runtime/ops.sh exactly.
func rbCompensate(dir, typ string) {
	switch typ {
	case "download":
		for _, pat := range []string{"*.part", "*.zsync*", "*.tmp"} {
			for _, f := range globVM(dir, pat) {
				os.Remove(f)
			}
		}
	case "import", "export":
		for _, pat := range []string{".import-*", ".export-*"} {
			for _, f := range globVM(dir, pat) {
				os.RemoveAll(f)
			}
		}
	case "clone", "rename":
		for _, f := range globVM(dir, ".partial-*") {
			os.RemoveAll(f)
		}
	case "delete":
		trash := filepath.Join(filepath.Dir(dir), ".trash")
		vm := filepath.Join(filepath.Dir(dir), "vm")
		if _, err := os.Stat(trash); err == nil {
			if _, err := os.Stat(vm); os.IsNotExist(err) {
				os.Rename(trash, vm)
			}
		}
	case "compact", "snapshot":
		for _, pat := range []string{"*.tmp.qcow2", "*.compact-*"} {
			for _, f := range globVM(dir, pat) {
				os.Remove(f)
			}
		}
	}
}

// globVM globs a pattern inside the VM dir (parent of the ops dir).
func globVM(dir, pat string) []string {
	ents, _ := filepath.Glob(filepath.Join(filepath.Dir(dir), pat))
	return ents
}

func flag(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func must(p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		die("OPERATION_NOT_FOUND", p)
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// normArgs expands GNU-style --flag=value into --flag value so the
// positional pair parsers stay uniform (pflag parity without the dep).
func normArgs(a []string) []string {
	out := a[:0]
	for _, s := range a {
		if i := strings.IndexByte(s, '='); strings.HasPrefix(s, "--") && i > 2 {
			out = append(out, s[:i], s[i+1:])
		} else {
			out = append(out, s)
		}
	}
	return out
}
