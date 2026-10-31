// helpers.go — shared plumbing for the v1 API handlers: the v1
// envelope writers ({"ok":true,"data":...} /
// {"ok":false,"error":{code,message}}; the legacy "errors" array rides
// along for pre-QE-080 clients), VM identity/dir resolution against
// the on-disk layout (<datadir>/vms.json inventory + <vmdir> state
// files), conf/.ports parsing, the FSM-state readers, allowedOps and
// the bounded subprocess helper every tool() call goes through.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// ---------- v1 envelope ----------

// okData: the success envelope {"ok":true,"data":...}. HTTP code is
// always 200 unless the caller says otherwise (POST enqueue → 202).
func okData(w http.ResponseWriter, data any) { okDataCode(w, 200, data) }

func okDataCode(w http.ResponseWriter, code int, data any) {
	write(w, code, map[string]any{"ok": true, "data": data})
}

// errData: the error envelope {"ok":false,"error":{code,message}} —
// "errors" is kept as a one-element array so clients written against
// the pre-QE-080 shape keep working.
func errData(w http.ResponseWriter, code int, ecode, msg string) {
	e := map[string]any{"code": ecode, "message": msg}
	write(w, code, map[string]any{
		"ok": false, "error": e, "errors": []any{e}})
}

// ---------- identity ----------

// badVMName rejects names that would traverse out of the VM tree or
// re-shape filepath.Glob patterns — same rule the /v1/vm/ router used.
func badVMName(n string) bool {
	return n == "" || strings.Contains(n, "..") ||
		strings.ContainsAny(n, `/\?*[]`)
}

// badOpID: operation ids join into <vmdir>/ops/<id>.json — the ops
// tool's safeID rule (no separators, no "..").
func badOpID(id string) bool {
	return id == "" || strings.Contains(id, "..") ||
		strings.ContainsAny(id, `/\`)
}

// ---------- inventory / dir resolution ----------

// vmsJSON reads <dataDir>/vms.json — a map keyed by instance id whose
// values carry {vm, state, dir, lastSeen}. Missing/corrupt → nil (the
// registry is advisory; disk state is authoritative).
func vmsJSON() map[string]map[string]any {
	b, err := os.ReadFile(filepath.Join(dataDir, "vms.json"))
	if err != nil {
		return nil
	}
	var m map[string]map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

// vmDirFor resolves a VM name to its state directory:
//  1. <dataDir>/vms/<name> when it exists (API-managed layout),
//  2. the inventory's dir for an entry naming this VM,
//  3. <dataDir>/vms/<name> again as the conventional guess when the
//     inventory has no entry (it need not exist — callers 404 when
//     nothing inside the dir resolves either).
func vmDirFor(name string) string {
	d := filepath.Join(dataDir, "vms", name)
	if st, err := os.Stat(d); err == nil && st.IsDir() {
		return d
	}
	for _, e := range vmsJSON() {
		if v, _ := e["vm"].(string); v != name {
			continue
		}
		if dir, _ := e["dir"].(string); dir != "" {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				return dir
			}
		}
	}
	if st, err := os.Stat(d); err == nil && st.IsDir() {
		return d
	}
	// A loose <name>.conf in the vms root follows the quickget
	// convention <confdir>/<vmname>.
	if _, err := os.Stat(filepath.Join(dataDir, "vms", name+".conf")); err == nil {
		return d
	}
	return ""
}

// vmConfFor: <dir>/<name>.conf preferred, else the first *.conf in the
// dir (clones can carry several; QE_VMCONF is a CLI concern).
func vmConfFor(dir, name string) string {
	if _, err := os.Stat(filepath.Join(dir, name+".conf")); err == nil {
		return filepath.Join(dir, name+".conf")
	}
	ents, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
	if len(ents) > 0 {
		sort.Strings(ents)
		return ents[0]
	}
	return ""
}

// vmEnt is one row of the merged VM inventory.
type vmEnt struct {
	Key      string // inventory key (instance id or dir basename)
	Name     string
	Dir      string
	Recorded string // inventory-recorded state (advisory)
	LastSeen string
}

// collectVMs merges <dataDir>/vms/*/ (on-disk layout) with the
// vms.json registry, deduped by VM name; disk wins for the dir.
func collectVMs() []vmEnt {
	out := []vmEnt{}
	seen := map[string]int{}
	if ents, err := os.ReadDir(filepath.Join(dataDir, "vms")); err == nil {
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			seen[name] = len(out)
			out = append(out, vmEnt{Key: name, Name: name,
				Dir: filepath.Join(dataDir, "vms", name)})
		}
	}
	for k, e := range vmsJSON() {
		name, _ := e["vm"].(string)
		if name == "" {
			name = k
		}
		dir, _ := e["dir"].(string)
		rec, _ := e["state"].(string)
		ls, _ := e["lastSeen"].(string)
		if i, ok := seen[name]; ok {
			out[i].Recorded, out[i].LastSeen = rec, ls
			if out[i].Dir == "" {
				out[i].Dir = dir
			}
			continue
		}
		if dir == "" {
			dir = filepath.Join(dataDir, "vms", name)
		}
		seen[name] = len(out)
		out = append(out, vmEnt{Key: k, Name: name, Dir: dir,
			Recorded: rec, LastSeen: ls})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ---------- conf / ports ----------

// confMap parses the VM's .conf key="value" lines (quotes stripped —
// same surface confGet uses).
func confMap(dir, name string) map[string]string {
	out := map[string]string{}
	conf := vmConfFor(dir, name)
	if conf == "" {
		return out
	}
	b, err := os.ReadFile(conf)
	if err != nil {
		return out
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		kv := strings.SplitN(l, "=", 2)
		if len(kv) != 2 {
			continue
		}
		out[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"'`)
	}
	return out
}

