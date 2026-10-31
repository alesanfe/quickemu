// runner.go: the run pipeline — the OBTAIN→PREPARE→INSTALL→PROVISION→
// VERIFY→SEAL cycle as discrete journal steps. Every step boundary is
// a cancellation checkpoint and every event is a JSONL record, so a
// watching UI always knows where a run is and a failed run leaves a
// diagnosable trail.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

const version = "0.1.0"

// runCtx is everything a step needs — resolved conf, artifacts dir,
// the ephemeral key and the chosen SSH forward.
type runCtx struct {
	p          *Profile
	prov       provider
	workdir    string // where quickget places the VM dir
	profileDir string // profile-relative file sources resolve here
	confPath   string
	vmDir      string
	vmName     string
	artDir     string
	runID      string
	opID       string
	sshPort    int
	qemuPID    int // spawned by this run's launch — 0 until then
	kp         *keypair
	e          *emitter
}

// planSteps is the step list a run will execute — surfaced by `plan`
// so the UI can review before anything destructive happens.
func planSteps(prov provider) []string {
	steps := []string{"resolve", "acquire-image", "prepare"}
	if prov.needsSecondBoot() {
		steps = append(steps, "launch-install", "await-install", "reconfigure", "launch-installed")
	} else {
		steps = append(steps, "launch")
	}
	return append(steps, "wait-ssh", "provision", "verify", "shutdown", "seal")
}

// run executes the pipeline. Errors are reported via run.failed +
// opFail; cancel is checked at every step boundary.
func run(p *Profile, profileDir, workdir string, keepVM bool) int {
	return runSteps(p, profileDir, workdir, keepVM, nil)
}

// runState is the resume checkpoint — persisted to the artifacts dir
// after every step so a killed or failed run can be picked up at the
// first step that did not complete.
type runState struct {
	RunID          string `json:"runId"`
	Profile        string `json:"profile"`
	Workdir        string `json:"workdir"`
	SSHPort        int    `json:"sshPort"`
	CompletedIndex int    `json:"completedIndex"` // steps[0:CompletedIndex] done
	Status         string `json:"status"`         // running|failed|completed
}

func (r *runCtx) saveState(completed int, status string) {
	st := runState{
		RunID: r.runID, Profile: r.p.sourcePath,
		Workdir: r.workdir, SSHPort: r.sshPort,
		CompletedIndex: completed, Status: status,
	}
	if b, err := json.MarshalIndent(st, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(r.artDir, "state.json"), b, 0o600)
	}
}

