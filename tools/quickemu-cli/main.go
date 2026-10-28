// quickemu-cli: the unified `quickemu` entry point in Go — replaces
// bin/quickemu + lib/quickemu/cli/*. It parses the same flag surface,
// resolves actions, and dispatches to the Go backend tools
// (launch/vm/qmp/ops/archive) — zero Bash delegation.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/alesanfe/quickemu/tools/qelib"
)

var (
	vmConf       string
	outFmt       = "text"
	actions      []string
	confOpts     = map[string]string{} // CLI overrides merged into the conf
	extra        = map[string]string{} // action args (--monitor-cmd value etc.)
	trashFlag    bool                  // --trash: delete_vm stages to XDG trash
	externalFlag bool                  // --external: external qcow2 snapshot
	doctorFix    bool                  // --fix: doctor attempts repairs
	persistFlag  bool                  // --persist: record hotplug in the conf
	shortcutOpts []string              // SHORTCUT_OPTIONS accumulated for --shortcut
	snapshotAct  string                // --snapshot <action>
	snapshotTag  string                // --snapshot <action> <tag>
	dryRunFlag   bool                  // --dry-run/--echo
)

// unameArch maps Go runtime.GOARCH to uname -m spellings.
func unameArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return runtime.GOARCH
	}
}

// unameFields: `uname -s -r -m` without spawning uname — same kernel
// info via the uname(2) syscall (x/sys/unix).
func unameFields(flags string) string {
	var u unix.Utsname
	if unix.Uname(&u) != nil {
		return ""
	}
	m := map[byte]string{
		's': unix.ByteSliceToString(u.Sysname[:]),
		'r': unix.ByteSliceToString(u.Release[:]),
		'v': unix.ByteSliceToString(u.Version[:]),
		'm': unix.ByteSliceToString(u.Machine[:]),
		'n': unix.ByteSliceToString(u.Nodename[:]),
	}
	var out []string
	for _, f := range strings.Fields(flags) {
		for _, c := range []byte(strings.TrimPrefix(f, "-")) {
			if v, ok := m[c]; ok && v != "" {
				out = append(out, v)
			}
		}
	}
	return strings.Join(out, " ")
}

func tool(name string, args ...string) (string, error) {
	return toolEnv(name, nil, args...)
}

// toolEnv: tool() with extra env vars — used to pass QUICKEMU_QMP_RAW=1
// only to trusted internal flows (hotplug), never to user QMP calls.
func toolEnv(name string, env []string, args ...string) (string, error) {
	// stdout is captured for the caller; stderr passes straight through
	// to the user — Bash subprocess semantics (ERROR! lines must stay
	// on stderr for contract callers).
	return qelib.Tool(name, env, args...)
}

// qmpRaw sends a privileged QMP command from a trusted internal flow
// (hotplug), bypassing the user-channel allowlist — qmp_query_raw
// parity.
func qmpRaw(payload string) (string, error) {
	return toolEnv("quickemu-qmp", []string{"QUICKEMU_QMP_RAW=1"},
		"execute", "--socket", qmpSock(), "--command", payload)
}

// vmdir: vm_identify parity — the VM state dir is dirname(disk_img)
// when the conf sets it, else <confdir>/<vmname> (the quickget
// convention of a per-VM directory beside the conf). Never just the
// conf's directory: state files must not mix into the caller's cwd.
func vmdir() string {
	confDir := filepath.Dir(vmConf)
	if d := confKV()["disk_img"]; d != "" {
		if !filepath.IsAbs(d) {
			d = filepath.Join(confDir, d)
		}
		return filepath.Dir(d)
	}
	return filepath.Join(confDir, vmname())
}
func vmname() string {
	return strings.TrimSuffix(filepath.Base(vmConf), filepath.Ext(vmConf))
}
func qmpSock() string { return filepath.Join(vmdir(), vmname()+"-qmp.socket") }
func pidPath() string { return filepath.Join(vmdir(), vmname()+".pid") }

// qmp sends a QMP command via quickemu-qmp execute.
func qmp(cmd string) (string, error) {
	return tool("quickemu-qmp", "execute", "--socket", qmpSock(),
		"--command", qmpCmd(cmd, nil))
}

// hmp sends a human-monitor-command (sendkey/type/screendump).
func hmp(cmd string) (string, error) {
	return tool("quickemu-qmp", "execute", "--socket", qmpSock(),
		"--command", qmpCmd("human-monitor-command",
			map[string]any{"command-line": cmd}))
}

func readPID() int {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return 0
	}
	pid := 0
	fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid)
	return pid
}

// alive reports whether pid is a live process (EPERM-aware — a
// foreign owner's lock must never be reclaimed).
func alive(pid int) bool { return qelib.Alive(pid) }

