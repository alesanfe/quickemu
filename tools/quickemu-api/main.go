// quickemu-api: persistent HTTP daemon serving the v1 surface over
// localhost — read-only introspection of the on-disk formats (no
// shell-outs on the hot path). Auth: optional --token bearer.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

var (
	dataDir = qelib.XDGData() + "/quickemu"
	// tokens accepts multiple bearer tokens — rotation runs old+new
	// together during the window; a single --token is just a 1-list.
	tokens []string

	authMu    sync.Mutex
	authFails = map[string]*failRec{}
)

type failRec struct {
	n     int
	until time.Time
}

func runCmd(name string, args ...string) (string, error) {
	bin, err := qelib.FindTool(name)
	if err != nil {
		return "", err
	}
	out, err := execCommand(bin, args...).CombinedOutput()
	return string(out), err
}

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

const (
	authFailMax      = 10
	authFailWindow   = time.Minute
	authFailMaxHosts = 4096
)

// authThrottle returns false once addr has burned through its
// failed-auth allowance for the current window — an exposed daemon
// must not serve as an unbounded token-guessing oracle. The table is
// capped so an attacker spraying source IPs cannot grow the daemon's
// memory without bound: expired windows are swept at the cap, and a
// still-full table denies the new entry rather than evicting an
// in-progress record.
func authThrottle(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	authMu.Lock()
	defer authMu.Unlock()
	if len(authFails) >= authFailMaxHosts {
		if _, known := authFails[host]; !known {
			now := time.Now()
			for h, fr := range authFails {
				if now.After(fr.until) {
					delete(authFails, h)
				}
			}
			if len(authFails) >= authFailMaxHosts {
				return false
			}
		}
	}
	fr, ok := authFails[host]
	if !ok || time.Now().After(fr.until) {
		authFails[host] = &failRec{n: 1, until: time.Now().Add(authFailWindow)}
		return true
	}
	fr.n++
	return fr.n <= authFailMax
}

// tokenMatch runs the candidate against every accepted token in
// constant time per entry — no early-exit on the first match.
func tokenMatch(given string) bool {
	ok := false
	for _, t := range tokens {
		if subtle.ConstantTimeCompare([]byte(given), []byte(t)) == 1 {
			ok = true
		}
	}
	return ok
}

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(tokens) == 0 {
			next(w, r)
			return
		}
		// Constant-time compare keeps the token's length/content out
		// of timing side channels; CutPrefix enforces the scheme.
		given, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !tokenMatch(given) {
			if !authThrottle(r.RemoteAddr) {
				write(w, 429, map[string]any{"errors": []any{map[string]any{"code": "AUTH_THROTTLED"}}})
				return
			}
			write(w, 401, map[string]any{"errors": []any{map[string]any{"code": "UNAUTHORIZED"}}})
			return
		}
		next(w, r)
	}
}

