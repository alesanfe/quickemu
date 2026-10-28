// quickemu-api: persistent HTTP daemon serving the v1 surface over
// localhost — read-only introspection of the on-disk formats (no
// shell-outs on the hot path). Auth: optional --token bearer.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

var (
	dataDir = qelib.XDGData() + "/quickemu"
	token   string
)

func runCmd(name string, args ...string) (string, error) {
	bin, err := qelib.FindTool(name)
	if err != nil {
		return "", err
	}
	out, err := exec.Command(bin, args...).CombinedOutput()
	return string(out), err
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			write(w, 401, map[string]any{"errors": []any{map[string]any{"code": "UNAUTHORIZED"}}})
			return
		}
		next(w, r)
	}
}

func main() {
	var listen = "127.0.0.1:8490"
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--listen":
			i++
			listen = os.Args[i]
		case "--unix":
			i++
			listen = os.Args[i]
		case "--data-dir":
			i++
			dataDir = os.Args[i]
		case "--token":
			i++
			token = os.Args[i]
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/vms", auth(func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(filepath.Join(dataDir, "vms.json"))
		if err != nil {
			write(w, 200, map[string]any{"data": map[string]any{}})
			return
		}
		var v any
		json.Unmarshal(b, &v)
		write(w, 200, map[string]any{"data": v})
	}))
	mux.HandleFunc("/v1/vm/", auth(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/v1/vm/")
		parts := strings.SplitN(rest, "/", 2)
		vm := parts[0]
		sub := ""
		if len(parts) == 2 {
			sub = parts[1]
		}
		if strings.Contains(vm, "..") || strings.ContainsAny(vm, "/\\") {
			write(w, 400, map[string]any{"errors": []any{map[string]any{"code": "INVALID_ARGUMENT"}}})
			return
		}
		switch sub {
		case "state":
			b, err := os.ReadFile(filepath.Join(dataDir, "vms", vm, vm+".runtime.json"))
			if err != nil {
				write(w, 200, map[string]any{"data": map[string]any{"state": "stopped"}})
				return
			}
			var v any
			json.Unmarshal(b, &v)
			write(w, 200, map[string]any{"data": map[string]any{"state": "running", "runtime": v}})
		case "ops":
			ents, _ := filepath.Glob(filepath.Join(dataDir, "vms", vm, "ops", "*.json"))
			ops := make([]any, 0, len(ents))
			for _, e := range ents {
				var v any
				if b, err := os.ReadFile(e); err == nil {
					json.Unmarshal(b, &v)
					ops = append(ops, v)
				}
			}
			write(w, 200, map[string]any{"data": map[string]any{"operations": ops}})
		default:
			write(w, 404, map[string]any{"errors": []any{map[string]any{"code": "NOT_FOUND"}}})
		}
	}))
	// POST /v1/vm/<n>/cmd — authenticated, whitelisted mutating ops
	// proxied to the CLI (the process boundary that already enforces
	// locks, plans and the state machine). Read-only without --token.
	safeCmds := map[string]bool{"start": true, "stop": true, "shutdown": true, "pause": true, "resume": true, "kill": true, "reset": true}
	mux.HandleFunc("/v1/vm-cmd/", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			write(w, 405, map[string]any{"errors": []any{map[string]any{"code": "METHOD"}}})
			return
		}
		var req struct {
			VM  string `json:"vm"`
			Cmd string `json:"cmd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
			strings.Contains(req.VM, "..") ||
			strings.ContainsAny(req.VM, "/\\") || !safeCmds[req.Cmd] {
			write(w, 400, map[string]any{"errors": []any{map[string]any{"code": "INVALID_ARGUMENT"}}})
			return
		}
		if token == "" { // write endpoints require the token to be configured
			write(w, 403, map[string]any{"errors": []any{map[string]any{"code": "WRITE_DISABLED"}}})
			return
		}
		out, err := vmCmd(req.VM, req.Cmd)
		write(w, 200, map[string]any{"data": map[string]any{"output": out, "error": fmt.Sprint(err)}})
	}))
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"data": map[string]any{"ok": true}})
	})
	var ln net.Listener
	var err error
	if strings.HasPrefix(listen, "/") {
		os.Remove(listen)
		ln, err = net.Listen("unix", listen)
		os.Chmod(listen, 0600)
	} else {
		ln, err = net.Listen("tcp", listen)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	fmt.Println("quickemu-api listening on", listen)
	http.Serve(ln, mux)
}

// vmCmd dispatches whitelisted VM ops to the Go backend tools —
// quickemu-launch for start, quickemu-qmp for monitor commands. No
// Bash delegation: the .paused marker mirrors pause_vm/resume_vm.
func vmCmd(vm, cmd string) (string, error) {
	dir := filepath.Join(dataDir, "vms", vm)
	conf := filepath.Join(dir, vm+".conf")
	sock := filepath.Join(dir, vm+"-qmp.socket")
	qmp := func(c string) (string, error) {
		payload, _ := json.Marshal(map[string]any{"execute": c})
		return runCmd("quickemu-qmp", "execute", "--socket", sock,
			"--command", string(payload))
	}
	switch cmd {
	case "start":
		return runCmd("quickemu-launch", conf, "--run")
	case "shutdown":
		return qmp("system_powerdown")
	case "pause":
		out, err := qmp("stop")
		if err == nil {
			os.WriteFile(filepath.Join(dir, vm+".paused"), nil, 0644)
		}
		return out, err
	case "resume":
		out, err := qmp("cont")
		if err == nil {
			os.Remove(filepath.Join(dir, vm+".paused"))
		}
		return out, err
	case "reset":
		return qmp("system_reset")
	case "stop", "kill":
		return qmp("quit")
	}
	return "", fmt.Errorf("unsupported cmd %q", cmd)
}