// vmLockAcquire/vmLockRelease: vm_lock_* parity — mkdir-based
// exclusive lock at <vmdir>/<vm>.lock with a pid file for stale-owner
// reclaim. Bash defaults VM_LOCK_TRIES=1: one attempt, then LOCK_BUSY.
func vmLockAcquire() bool {
	lockDir := filepath.Join(vmdir(), vmname()+".lock")
	os.MkdirAll(vmdir(), 0755)
	tries := 1
	if t := os.Getenv("VM_LOCK_TRIES"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			tries = n
		}
	}
	writeOwner := func() bool {
		// A lock without a readable owner is reclaimable by the next
		// acquirer — on write failure drop the lock rather than hold
		// an anonymous one.
		if os.WriteFile(filepath.Join(lockDir, "pid"),
			[]byte(strconv.Itoa(os.Getpid())), 0644) != nil {
			os.RemoveAll(lockDir)
			return false
		}
		return true
	}
	for i := 0; i < tries; i++ {
		if err := os.Mkdir(lockDir, 0755); err == nil {
			if writeOwner() {
				return true
			}
		}
		// Reclaim a dead writer (crash-safe, like Bash's kill -0 check)
		// then retry the mkdir immediately — a reclaimed lock is
		// ours to take, not a reason to fail this attempt.
		if b, err := os.ReadFile(filepath.Join(lockDir, "pid")); err == nil {
			owner, _ := strconv.Atoi(strings.TrimSpace(string(b)))
			switch {
			case owner > 0 && !alive(owner):
				// Dead owner — safe to reclaim.
				os.RemoveAll(lockDir)
				if err := os.Mkdir(lockDir, 0755); err == nil && writeOwner() {
					return true
				}
			case owner <= 0:
				// Corrupt/torn pid write — reclaim only once the
				// lock is old enough that the writer cannot be
				// mid-creation (same 2s window as a missing file).
				if st, serr := os.Stat(lockDir); serr == nil &&
					time.Since(st.ModTime()) > 2*time.Second {
					os.RemoveAll(lockDir)
					if err := os.Mkdir(lockDir, 0755); err == nil && writeOwner() {
						return true
					}
				}
			}
		} else if st, err := os.Stat(lockDir); err == nil &&
			time.Since(st.ModTime()) > 2*time.Second {
			// Pid-less lock older than ~2s: crash remnant between
			// mkdir and the pid write.
			os.RemoveAll(lockDir)
			if err := os.Mkdir(lockDir, 0755); err == nil && writeOwner() {
				return true
			}
		}
		if i+1 < tries {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if outFmt == "json" {
		emitFail("vm.lock", "VM_LOCKED", vmname()+" is locked by another operation.")
	} else {
		fmt.Fprintf(os.Stderr, "ERROR! %s is locked by another operation.\n", vmname())
	}
	return false
}

func vmLockRelease() {
	os.RemoveAll(filepath.Join(vmdir(), vmname()+".lock"))
}

// procComm: /proc/<pid>/comm (linux) — vm_proc_comm parity; empty on
// non-linux hosts where the fallback is plain liveness.
func procComm(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// vmPidAlive: vm_pid_alive parity — pid alive AND the process belongs
// to this VM (comm matches the vm name, its 15-char truncation or
// qemu*). When comm is unreadable (macOS, other users' procs) liveness
// alone decides — same as the Bash legacy path.
func vmPidAlive() bool {
	pid := readPID()
	if !alive(pid) {
		return false
	}
	comm := procComm(pid)
	if comm == "" {
		return true
	}
	name := vmname()
	short := name
	if len(short) > 15 {
		short = short[:15]
	}
	return comm == name || comm == short || strings.HasPrefix(comm, "qemu")
}

func vmRunning() bool { return vmPidAlive() }

func paused() bool {
	_, err := os.Stat(filepath.Join(vmdir(), vmname()+".paused"))
	return err == nil
}

func state() (string, int) {
	pid := readPID()
	if !vmPidAlive() {
		if pid > 0 {
			return "stale", 0
		}
		return "stopped", 0
	}
	if paused() {
		return "paused", pid
	}
	return "running", pid
}

// runtimeCleanup: vm_runtime_cleanup parity — when a pidfile exists but
// its process is dead the VM died uncleanly; drop every runtime
// artefact a boot can leave behind plus stale op locks, and record a
// vm.stopped event. Runs before actions and before boot, like Bash's
// main flow.
func runtimeCleanup() {
	if !(readPID() > 0 && !vmPidAlive()) {
		return
	}
	dir, name := vmdir(), vmname()
	for _, ext := range []string{
		".pid", ".paused", ".spice", ".sock", ".ports", ".runtime.json",
		"-qmp.socket", "-monitor.socket", "-serial.socket", "-agent.sock",
	} {
		os.Remove(filepath.Join(dir, name+ext))
	}
	os.RemoveAll(filepath.Join(dir, name+".lock"))
	os.RemoveAll(filepath.Join(dir, name+".lock.shared"))
	qeEvent("vm.stopped", "")
}

// emitEnvelope: full v1 contract envelope — {schemaVersion, command,
// success, data, warnings, errors, meta{quickemuVersion,timestamp,
// operationId?}} — identical shape to qe_emit.
const qeVersion = qelib.QEVersion

func emitEnvelope(cmd string, success bool, data any, errors any) {
	qelib.Emit(cmd, "quickemu-cli/"+qeVersion, success, data, errors)
}

func emitOK(cmd string, data any) {
	emitEnvelope(cmd, true, data, []any{})
}

// ---------- action implementations ----------

// allowedOps: vm_allowed_ops parity — the ops the UI may offer per
// state, as a whitespace-separated list split into a JSON array.
func allowedOps(state string) []string {
	var ops string
	switch state {
	case "running":
		ops = "pause resume shutdown reset kill status monitor-cmd screenshot send-keys type snapshot-clone export"
	case "paused":
		ops = "resume shutdown kill status monitor-cmd screenshot"
	case "stopped", "stale":
		ops = "start delete clone rename export import snapshot apply create delete info list inspect-conf delete-plan"
	case "archived":
		ops = "restore trash-list status inspect-conf delete"
	case "unknown":
		ops = "status inspect-conf"
	default:
		ops = "status"
	}
	return strings.Fields(ops)
}

func actStatus() int {
	st, pid := state()
	if outFmt == "json" {
		var p any
		if pid > 0 {
			p = pid
		}
		emitOK("vm.status", map[string]any{"name": vmname(), "state": st,
			"pid": p, "allowedOps": allowedOps(st)})
		return 0
	}
	if pid > 0 {
		fmt.Printf("%s %d\n", st, pid)
	} else {
		fmt.Println(st)
	}
	return 0
}

func glob(pat string) []string {
	m, _ := filepath.Glob(pat)
	return m
}

func actListVMs() int {
	type ent struct {
		Name  string `json:"name"`
		OS    string `json:"guest_os"`
		State string `json:"state"`
		PID   any    `json:"pid"`
	}
	vms := []ent{}
	// A VM may appear twice — once as ./name.conf and once as
	// name/name.conf (quickget layout). Identity is the basename, so
	// dedupe by it; top-level confs list first.
	seen := map[string]bool{}
	for _, pat := range []string{"*.conf", "*/*.conf"} {
		for _, f := range glob(pat) {
			name := strings.TrimSuffix(filepath.Base(f), ".conf")
			if seen[name] {
				continue
			}
			seen[name] = true
			gos := "linux"
			if b, err := os.ReadFile(f); err == nil {
				for _, l := range strings.Split(string(b), "\n") {
					if kv := strings.SplitN(l, "=", 2); len(kv) == 2 &&
						strings.TrimSpace(kv[0]) == "guest_os" {
						gos = strings.Trim(strings.TrimSpace(kv[1]), `"' `)
					}
				}
			}
			vmConf = f
			st, pid := state()
			var p any
			if pid > 0 {
				p = pid
			}
			vms = append(vms, ent{name, gos, st, p})
		}
	}
	if outFmt == "json" {
		emitOK("vm.list", map[string]any{"vms": vms})
		return 0
	}
	fmt.Printf("%-28s %-16s %-9s %s\n", "VM", "GUEST OS", "STATE", "PID")
	for _, v := range vms {
		fmt.Printf("%-28s %-16s %-9s %v\n", v.Name, v.OS, v.State, v.PID)
	}
	return 0
}

func actPause() int {
	out, err := qmp("stop")
	if err == nil {
		if werr := os.WriteFile(filepath.Join(vmdir(), vmname()+".paused"), nil, 0644); werr != nil {
			fmt.Fprintf(os.Stderr, "WARNING: paused but marker write failed: %v\n", werr)
		}
		fmt.Println(" - " + vmname() + " paused.")
	}
	fmt.Print(out)
	return rc(err)
}

func actResume() int {
	out, err := qmp("cont")
	if err == nil {
		os.Remove(filepath.Join(vmdir(), vmname()+".paused"))
		fmt.Println(" - " + vmname() + " resumed.")
	}
	fmt.Print(out)
	return rc(err)
}

// actMonitorCmd: monitor_send_cmd parity — send an arbitrary monitor
// command. Order: raw HMP socket (monitor=socket) → telnet
// (monitor=telnet) → QMP human-monitor-command as last resort.
func actMonitorCmd() int {
	msg := extra["monitor-cmd"]
	if msg == "" {
		fmt.Fprintln(os.Stderr, "WARNING! Send to QEMU-Monitor: Message empty!")
		return 1
	}
	conf := confKV()
	mon := confOpts["monitor"]
	if mon == "" {
		mon = conf["monitor"]
	}
	switch mon {
	case "", "socket":
		sock := filepath.Join(vmdir(), vmname()+"-monitor.socket")
		if _, err := os.Stat(sock); err == nil {
			fmt.Println(" - Sending:  via socket " + msg)
			if out, err := monitorSend("unix", sock, msg); err == nil {
				fmt.Print(out)
				return 0
			}
			fmt.Fprintln(os.Stderr, "WARNING! No qemu-monitor socket at "+sock+" - Couldn't send message to monitor!")
			return 1
		}
		// QMP human-monitor-command covers the same surface.
		out, err := hmp(msg)
		fmt.Print(out)
		return rc(err)
	case "telnet":
		fmt.Println(" - Sending:  via telnet " + msg)
		addr := conf["monitor_telnet_host"] + ":" + conf["monitor_telnet_port"]
		out, err := monitorSend("tcp", addr, msg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "WARNING! No qemu-monitor channel available - Couldn't send message to monitor!")
			return 1
		}
		fmt.Print(out)
		return 0
	default:
		fmt.Fprintln(os.Stderr, "WARNING! No qemu-monitor channel available - Couldn't send message to monitor!")
		return 1
	}
}

// monitorSend: write a line to a QEMU human-monitor endpoint and read
// the reply until the (qemu) prompt. The greeting and the per-char
// readline echo are drained so only the reply body is returned (the
// Bash socat+sed pipeline leaves the echo in; the payload is the same).
func monitorSend(network, addr, msg string) (string, error) {
	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.Dial(network, addr)
	if err != nil {
		return "", err
	}
	defer c.Close()
	readUntil := func(token string, timeout time.Duration) string {
		c.SetDeadline(time.Now().Add(timeout))
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
				if strings.Contains(b.String(), token) {
					break
				}
			}
			if err != nil {
				break
			}
		}
		return b.String()
	}
	readUntil("(qemu)", 1500*time.Millisecond) // greeting
	if _, err := c.Write([]byte(msg + "\n")); err != nil {
		return "", err
	}
	out := readUntil("(qemu)", 1500*time.Millisecond)
	out = strings.ReplaceAll(out, "(qemu) ", "")
	// Drop the echo line: everything up to the first newline is the
	// command being re-typed by the monitor's readline.
	if i := strings.Index(out, "\n"); i >= 0 {
		out = out[i+1:]
	}
	out = ansiRe.ReplaceAllString(out, "")
	return strings.ReplaceAll(out, "\r", ""), nil
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]|[\x00-\x08\x0b\x0e-\x1f]")

func actSendKeys() int {
	out, err := hmp("sendkey " + extra["send-keys"])
	fmt.Print(out)
	return rc(err)
}

func actType() int {
	for _, ch := range extra["type"] {
		k := qelib.SendkeyName(ch)
		if k == "" {
			continue
		}
		if _, err := hmp("sendkey " + k); err != nil {
			return rc(err)
		}
		time.Sleep(60 * time.Millisecond)
	}
	return 0
}

func actScreenshot() int {
	dst := extra["screenshot"]
	if dst == "" {
		dst = filepath.Join(vmdir(), vmname()+".ppm")
	}
	out, err := hmp("screendump " + dst)
	fmt.Print(out)
	return rc(err)
}

// vmOp runs `quickemu-vm <sub> <rest...>`; args are produced lazily at
// dispatch time so vmdir()/vmname() see the parsed --vm flag.
// vmLockedOps: the mutating ops Bash wraps in vm_lock_acquire in its
// main dispatch (delete_vm, clone_vm, export_vm, autostart_vm,
// create/delete_shortcut, delete_disk, rename_vm).
var vmLockedOps = map[string]bool{
	"delete": true, "delete-disk": true, "clone": true, "linked-clone": true,
	"rename": true, "export": true, "autostart": true,
	"shortcut": true, "shortcut-delete": true,
}

func vmOp(sub string, rest ...string) func() int {
	return func() int {
		// The vm tool locates the conf by glob; QE_VMCONF tells it which
		// one the user actually passed (dirs can hold several).
		if vmConf != "" {
			os.Setenv("QE_VMCONF", vmConf)
		}
		// bin/quickemu (the entry wrapper) pre-sets QE_LAUNCHER so
		// generated shortcuts/services invoke the canonical entry point
		// rather than the inner Go binary; keep it when present.
		if os.Getenv("QE_LAUNCHER") == "" {
			if self, err := os.Executable(); err == nil {
				os.Setenv("QE_LAUNCHER", self)
			}
		}
		if vmLockedOps[sub] && !vmLockAcquire() {
			return 1
		}
		out, err := tool("quickemu-vm", append([]string{sub}, rest...)...)
		if vmLockedOps[sub] {
			vmLockRelease()
		}
		fmt.Print(out)
		return rc(err)
	}
}

func opsOp(sub string, rest ...string) func() int {
	return func() int {
		out, err := tool("quickemu-ops", append([]string{sub, "--vmdir", vmdir()}, rest...)...)
		fmt.Print(out)
		return rc(err)
	}
}

func qmpOp(cmd, okMsg func() string) func() int {
	return func() int {
		out, err := qmp(cmd())
		fmt.Print(out)
		if err == nil && okMsg != nil {
			fmt.Println(okMsg())
		}
		return rc(err)
	}
}

func actStop(force bool) int {
	st, pid := state()
	if st != "running" && st != "paused" {
		fmt.Println(vmname() + " is not running")
		return 0
	}
	if force {
		if p, err := os.FindProcess(pid); err == nil {
			p.Signal(syscall.SIGKILL)
		}
	} else {
		// "quit" is not allowlisted on the structured QMP channel — the
		// HMP passthrough is how Bash issues it.
		hmp("quit")
	}
	for i := 0; i < 40 && alive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	for _, ext := range []string{".pid", ".paused", "-qmp.socket", "-monitor.socket", "-serial.socket", "-agent.sock"} {
		os.Remove(filepath.Join(vmdir(), vmname()+ext))
	}
	fmt.Println(" - " + vmname() + " stopped.")
	return 0
}

