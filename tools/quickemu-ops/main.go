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
	"syscall"
	"time"
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
		die("NOT_FOUND", "unknown operation "+id)
	}
	var o Op
	if json.Unmarshal(b, &o) != nil {
		die("OP_CORRUPT", id+" is not parseable")
	}
	return &o
}

func (o *Op) save(dir string) {
	b, _ := json.Marshal(o)
	tmp := filepath.Join(dir, o.OperationID+".json.tmp")
	os.WriteFile(tmp, append(b, '\n'), 0o644)
	os.Rename(tmp, filepath.Join(dir, o.OperationID+".json"))
}

func writeOp(dir string, o *Op) {
	b, _ := json.Marshal(o)
	fmt.Println(string(b))
	o.save(dir)
}

func main() {
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
		id := fmt.Sprintf("%d-%d", time.Now().UnixNano()/1e6, os.Getpid())
		o := &Op{SchemaVersion: 1, OperationID: id, Type: typ, VM: vm,
			State: "queued", Progress: 0, StartedAt: time.Now().UTC().Format(time.RFC3339),
			Result: json.RawMessage("null"), Error: json.RawMessage("null")}
		o.save(dir)
		fmt.Println(id)
	case "set":
		var id, state string
		var progress int
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "--id":
				id = args[i+1]
			case "--state":
				state = args[i+1]
			case "--progress":
				fmt.Sscanf(args[i+1], "%d", &progress)
			case "--worker-pid":
				// parsed below via load
			}
		}
		o := load(dir, id)
		o.State = state
		o.Progress = progress
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
		}
		o.State = "cancelled"
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
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
		os.Exit(2)
	}
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
		die("NOT_FOUND", p)
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