// runSteps is run() plus an optional resume checkpoint. With prior
// set, the run keeps its id, artifacts, key and SSH port, and the loop
// starts at the first step that never completed.
func runSteps(p *Profile, profileDir, workdir string, keepVM bool, prior *runState) int {
	prov, err := pickProvider(p)
	if err != nil {
		emitErr("NO_PROVIDER", err.Error())
		return 1
	}
	steps := planSteps(prov)
	r := &runCtx{p: p, prov: prov, workdir: workdir,
		profileDir: profileDir, runID: newRunID()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 1: resolve — find (or plan to create) the VM conf before
	// the journal opens; we need vmDir for the ops store.
	confPath, vmDir, vmName, ferr := findVMConf(workdir)
	if ferr == nil {
		r.confPath, r.vmDir, r.vmName = confPath, vmDir, vmName
		r.artDir = filepath.Join(vmDir, ".quicktest", r.runID)
	} else {
		// predicted layout — quickget names the dir <os>-<release>
		guess := filepath.Join(workdir, p.System.OS+"-"+p.System.Release)
		r.artDir = filepath.Join(guess, ".quicktest", r.runID)
	}

	start := 0
	if prior != nil {
		if ferr != nil {
			emitErr("RESUME", "no VM conf in "+workdir+" — nothing to resume")
			return 1
		}
		r.runID = prior.RunID
		r.artDir = filepath.Join(vmDir, ".quicktest", r.runID)
		start = prior.CompletedIndex
	}
	os.MkdirAll(r.artDir, 0o755)
	r.e = newEmitter(r.runID, r.vmName, r.vmDir, r.artDir, false)
	defer r.e.close()

	if prior != nil {
		// events.jsonl appends — seq must continue, not restart at 1.
		if b, err := os.ReadFile(filepath.Join(r.artDir, "events.jsonl")); err == nil {
			r.e.seq = strings.Count(string(b), "\n")
		}
		r.e.emit("run.resumed", map[string]any{
			"profile": p.Profile.Name, "fromStep": start,
			"steps": steps})
		if r.kp, err = loadKeypair(r.artDir); err != nil {
			return r.failRun("KEYPAIR", err)
		}
		r.sshPort = prior.SSHPort
	} else {
		r.e.emit("run.started", map[string]any{
			"profile": p.Profile.Name, "os": p.System.OS,
			"release": p.System.Release, "provider": prov.id(),
			"steps": steps, "reuse": ferr == nil})
		if r.kp, err = genKeypair(r.artDir); err != nil {
			return r.failRun("KEYPAIR", err)
		}
		if r.sshPort = p.Machine.SSHPort; r.sshPort <= 0 {
			if r.sshPort, err = freePort(); err != nil {
				return r.failRun("PORTS", err)
			}
		}
	}
	r.saveState(start, "running")

	total := len(steps)
	for i := start; i < total; i++ {
		id := steps[i]
		if opCancelled(r.vmDir, r.opID) {
			r.shutdownVM()
			r.saveState(i, "failed")
			return r.failRun("CANCELLED", fmt.Errorf("run cancelled"))
		}
		opSet(r.vmDir, r.opID, "running", i*100/total)
		err := r.e.step(id, i+1, total, func() error {
			return r.dispatch(ctx, id)
		})
		if err != nil {
			r.saveState(i, "failed")
			r.shutdownVM()
			return r.failRun("STEP_FAILED", fmt.Errorf("%s: %w", id, err))
		}
		r.saveState(i+1, "running")
	}
	opDone(r.vmDir, r.opID)
	r.saveState(total, "completed")
	r.e.emit("run.completed", map[string]any{"vm": r.vmName,
		"dir": r.vmDir, "artifacts": r.artDir})
	return 0
}

// dispatch maps a step id to its implementation — kept as a switch so
// `plan`'s declared list is the single source of truth.
func (r *runCtx) dispatch(ctx context.Context, id string) error {
	switch id {
	case "resolve":
		return r.resolve()
	case "acquire-image":
		return r.acquire()
	case "prepare":
		return r.prov.prepare(r)
	case "launch-install", "launch":
		return r.launch(ctx)
	case "await-install":
		return r.awaitInstall(ctx)
	case "reconfigure":
		return confDrop(r.confPath, r.prov.dropConfKeys()...)
	case "launch-installed":
		return r.launch(ctx)
	case "wait-ssh":
		return waitSSH(ctx, r.e, r.sshPort, r.prov.sshUser(r.p), r.kp,
			time.Duration(r.p.Automation.SSHTimeoutMin)*time.Minute)
	case "provision":
		return r.provision(ctx)
	case "verify":
		return r.verify(ctx)
	case "shutdown":
		return r.shutdown(ctx)
	case "seal":
		return r.seal()
	}
	return fmt.Errorf("unknown step %s", id)
}

// resolve opens the ops journal entry once the VM dir is known (or
// predicted) — nothing here can fail the run, it only annotates.
func (r *runCtx) resolve() error {
	if r.vmDir != "" {
		if id, err := opNew(r.vmDir, r.vmName); err == nil {
			r.opID = id
		}
	}
	r.e.emit("plan.resolved", map[string]any{
		"conf": r.confPath, "sshPort": r.sshPort,
		"steps": planSteps(r.prov)})
	return nil
}

// acquire pulls the OS image via quickget — sibling binary, same
// trust/hash pipeline a manual download would take.
func (r *runCtx) acquire() error {
	if r.confPath != "" {
		r.e.progress("acquire-image", "reusing existing "+r.confPath)
		if r.vmDir != "" && r.opID == "" {
			if id, err := opNew(r.vmDir, r.vmName); err == nil {
				r.opID = id
			}
		}
		return nil
	}
	args := []string{"--disable-unattended", r.p.System.OS, r.p.System.Release}
	if r.p.System.Edition != "" {
		args = append(args, r.p.System.Edition)
	}
	err := toolRun(r.workdir, "quickget", func(line string) {
		if strings.Contains(line, "%") || strings.Contains(line, "Downloading") {
			r.e.progress("acquire-image", line)
		}
	}, args...)
	if err != nil {
		return fmt.Errorf("quickget: %w", err)
	}
	confPath, vmDir, vmName, err := findVMConf(r.workdir)
	if err != nil {
		return err
	}
	r.confPath, r.vmDir, r.vmName = confPath, vmDir, vmName
	r.e.setVM(vmName, vmDir)
	// artifacts move next to the real VM dir now that it exists
	newArt := filepath.Join(vmDir, ".quicktest", r.runID)
	if r.artDir != newArt {
		os.MkdirAll(newArt, 0o755)
		if r.kp.privPath != "" {
			moveFile(r.kp.privPath, filepath.Join(newArt, "id_ed25519"))
			r.kp.privPath = filepath.Join(newArt, "id_ed25519")
		}
		r.artDir = newArt
		r.e.close()
		r.e = newEmitter(r.runID, vmName, vmDir, r.artDir, false)
	}
	if id, err := opNew(vmDir, vmName); err == nil {
		r.opID = id
	}
	if r.p.Machine.DiskSize != "" {
		_ = confSet(r.confPath, map[string]string{"disk_size": r.p.Machine.DiskSize})
	}
	return nil
}

// launch starts the VM headless — quickemu spawns qemu detached and
// exits, the .pid/qmp/serial sockets are the observable handles.
func (r *runCtx) launch(ctx context.Context) error {
	if _, err := toolOut("quickemu", "--vm", r.confPath); err != nil {
		return fmt.Errorf("quickemu --vm: %w", err)
	}
	// wait for the pidfile/sockets to materialise — and remember the
	// pid WE spawned: a stale quicktest sharing the VM must never
	// signal a qemu a newer run just launched under the same pidfile.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(r.vmDir, r.vmName+".pid")); err == nil {
			r.qemuPID = readPidFile(r.vmDir, r.vmName)
			go tailSerial(ctx, filepath.Join(r.vmDir, r.vmName+"-serial.socket"),
				filepath.Join(r.artDir, "serial.log"), r.e)
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("VM did not start (no pidfile after 60s)")
}

// awaitInstall: `shutdown: poweroff` in the answer file makes the end
// of install a plain qemu exit — no OCR, no serial scraping needed
// for the control flow (serial is still captured for diagnostics).
func (r *runCtx) awaitInstall(ctx context.Context) error {
	return waitPIDGone(ctx, r.e, r.vmDir, r.vmName,
		time.Duration(r.p.Automation.InstallTimeoutMin)*time.Minute)
}

// provision applies packages/services/files/commands over SSH. The
// package manager is detected on the guest — a guest without one
// gets a warning, not a failure (CirrOS has no apt).
func (r *runCtx) provision(ctx context.Context) error {
	c, err := sshDial(r.sshPort, r.prov.sshUser(r.p), r.kp.signer)
	if err != nil {
		return err
	}
	defer c.Close()
	// first boot may still be running cloud-init — wait it out so the
	// apt lock is free (no cloud-init binary → instant no-op). Reads
	// status without sudo: on images shipping doas-only (Alpine) sudo
	// only exists once cloud-init's own package stage installs it.
	_, _, _ = sshExec(c, "command -v cloud-init >/dev/null && cloud-init status --wait || true")
	for i := 0; i < 30; i++ {
		if _, rc, _ := sshExec(c, "command -v sudo >/dev/null"); rc == 0 {
			break
		}
		if i == 29 {
			r.e.progress("provision", "sudo still absent after cloud-init — installs may fail")
		}
		time.Sleep(10 * time.Second)
	}
	if pkgs := r.p.Provision.Packages; len(pkgs) > 0 {
		pm := detectPkgMgr(c)
		if pm == "" {
			r.e.progress("provision", "no supported package manager — skipping packages")
		} else {
			for i, pkg := range pkgs {
				r.e.emit("step.progress", map[string]any{
					"step": "provision", "current": i + 1,
					"total": len(pkgs), "message": "installing " + pkg})
				cmd, aerr := pkgInstallCmd(pm, pkg)
				if aerr != nil {
					return aerr
				}
				out, rc, err := sshExec(c, cmd)
				if err != nil || rc != 0 {
					return fmt.Errorf("install %s (rc=%d): %s", pkg, rc, tailLines(out, 5))
				}
			}
		}
	}
	svcMgr := "systemd"
	if len(r.p.Provision.Services) > 0 {
		if _, rc, _ := sshExec(c, "command -v rc-service >/dev/null"); rc == 0 {
			svcMgr = "openrc"
		}
	}
	for _, svc := range r.p.Provision.Services {
		cmd := "sudo systemctl enable --now " + shellQ(svc)
		if svcMgr == "openrc" {
			cmd = "sudo rc-update add " + shellQ(svc) + " default; sudo rc-service " + shellQ(svc) + " start"
		}
		out, _, _ := sshExec(c, cmd+" 2>&1 || true")
		r.e.progress("provision", svc+": "+tailLines(out, 1))
	}
	for _, f := range r.p.Provision.Files {
		src := f.Source
		if !filepath.IsAbs(src) {
			src = filepath.Join(r.profileDir, src)
		}
		b, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("provision file %s: %w", src, err)
		}
		mode := f.Mode
		if mode == "" {
			mode = "0644"
		}
		if err := sshWrite(c, f.Destination, mode, b); err != nil {
			return fmt.Errorf("write %s: %w", f.Destination, err)
		}
		if f.Owner != "" {
			_, _, _ = sshExec(c, "sudo chown "+shellQ(f.Owner)+" "+shellQ(f.Destination))
		}
	}
	for _, cmd := range r.p.Provision.Commands {
		run := cmd.Run
		if cmd.As != "" && cmd.As != "root" {
			run = "sudo -u " + shellQ(cmd.As) + " sh -c " + shellQ(cmd.Run)
		}
		out, rc, err := sshExec(c, run)
		r.e.emit("provision.command", map[string]any{
			"name": cmd.Name, "rc": rc, "output": tailLines(out, 10)})
		if err != nil || rc != 0 {
			return fmt.Errorf("command %s failed (rc=%d)", cmd.Name, rc)
		}
	}
	return nil
}