func actLaunch() int {
	conf := confKV()
	get := func(k string) string {
		if v := confOpts[k]; v != "" {
			return v
		}
		return conf[k]
	}
	// Already-running parity: a second `quickemu --vm x.conf` never
	// spawns a second QEMU — Bash attaches the viewer to the live
	// instance instead. Without this the two QEMU processes race over
	// the pidfile/sockets in the VM dir.
	if vmRunning() {
		fmt.Println(vmname())
		fmt.Printf(" - Process:  Already running %s as %s (%d)\n",
			vmConf, vmname(), readPID())
		startViewer(get)
		if mc := get("monitor_cmd"); mc != "" {
			monitorSendBest(mc)
		}
		return 0
	}
	// vm_runtime_cleanup parity: a stale pidfile means an unclean
	// shutdown — remove leftover state before booting (Bash does this
	// in its main flow before any launch).
	runtimeCleanup()
	// viewer_param_check (launch path only — skipped on --dry-run):
	// viewer must be none|spicy|remote-viewer and the binary installed.
	viewer := get("viewer")
	if viewer == "" {
		viewer = "spicy"
	}
	switch viewer {
	case "none", "spicy", "remote-viewer":
	default:
		fmt.Fprintf(os.Stderr, "ERROR! Requested viewer '%s' is not recognised.\n", viewer)
		return 1
	}
	if viewer != "none" {
		if _, err := exec.LookPath(viewer); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! Requested '%s' as viewer, but '%s' is not installed.\n", viewer, viewer)
			return 1
		}
	}
	// vm_lock_acquire parity: the boot runs under the exclusive VM
	// lock so a concurrent launch fails LOCK_BUSY instead of racing
	// the pidfile/sockets window (release right after spawn, like
	// Bash's vm_lock_release post-qemu_spawn).
	if !vmLockAcquire() {
		return 1
	}
	out, err := tool("quickemu-launch", applyOverrides(), "--run")
	fmt.Print(out)
	vmLockRelease()
	if err != nil {
		return 1
	}
	startViewer(get)
	// Auto-skip the press-any-key prompt on uninstalled Windows guests.
	if get("iso") != "" && strings.HasPrefix(get("guest_os"), "windows") {
		for i := 0; i < 5; i++ {
			time.Sleep(1 * time.Second)
			monitorSendBest("sendkey ret")
		}
	}
	// monitor_cmd conf key: Bash sends it after every boot.
	if mc := get("monitor_cmd"); mc != "" && extra["monitor-cmd"] == "" {
		monitorSendBest(mc)
	}
	return 0
}

// monitorSendBest: try the raw monitor socket first, then QMP hmp —
// used for post-boot commands where either channel may be up.
func monitorSendBest(msg string) {
	sock := filepath.Join(vmdir(), vmname()+"-monitor.socket")
	if _, err := os.Stat(sock); err == nil {
		if _, err := monitorSend("unix", sock, msg); err == nil {
			return
		}
	}
	hmp(msg)
}

// startViewer: start_viewer parity — attach spicy/remote-viewer when
// display=spice. Unix socket (path in .spice or default <vm>.sock for
// local access) or TCP (port in .spice / spice_port for remote).
func startViewer(get func(string) string) {
	viewer := get("viewer")
	if viewer == "" {
		viewer = "spicy"
	}
	if viewer == "none" || get("display") != "spice" {
		return
	}
	bin, err := exec.LookPath(viewer)
	if err != nil {
		return
	}
	var args []string
	spiceInfo := ""
	if b, err := os.ReadFile(filepath.Join(vmdir(), vmname()+".spice")); err == nil {
		spiceInfo = strings.TrimSpace(string(b))
	}
	switch {
	case strings.Contains(spiceInfo, "/"):
		if viewer == "spicy" {
			args = append(args, "--uri=spice+unix://"+spiceInfo)
		} else {
			args = append(args, "spice+unix://"+spiceInfo)
		}
	case spiceInfo != "":
		if viewer == "spicy" {
			args = append(args, "--port", spiceInfo)
		} else {
			args = append(args, "spice://localhost:"+spiceInfo)
		}
	default:
		access := get("access")
		if access == "" || access == "local" {
			sock := filepath.Join(vmdir(), vmname()+".sock")
			if viewer == "spicy" {
				args = append(args, "--uri=spice+unix://"+sock)
			} else {
				args = append(args, "spice+unix://"+sock)
			}
		} else {
			port := get("spice_port")
			if viewer == "spicy" {
				args = append(args, "--port", port)
			} else {
				args = append(args, "spice://localhost:"+port)
			}
		}
	}
	args = append(args, "--title", vmname())
	if get("guest_os") != "macos" {
		if pub := get("public_dir"); pub != "" {
			args = append(args, "--spice-shared-dir", pub)
		}
	}
	if get("fullscreen") == "true" || get("fullscreen") == "on" {
		args = append(args, "--full-screen")
	}
	if vea := get("viewer_extra_args"); vea != "" {
		args = append(args, strings.Fields(vea)...)
	}
	cmd := exec.Command(bin, args...)
	cmd.Start() // detached — viewer outlives the CLI process
}

// actSpiceConsole: spice-console parity target — attach the configured
// SPICE viewer to an already-running VM (start_viewer on attach).
func actSpiceConsole() int {
	if !vmRunning() {
		fmt.Fprintf(os.Stderr, "ERROR! %s is not running — no SPICE session to attach to.\n", vmname())
		return 1
	}
	conf := confKV()
	get := func(k string) string {
		if v := confOpts[k]; v != "" {
			return v
		}
		return conf[k]
	}
	if get("display") != "spice" && get("display") != "spice-app" {
		fmt.Fprintln(os.Stderr, "ERROR! VM display is '"+get("display")+"' — spice-console needs display=spice.")
		return 1
	}
	startViewer(get)
	fmt.Println(" - Viewer:   attached to " + vmname())
	return 0
}

// actVNC: vnc parity target — open a VNC client against a running VM
// configured with display=vnc. vnc_listen is host:display (display N
// maps to TCP 5900+N, same arithmetic as QEMU -vnc).
func actVNC() int {
	if !vmRunning() {
		fmt.Fprintf(os.Stderr, "ERROR! %s is not running — no VNC session to attach to.\n", vmname())
		return 1
	}
	conf := confKV()
	if conf["display"] != "vnc" {
		fmt.Fprintln(os.Stderr, "ERROR! VM display is '"+conf["display"]+"' — vnc needs display=vnc.")
		return 1
	}
	listen := conf["vnc_listen"]
	if listen == "" {
		listen = "127.0.0.1:0" // bin/quickemu default
	}
	host, disp := listen, "0"
	if i := strings.LastIndex(listen, ":"); i >= 0 {
		host, disp = listen[:i], listen[i+1:]
	}
	if host == "" {
		host = "127.0.0.1"
	}
	d, err := strconv.Atoi(disp)
	if err != nil || d < 0 || d > 99 {
		fmt.Fprintf(os.Stderr, "ERROR! '%s' is not a valid VNC listen address (expected host:display).\n", listen)
		return 1
	}
	port := strconv.Itoa(5900 + d)
	// remote-viewer understands vnc:// URIs; plain vncviewer wants host::port.
	for _, cand := range []string{"remote-viewer", "vncviewer", "xtightvncviewer", "gvncviewer"} {
		if bin, err := exec.LookPath(cand); err == nil {
			var c *exec.Cmd
			if cand == "remote-viewer" {
				c = exec.Command(bin, "vnc://"+host+":"+port)
			} else {
				c = exec.Command(bin, host+"::"+port)
			}
			fmt.Println(" - VNC:      attaching " + cand + " to " + host + ":" + port)
			c.Start()
			return 0
		}
	}
	fmt.Fprintln(os.Stderr, "ERROR! No VNC viewer found (remote-viewer, vncviewer, xtightvncviewer, gvncviewer).")
	fmt.Fprintln(os.Stderr, " - VNC:      VM listens on "+host+":"+port)
	return 1
}

