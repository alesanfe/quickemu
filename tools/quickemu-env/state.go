// state.go: the env state dir <envdir>/.quickemu-env/ — state.json
// holds resolved conf paths, which artifacts the env created (destroy
// only touches those), net port assignments and last-known pids. The
// ports/ subdirectory is a quickemu-ports registry handed to the
// sibling binary via --registry.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// stateDir is where per-env bookkeeping lives.
func (e *Env) stateDir() string {
	return filepath.Join(e.Dir, ".quickemu-env")
}

func (e *Env) stateFile() string {
	return filepath.Join(e.stateDir(), "state.json")
}

// portsReg is the quickemu-ports registry dir for this env.
func (e *Env) portsReg() string {
	return filepath.Join(e.stateDir(), "ports")
}

// vmState is one state.json vms[] record.
type vmState struct {
	Conf    string `json:"conf"`    // resolved .conf path (abs)
	Created bool   `json:"created"` // the env produced this conf (quickget/mkconf)
	VMdir   string `json:"vmdir"`   // env-managed VM dir (created VMs only)
	PID     int    `json:"pid"`     // last observed qemu pid
	Started bool   `json:"started"` // started by the latest `up`
}

// netState is one state.json nets[] record — the persisted port
// assignment for socket/mcast nets plus the qemu pid owning the
// listener (0 until the listener VM has started).
type netState struct {
	Kind     string `json:"kind"`
	Port     int    `json:"port"`
	Listener string `json:"listener"` // vm holding listen= (socket kind)
	Owner    int    `json:"owner"`    // pid responsible for the port
}

// envState is the whole persisted record.
type envState struct {
	Version int                  `json:"version"`
	Env     string               `json:"env"`
	File    string               `json:"file"`
	VMs     map[string]*vmState  `json:"vms"`
	Nets    map[string]*netState `json:"nets"`
}

func newState(e *Env) *envState {
	return &envState{
		Version: 1, Env: e.Name, File: e.File,
		VMs: map[string]*vmState{}, Nets: map[string]*netState{},
	}
}

// loadState reads state.json; a missing file yields a fresh record.
func (e *Env) loadState() *envState {
	st := newState(e)
	b, err := os.ReadFile(e.stateFile())
	if err != nil {
		return st
	}
	var s envState
	if json.Unmarshal(b, &s) != nil {
		return st // corrupt state starts clean rather than wedging `up`
	}
	if s.VMs == nil {
		s.VMs = map[string]*vmState{}
	}
	if s.Nets == nil {
		s.Nets = map[string]*netState{}
	}
	return &s
}

// save persists state.json atomically (tmp + rename).
func (e *Env) save(st *envState) {
	if err := os.MkdirAll(e.portsReg(), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: env state dir: %v\n", err)
		return
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	tmp := e.stateFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: env state save: %v\n", err)
		return
	}
	os.Rename(tmp, e.stateFile())
}

// ---------- .conf reading ----------

// confKV reads a quickemu .conf into a key->value map. The conf syntax
// is shell-ish key="value" lines; only the simple unquoted/quoted
// forms quickget/mkconf emit are decoded (lines we cannot parse are
// skipped — the launcher is the authoritative parser).
func confKV(path string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		i := strings.IndexByte(l, '=')
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(l[:i])
		v := strings.TrimSpace(l[i+1:])
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' ||
			v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out
}

// confName mirrors quickemu-cli vmname(): conf basename minus ext.
func confName(conf string) string {
	return strings.TrimSuffix(filepath.Base(conf), filepath.Ext(conf))
}

// vmDir mirrors quickemu-cli vmdir(): dirname(disk_img) when the conf
// sets it (relative stays inside the conf dir), else <confdir>/<name>.
func vmDir(conf string) string {
	confDir := filepath.Dir(conf)
	if d := confKV(conf)["disk_img"]; d != "" {
		if filepath.IsAbs(d) {
			return filepath.Dir(d)
		}
		j := filepath.Join(confDir, d)
		if rel, err := filepath.Rel(confDir, j); err == nil &&
			rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.Dir(j)
		}
	}
	return filepath.Join(confDir, confName(conf))
}

// ---------- runtime state ----------

// vmRuntime is the live state of one VM for status/inventory.
type vmRuntime struct {
	State string // running|paused|stopped|stale|missing
	PID   int
	Ports map[string]int // .ports file: name -> host port
}

// inspect reads pidfile/.paused/.ports under vmdir. alive is the
// EPERM-aware liveness check (qelib.Alive).
func inspect(conf string) vmRuntime {
	r := vmRuntime{State: "stopped", Ports: map[string]int{}}
	if conf == "" {
		r.State = "missing"
		return r
	}
	dir, name := vmDir(conf), confName(conf)
	pid := 0
	if b, err := os.ReadFile(filepath.Join(dir, name+".pid")); err == nil {
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	if pid > 0 {
		if alive(pid) {
			r.PID = pid
			if _, err := os.Stat(filepath.Join(dir, name+".paused")); err == nil {
				r.State = "paused"
			} else {
				r.State = "running"
			}
		} else {
			r.State = "stale" // pidfile owner is gone — unclean end
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, name+".ports")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			f := strings.Split(strings.TrimSpace(l), ",")
			if len(f) >= 2 {
				if p, err := strconv.Atoi(f[1]); err == nil {
					r.Ports[f[0]] = p
				}
			}
		}
	}
	return r
}

// sshPort resolves the host-side SSH forward for a VM: the live
// .ports record first, then the conf's ssh_port key.
func sshPort(conf string, r vmRuntime) int {
	if p, ok := r.Ports["ssh"]; ok {
		return p
	}
	if p, err := strconv.Atoi(confKV(conf)["ssh_port"]); err == nil {
		return p
	}
	return 0
}

// agentSock is the QEMU guest-agent channel path (quickemu-launch
// qgaSockPath parity): the .s/agent socket, with the legacy
// <name>-agent.sock still resolving for pre-.s launches.
func agentSock(conf string) string {
	return qelib.SockPath(vmDir(conf), confName(conf), qelib.SockAgent)
}

// ---------- small fs helpers ----------

func readFile(p string) ([]byte, error) { return os.ReadFile(p) }

func absPath(p string) (string, error) { return filepath.Abs(p) }

func dirOf(p string) string { return filepath.Dir(p) }

// alive is the liveness seam — tests stub it to model dead pids.
var alive = func(pid int) bool { return qelibAlive(pid) }

// nowUTC is the timestamp seam for state records.
var nowUTC = func() time.Time { return time.Now().UTC() }