// confSafeKeys: the allowlist exposed by GET /v1/vm/<n>. Anything
// carrying host command-lines or secrets-shaped values (extra_args,
// passwords, tokens) is excluded — conf contents are user data.
var confSafeKeys = map[string]bool{
	"guest_os": true, "arch": true, "ram": true, "cpu_cores": true,
	"disk_img": true, "disk_format": true, "iso": true, "img": true,
	"display": true, "network": true, "port_forwards": true,
	"ssh_port": true, "spice_port": true, "public_dir": true,
	"macaddr": true, "tpm": true, "efi": true, "secure_boot": true,
	"boot": true, "video": true, "sound_card": true, "sound_duplex": true,
	"mouse": true, "keyboard": true, "keyboard_layout": true,
	"monitor": true, "serial": true, "viewer": true, "fullscreen": true,
	"snapshot": true, "qga": true, "nested": true, "pxe": true,
	"iothreads": true, "mq": true, "watchdog": true,
	"watchdog_action": true, "vsock": true, "cpu_pinning": true,
	"gl": true, "usb_redir": true, "access": true, "ssh_access": true,
	"braille": true, "width": true, "height": true, "tags": true,
	"group": true, "floppy": true, "fixed_iso": true,
	"monitor_telnet_host": true, "monitor_telnet_port": true,
	"serial_telnet_host": true, "serial_telnet_port": true,
}

// confSecretShape: keys whose name smells like a secret never surface
// even if a future conf knob uses them.
func confSecretShape(k string) bool {
	k = strings.ToLower(k)
	for _, bad := range []string{"pass", "secret", "token", "key", "cred"} {
		if strings.Contains(k, bad) {
			return true
		}
	}
	return false
}

func confSafe(dir, name string) map[string]string {
	out := map[string]string{}
	for k, v := range confMap(dir, name) {
		if confSafeKeys[k] && !confSecretShape(k) {
			out[k] = v
		}
	}
	return out
}

// readPorts parses <vmdir>/<vm>.ports — "name,port,host" records the
// launcher appends in run mode (the "unix,<path>" records carry a
// socket path in the port column and are reported as-is).
func readPorts(dir, name string) []map[string]any {
	out := []map[string]any{}
	b, err := os.ReadFile(filepath.Join(dir, name+".ports"))
	if err != nil {
		return out
	}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.SplitN(l, ",", 3)
		if len(f) == 0 || f[0] == "" {
			continue
		}
		rec := map[string]any{"name": f[0]}
		if len(f) > 1 {
			rec["port"] = f[1]
		}
		if len(f) > 2 {
			rec["host"] = f[2]
		}
		out = append(out, rec)
	}
	return out
}

// ---------- state ----------