// actPublicDir: open the VM's public_dir in the host file manager,
// creating it if needed — the 'publicdir' subcommand.
func actPublicDir() int {
	conf := confKV()
	pub := conf["public_dir"]
	if pub == "" {
		fmt.Fprintln(os.Stderr, "ERROR! public_dir is not set in "+vmname()+" config.")
		return 1
	}
	if !filepath.IsAbs(pub) {
		pub = filepath.Join(vmdir(), pub)
	}
	if err := os.MkdirAll(pub, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! Cannot create "+pub+": "+err.Error())
		return 1
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	bin, err := exec.LookPath(opener)
	if err != nil {
		fmt.Println(" - Public:   " + pub) // no file manager — still give the path
		return 0
	}
	exec.Command(bin, pub).Start()
	fmt.Println(" - Public:   " + pub)
	return 0
}

// actTelnet: interactive attach to a QEMU telnet chardev — used for
// serial-console (serial=telnet) and monitor-console (monitor=telnet).
func actTelnet(kind string) int {
	if !vmRunning() {
		fmt.Fprintf(os.Stderr, "ERROR! %s is not running.\n", vmname())
		return 1
	}
	conf := confKV()
	if conf[kind] != "telnet" {
		fmt.Fprintf(os.Stderr, "ERROR! %s is '%s' — attach needs %s=telnet.\n", kind, conf[kind], kind)
		return 1
	}
	host := conf[kind+"_telnet_host"]
	if host == "" {
		host = "localhost"
	}
	port := conf[kind+"_telnet_port"]
	if port == "" {
		if kind == "monitor" {
			port = "4440"
		} else {
			port = "6660"
		}
	}
	fmt.Println(" - " + strings.ToUpper(kind[:1]) + kind[1:] + ":   telnet " + host + " " + port)
	return telnetAttach(host, port)
}

func actDryRun() int {
	conf := applyOverrides()
	out, err := tool("quickemu-launch", conf)
	fmt.Print(out)
	if outFmt == "json" && err == nil {
		// execution-plan.schema parity: human plan lines above, then the
		// vm.plan envelope carrying the assembled QEMU command.
		emitOK("vm.plan", map[string]any{"vm": vmname(),
			"command": strings.TrimSpace(out)})
	}
	return rc(err)
}

// applyOverrides writes CLI conf overrides into a temp conf alongside
// the original (Bash exports them as env vars; quickemu-launch reads
// only the file, so overrides are materialised there).
func applyOverrides() string {
	if len(confOpts) == 0 {
		return vmConf
	}
	b, _ := os.ReadFile(vmConf)
	c := string(b)
	for k, v := range confOpts {
		kv := k + `="` + v + `"`
		if strings.Contains(c, k+"=") {
			lines := strings.Split(c, "\n")
			for i, l := range lines {
				if strings.HasPrefix(l, k+"=") {
					lines[i] = kv
				}
			}
			c = strings.Join(lines, "\n")
		} else {
			c += kv + "\n"
		}
	}
	tmp := filepath.Join(vmdir(), "."+vmname()+".cli.conf")
	if err := os.WriteFile(tmp, []byte(c), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: override conf write failed (%v) — launching without overrides\n", err)
		return vmConf
	}
	return tmp
}

func rc(err error) int {
	if err != nil {
		return 1
	}
	return 0
}

func dispatch(act string) func() int {
	msg := func(s string) func() string { return func() string { return s } }
	switch act {
	case "status_vm":
		return actStatus
	case "list_vms":
		return actListVMs
	case "pause_vm":
		return actPause
	case "resume_vm":
		return actResume
	case "shutdown_vm":
		return qmpOp(msg("system_powerdown"), func() string { return " - " + vmname() + " shutting down." })
	case "reset_vm":
		// system_reset is not on the QMP allowlist — Bash sends it over
		// the HMP channel (human-monitor-command).
		return func() int {
			out, err := hmp("system_reset")
			fmt.Print(out)
			return rc(err)
		}
	case "kill_vm":
		return func() int { return actStop(true) }
	case "stop_vm":
		return func() int { return actStop(false) }
	case "monitor_cmd":
		return actMonitorCmd
	case "send_keys":
		return actSendKeys
	case "type_text":
		return actType
	case "screenshot_vm":
		return actScreenshot
	case "delete_vm":
		return func() int {
			a := []string{"delete", vmdir()}
			if trashFlag {
				a = append(a, "--trash")
			}
			if extra["plan-id"] != "" {
				a = append(a, "--plan-id="+extra["plan-id"])
			}
			return vmOp(a[0], a[1:]...)()
		}
	case "clone_vm":
		return func() int {
			a := []string{"clone", vmdir(), extra["clone"]}
			if extra["full"] != "" {
				a = append(a, "--full")
			}
			return vmOp(a[0], a[1:]...)()
		}
	case "linked_clone_vm":
		return vmOp("linked-clone", vmdir(), extra["linked-clone"])
	case "rename_vm":
		return vmOp("rename", vmdir(), extra["rename"])
	case "move_vm":
		return vmOp("move", vmdir(), extra["move"])
	case "backup_vm":
		return vmOp("backup", vmdir())
	case "backup_list":
		return vmOp("backup-list", vmdir())
	case "trash_list":
		return vmOp("trash-list", vmdir())
	case "snapshot_create":
		return vmOp("snapshot-create", vmdir(), extra["snapshot-create"])
	case "snapshot_apply":
		return vmOp("snapshot-apply", vmdir(), extra["snapshot-apply"])
	case "snapshot_delete":
		return vmOp("snapshot-delete", vmdir(), extra["snapshot-delete"])
	case "snapshot_list":
		return vmOp("snapshot-list", vmdir())
	case "disk_compact":
		return vmOp("compact", vmdir())
	case "disk_resize":
		return vmOp("resize", vmdir(), extra["disk-resize"])
	case "delete_disk":
		return vmOp("delete-disk", vmdir(), pidPath())
	case "export_vm":
		return func() int { return vmOp("export", vmdir(), extra["export"])() }
	case "import_vm":
		return func() int { return vmOp("import", vmdir(), extra["import"])() }
	case "backup_verify":
		return func() int { return vmOp("backup-verify", vmdir(), extra["backup-verify"])() }
	case "backup_restore":
		return func() int { return vmOp("backup-restore", vmdir(), extra["backup-restore"], ".")() }
	case "backup_prune":
		return func() int { return vmOp("backup-prune", vmdir(), extra["backup-prune"])() }
	case "trash_empty":
		return vmOp("trash-empty", vmdir())
	case "trash_restore":
		return func() int { return vmOp("trash-restore", vmdir(), extra["trash-restore"])() }
	case "vfio_scan":
		return vmOp("vfio-scan", vmdir())
	case "autostart_vm":
		return vmOp("autostart", vmdir())
	case "shortcut_create":
		return func() int {
			a := []string{"shortcut", vmdir()}
			return vmOp(a[0], append(a[1:], shortcutOpts...)...)()
		}
	case "delete_shortcut":
		return vmOp("shortcut-delete", vmdir())
	case "snapshot_tree":
		return vmOp("snapshot-tree", vmdir())
	case "snapshot_verify":
		return vmOp("snapshot-verify", vmdir())
	case "snapshot_cmd":
		return actSnapshotCmd
	case "host_info":
		return actHostInfo
	case "capabilities":
		return actCapabilities
	case "inventory_show":
		return actInventory
	case "vm_stats":
		return actStats
	case "inspect_conf":
		return actInspectConf
	case "migrate_conf":
		return actMigrateConf
	case "events_read":
		return actEvents
	case "plugin_list":
		return actPlugins
	case "vm_hotplug_usb":
		return actHotplugUSB
	case "vm_hotplug_disk":
		return actHotplugDisk
	case "vm_hotunplug":
		return actHotunplug
	case "operation_status":
		return opsOp("status", "--id", extra["op-status"])
	case "operation_list":
		return opsOp("list")
	case "operation_cancel":
		return opsOp("cancel", "--id", extra["op-cancel"])
	case "operation_watch":
		return opsOp("watch", extra["op-watch"])
	case "doctor_vm":
		return actDoctor
	case "delete_plan":
		return vmOp("delete-plan", vmdir(), "--format", outFmt)
	case "ignore_msrs_always":
		return actIgnoreMSRsAlways
	case "spice_console":
		return actSpiceConsole
	case "vnc_viewer":
		return actVNC
	case "publicdir_open":
		return actPublicDir
	case "serial_console":
		return func() int { return actTelnet("serial") }
	case "monitor_console":
		return func() int { return actTelnet("monitor") }
	}
	return nil
}

// actIgnoreMSRsAlways: ignore_msrs_always — persist kvm ignore_msrs=Y
// via /etc/modprobe.d and rebuild initramfs (update-initramfs or
// mkinitcpio). Requires sudo, same as Bash.
func actIgnoreMSRsAlways() int {
	if !fileExists("/etc/modprobe.d") {
		fmt.Fprintln(os.Stderr, "ERROR! /etc/modprobe.d was not found, I don't know how to configure this system.")
		return 1
	}
	if b, err := os.ReadFile("/etc/modprobe.d/kvm-quickemu.conf"); err == nil && strings.Contains(string(b), "ignore_msrs=Y") {
		return 0 // already enabled (initramfs rebuilt previously)
	}
	sh := exec.Command("sudo", "tee", "/etc/modprobe.d/kvm-quickemu.conf")
	sh.Stdin = strings.NewReader("options kvm ignore_msrs=Y\n")
	sh.Stdout, sh.Stderr = os.Stdout, os.Stderr
	if err := sh.Run(); err != nil {
		return 1
	}
	switch {
	case toolExists("update-initramfs"):
		return rc(exec.Command("sudo", "update-initramfs", "-k", "all", "-u").Run())
	case toolExists("mkinitcpio"):
		return rc(exec.Command("sudo", "mkinitcpio", "-P").Run())
	default:
		fmt.Fprintln(os.Stderr, "ERROR! User does not have update-initramfs or mkinitcpio installed, please find out respective utility to update/regenrate intiramfs.")
		fmt.Fprintln(os.Stderr, "Run respective command for rebuilding initramfs: 'sudo update-initramfs -k all -u'(debian) or 'sudo mkinitcpio -P'(arch)")
		return 1
	}
}

func toolExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// vmVerifyInvariants: lifecycle.sh parity — a state claims only what's
// true: running/paused need pidfile+runtime.json; stopped/archived
// must not have a stale pidfile.
func vmVerifyInvariants() bool {
	st, _ := state()
	dir, name := vmdir(), vmname()
	pf := filepath.Join(dir, name+".pid")
	rt := filepath.Join(dir, name+".runtime.json")
	switch st {
	case "running", "paused":
		if !fileExists(pf) {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s without pidfile\n", st)
			return false
		}
		if !fileExists(rt) {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s without runtime.json\n", st)
			return false
		}
	case "stopped", "archived":
		if fileExists(pf) {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s with stale pidfile\n", st)
			return false
		}
	}
	return true
}

// qeDoctor: doctor.sh parity — diagnoses stale/corrupt runtime state;
// fix==true repairs what's safe (never touches disks or the conf).
// Text mode prints " - <finding>" lines; --format json emits a
// vm.doctor envelope. Runs before the capability report, like
// doctor_vm does.
func qeDoctor(fix bool) {
	vmVerifyInvariants()
	findings := []string{}
	dir, name := vmdir(), vmname()

	rt := filepath.Join(dir, name+".runtime.json")
	if b, err := os.ReadFile(rt); err == nil && !strings.Contains(string(b), `"schemaVersion"`) {
		findings = append(findings, "runtime-corrupt:"+rt)
		if fix {
			os.Remove(rt)
			findings = append(findings, "fixed:removed-corrupt-runtime")
		}
	}

	pf := filepath.Join(dir, name+".pid")
	live := false
	if b, err := os.ReadFile(pf); err == nil {
		p := strings.TrimSpace(string(b))
		if n, err := strconv.Atoi(p); err == nil && alive(n) {
			live = true
		} else if p != "" {
			findings = append(findings, "stale-pidfile:"+p)
			if fix {
				os.Remove(pf)
				os.Remove(filepath.Join(dir, name+".paused"))
				findings = append(findings, "fixed:removed-stale-pid")
			}
		}
	}

	lockPaths := []string{filepath.Join(dir, name+".lock")}
	if ents, _ := filepath.Glob(filepath.Join(dir, name+".lock.shared", "*")); len(ents) > 0 {
		lockPaths = append(lockPaths, ents...)
	}
	for _, l := range lockPaths {
		st, err := os.Stat(l)
		if err != nil || !st.IsDir() {
			continue
		}
		lp := ""
		if b, err := os.ReadFile(filepath.Join(l, "pid")); err == nil {
			lp = strings.TrimSpace(string(b))
		} else {
			lp = filepath.Base(l)
		}
		if n, err := strconv.Atoi(lp); err == nil && !alive(n) {
			findings = append(findings, "stale-lock:"+l)
			if fix {
				os.RemoveAll(l)
				findings = append(findings, "fixed:reclaimed-lock")
			}
		}
	}

	var socks []string
	for _, pat := range []string{name + "*.socket", name + ".monitor", name + ".qmp"} {
		if m, _ := filepath.Glob(filepath.Join(dir, pat)); len(m) > 0 {
			socks = append(socks, m...)
		}
	}
	for _, s := range socks {
		st, err := os.Stat(s)
		if err != nil || st.Mode()&os.ModeSocket == 0 {
			continue
		}
		if !live {
			findings = append(findings, "stale-socket:"+s)
			if fix {
				os.Remove(s)
				findings = append(findings, "fixed:removed-socket")
			}
		}
	}

	ev := filepath.Join(dir, name+".events.jsonl")
	if b, err := os.ReadFile(ev); err == nil && len(b) > 0 {
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		if !strings.Contains(lines[len(lines)-1], "}") {
			findings = append(findings, "event-truncated:"+ev)
			if fix {
				rest := strings.Join(lines[:len(lines)-1], "\n")
				if rest != "" {
					rest += "\n"
				}
				os.WriteFile(ev, []byte(rest), 0644)
				findings = append(findings, "fixed:dropped-truncated-event")
			}
		}
	}

	for _, op := range func() []string {
		m, _ := filepath.Glob(filepath.Join(dir, "ops", "*.json"))
		return m
	}() {
		b, err := os.ReadFile(op)
		if err != nil {
			continue
		}
		var o struct {
			State     string `json:"state"`
			WorkerPid int    `json:"workerPid"`
		}
		if json.Unmarshal(b, &o) != nil ||
			(o.State != "queued" && o.State != "running") ||
			(o.WorkerPid > 0 && alive(o.WorkerPid)) {
			continue
		}
		findings = append(findings, "orphan-operation:"+op)
		if fix {
			s := regexp.MustCompile(`"state":"[^"]*"`).ReplaceAllString(string(b), `"state":"interrupted"`)
			os.WriteFile(op, []byte(s), 0644)
			findings = append(findings, "fixed:marked-interrupted")
		}
	}

	if outFmt == "json" {
		emitOK("vm.doctor", map[string]any{"vm": name, "fix": fix, "findings": findings})
	} else {
		if len(findings) == 0 {
			fmt.Println(" - " + name + ": healthy.")
		}
		for _, f := range findings {
			fmt.Println(" - " + f)
		}
	}
}

// actDoctor: doctor_vm parity — config vs host capabilities report.
func actDoctor() int {
	qeDoctor(doctorFix)
	conf := confKV()
	issues := 0
	fmt.Println("Checking " + vmname() + " against host capabilities:")
	if fileExists("/dev/kvm") {
		fmt.Println(" o KVM:  /dev/kvm available")
	} else {
		fmt.Println(" x KVM:  /dev/kvm not writable, VM will use slow TCG emulation")
		issues++
	}
	if _, err := exec.LookPath("qemu-system-x86_64"); err != nil {
		fmt.Println(" x QEMU: qemu-system-x86_64 not executable")
		issues++
	}
	if d := conf["disk_img"]; d != "" && !fileExists(d) {
		fmt.Printf(" x Disk: %s missing (will be created, %s)\n", d, conf["disk_size"])
	}
	if i := conf["iso"]; i != "" && !fileExists(i) {
		fmt.Println(" x ISO:  " + i + " missing")
		issues++
	}
	if strings.Contains(conf["boot"], "efi") {
		found := false
		for _, g := range []string{"OVMF_CODE*.fd", "AAVMF*.fd"} {
			if m, _ := filepath.Glob(filepath.Join(vmdir(), g)); len(m) > 0 {
				found = true
			}
		}
		if found {
			fmt.Println(" o EFI:  firmware files present")
		} else {
			fmt.Println(" x EFI:  no OVMF/AAVMF firmware found")
			issues++
		}
	}
	if ram := conf["ram"]; ram != "" {
		if want := memMB(ram); want > 0 {
			if free := hostFreeMemMB(); free > 0 && want*1024 > free*1024 {
				fmt.Printf(" x RAM:  VM wants %s but only %dM available\n", ram, free)
				issues++
			} else {
				fmt.Printf(" o RAM:  %s requested (%dM free)\n", ram, free)
			}
		}
	}
	if conf["tpm"] == "on" {
		if _, err := exec.LookPath("swtpm"); err != nil {
			fmt.Println(" x TPM:  tpm=on but swtpm not installed")
			issues++
		}
	}
	switch conf["network"] {
	case "", "none", "restrict", "passt", "slirp":
	default:
		if _, err := os.Stat("/sys/class/net/" + conf["network"]); err == nil {
			fmt.Println(" o NET:  bridge " + conf["network"] + " up")
		} else {
			fmt.Println(" x NET:  bridge interface " + conf["network"] + " does not exist")
			issues++
		}
	}
	if issues == 0 {
		fmt.Println(" - No problems detected.")
	} else {
		fmt.Printf(" - %d issue(s) found.\n", issues)
	}
	return 0
}

// confKV caches the parsed conf keyed on path+mtime+size — ~10
// call sites per command no longer re-read and re-parse the file
// each time, while an on-disk change is still picked up.
var confKVCache = struct {
	sync.Mutex
	path    string
	modTime int64
	size    int64
	m       map[string]string
}{}

func confKV() map[string]string {
	confKVCache.Lock()
	defer confKVCache.Unlock()
	var mt, sz int64
	if st, err := os.Stat(vmConf); err == nil {
		mt, sz = st.ModTime().UnixNano(), st.Size()
	}
	if confKVCache.path == vmConf && confKVCache.modTime == mt &&
		confKVCache.size == sz && confKVCache.m != nil {
		return confKVCache.m
	}
	m := map[string]string{}
	if b, err := os.ReadFile(vmConf); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if i := strings.Index(l, "="); i > 0 && !strings.HasPrefix(l, "#") {
				m[strings.TrimSpace(l[:i])] = strings.Trim(strings.TrimSpace(l[i+1:]), `"'`)
			}
		}
	}
	confKVCache.path, confKVCache.modTime, confKVCache.size, confKVCache.m = vmConf, mt, sz, m
	return m
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func memMB(s string) int { return qelib.MemMB(s) }

