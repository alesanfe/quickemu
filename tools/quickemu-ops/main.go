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
	Phase         string          `json:"phase,omitempty"`
	Message       string          `json:"message,omitempty"`
	StartedAt     string          `json:"startedAt"`
	WorkerPid     int             `json:"workerPid,omitempty"`
	BootID        string          `json:"bootId,omitempty"`
	Result        json.RawMessage `json:"result"`
	Error         json.RawMessage `json:"error"`
}

// procBootID is the kernel boot-id source — a seam so tests can point
// it at a fixture file and simulate a host restart.
var procBootID = "/proc/sys/kernel/random/boot_id"

// bootID: the host's boot identity — an op whose recorded bootId
// differs from the current one was orphaned by a host restart
// (workerPid alone can't tell: pids recycle across boots).
func bootID() string {
	if b, err := os.ReadFile(procBootID); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

func die(code, msg string) { qelib.Fail(opsCmd(), "quickemu-ops/"+version, code, msg) }

// opsCmd: the v1 command name for this invocation — the user-facing
// query subs name the CLI flag they back (--op-status → ops.op-status),
// internal journal verbs keep ops.<verb>.
func opsCmd() string {
	sub := ""
	if len(os.Args) > 1 {
		sub = os.Args[1]
	}
	switch sub {
	case "status", "list", "cancel", "watch":
		return "ops.op-" + sub
	}
	return "ops." + sub
}

// jsonMode: --format json propagates as OUTPUT_FORMAT=json through the
// spawning CLI — query subs emit envelopes, journal verbs stay raw.
func jsonMode() bool { return os.Getenv("OUTPUT_FORMAT") == "json" }

func emitOK(cmd string, data any) {
	qelib.EmitOK(cmd, "quickemu-ops/"+version, data)
}

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
		var typ, vm, id, phase, msg string
		var maxOps int
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "--type":
				typ = args[i+1]
			case "--vm":
				vm = args[i+1]
			case "--id":
				id = args[i+1]
			case "--phase":
				phase = args[i+1]
			case "--message":
				msg = args[i+1]
			case "--max":
				fmt.Sscanf(args[i+1], "%d", &maxOps)
			}
		}
		if id == "" {
			id = uuid.NewString()
		} else {
			// Caller-chosen stable ids get the same traversal check as
			// --id flags and must not collide with a live journal entry.
			checkID(id)
			if _, err := os.Stat(filepath.Join(dir, id+".json")); err == nil {
				die("OP_EXISTS", "operation "+id+" already exists")
			}
		}
		// Admission control (QE-004): per-VM concurrency cap and
		// incompatible-type serialization refuse before the journal
		// entry exists, so a rejected op leaves no residue.
		admitNewOp(dir, vm, typ, maxOps)
		o := &Op{SchemaVersion: 1, OperationID: id, Type: typ, VM: vm,
			State: "queued", Progress: 0, Phase: phase, Message: msg,
			StartedAt: time.Now().UTC().Format(time.RFC3339),
			BootID:    bootID(),
			Result:    json.RawMessage("null"), Error: json.RawMessage("null")}
		o.save(dir)
		fmt.Println(id)
		// Retention policy (QE-004): env-driven sweep keeps the
		// journal bounded across runs.
		retentionSweep(dir)
	case "set":
		var id, state, phase, msg string
		var progress, wpid int
		haveProgress, havePhase, haveMsg := false, false, false
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "--id":
				id = args[i+1]
			case "--state":
				state = args[i+1]
			case "--progress":
				fmt.Sscanf(args[i+1], "%d", &progress)
				haveProgress = true
			case "--phase":
				phase = args[i+1]
				havePhase = true
			case "--message":
				msg = args[i+1]
				haveMsg = true
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
		if haveProgress {
			o.Progress = progress
		}
		if havePhase {
			o.Phase = phase
		}
		if haveMsg {
			o.Message = msg
		}
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
		b := must(filepath.Join(dir, id+".json"))
		if jsonMode() {
			emitOK("ops.op-status", json.RawMessage(b))
		} else {
			fmt.Println(string(b))
		}
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
		if jsonMode() {
			emitOK("ops.op-list", json.RawMessage(b))
		} else {
			fmt.Println(string(b))
		}
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
			if jsonMode() {
				emitOK("ops.op-cancel", map[string]any{
					"operationId": id, "state": o.State})
			}
			return 0
		}
		o.State = "cancelled" // no live worker: cancel is final now
		writeOp(dir, o)
		if jsonMode() {
			emitOK("ops.op-cancel", map[string]any{
				"operationId": id, "state": o.State})
		}
	case "prune":
		// Retention policy: --keep N bounds entries, --days D bounds
		// age; unset flags fall back to QE_OPS_KEEP / QE_OPS_DAYS.
		keep := 10
		fmt.Sscanf(envOr("QE_OPS_KEEP", flag(args, "--keep")), "%d", &keep)
		days := 0
		fmt.Sscanf(envOr("QE_OPS_DAYS", flag(args, "--days")), "%d", &days)
		fmt.Printf("{\"pruned\":%d}\n", pruneOps(dir, keep, days))
	case "recover":
		ents, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		reaped := 0
		boot := bootID()
		for _, e := range ents {
			var o Op
			if json.Unmarshal(must(e), &o) != nil {
				continue
			}
			if o.State != "queued" && o.State != "running" && o.State != "cancelRequested" {
				continue
			}
			// Host-restart reconnect (QE-004): an op journaled under a
			// different boot is orphaned regardless of what its pid
			// numbers look like now — they may name a recycled process.
			if o.BootID != "" && boot != "" && o.BootID != boot {
				o.State = "failed"
				o.Error = json.RawMessage(`{"code":"HOST_RESTART","message":"host restart"}`)
				writeOp(dir, &o)
				reaped++
				qeVerbose(fmt.Sprintf("op_recover: marked %s failed (host restart)", o.OperationID))
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

// envOr: flag value wins over env, env over "" — the layered config
// QE_OPS_* knobs use.
func envOr(env, v string) string {
	if v != "" {
		return v
	}
	return os.Getenv(env)
}

// exclusiveOpType: op types that must serialize against EVERY other
// active op on the same VM — a delete racing an export corrupts both
// sides. Non-exclusive types only conflict with exclusive ones.
func exclusiveOpType(t string) bool {
	switch t {
	case "delete", "import", "export", "clone", "linked-clone",
		"rename", "move", "start", "stop", "kill",
		"snapshot-apply", "snapshot-create", "snapshot-delete",
		"backup", "backup-restore", "compact", "resize":
		return true
	}
	return false
}

// activeOps lists the non-terminal ops of a VM (empty vm matches all).
// Unparseable files are skipped, never trusted.
func activeOps(dir, vm string) []*Op {
	out := []*Op{}
	ents, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, e := range ents {
		b, err := os.ReadFile(e)
		if err != nil {
			continue
		}
		var o Op
		if json.Unmarshal(b, &o) != nil {
			continue
		}
		if vm != "" && o.VM != vm {
			continue
		}
		switch o.State {
		case "queued", "running", "cancelRequested":
			out = append(out, &o)
		}
	}
	return out
}

// admitNewOp: QE-004 admission control — a per-VM concurrency cap
// (flag --max, else QE_OPS_MAX_CONCURRENT) plus incompatible-type
// serialization: an exclusive-type op coexists with nothing, a
// non-exclusive op only refuses exclusive holders.
func admitNewOp(dir, vm, typ string, maxOps int) {
	act := activeOps(dir, vm)
	if maxOps <= 0 {
		fmt.Sscanf(os.Getenv("QE_OPS_MAX_CONCURRENT"), "%d", &maxOps)
	}
	if maxOps > 0 && len(act) >= maxOps {
		die("OPS_LIMIT", fmt.Sprintf(
			"too many concurrent operations on %s (%d/%d)", vm, len(act), maxOps))
	}
	for _, o := range act {
		if exclusiveOpType(o.Type) || exclusiveOpType(typ) {
			die("OP_CONFLICT", fmt.Sprintf(
				"operation %s (%s) already active on %s", o.OperationID, o.Type, vm))
		}
	}
}

// opAge: when an op file claims it started — StartedAt first, file
// mtime as fallback (hand-planted journals often lack the field).
func opAge(e string) time.Time {
	var o Op
	if b, err := os.ReadFile(e); err == nil && json.Unmarshal(b, &o) == nil {
		if t, perr := time.Parse(time.RFC3339, o.StartedAt); perr == nil {
			return t
		}
	}
	if st, err := os.Stat(e); err == nil {
		return st.ModTime()
	}
	return time.Now()
}

// pruneOps enforces the retention policy: entries older than `days`
// go first, then the journal is bounded to the newest `keep` files
// (keep<0 disables the count bound). Returns the removed count.
func pruneOps(dir string, keep, days int) int {
	ents, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	pruned := 0
	var rest []string
	if days > 0 {
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		for _, e := range ents {
			if opAge(e).Before(cutoff) {
				if os.Remove(e) == nil {
					pruned++
				}
			} else {
				rest = append(rest, e)
			}
		}
	} else {
		rest = ents
	}
	slices.Sort(rest)
	for i := 0; keep >= 0 && i < len(rest)-keep; i++ {
		if os.Remove(rest[i]) == nil {
			pruned++
		}
	}
	return pruned
}

// retentionSweep: env-driven auto-retention after `new` — QE_OPS_KEEP
// bounds the journal length, QE_OPS_DAYS bounds entry age. Silent:
// retention never breaks the caller's op.
func retentionSweep(dir string) {
	kd, dd := os.Getenv("QE_OPS_KEEP"), os.Getenv("QE_OPS_DAYS")
	if kd == "" && dd == "" {
		return
	}
	keep := -1
	fmt.Sscanf(kd, "%d", &keep)
	days := 0
	fmt.Sscanf(dd, "%d", &days)
	pruneOps(dir, keep, days)
}

func must(p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		die("OPERATION_NOT_FOUND", p)
	}
	return b
}