// readTokenFile loads the bearer token from a file, refusing modes
// that let group/other read the secret.
func readTokenFile(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("token file must not be group/other-accessible (mode %04o)", st.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// isLocalBind reports whether listen stays off the network: unix
// sockets and loopback TCP addresses only (":port" binds every
// interface and is therefore not local).
func isLocalBind(listen string) bool {
	if strings.HasPrefix(listen, "/") {
		return true
	}
	host, _, err := net.SplitHostPort(listen)
	if err != nil || host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

var exitFn = os.Exit

func main() {
	listen := "127.0.0.1:8490"
	var allowExposed bool
	for i := 1; i < len(os.Args); i++ {
		fl := os.Args[i]
		switch fl {
		case "--insecure-allow-exposed":
			allowExposed = true
		case "--listen", "--unix", "--data-dir", "--token", "--token-file":
			if i+1 >= len(os.Args) {
				fmt.Fprintln(os.Stderr, fl+" needs a value")
				exitFn(2)
				return
			}
			v := os.Args[i+1]
			i++
			switch fl {
			case "--listen", "--unix":
				listen = v
			case "--data-dir":
				dataDir = v
			case "--token":
				// repeatable: --token old --token new during rotation
				tokens = append(tokens, v)
			case "--token-file":
				t, err := readTokenFile(v)
				if err != nil {
					fmt.Fprintln(os.Stderr, "token-file:", err)
					exitFn(1)
					return
				}
				tokens = append(tokens, t)
			}
		}
	}
	// A token under 16 chars is brute-forceable — reject at startup.
	for _, t := range tokens {
		if len(t) < 16 {
			fmt.Fprintln(os.Stderr, "--token must be at least 16 characters")
			exitFn(1)
			return
		}
	}
	// Loopback-only without a token is the safe default; anything
	// exposed to the network requires auth unless explicitly waived.
	if len(tokens) == 0 && !allowExposed && !isLocalBind(listen) {
		fmt.Fprintln(os.Stderr, "refusing to bind "+listen+" without a token (set --insecure-allow-exposed to override)")
		exitFn(1)
		return
	}
	ln, err := openListener(listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		exitFn(1)
		return
	}
	if !serveAPI(ln, listen) {
		return
	}
}

var serveAPI = func(ln net.Listener, listen string) bool {
	fmt.Println("quickemu-api listening on", listen)
	http.Serve(ln, newMux())
	return true
}

// openListener binds a unix socket (paths starting with /) or TCP
// address, refusing to steal a live unix socket.
func openListener(listen string) (net.Listener, error) {
	if strings.HasPrefix(listen, "/") {
		// Refuse to steal a live socket: if it answers, a daemon is
		// already bound — unlinking it would orphan its listener.
		if c, derr := net.DialTimeout("unix", listen, 500*time.Millisecond); derr == nil {
			c.Close()
			return nil, fmt.Errorf("socket already in use: %s", listen)
		}
		os.Remove(listen)
		ln, err := net.Listen("unix", listen)
		if err == nil {
			os.Chmod(listen, 0600)
		}
		return ln, err
	}
	return net.Listen("tcp", listen)
}

func newMux() *http.ServeMux {
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
		// Encoded separators (%2f, %5c) decode into the path *after*
		// routing — a vm name that smuggles one in is traversal
		// shaping, so reject on the escaped form too.
		ep := strings.ToLower(r.URL.EscapedPath())
		if strings.Contains(ep, "%2f") || strings.Contains(ep, "%5c") {
			write(w, 400, map[string]any{"errors": []any{map[string]any{"code": "INVALID_ARGUMENT"}}})
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/v1/vm/")
		parts := strings.SplitN(rest, "/", 2)
		vm := parts[0]
		sub := ""
		if len(parts) == 2 {
			sub = parts[1]
		}
		// vm names feed filepath.Join AND filepath.Glob — separators and
		// ".." are traversal, but glob metachars (* ? [) also re-shape the
		// match and leak other VMs' ops/state.
		if strings.Contains(vm, "..") || strings.ContainsAny(vm, `/\?*[]`) {
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
			var v struct {
				Pid int `json:"pid"`
			}
			json.Unmarshal(b, &v)
			// runtime.json alone is not proof — a hard host crash
			// leaves it behind; report stopped for a dead owner.
			if v.Pid <= 0 || !qelib.Alive(v.Pid) {
				write(w, 200, map[string]any{"data": map[string]any{"state": "stopped"}})
				return
			}
			write(w, 200, map[string]any{"data": map[string]any{"state": "running", "runtime": json.RawMessage(b)}})
		case "ops":
			ents, _ := filepath.Glob(filepath.Join(dataDir, "vms", vm, "ops", "*.json"))
			ops := make([]any, 0, len(ents))
			for _, e := range ents {
				var v any
				if b, err := os.ReadFile(e); err == nil &&
					json.Unmarshal(b, &v) == nil {
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
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil ||
			strings.Contains(req.VM, "..") ||
			strings.ContainsAny(req.VM, "/\\") || !safeCmds[req.Cmd] {
			write(w, 400, map[string]any{"errors": []any{map[string]any{"code": "INVALID_ARGUMENT"}}})
			return
		}
		if len(tokens) == 0 { // write endpoints require the token to be configured
			write(w, 403, map[string]any{"errors": []any{map[string]any{"code": "WRITE_DISABLED"}}})
			return
		}
		out, err := vmCmd(req.VM, req.Cmd)
		write(w, 200, map[string]any{"data": map[string]any{"output": out, "error": fmt.Sprint(err)}})
	}))
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"data": map[string]any{"ok": true}})
	})
	return mux
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
			err = os.WriteFile(filepath.Join(dir, vm+".paused"), nil, 0644)
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