// hostFreeMemMB: MemAvailable from /proc/meminfo (host_free_mem_mb).
func hostFreeMemMB() int { return qelib.HostFreeMemMB() }

// ---------- additional actions (options.sh parity round 2) ----------

func isNum(s string) bool {
	if s == "" {
		return false
	}
	// Signed integers are valid values (e.g. --events -1 replays the
	// stream from the start) — Bash consumes the next token as-is.
	if s[0] == '-' || s[0] == '+' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// actSnapshotCmd: `--snapshot <action> [tag]` — the compound snapshot
// command from options.sh (apply/create/tree/verify/consolidate/
// delete/info|list), with --external for create.
func actSnapshotCmd() int {
	switch snapshotAct {
	case "apply":
		r := vmOp("snapshot-apply", vmdir(), snapshotTag)()
		vmOp("snapshot-info", vmdir())()
		return r
	case "create":
		args := []string{"snapshot-create", vmdir(), snapshotTag}
		if externalFlag {
			args = append(args, "--external")
		}
		r := vmOp(args[0], args[1:]...)()
		vmOp("snapshot-info", vmdir())()
		return r
	case "tree":
		return vmOp("snapshot-tree", vmdir())()
	case "verify":
		return vmOp("snapshot-verify", vmdir())()
	case "consolidate":
		r := vmOp("snapshot-consolidate", vmdir(), snapshotTag)()
		vmOp("snapshot-tree", vmdir())()
		return r
	case "delete":
		r := vmOp("snapshot-delete", vmdir(), snapshotTag)()
		vmOp("snapshot-info", vmdir())()
		return r
	case "info", "list":
		if outFmt == "json" {
			return vmOp("snapshot-list-json", vmdir())()
		}
		return vmOp("snapshot-info", vmdir())()
	default:
		fmt.Fprintf(os.Stderr, "ERROR! %q is not a supported snapshot action.\n", snapshotAct)
		usage()
		return 1
	}
}

// actCapabilities: static contract envelope (capabilities parity —
// keep in sync with schemas/v1/capabilities.schema.json).
func actCapabilities() int {
	emitOK("capabilities", map[string]any{
		"applicationVersion": qeVersion,
		"capabilities": map[string]bool{
			"structuredCatalog": true, "structuredStatus": true, "listVms": true,
			"snapshots": true, "structuredSnapshots": true, "monitorCommands": true,
			"sendKeys": true, "screenshot": true, "clone": true, "rename": true,
			"exportImport": true, "diskOps": true, "downloadProgress": false,
			"safeDeletePlan": true, "verifiableDeletePlan": true, "qmpChannel": true,
			"instanceIdentity": true, "sharedExclusiveLocks": true,
			"portRegistry": true, "verifiedArchives": true, "structuredEvents": true,
			"asyncOperations": true, "tomlConfig": true, "hostInventory": true,
		},
	})
	return 0
}

// actHostInfo: host_info parity — machine-readable host inventory
// (acceleration, deps, machine facts) for QuickGUI.
func actHostInfo() int {
	kvm := fileExists("/dev/kvm")
	iommu := false
	if ents, err := os.ReadDir("/sys/kernel/iommu_groups"); err == nil {
		for _, e := range ents {
			if sub, err := os.ReadDir(filepath.Join("/sys/kernel/iommu_groups", e.Name())); err == nil && len(sub) > 0 {
				iommu = true
			}
		}
	}
	var gpus []string
	for i, l := range pciGPUs() {
		if i == 8 {
			break
		}
		f := strings.Fields(l)
		if len(f) > 1 {
			gpus = append(gpus, strings.Join(f[1:], " "))
		}
	}
	qemuVer := ""
	if q, err := exec.LookPath("qemu-system-x86_64"); err == nil {
		if out, err := exec.Command(q, "--version").Output(); err == nil {
			qemuVer = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		}
	}
	has := func(name string) bool { _, e := exec.LookPath(name); return e == nil }
	globAny := func(pats ...string) bool {
		for _, p := range pats {
			if m, _ := filepath.Glob(p); len(m) > 0 {
				return true
			}
		}
		return false
	}
	var memTotal, hugePages int
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "MemTotal:") {
				fmt.Sscanf(l, "MemTotal: %d kB", &memTotal)
			}
			if strings.HasPrefix(l, "HugePages_Total:") {
				fmt.Sscanf(l, "HugePages_Total: %d", &hugePages)
			}
		}
	}
	freeKb := int64(0)
	var st syscall.Statfs_t
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		dataDir = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	if syscall.Statfs(dataDir, &st) == nil {
		freeKb = int64(st.Bavail) * int64(st.Bsize) / 1024
	}
	kernel := unameFields("-s -r")
	emitOK("host.info", map[string]any{
		"kernel": kernel, "arch": unameArch(), "cpuCores": runtime.NumCPU(),
		"memTotalKb": memTotal, "kvm": kvm, "iommu": iommu, "qemu": qemuVer,
		"qemuAarch64": has("qemu-system-aarch64"), "qemuImg": has("qemu-img"),
		"swtpm": has("swtpm"),
		"ovmf":  globAny("/usr/share/OVMF*/OVMF_CODE.fd", "/usr/share/edk2/ovmf/OVMF_CODE.fd"),
		"aavmf": globAny("/usr/share/AAVMF*", "/usr/share/edk2/aarch64/QEMU_EFI.fd"),
		"passt": has("passt"), "virtiofsd": has("virtiofsd"), "spicy": has("spicy"),
		"remoteViewer": has("remote-viewer"), "freeKb": freeKb,
		"hugePages": hugePages, "gpus": gpus,
	})
	return 0
}