// fsmRec reads <vmdir>/<vm>.state.json — the QE-003 machine record
// {state,pid,lastFrom,lastTo,cause,actor,transitionedAt,updatedAt}.
// nil when absent/corrupt (a corrupt record is no record).
func fsmRec(dir, name string) map[string]any {
	b, err := os.ReadFile(filepath.Join(dir, name+".state.json"))
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	if s, _ := m["state"].(string); s == "" {
		return nil
	}
	return m
}

// runtimeRec reads <vmdir>/<vm>.runtime.json (the launcher's identity
// record — pid + fingerprints).
func runtimeRec(dir, name string) map[string]any {
	b, err := os.ReadFile(filepath.Join(dir, name+".runtime.json"))
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

func recPID(m map[string]any) int {
	if m == nil {
		return 0
	}
	switch v := m["pid"].(type) {
	case float64:
		return int(v)
	case string:
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}

// pidFilePID: <vmdir>/<vm>.pid — the cheap liveness handle the CLI
// uses; 0 on any problem.
func pidFilePID(dir, name string) int {
	b, err := os.ReadFile(filepath.Join(dir, name+".pid"))
	if err != nil {
		return 0
	}
	pid := 0
	fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid)
	return pid
}

// liveishFSM: machine states that imply a QEMU process should exist.
func liveishFSM(st string) bool {
	switch st {
	case "running", "paused", "starting", "saving", "stopping", "suspended":
		return true
	}
	return false
}

// vmEvidence is the resolved truth for one VM: the FSM record when it
// exists plus process liveness — the API never writes, so a recorded
// live-state with a dead pid degrades to a derived word, not a
// reconcile write.
type vmEvidence struct {
	State   string
	Pid     int
	Alive   bool
	FSM     map[string]any
	Runtime map[string]any
}

func probeVM(dir, name, recorded string) vmEvidence {
	fsm := fsmRec(dir, name)
	rt := runtimeRec(dir, name)
	paused := fileExists(filepath.Join(dir, name+".paused"))
	pid := recPID(rt)
	if pid <= 0 && fsm != nil {
		pid = recPID(fsm)
	}
	if pid <= 0 {
		pid = pidFilePID(dir, name)
	}
	alive := pid > 0 && qelib.Alive(pid)
	st := ""
	if fsm != nil {
		st, _ = fsm["state"].(string)
	}
	switch {
	case fsm != nil && !liveishFSM(st):
		// non-live machine word is the truth (stopped/crashed/error/…)
	case alive:
		if paused && st != "paused" {
			st = "paused"
		}
		if st == "" {
			st = "paused"
			if !paused {
				st = "running"
			}
		}
	case fsm != nil:
		// Recorded live-ish but no live process — degrade honestly.
		if st == "stopping" {
			st = "stopped"
		} else {
			st = "crashed"
		}
	case rt != nil || pid > 0:
		// No machine memory: pidfile/runtime evidence only.
		st = "stale"
	case recorded != "":
		st = recorded
	default:
		st = "stopped"
	}
	return vmEvidence{State: st, Pid: pid, Alive: alive, FSM: fsm, Runtime: rt}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// allowedOpsFor maps the FSM/derived state onto the allowed-ops list —
// same tables the CLI publishes (quickemu-cli allowedOps parity) plus
// the FSM-only words the CLI never derived.
func allowedOpsFor(state string) []string {
	var ops string
	switch state {
	case "running":
		ops = "pause resume shutdown reset kill status monitor-cmd screenshot send-keys type snapshot-clone export"
	case "paused", "suspended", "saving":
		ops = "resume shutdown kill status monitor-cmd screenshot"
	case "stopped", "stale", "defined", "crashed", "error":
		ops = "start delete clone rename export import snapshot apply create delete info list inspect-conf delete-plan"
	case "archived":
		ops = "restore trash-list status inspect-conf delete"
	case "starting", "stopping", "recovering":
		// mid-transition: observe only, plus kill for a stuck boot.
		ops = "status kill"
	default:
		// unknown, preparing, importing, exporting, deleting, …
		ops = "status"
	}
	return strings.Fields(ops)
}

// ---------- tool calls ----------

// toolTimeout bounds every subprocess the API spawns — the daemon
// must never hang a request on a stuck sibling tool.
var toolTimeout = 15 * time.Second

// toolT runs a sibling tool with a hard wall-clock bound. Resolution
// is qelib.FindTool (sibling-first, PATH last); execution goes through
// the execCommand seam so tests substitute stubs.
func toolT(name string, d time.Duration, env []string, args ...string) (string, error) {
	bin, err := qelib.FindTool(name)
	if err != nil {
		return "", err
	}
	cmd := execCommand(bin, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	type res struct {
		out []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		o, e := cmd.CombinedOutput()
		ch <- res{o, e}
	}()
	select {
	case r := <-ch:
		return string(r.out), r.err
	case <-time.After(d):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		<-ch
		return "", fmt.Errorf("%s timed out after %s", name, d)
	}
}

// cliTool resolves the unified CLI entry — the packaged bin/quickemu
// entrypoint first, then the dev-tree quickemu-cli binary.
func cliTool() (string, error) {
	for _, n := range []string{"quickemu", "quickemu-cli"} {
		if b, err := qelib.FindTool(n); err == nil {
			return b, nil
		}
	}
	return "", fmt.Errorf("quickemu CLI: not found")
}

// toolTCLI runs quickemu <args...> through the resolved entrypoint.
// The ctx-owned kill avoids racing on cmd.Process: Start() publishes
// it inside the goroutine that ran CombinedOutput, so a parallel
// timeout branch could never read it safely.
func toolTCLI(d time.Duration, args ...string) (string, error) {
	bin, err := cliTool()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	out, err := execCommandCtx(ctx, bin, args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("quickemu timed out after %s", d)
	}
	return string(out), err
}

// xdgEnv makes sibling tools that resolve XDG paths (trash root,
// inventory) see the same data-dir this daemon serves when --data-dir
// overrode the default.
func xdgEnv() []string {
	return []string{"XDG_DATA_HOME=" + filepath.Dir(dataDir)}
}

// ---------- op journal ----------

// opRec mirrors tools/quickemu-ops' Op schema (<vmdir>/ops/<id>.json).
type opRec struct {
	OperationID string          `json:"operationId"`
	Type        string          `json:"type"`
	VM          string          `json:"vm"`
	State       string          `json:"state"`
	Progress    int             `json:"progress"`
	Phase       string          `json:"phase"`
	Message     string          `json:"message"`
	StartedAt   string          `json:"startedAt"`
	WorkerPid   int             `json:"workerPid"`
	Result      json.RawMessage `json:"result"`
	Error       json.RawMessage `json:"error"`
	File        string          `json:"-"`
}

// opActive: states that still hold an admission slot.
func opActive(st string) bool {
	switch st {
	case "queued", "running", "cancelRequested":
		return true
	}
	return false
}

// exclusiveOpType mirrors the ops tool's exclusive table — an op of
// one of these types serializes against EVERY other active op on the
// same VM.
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

// apiOpTypes: the op types POST /v1/ops accepts — everything the ops
// tool knows plus the async jobs the API itself enqueues.
var apiOpTypes = map[string]bool{
	"start": true, "stop": true, "kill": true, "delete": true,
	"clone": true, "linked-clone": true, "rename": true, "move": true,
	"import": true, "export": true, "download": true, "install": true,
	"snapshot": true, "snapshot-create": true, "snapshot-apply": true,
	"snapshot-delete": true, "backup": true, "backup-restore": true,
	"backup-verify": true, "compact": true, "resize": true,
	"trash-restore": true, "trash-empty": true,
}

// opsDirs lists every journal dir the API watches — one per known VM
// (the on-disk vms/ layout plus inventory-recorded dirs).
func opsDirs() map[string]string {
	out := map[string]string{}
	for _, e := range collectVMs() {
		if e.Dir != "" {
			out[e.Dir] = e.Name
		}
	}
	return out
}

// scanOps reads one journal dir; corrupt entries are skipped, never
// trusted (same posture as the ops tool's list).
func scanOps(dir string) []*opRec {
	ents, _ := filepath.Glob(filepath.Join(dir, "ops", "*.json"))
	sort.Strings(ents)
	out := []*opRec{}
	for _, e := range ents {
		b, err := os.ReadFile(e)
		if err != nil {
			continue
		}
		var o opRec
		if json.Unmarshal(b, &o) != nil || badOpID(o.OperationID) {
			continue
		}
		if o.VM == "" {
			o.VM = filepath.Base(dir)
		}
		o.File = e
		out = append(out, &o)
	}
	return out
}

// allOps aggregates the journal of every known VM dir.
func allOps() []*opRec {
	out := []*opRec{}
	for dir := range opsDirs() {
		out = append(out, scanOps(dir)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt != out[j].StartedAt {
			return out[i].StartedAt < out[j].StartedAt
		}
		return out[i].OperationID < out[j].OperationID
	})
	return out
}

func findOp(id string) *opRec {
	for _, o := range allOps() {
		if o.OperationID == id {
			return o
		}
	}
	return nil
}

// procBootID: boot identity for direct journal writes (same source
// the ops tool uses) — orphaned ops from a previous boot get reaped.
var procBootID = "/proc/sys/kernel/random/boot_id"

func bootID() string {
	if b, err := os.ReadFile(procBootID); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

// genOpID: stable, filesystem-safe id for journal entries written by
// the API fallback path ("op-<utc>-<rand>").
func genOpID() string {
	rb := make([]byte, 4)
	if _, err := rand.Read(rb); err != nil {
		copy(rb, []byte("qe00"))
	}
	return "op-" + time.Now().UTC().Format("20060102T150405Z") +
		"-" + hex.EncodeToString(rb)
}

// enqueueOp registers a queued op in the journal — preferring the
// canonical quickemu-ops writer (admission control + retention live
// there); when the sibling is missing the API writes the identical
// record itself after a lightweight conflict check, so an ops-less
// install still queues work instead of refusing it.
func enqueueOp(dir, vm, typ, phase, msg, wantID string) (string, error) {
	args := []string{"new", "--vmdir", dir, "--type", typ, "--vm", vm}
	if phase != "" {
		args = append(args, "--phase", phase)
	}
	if msg != "" {
		args = append(args, "--message", msg)
	}
	if wantID != "" {
		args = append(args, "--id", wantID)
	}
	out, err := toolT("quickemu-ops", toolTimeout, nil, args...)
	if err == nil {
		if id := strings.TrimSpace(out); id != "" && !badOpID(id) {
			return id, nil
		}
	}
	// Fallback path: write the journal record directly (identical
	// schema to the ops tool's Op).
	act := 0
	for _, o := range scanOps(dir) {
		if !opActive(o.State) {
			continue
		}
		act++
		if exclusiveOpType(o.Type) || exclusiveOpType(typ) {
			return "", fmt.Errorf("OP_CONFLICT: op %s (%s) already active on %s",
				o.OperationID, o.Type, vm)
		}
	}
	_ = act
	id := wantID
	if id == "" {
		id = genOpID()
	}
	if badOpID(id) {
		return "", fmt.Errorf("INVALID_ARGS: bad operation id")
	}
	opsDir := filepath.Join(dir, "ops")
	if err := os.MkdirAll(opsDir, 0o755); err != nil {
		return "", err
	}
	f := filepath.Join(opsDir, id+".json")
	if _, err := os.Stat(f); err == nil {
		return "", fmt.Errorf("OP_EXISTS: operation %s already exists", id)
	}
	rec := map[string]any{
		"schemaVersion": 1, "operationId": id, "type": typ, "vm": vm,
		"state": "queued", "progress": 0, "startedAt": time.Now().UTC().Format(time.RFC3339),
		"bootId": bootID(), "result": nil, "error": nil,
	}
	if phase != "" {
		rec["phase"] = phase
	}
	if msg != "" {
		rec["message"] = msg
	}
	b, _ := json.Marshal(rec)
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, f); err != nil {
		os.Remove(tmp)
		return "", err
	}
	// Surface the tool error when the direct write was a fallback for
	// a reason other than "tool missing" — the conflict the tool
	// refused still needs reporting. Distinguish by FindTool: a
	// resolved tool that failed had something real to say.
	if _, ferr := qelib.FindTool("quickemu-ops"); ferr == nil && err != nil {
		if code := envelopeErrCode(out); code != "" {
			return "", fmt.Errorf("%s", code)
		}
	}
	return id, nil
}

// envelopeErrCode extracts errors[0].code from a v1 envelope the
// sibling printed on stdout before exiting non-zero.
func envelopeErrCode(out string) string {
	for _, l := range strings.Split(out, "\n") {
		var env struct {
			Errors []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"errors"`
		}
		if json.Unmarshal([]byte(l), &env) == nil && len(env.Errors) > 0 {
			if env.Errors[0].Message != "" {
				return env.Errors[0].Code + ": " + env.Errors[0].Message
			}
			return env.Errors[0].Code
		}
	}
	return ""
}
