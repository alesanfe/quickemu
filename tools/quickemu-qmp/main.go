// quickemu-qmp: phase-1 incremental extraction per ADR-0001.
// A single-binary QMP client the Bash backend calls instead of socat.
// Contract: JSON envelope on stdout only, logs on stderr, typed errors.
package main

import (
	"strings"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

const version = "0.1.0"

type errObj struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	SchemaVersion int      `json:"schemaVersion"`
	Command       string   `json:"command"`
	Success       bool     `json:"success"`
	Data          any      `json:"data,omitempty"`
	Warnings      []string `json:"warnings"`
	Errors        []errObj `json:"errors"`
	Meta          meta     `json:"meta"`
}

type meta struct {
	Backend        string `json:"backend"`
	BackendVersion string `json:"backendVersion"`
}

func emit(cmd string, data any, code, msg string) int {
	env := envelope{SchemaVersion: 1, Command: cmd, Success: code == "",
		Data: data, Warnings: []string{}, Errors: []errObj{},
		Meta: meta{Backend: "quickemu-qmp", BackendVersion: version}}
	if code != "" {
		env.Errors = []errObj{{Code: code, Message: msg}}
	}
	b, _ := json.Marshal(env)
	fmt.Println(string(b))
	if code != "" {
		return 1
	}
	return 0
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-qmp <execute|watch> --socket <path> [--command <json>] [--timeout 5]")
		os.Exit(2)
	}
	os.Args = normArgs(os.Args)
	switch os.Args[1] {
	case "execute":
		fs := flag.NewFlagSet("execute", flag.ExitOnError)
		sock := fs.String("socket", "", "QMP unix socket path")
		cmd := fs.String("command", "", "QMP command JSON")
		timeout := fs.Duration("timeout", 5*time.Second, "overall timeout")
		raw := fs.Bool("raw", false, "print the bare QMP reply ({return/error}) not the envelope")
		fs.Parse(os.Args[2:])
		if *sock == "" || *cmd == "" {
			fmt.Fprintln(os.Stderr, "execute needs --socket and --command")
			os.Exit(2)
		}
		qmpRaw = *raw
		os.Exit(runExecute(*sock, *cmd, *timeout))
	case "watch":
		fs := flag.NewFlagSet("watch", flag.ExitOnError)
		sock := fs.String("socket", "", "QMP unix socket path")
		fs.Parse(os.Args[2:])
		if *sock == "" {
			fmt.Fprintln(os.Stderr, "watch needs --socket")
			os.Exit(2)
		}
		os.Exit(runWatch(*sock))
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		os.Exit(2)
	}
}

// normArgs: --flag=value parity (see sibling helpers).
func normArgs(a []string) []string {
	out := make([]string, 0, len(a))
	for _, s := range a {
		if i := strings.IndexByte(s, '='); strings.HasPrefix(s, "--") && i > 2 {
			out = append(out, s[:i], s[i+1:])
		} else {
			out = append(out, s)
		}
	}
	return out
}