// actInventory: --inventory — the global VM registry at
// $XDG_DATA_HOME/quickemu/vms.json.
func actInventory() int {
	db := filepath.Join(qelib.XDGData(), "quickemu", "vms.json")
	b, err := os.ReadFile(db)
	if err != nil {
		if outFmt == "json" {
			fmt.Println("{}")
		} else {
			fmt.Println("No VMs registered yet.")
		}
		return 0
	}
	if outFmt == "json" {
		fmt.Println(string(b))
		return 0
	}
	var m map[string]struct {
		VM, State, Dir, LastSeen string
	}
	if json.Unmarshal(b, &m) == nil {
		for _, v := range m {
			fmt.Printf(" - %s [%s] %s (seen %s)\n", v.VM, v.State, v.Dir, v.LastSeen)
		}
	}
	return 0
}

// actStats: vm_stats parity — QMP cpus/block/balloon in one envelope.
func actStats() int {
	if _, err := os.Stat(qmpSock()); err != nil {
		if outFmt == "json" {
			emitFail("vm.stats", "VM_MUST_BE_RUNNING", "no running QMP")
		} else {
			fmt.Fprintln(os.Stderr, "ERROR! VM is not running (no QMP socket).")
		}
		return 1
	}
	get := func(cmd string) json.RawMessage {
		out, err := qmp(cmd)
		if err != nil {
			return json.RawMessage("null")
		}
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal([]byte(out), &env) == nil && env.Data != nil {
			return env.Data
		}
		return json.RawMessage("null")
	}
	out := map[string]json.RawMessage{
		"cpus": get("query-cpus-fast"), "block": get("query-blockstats"), "memory": get("query-balloon"),
	}
	b, _ := json.Marshal(out)
	if outFmt == "json" {
		emitOK("vm.stats", json.RawMessage(b))
		return 0
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	var ncpu int
	if c, ok := m["cpus"].(map[string]any); ok {
		if r, ok := c["return"].([]any); ok {
			ncpu = len(r)
		}
	}
	bal := "n/a"
	if c, ok := m["memory"].(map[string]any); ok {
		if r, ok := c["return"].(map[string]any); ok {
			if a, ok := r["actual"]; ok {
				bal = fmt.Sprintf("%v", a)
			}
		}
	}
	fmt.Printf("CPUs: %d  Balloon: %s\n", ncpu, bal)
	return 0
}

// actInspectConf: inspect_conf parity — read the conf WITHOUT
// executing it (confs are data to Go; foreign confs must be inspected).
func actInspectConf() int {
	b, err := os.ReadFile(vmConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", vmConf)
		return 1
	}
	keys := map[string]bool{}
	for _, k := range []string{"guest_os", "arch", "boot", "tpm", "secureboot", "disk_img",
		"disk_format", "disk_size", "iso", "img", "fixed_iso", "floppy", "ram", "cpu_cores",
		"network", "bridge", "macaddr", "port_forwards", "display", "monitor", "serial",
		"sound_card", "public_dir", "seed", "kernel", "initrd", "boot_args",
		"gpu_passthrough", "vnc_listen"} {
		keys[k] = true
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		kv := strings.SplitN(l, "=", 2)
		if len(kv) == 2 && keys[strings.TrimSpace(kv[0])] {
			fmt.Printf("%s=%s\n", strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
	}
	return 0
}

// actMigrateConf: migrate_conf parity — write a TOML migration of the
// conf next to it; --dry-run prints the TOML (or a diff vs existing).
func actMigrateConf() int {
	out := strings.TrimSuffix(vmConf, ".conf") + ".toml"
	if dryRunFlag {
		if _, err := os.Stat(out); err == nil {
			t, _ := tool("quickemu-toml", "from-conf", "--file", vmConf)
			fmt.Println("Diff (existing " + out + " vs regenerated):")
			fmt.Print(t)
		} else {
			t, terr := tool("quickemu-toml", "from-conf", "--file", vmConf)
			fmt.Print(t)
			if terr != nil {
				return 1
			}
		}
		return 0
	}
	if _, err := os.Stat(out); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", out)
		return 1
	}
	t, err := tool("quickemu-toml", "from-conf", "--file", vmConf)
	if err != nil {
		fmt.Print(t)
		return 1
	}
	if err := os.WriteFile(out, []byte(t), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! "+err.Error())
		return 1
	}
	fmt.Println(" - Migrated " + vmConf + " -> " + out + " (original kept)")
	return 0
}

// actEvents: qe_events_read parity — JSONL events with seq > cursor.
func actEvents() int {
	cur := 0
	fmt.Sscanf(extra["events"], "%d", &cur)
	f := filepath.Join(vmdir(), vmname()+".events.jsonl")
	b, err := os.ReadFile(f)
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.Contains(l, `"seq":`) {
			continue
		}
		var e struct {
			Seq int `json:"seq"`
		}
		if json.Unmarshal([]byte(l), &e) == nil && e.Seq > cur {
			fmt.Println(l)
		}
	}
	return 0
}

// actPlugins: plugin_list parity — quickemu-plugin-* on PATH.
func actPlugins() int {
	seen := map[string]bool{}
	var names []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if n := e.Name(); strings.HasPrefix(n, "quickemu-plugin-") && !seen[n] {
				seen[n] = true
				names = append(names, strings.TrimPrefix(n, "quickemu-plugin-"))
			}
		}
	}
	for _, n := range names {
		fmt.Println(n)
	}
	return 0
}

