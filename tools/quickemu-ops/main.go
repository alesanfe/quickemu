// quickemu-ops: ADR-0001 phase 3 — the async-operation registry moved
// to Go. Same on-disk schema as lib/quickemu/runtime/ops.sh
// (<vmdir>/ops/<operationId>.json) so files stay inter-operable.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/google/renameio/v2"
	"github.com/google/uuid"

	"github.com/alesanfe/quickemu/tools/qelib"
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

func die(code, msg string) { qelib.Fail(qelib.CmdName("ops"), "quickemu-ops/"+version, code, msg) }

func opsDir(vmdir string) string { return filepath.Join(vmdir, "ops") }

// safeID reports whether an operation id can be joined into an
// ops-dir path without escaping it — separators and .. are the
// traversal set.
func safeID(id string) bool {
	return id != "" && !strings.Contains(id, "..") &&
		!strings.ContainsAny(id, `/\`)
}

// checkID rejects operation ids that could escape the ops dir —
// --id is joined into a filesystem path, so separators and .. must
// never reach filepath.Join.
// verbosityRank/qeVerbose: QUICKEMU_VERBOSITY/VERBOSITY env gate —
// Bash verbosity.sh parity (quiet<normal<verbose<debug; the verbose
// line fires only at level >= verbose, on stderr).
func verbosityRank(s string) int {
	switch s {
	case "quiet":
		return 0
	case "verbose":
		return 2
	case "debug":
		return 3
	}
	return 1
}

func qeVerbose(msg string) {
	v := os.Getenv("QUICKEMU_VERBOSITY")
	if v == "" {
		v = os.Getenv("VERBOSITY")
	}
	if verbosityRank(v) >= 2 {
		fmt.Fprintln(os.Stderr, msg)
	}
}

func checkID(id string) {
	if !safeID(id) {
		die("INVALID_ARGS", "bad operation id")
	}
}

func load(dir, id string) *Op {
	checkID(id)
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		die("OPERATION_NOT_FOUND", "unknown operation "+id)
	}
	var o Op
	if json.Unmarshal(b, &o) != nil {
		die("OP_CORRUPT", id+" is not parseable")
	}
	// The embedded id feeds o.save()'s filepath.Join — file content is
	// untrusted, so it gets the same traversal check as the --id flag.
	if !safeID(o.OperationID) {
		die("OP_CORRUPT", id+" has unsafe embedded operation id")
	}
	return &o
}

// AtomicStore: durability requires fsync on the file AND its dir —
// renameio handles staging on the same filesystem, atomic replace and
// cleanup; Sync() gives the file-level durability guarantee.
func (o *Op) save(dir string) {
	// The embedded id is joined into the destination path — validate
	// at the sink so EVERY caller (set/done/recover/rollback/…) is
	// covered, not just paths that go through load().
	if !safeID(o.OperationID) {
		die("OP_CORRUPT", "unsafe embedded operation id "+o.OperationID)
	}
	b, _ := json.Marshal(o)
	f, err := renameio.TempFile(dir, filepath.Join(dir, o.OperationID+".json"))
	if err != nil {
		die("OP_PERSIST", err.Error())
	}
	defer f.Cleanup()
	// die() calls os.Exit — deferred cleanup would never run, so the
	// temp file is removed explicitly on every failure path.
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Cleanup()
		die("OP_PERSIST", err.Error())
	}
	if err := f.Sync(); err != nil {
		f.Cleanup()
		die("OP_PERSIST", err.Error())
	}
	if err := f.CloseAtomicallyReplace(); err != nil {
		f.Cleanup()
		die("OP_PERSIST", err.Error())
	}
}

func writeOp(dir string, o *Op) {
	b, _ := json.Marshal(o)
	fmt.Println(string(b))
	o.save(dir)
}

var exitFn = os.Exit

func main() { exitFn(run()) }

func run() int {
	os.Args = qelib.NormArgs(os.Args)
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-ops <new|set|done|fail|status|list|cancel|prune> --vmdir <dir> [args]")
		return 2
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
			return 0
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
		b, _ := json.Marshal(map[string]any{
			"finishedAt": time.Now().UTC().Format(time.RFC3339)})
		o.Result = json.RawMessage(b)
		writeOp(dir, o)
	case "fail":
		id, code, msg := flag(args, "--id"), flag(args, "--code"), flag(args, "--message")
		o := load(dir, id)
		o.State = "failed"
		b, _ := json.Marshal(map[string]any{"code": code, "message": msg})
		o.Error = json.RawMessage(b)
		writeOp(dir, o)
	case "status":
		id := flag(args, "--id")
		checkID(id)
		fmt.Println(string(must(filepath.Join(dir, id+".json"))))
	case "watch":
		return runWatch(dir, os.Stdout)
	case "list":
		ents, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		slices.Sort(ents)
		ops := make([]json.RawMessage, 0, len(ents))
		for _, e := range ents {
			// TOCTOU: an op file may be pruned between glob and read —
			// skip it instead of killing the whole list.
			if b, err := os.ReadFile(e); err == nil {
				ops = append(ops, json.RawMessage(b))
			}
		}
		b, _ := json.Marshal(map[string]any{"operations": ops})
		fmt.Println(string(b))
	case "cancel":
		id := flag(args, "--id")
		o := load(dir, id)
		switch o.State {
		case "completed", "failed", "cancelled":
			die("OP_FINISHED", "operation already finished")
		}
		// workerPid comes from file content — a crafted ops/*.json
		// could point SIGTERM at an unrelated process. Same ownership
		// proof the .pid consumers require: the pid's cmdline must
		// carry this ops tree or a quickemu*/quickget* binary; an
		// unreadable cmdline (foreign owner) is not a worker either.
		if o.WorkerPid > 0 && qelib.Alive(o.WorkerPid) && isWorker(o.WorkerPid, dir) {
			syscall.Kill(o.WorkerPid, syscall.SIGTERM)
			o.State = "cancelRequested" // worker confirms via set/cancelled
			writeOp(dir, o)
			return 0
		}
		o.State = "cancelled" // no live worker: cancel is final now
		writeOp(dir, o)
	case "prune":
		keep := 10
		fmt.Sscanf(flag(args, "--keep"), "%d", &keep)
		ents, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		slices.Sort(ents)
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
			if o.WorkerPid > 0 && qelib.Alive(o.WorkerPid) {
				continue
			}
			o.State = "failed"
			o.Error = json.RawMessage(`{"code":"OPERATION_INTERRUPTED","message":"worker dead"}`)
			writeOp(dir, &o)
			reaped++
			owner := "none"
			if o.WorkerPid > 0 {
				owner = fmt.Sprint(o.WorkerPid)
			}
			qeVerbose(fmt.Sprintf("op_recover: marked %s interrupted (owner=%s)", o.OperationID, owner))
		}
		fmt.Printf("{\"reaped\":%d}\n", reaped)
	case "rollback":
		id := flag(args, "--id")
		checkID(id)
		var o Op
		if json.Unmarshal(must(filepath.Join(dir, id+".json")), &o) != nil {
			die("OPERATION_NOT_FOUND", id)
		}
		rbCompensate(dir, o.Type)
		o.State = "failed"
		// o.Type is file content — marshal the message so a quote in
		// type cannot inject fields into the rewritten op.
		msg, _ := json.Marshal("rolled back " + o.Type + " effects")
		o.Error = json.RawMessage(`{"code":"OPERATION_ROLLBACK","message":` + string(msg) + `}`)
		writeOp(dir, &o)
		fmt.Printf("{\"rolledBack\":\"%s\"}\n", o.Type)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
		return 2
	}
	return 0
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
				if err := os.Rename(trash, vm); err != nil {
					fmt.Fprintln(os.Stderr, "rollback: trash restore:", err)
				}
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

// isWorker: pid ownership proof for cancel — the process must carry
// the ops tree (a --vmdir/vm path argument) or run a quickemu*/
// quickget* binary. Unreadable cmdline (foreign user, kernel thread,
// non-linux) proves nothing, so the pid is not a worker.
func isWorker(pid int, dir string) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(b) == 0 {
		return false
	}
	argv := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
	if len(argv) == 0 {
		return false
	}
	if base := filepath.Base(argv[0]); strings.HasPrefix(base, "quickemu") || strings.HasPrefix(base, "quickget") {
		return true
	}
	// Abs first: a relative dir degenerates the contains check
	// (vmdir "." would match nearly any argument).
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	vmdir := filepath.Dir(abs)
	// Path-boundary match: vmdir "/vms/myvm" must not match a sibling
	// worker's "/vms/myvm2/disk.qcow2" argument.
	mentions := func(a, d string) bool {
		return a == d || strings.HasPrefix(a, d+"/") ||
			strings.HasSuffix(a, "="+d) || strings.Contains(a, "="+d+"/")
	}
	for _, a := range argv[1:] {
		if mentions(a, vmdir) || mentions(a, abs) {
			return true
		}
	}
	return false
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
