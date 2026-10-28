// quickemu-qmp: phase-1 incremental extraction per ADR-0001.
// A single-binary QMP client the Bash backend calls instead of socat.
// Contract: JSON envelope on stdout only, logs on stderr, typed errors.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const version = "0.1.0"

func emit(cmd string, data any, code, msg string) int {
	errs := []qelib.QEError{}
	if code != "" {
		errs = []qelib.QEError{{Code: code, Message: msg,
			Severity: "error", Retryable: false}}
	}
	qelib.Emit(cmd, "quickemu-qmp/"+version, code == "", data, errs)
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
	os.Args = qelib.NormArgs(os.Args)
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