// actPlugin: plugin_dispatch parity — exec quickemu-plugin-<name> with
// the plugin env contract.
func actPlugin(name string, pargs []string) int {
	bin, err := exec.LookPath("quickemu-plugin-" + name)
	if err != nil {
		if outFmt == "json" {
			emitFail("vm.plugin", "NOT_FOUND", "plugin '"+name+"' not installed")
		} else {
			fmt.Fprintf(os.Stderr, "ERROR! Plugin 'quickemu-plugin-%s' not found on PATH.\n", name)
		}
		return 1
	}
	cmd := exec.Command(bin, pargs...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	cmd.Env = append(os.Environ(),
		"VMDIR="+vmdir(), "VMNAME="+vmname(),
		"OUTPUT_FORMAT="+outFmt, "QE_SCHEMA_VERSION=1")
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}

// jsonStr marshals a small object to a JSON string — for event
// payloads that travel inside the envelope rather than standalone.
func jsonStr(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// qmpCmd marshals a QMP execute envelope — shared helper so embedded
// ids/filenames are escaped by encoding/json, not hand-rolled Sprintf.
func qmpCmd(execute string, args map[string]any) string {
	return qelib.QMPCmd(execute, args)
}

// actHotplugUSB: vm_hotplug_usb parity — usb-host device_add.
func actHotplugUSB() int {
	vp := extra["hotplug-usb"]
	if ok, _ := filepath.Match("[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]:[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]", vp); !ok {
		fmt.Fprintln(os.Stderr, "ERROR! --hotplug-usb needs vendor:product hex (e.g. 05ac:12a8).")
		return 1
	}
	var vid, pid uint64
	fmt.Sscanf(vp, "%x:%x", &vid, &pid)
	id := "hotusb" + strings.ReplaceAll(vp, ":", "")
	payload := qmpCmd("device_add", map[string]any{"driver": "usb-host", "id": id, "vendorid": vid, "productid": pid})
	out, err := qmpRaw(payload)
	fmt.Print(out)
	if persistFlag {
		// Desired state vs observed QMP state (vm_hotplug_usb parity):
		// the conf records the device for next boot even when the live
		// attach failed.
		hotplugPersist("usb", vp)
	}
	if err == nil {
		qeEvent("vm.hotplug", jsonStr(map[string]any{"device": "usb", "id": id}))
		fmt.Println(" - Hot-plugged USB " + vp + " as " + id + ".")
	}
	return rc(err)
}

// actHotplugDisk: vm_hotplug_disk parity — virtio-blk + blockdev-add.
func actHotplugDisk() int {
	f := extra["hotplug-disk"]
	abs, err := filepath.Abs(f)
	if err != nil || !fileExists(abs) {
		fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", f)
		return 1
	}
	base := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, filepath.Base(f))
	id := "hotdisk" + base
	node := id + "node"
	out, err := qmpRaw(qmpCmd("blockdev-add", map[string]any{"node-name": node,
		"driver": "qcow2", "file": map[string]any{"driver": "file", "filename": abs}}))
	fmt.Print(out)
	if err == nil {
		out, err = qmpRaw(qmpCmd("device_add",
			map[string]any{"driver": "virtio-blk-pci", "id": id, "drive": node}))
		fmt.Print(out)
	}
	if persistFlag {
		hotplugPersist("disk", f)
	}
	if err == nil {
		qeEvent("vm.hotplug", jsonStr(map[string]any{"device": "disk", "id": id}))
		fmt.Println(" - Hot-plugged disk " + f + " as " + id + ".")
	}
	return rc(err)
}

// actHotunplug: vm_hotunplug parity — device_del by id.
func actHotunplug() int {
	id := extra["hotunplug"]
	payload := qmpCmd("device_del", map[string]any{"id": id})
	out, err := qmpRaw(payload)
	fmt.Print(out)
	if err == nil {
		qeEvent("vm.hotunplug", jsonStr(map[string]any{"id": id}))
		fmt.Println(" - Hot-unplugged " + id + ".")
	}
	return rc(err)
}

// hotplugPersist: append the device to the conf so it reattaches on
// next boot (--persist parity with monitor.sh). Shared with
// quickemu-qmp via qelib — a single implementation for the splice.
func hotplugPersist(kind, val string) {
	if err := qelib.HotplugPersist(vmConf, kind, val); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not persist %s %s: %v\n", kind, val, err)
		return
	}
	confKVCache.Lock()
	confKVCache.path = "" // conf changed on disk — next confKV reparses
	confKVCache.Unlock()
	fmt.Println(" - Persisted " + kind + " " + val + " into " + vmConf + " (applies next boot).")
}

// qeEvent: append a JSONL event to <vmdir>/<vm>.events.jsonl
// (qe_event parity: seq, ts, event, vm, pid, data).
func qeEvent(typ, data string) {
	f := filepath.Join(vmdir(), vmname()+".events.jsonl")
	seq := 1
	if b, err := os.ReadFile(f); err == nil {
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		var last struct {
			Seq int `json:"seq"`
		}
		if json.Unmarshal([]byte(lines[len(lines)-1]), &last) == nil {
			seq = last.Seq + 1
		}
	}
	var pid any
	if p := readPID(); p > 0 {
		pid = p
	}
	if data == "" {
		data = "null"
	}
	b, _ := json.Marshal(map[string]any{
		"seq": seq, "ts": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"event": typ, "vm": vmname(), "pid": pid,
		"data": json.RawMessage(data)})
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer fh.Close()
	fmt.Fprintln(fh, string(b))
}

// emitFail: failure envelope matching the Bash qe_fail contract.
func emitFail(cmd, code, msg string) {
	emitEnvelope(cmd, false, nil, []any{map[string]any{
		"code": code, "message": msg, "severity": "error", "retryable": false}})
}

func usage() {
	fmt.Println(`quickemu — create and control VMs (Go CLI)
Usage: quickemu --vm <conf> [action flags]

Actions: --start --dry-run|--echo --status --status-quo --pause --resume
  --shutdown --reset --stop --kill --restart --list-vms --delete-vm
  --delete-disk --clone <n> --linked-clone <d> --rename <n> --move <dir>
  --backup --backup-list --backup-verify <f> --backup-restore <f>
  --backup-prune [n] --export [f] --import <f> --trash --trash-list
  --trash-empty --trash-restore <vm> --snapshot <act> [tag] --external
  --snapshot-tree --snapshot-verify --monitor-cmd <c> --send-keys <k>
  --type <t> --screenshot [f] --disk-compact --disk-resize <sz>
  --spice-console --vnc --publicdir --serial-console --monitor-console
  --hotplug-usb <v:p> --hotplug-disk <f> --hotunplug <id> --persist
  --autostart --shortcut --delete-shortcut --doctor|--check --fix
  --delete-plan --plan-id <id> --inspect-conf --migrate-conf --stats
Host: --capabilities --host-info --inventory --vfio-scan --events [n]
  --plugins --plugin <n> [args] --op-list --op-status --op-watch
Conf overrides: --display --viewer --width --height --ssh-port
  --spice-port --monitor --serial --keyboard --mouse --usb-controller
  --sound-card --public-dir --profile --offline --format json|text`)
}