// verify runs the profile's checks; failures are collected so one
// bad check doesn't hide the rest — the run still fails at the end.
func (r *runCtx) verify(ctx context.Context) error {
	if len(r.p.Verify) == 0 {
		r.e.progress("verify", "no checks declared")
		return nil
	}
	c, err := sshDial(r.sshPort, r.prov.sshUser(r.p), r.kp.signer)
	if err != nil {
		return err
	}
	defer c.Close()
	var failed []string
	for _, chk := range r.p.Verify {
		out, rc, err := sshExec(c, chk.Run)
		pass := err == nil && rc == 0 &&
			(chk.Expect == "" || strings.Contains(out, chk.Expect))
		r.e.emit("verify.result", map[string]any{
			"name": chk.Name, "pass": pass, "rc": rc,
			"output": tailLines(out, 5)})
		if !pass {
			failed = append(failed, chk.Name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("checks failed: %s", strings.Join(failed, ", "))
	}
	return nil
}

// shutdown asks the guest to power off over SSH, then escalates to a
// pid kill — a guest that won't die still ends the run.
func (r *runCtx) shutdown(ctx context.Context) error {
	r.shutdownVM()
	return waitPIDGone(ctx, r.e, r.vmDir, r.vmName, 3*time.Minute)
}

// shutdownVM is the synchronous best-effort poweroff used by both the
// shutdown step and cancel/fail cleanup.
func (r *runCtx) shutdownVM() {
	if c, err := sshDial(r.sshPort, r.prov.sshUser(r.p), r.kp.signer); err == nil {
		_, _, _ = sshExec(c, "sudo poweroff || sudo halt || true")
		c.Close()
	}
	// Kill only the pid this run spawned — a pidfile read now could
	// name a qemu a *different* run just launched (resume overlap).
	pid := r.qemuPID
	if pid <= 0 {
		pid = readPidFile(r.vmDir, r.vmName)
	}
	if pid > 0 && qemuProc(pid) {
		// give ACPI 30s, then TERM, then KILL
		for i := 0; i < 6 && pidAlive(pid); i++ {
			time.Sleep(5 * time.Second)
		}
		if pidAlive(pid) {
			syscall.Kill(pid, syscall.SIGTERM)
			for i := 0; i < 6 && pidAlive(pid); i++ {
				time.Sleep(5 * time.Second)
			}
		}
		if pidAlive(pid) {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

// readPidFile parses <vmDir>/<vm>.pid; 0 on any miss.
func readPidFile(vmDir, vm string) int {
	b, err := os.ReadFile(filepath.Join(vmDir, vm+".pid"))
	if err != nil {
		return 0
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid); err != nil {
		return 0
	}
	return pid
}

// qemuProc reports whether pid still looks like a qemu process —
// /proc absent (tests, non-Linux) fails open so real kills proceed.
func qemuProc(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return true
	}
	return strings.Contains(string(b), "qemu")
}

// seal snapshots the disk and writes the run manifest — the
// reproducibility record: profile, checks, artifacts, image names.
func (r *runCtx) seal() error {
	disk := confGet(r.confPath, "disk_img")
	if disk == "" {
		disk = confGet(r.confPath, "img")
	}
	snap := r.p.Finalize.Snapshot
	var snapErr, snapDone string
	if snap != "" && disk != "" {
		disk = resolveConfPath(r.confPath, disk)
		// Internal snapshots need a real qcow2 backing file — the
		// extension lies (cloudimg .img files ARE qcow2), so ask the
		// conf's declared format and fall back to the QFI magic.
		format := confGet(r.confPath, "disk_format")
		if format == "" {
			if f, err := os.Open(disk); err == nil {
				var hdr [4]byte
				if _, err := io.ReadFull(f, hdr[:]); err == nil &&
					hdr == [4]byte{'Q', 'F', 'I', 0xfb} {
					format = "qcow2"
				}
				f.Close()
			}
		}
		if format == "qcow2" {
			cmd := exec.Command("qemu-img", "snapshot", "-c", snap, disk)
			if out, err := cmd.CombinedOutput(); err != nil {
				snapErr = strings.TrimSpace(string(out))
			} else {
				snapDone = snap
			}
		} else {
			snapErr = fmt.Sprintf("disk format %q cannot hold internal snapshots", format)
		}
	}
	manifest := map[string]any{
		"runId": r.runID, "profile": r.p.Profile.Name,
		"os": r.p.System.OS, "release": r.p.System.Release,
		"provider": r.prov.id(), "vm": r.vmName, "vmDir": r.vmDir,
		"conf": r.confPath, "disk": disk, "snapshot": snapDone,
		"sshPort": r.sshPort, "finishedAt": time.Now().UTC().Format(time.RFC3339),
	}
	if snapErr != "" {
		manifest["snapshotError"] = snapErr
	}
	b, _ := json.MarshalIndent(manifest, "", "  ")
	return os.WriteFile(filepath.Join(r.artDir, "manifest.json"), b, 0o644)
}

func (r *runCtx) failRun(code string, err error) int {
	r.e.emit("run.failed", map[string]any{
		"error": err.Error(), "code": code})
	if r.opID != "" {
		opFail(r.vmDir, r.opID, code, err.Error())
	}
	return 1
}

// --- guest helpers -----------------------------------------------------

// detectPkgMgr sniffs the guest for a supported package manager.
func detectPkgMgr(c *ssh.Client) string {
	for _, pm := range []struct{ name, probe string }{
		{"apt", "command -v apt-get"},
		{"apk", "command -v apk"},
		{"dnf", "command -v dnf"},
		{"pacman", "command -v pacman"},
	} {
		if _, rc, err := sshExec(c, pm.probe); err == nil && rc == 0 {
			return pm.name
		}
	}
	return ""
}

func pkgInstallCmd(pm, pkg string) (string, error) {
	q := shellQ(pkg)
	switch pm {
	case "apt":
		// DPkg::Lock::Timeout: unattended-upgrades/cloud-init hold the
		// dpkg lock well past 'cloud-init status --wait' on Debian.
		lock := " -o DPkg::Lock::Timeout=600 "
		return "sudo DEBIAN_FRONTEND=noninteractive apt-get" + lock + "install -y " + q +
			" || (sudo apt-get" + lock + "update && sudo DEBIAN_FRONTEND=noninteractive apt-get" + lock + "install -y " + q + ")", nil
	case "apk":
		return "sudo apk add " + q, nil
	case "dnf":
		return "sudo dnf install -y " + q, nil
	case "pacman":
		return "sudo pacman -S --noconfirm " + q, nil
	}
	return "", fmt.Errorf("unsupported package manager %s", pm)
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func moveFile(src, dst string) {
	if b, err := os.ReadFile(src); err == nil {
		_ = os.WriteFile(dst, b, 0o600)
		_ = os.Remove(src)
	}
}