// subcommandAliases: options.sh line-16 parity — `quickemu status
// --vm x` is rewritten to `quickemu --status --vm x`. Unknown bare
// words fall through to the normal unknown-parameter error.
var subcommandAliases = map[string]bool{
	"status": true, "vfio-scan": true, "disk-compact": true, "move": true,
	"pause": true, "resume": true, "reset": true, "shutdown": true,
	"kill": true, "restart": true, "delete-vm": true, "delete-plan": true,
	"list-vms": true, "capabilities": true, "host-info": true,
	"migrate-conf": true, "inspect-conf": true, "backup": true,
	"backup-list": true, "trash": true, "trash-list": true, "clone": true,
	"rename": true, "snapshot-apply": true, "snapshot-create": true,
	"snapshot-delete": true, "snapshot-info": true, "snapshot-list": true,
	"monitor-cmd": true, "send-keys": true, "type": true, "screenshot": true,
	"viewer": true, "fullscreen": true, "status-quo": true, "vnc": true,
	"spice-console": true, "publicdir": true, "serial-console": true,
	"monitor-console": true, "ignore-msrs": true,
	"ignore-msrs-always": true, "create-disk": true, "disk-resize": true,
	"disk-resize-commit": true, "backup-verify": true, "backup-restore": true,
	"trash-restore": true, "op-status": true, "operation-status": true,
	"operation-list": true, "operation-cancel": true, "op-watch": true,
	"operation-watch": true, "op-list": true,
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	args := os.Args[1:]
	// Subcommand alias: `quickemu <action> ...` -> `quickemu --<action>`.
	if !strings.HasPrefix(args[0], "-") && subcommandAliases[args[0]] {
		args[0] = "--" + args[0]
	}
	// Normalise: Bash accepts -flag and --flag equally, plus --flag=value.
	var norm []string
	for _, a := range args {
		if strings.HasPrefix(a, "--") && strings.Contains(a, "=") {
			kv := strings.SplitN(a, "=", 2)
			norm = append(norm, kv[0], kv[1])
		} else if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 {
			norm = append(norm, "-"+a)
		} else {
			norm = append(norm, a)
		}
	}
	args = norm
	dryRunFlag = false
	for i := 0; i < len(args); i++ {
		a := args[i]
		val := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		// scOpt accumulates the flag verbatim into the shortcut Exec line
		// (SHORTCUT_OPTIONS parity).
		scOpt := func() { shortcutOpts = append(shortcutOpts, a) }
		scOptVal := func() { shortcutOpts = append(shortcutOpts, a, val()) }
		switch a {
		case "-h", "--help":
			usage()
			return
		case "--version":
			fmt.Println(qeVersion)
			return
		case "--vm":
			vmConf = val()
		case "--format":
			outFmt = val()
		case "--dry-run", "--echo":
			dryRunFlag = true
		case "--status":
			actions = append(actions, "status_vm")
		case "--status-quo":
			confOpts["snapshot"] = "on"
		case "--capabilities":
			actions = append(actions, "capabilities")
		case "--host-info":
			actions = append(actions, "host_info")
		case "--inventory":
			actions = append(actions, "inventory_show")
		case "--stats":
			actions = append(actions, "vm_stats")
		case "--events":
			cur := "0"
			if i+1 < len(args) && isNum(args[i+1]) {
				cur = val()
			}
			extra["events"] = cur
			actions = append(actions, "events_read")
		case "--vfio-scan":
			actions = append(actions, "vfio_scan")
		case "--migrate-conf":
			actions = append(actions, "migrate_conf")
		case "--inspect-conf":
			actions = append(actions, "inspect_conf")
		case "--export":
			extra["export"] = val()
			actions = append(actions, "export_vm")
		case "--import":
			extra["import"] = val()
			actions = append(actions, "import_vm")
		case "--backup-verify":
			extra["backup-verify"] = val()
			actions = append(actions, "backup_verify")
		case "--backup-restore":
			extra["backup-restore"] = val()
			actions = append(actions, "backup_restore")
		case "--backup-prune":
			k := val()
			if k == "" {
				k = "5"
			}
			extra["backup-prune"] = k
			actions = append(actions, "backup_prune")
		case "--trash-empty":
			actions = append(actions, "trash_empty")
		case "--trash":
			trashFlag = true
		case "--trash-restore":
			extra["trash-restore"] = val()
			actions = append(actions, "trash_restore")
		case "--autostart":
			actions = append(actions, "autostart_vm")
		case "--shortcut":
			actions = append(actions, "shortcut_create")
		case "--delete-shortcut":
			actions = append(actions, "delete_shortcut")
		case "--snapshot-tree":
			actions = append(actions, "snapshot_tree")
		case "--snapshot-verify":
			actions = append(actions, "snapshot_verify")
		case "--snapshot":
			snapshotAct = val()
			switch snapshotAct {
			case "info", "list", "tree", "verify":
			default:
				snapshotTag = val()
			}
			actions = append(actions, "snapshot_cmd")
		case "--external":
			externalFlag = true
		case "--full":
			extra["full"] = "1"
		case "--plugin":
			os.Exit(actPlugin(val(), args[i+1:]))
		case "--plugins":
			actions = append(actions, "plugin_list")
		case "--fix":
			doctorFix = true
		case "--profile":
			confOpts["profile"] = val()
		case "--plan-id":
			extra["plan-id"] = val()
		case "--hotplug-usb":
			extra["hotplug-usb"] = val()
			actions = append(actions, "vm_hotplug_usb")
		case "--hotplug-disk":
			extra["hotplug-disk"] = val()
			actions = append(actions, "vm_hotplug_disk")
		case "--hotunplug":
			extra["hotunplug"] = val()
			actions = append(actions, "vm_hotunplug")
		case "--persist":
			persistFlag = true
		case "--offline":
			scOpt()
			confOpts["network"] = "none"
		case "--width", "--height", "--ssh-access", "--ssh-port", "--spice-port",
			"--monitor-telnet-host", "--monitor-telnet-port",
			"--serial-telnet-host", "--serial-telnet-port",
			"--keyboard", "--mouse", "--usb-controller", "--usb-passthrough",
			"--sound-card", "--sound-duplex":
			scOptVal()
			confOpts[strings.TrimPrefix(a, "--")] = args[i]
		case "--list-vms", "--list":
			actions = append(actions, "list_vms")
		case "--pause":
			actions = append(actions, "pause_vm")
		case "--resume":
			actions = append(actions, "resume_vm")
		case "--shutdown":
			actions = append(actions, "shutdown_vm")
		case "--reset":
			actions = append(actions, "reset_vm")
		case "--stop":
			actions = append(actions, "stop_vm")
		case "--kill":
			actions = append(actions, "kill_vm")
		case "--restart":
			actions = append(actions, "stop_vm", "start")
		case "--start":
			actions = append(actions, "start")
		case "--monitor-cmd":
			extra["monitor-cmd"] = val()
			actions = append(actions, "monitor_cmd")
		case "--send-keys":
			extra["send-keys"] = val()
			actions = append(actions, "send_keys")
		case "--type":
			extra["type"] = val()
			actions = append(actions, "type_text")
		case "--screenshot":
			extra["screenshot"] = ""
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				extra["screenshot"] = val()
			}
			actions = append(actions, "screenshot_vm")
		case "--delete", "--delete-disk":
			actions = append(actions, "delete_disk")
		case "--delete-vm":
			actions = append(actions, "delete_vm")
		case "--delete-plan":
			actions = append(actions, "delete_plan")
		case "--doctor", "--check":
			actions = append(actions, "doctor_vm")
		case "--clone":
			extra["clone"] = val()
			actions = append(actions, "clone_vm")
		case "--linked-clone":
			extra["linked-clone"] = val()
			actions = append(actions, "linked_clone_vm")
		case "--rename":
			extra["rename"] = val()
			actions = append(actions, "rename_vm")
		case "--move":
			extra["move"] = val()
			actions = append(actions, "move_vm")
		case "--backup":
			actions = append(actions, "backup_vm")
		case "--backup-list":
			actions = append(actions, "backup_list")
		case "--trash-list":
			actions = append(actions, "trash_list")
		case "--snapshot-create":
			extra["snapshot-create"] = val()
			actions = append(actions, "snapshot_create")
		case "--snapshot-apply":
			extra["snapshot-apply"] = val()
			actions = append(actions, "snapshot_apply")
		case "--snapshot-delete":
			extra["snapshot-delete"] = val()
			actions = append(actions, "snapshot_delete")
		case "--snapshot-list":
			actions = append(actions, "snapshot_list")
		case "--disk-compact":
			actions = append(actions, "disk_compact")
		case "--disk-resize":
			extra["disk-resize"] = val()
			actions = append(actions, "disk_resize")
		case "--op-status", "--operation-status":
			extra["op-status"] = val()
			actions = append(actions, "operation_status")
		case "--op-list", "--operation-list":
			actions = append(actions, "operation_list")
		case "--op-cancel", "--operation-cancel":
			extra["op-cancel"] = val()
			actions = append(actions, "operation_cancel")
		case "--op-watch", "--operation-watch":
			extra["op-watch"] = val()
			actions = append(actions, "operation_watch")
		// conf overrides consumed by quickemu-launch
		case "--display":
			confOpts["display"] = val()
		case "--access":
			confOpts["access"] = val()
		case "--public-dir":
			confOpts["public_dir"] = val()
		case "--monitor":
			confOpts["monitor"] = val()
		case "--serial":
			confOpts["serial"] = val()
		case "--viewer":
			confOpts["viewer"] = val()
		case "--spice-console":
			actions = append(actions, "spice_console")
		case "--vnc":
			actions = append(actions, "vnc_viewer")
		case "--publicdir":
			actions = append(actions, "publicdir_open")
		case "--serial-console":
			actions = append(actions, "serial_console")
		case "--monitor-console":
			actions = append(actions, "monitor_console")
		case "--fullscreen", "--full-screen":
			confOpts["fullscreen"] = "true"
		case "--braille":
			confOpts["braille"] = "true"
		case "--keyboard_layout", "--keyboard-layout":
			confOpts["keyboard_layout"] = val()
		case "--cpu-pinning":
			confOpts["cpu_pinning"] = val()
		case "--extra_args", "--extra-args":
			confOpts["extra_args"] = val()
		case "--ignore-tsc-warning":
			confOpts["ignore_tsc_warning"] = "on"
		case "--ignore-msrs-always":
			actions = append(actions, "ignore_msrs_always")
		case "--ignore-msrs":
			confOpts["ignore_msrs"] = "on"
		default:
			if strings.HasPrefix(a, "-") {
				// options.sh parity: the message and usage go to
				// stdout (echo), exit 1 — structured callers rely on
				// an empty-stderr+nonzero-exit shape to classify the
				// failure as a missing envelope.
				fmt.Printf("ERROR! \"%s\" is not a supported parameter.\n", a)
				usage()
				os.Exit(1)
			}
		}
	}
	if vmConf == "" && len(actions) == 0 {
		usage()
		os.Exit(2)
	}
	// --format propagates to delegated tools via the env (Bash reads
	// OUTPUT_FORMAT the same way; quickemu-vm honors it too).
	if outFmt == "json" {
		os.Setenv("OUTPUT_FORMAT", "json")
	}
	// Fixes running VMs when PWD is not relative to the VM directory
	// (bin/quickemu.bash parity): when the configured disk is not
	// reachable from the cwd, operate from the conf's directory so
	// relative paths in the conf resolve the way Bash resolves them.
	if vmConf != "" {
		if abs, err := filepath.Abs(vmConf); err == nil {
			vmConf = abs
		}
		// conf-load parity: Bash dies here ("Virtual machine
		// configuration not found.") before any action runs — the
		// message goes to stdout like every Bash echo.
		if _, err := os.Stat(vmConf); err != nil {
			fmt.Println("ERROR! Virtual machine configuration not found.")
			os.Exit(1)
		}
		disk := confKV()["disk_img"]
		if disk == "" {
			df := confKV()["disk_format"]
			if df == "" {
				df = "qcow2"
			}
			disk = filepath.Join(vmdir(), "disk."+df)
		}
		if _, err := os.Stat(disk); err != nil {
			os.Chdir(filepath.Dir(vmConf))
		}
	}
	// default action: boot the VM (dry-run prints the argv)
	if len(actions) == 0 {
		if dryRunFlag {
			os.Exit(actDryRun())
		}
		os.Exit(actLaunch())
	}
	// Bash main flow: a stale pidfile means an unclean shutdown — clean
	// runtime artefacts before running any action (vm_runtime_cleanup).
	runtimeCleanup()
	rcv := 0
	for _, act := range actions {
		if act == "start" {
			if dryRunFlag {
				rcv = actDryRun()
			} else {
				rcv = actLaunch()
			}
			continue
		}
		// Host-level actions do not need --vm (early-action parity:
		// Bash runs capabilities|list_vms|import_vm before conf load;
		// the rest are host-scope ops that only read machine state).
		hostOnly := map[string]bool{
			"capabilities": true, "host_info": true, "list_vms": true,
			"inventory_show": true, "plugin_list": true, "import_vm": true,
			"trash_list": true, "trash_empty": true, "trash_restore": true,
			"vfio_scan": true,
		}
		if vmConf == "" && !hostOnly[act] {
			fmt.Fprintln(os.Stderr, "ERROR! --vm is required for "+act)
			rcv = 2
			continue
		}
		// vm_lock parity: Bash runs mutating actions under the
		// exclusive VM lock (kill|pause|resume|shutdown|reset|
		// send_keys|type_text|screenshot; vm/fs ops lock inside vmOp).
		locked := map[string]bool{
			"kill_vm": true, "pause_vm": true, "resume_vm": true,
			"shutdown_vm": true, "reset_vm": true, "stop_vm": true,
			"send_keys": true, "type_text": true, "screenshot_vm": true,
		}
		if fn := dispatch(act); fn != nil {
			if locked[act] && !vmLockAcquire() {
				rcv = 1
				continue
			}
			r := fn()
			if locked[act] {
				vmLockRelease()
			}
			if r != 0 {
				rcv = r
			}
		}
	}
	os.Exit(rcv)
}
