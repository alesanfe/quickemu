// orchestrate.go: QE-071 — up/down/status/restart/destroy.
//
// `up` honours the dependency DAG: levels run sequentially, VMs
// inside a level start in parallel. For each VM: materialise the
// conf (quickget for inline os/release specs, mkconf for bare specs —
// skipped when the conf already exists), start through
// `quickemu --vm <conf> --start [--extra_args ...]` then run its
// healthchecks. --rollback stops everything this run started when
// any VM fails.
//
// `down` walks the DAG in reverse level order; a VM that ignores the
// graceful stop past --grace gets --kill.
//
// All child tools resolve via qelib.FindTool (sibling-first, PATH
// last) — never hardcoded paths.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- logging ----------

// plog serialises progress lines from parallel VM workers — one line
// per event, "[vm] message", exactly the aggregate progress format.
var plogMu sync.Mutex

func plog(format string, args ...any) {
	plogMu.Lock()
	defer plogMu.Unlock()
	fmt.Printf(format+"\n", args...)
}

func vlog(vm, format string, args ...any) {
	plog("[" + vm + "] " + fmt.Sprintf(format, args...))
}

// ---------- conf materialisation ----------

// specConfName is the conf path a vm spec resolves to when it does
// not name one explicitly: <envdir>/<name>.conf.
func specConf(e *Env, vm *VM) string {
	if vm.Conf != "" {
		if filepath.IsAbs(vm.Conf) {
			return vm.Conf
		}
		return filepath.Join(e.Dir, vm.Conf)
	}
	return filepath.Join(e.Dir, vm.Name+".conf")
}

// quickgetConfName mirrors quickget-vm mkconf's naming:
// <os>-<release>[-<edition>].conf in the working dir.
func quickgetConfName(vm *VM) string {
	d := vm.OS + "-" + vm.Release
	if vm.Edition != "" {
		d += "-" + vm.Edition
	}
	return d
}

// ensureConf returns the .conf to boot, creating it when the spec
// requires. Persisted/created state is recorded into st.
func ensureConf(e *Env, st *envState, vm *VM) (string, error) {
	conf := specConf(e, vm)
	if vs, ok := st.VMs[vm.Name]; ok && vs.Conf != "" {
		conf = vs.Conf // an earlier run already resolved it (e.g. renamed)
	}
	if _, err := os.Stat(conf); err == nil {
		st.VMs[vm.Name] = mergeVS(st.VMs[vm.Name], conf)
		return conf, nil // exists — never re-fetch (the documented skip)
	}
	if vm.Conf != "" {
		return "", fmt.Errorf("conf %s not found", conf)
	}
	if vm.OS != "" {
		return quickgetConf(e, st, vm, conf)
	}
	return mkconf(e, st, vm, conf)
}

func mergeVS(vs *vmState, conf string) *vmState {
	if vs == nil {
		vs = &vmState{}
	}
	vs.Conf = conf
	return vs
}

// quickgetConf runs `quickget <os> <release> [edition]` in the env
// dir, then renames the produced dir+conf to the env VM name and
// rewrites the dir-prefixed paths inside the conf.
func quickgetConf(e *Env, st *envState, vm *VM, want string) (string, error) {
	vlog(vm.Name, "fetching %s %s %s via quickget", vm.OS, vm.Release, vm.Edition)
	args := []string{vm.OS, vm.Release}
	if vm.Edition != "" {
		args = append(args, vm.Edition)
	}
	out, err := runTool("quickget", e.Dir, nil, args...)
	if out != "" {
		vlog(vm.Name, "%s", strings.TrimSpace(out))
	}
	if err != nil {
		return "", fmt.Errorf("quickget %s %s: %v", vm.OS, vm.Release, err)
	}
	produced := filepath.Join(e.Dir, quickgetConfName(vm)+".conf")
	if _, err := os.Stat(produced); err != nil {
		// quickget may name the dir differently — glob as a fallback.
		m, _ := filepath.Glob(filepath.Join(e.Dir, "*.conf"))
		if len(m) == 0 {
			return "", fmt.Errorf("quickget produced no conf for %s %s", vm.OS, vm.Release)
		}
		sort.Strings(m)
		produced = m[len(m)-1]
	}
	wantName := confName(want)
	gotName := confName(produced)
	if gotName == wantName {
		st.VMs[vm.Name] = &vmState{Conf: produced, Created: true, VMdir: vmDir(produced)}
		return produced, nil
	}
	// Rename conf + vm dir and rewrite the dir prefix inside the conf.
	oldDir := filepath.Join(e.Dir, gotName)
	newDir := filepath.Join(e.Dir, wantName)
	if b, err := os.ReadFile(produced); err == nil {
		body := strings.ReplaceAll(string(b), `"`+gotName+`/`, `"`+wantName+`/`)
		body = strings.ReplaceAll(body, gotName+`/`, wantName+`/`)
		if err := os.WriteFile(produced, []byte(body), 0o755); err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(oldDir); err == nil {
		if _, err2 := os.Stat(newDir); err2 != nil {
			if err := os.Rename(oldDir, newDir); err != nil {
				return "", fmt.Errorf("rename %s -> %s: %v", oldDir, newDir, err)
			}
		} else {
			return "", fmt.Errorf("cannot rename %s: %s already exists", oldDir, newDir)
		}
	}
	if err := os.Rename(produced, want); err != nil {
		return "", fmt.Errorf("rename %s: %v", produced, err)
	}
	st.VMs[vm.Name] = &vmState{Conf: want, Created: true, VMdir: newDir}
	return want, nil
}

// mkconf writes the minimal .conf for an inline spec without an os —
// the launcher creates the disk image itself via qemu-img when
// disk_img is missing (configure_storage parity).
func mkconf(e *Env, st *envState, vm *VM, conf string) (string, error) {
	guest := "linux"
	ram, cpu := vm.RAM, vm.CPU
	if vm.Profile != "" {
		if defs, ok := profileDefaults(vm.Profile); ok {
			if guest = defs["guest_os"]; guest == "" {
				guest = "linux"
			}
			if ram == "" {
				ram = defs["ram"]
			}
			if cpu == 0 {
				if n, err := strconv.Atoi(defs["cpu_cores"]); err == nil {
					cpu = n
				}
			}
		}
	}
	if ram == "" {
		ram = "2G"
	}
	if cpu == 0 {
		cpu = 2
	}
	size := vm.DiskSize
	if size == "" {
		size = "16G"
	}
	var b strings.Builder
	b.WriteString("# env.toml inline spec — generated by quickemu-env\n")
	b.WriteString(`guest_os="` + guest + `"` + "\n")
	b.WriteString(`disk_img="` + vm.Name + `/disk.qcow2"` + "\n")
	b.WriteString(`disk_format="qcow2"` + "\n")
	b.WriteString(`disk_size="` + size + `"` + "\n")
	b.WriteString(`ram="` + ram + `"` + "\n")
	b.WriteString(`cpu_cores="` + strconv.Itoa(cpu) + `"` + "\n")
	if vm.Profile != "" {
		b.WriteString(`profile="` + vm.Profile + `"` + "\n")
	}
	if err := os.MkdirAll(filepath.Join(e.Dir, vm.Name), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(conf, []byte(b.String()), 0o755); err != nil {
		return "", err
	}
	vlog(vm.Name, "wrote %s (profile=%s ram=%s cpu=%d)", conf, vm.Profile, ram, cpu)
	st.VMs[vm.Name] = &vmState{Conf: conf, Created: true, VMdir: filepath.Join(e.Dir, vm.Name)}
	return conf, nil
}

// profileDefaults wraps qelib's preset table (seam for tests).
var profileDefaults = func(name string) (map[string]string, bool) {
	return qelibProfileDefaults(name)
}

// ---------- start/stop ----------

// startVM boots one VM through the quickemu CLI. The env's net/share
// args ride --extra_args; the launcher merges them into a temp conf.
func startVM(e *Env, st *envState, vm *VM) error {
	conf, err := ensureConf(e, st, vm)
	if err != nil {
		return err
	}
	args := []string{"--vm", conf, "--start"}
	if ea := extraArgs(vm, e, st); ea != "" {
		args = append(args, "--extra_args", ea)
	}
	vlog(vm.Name, "starting")
	out, err := runTool("quickemu", filepath.Dir(conf), nil, args...)
	if out != "" {
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			vlog(vm.Name, "%s", l)
		}
	}
	if err != nil {
		return fmt.Errorf("start failed: %v", err)
	}
	// Record the fresh qemu pid as the net-listener owner so orphan
	// detection attributes socket ports to a live process.
	rt := inspect(conf)
	if vs := st.VMs[vm.Name]; vs != nil {
		vs.PID, vs.Started = rt.PID, true
	}
	for _, n := range e.Nets {
		if n.Kind == "socket" && n.listener() == vm.Name {
			if ns := st.Nets[n.Name]; ns != nil && rt.PID > 0 {
				ns.Owner = rt.PID
				e.portsReserve(ns.Port, rt.PID, "env:"+e.Name+"/"+n.Name)
			}
		}
	}
	return nil
}

// healthFor lists the checks bound to a VM, in declaration order.
func (e *Env) healthFor(name string) []*Health {
	var out []*Health
	for _, h := range e.Health {
		if h.VM == name {
			out = append(out, h)
		}
	}
	return out
}

// vmConf resolves the conf path from state or spec ("" when unknown).
func vmConf(e *Env, st *envState, vm *VM) string {
	if st != nil {
		if vs := st.VMs[vm.Name]; vs != nil && vs.Conf != "" {
			return vs.Conf
		}
	}
	return specConf(e, vm)
}

// stopVM runs `quickemu --stop`, waits out the grace period watching
// the pidfile, then escalates to --kill when the pid refuses to die.
func stopVM(e *Env, st *envState, vm *VM, grace time.Duration, force bool) error {
	conf := vmConf(e, st, vm)
	rt := inspect(conf)
	if rt.State == "stopped" || rt.State == "missing" {
		vlog(vm.Name, "already stopped")
		return nil
	}
	vlog(vm.Name, "stopping")
	out, err := runTool("quickemu", filepath.Dir(conf), nil, "--vm", conf, "--stop")
	if out != "" {
		vlog(vm.Name, "%s", strings.TrimSpace(out))
	}
	if err != nil {
		vlog(vm.Name, "stop reported %v — checking", err)
	}
	deadline := nowUTC().Add(grace)
	for nowUTC().Before(deadline) {
		rt = inspect(conf)
		if rt.PID == 0 || !alive(rt.PID) {
			if vs := st.VMs[vm.Name]; vs != nil {
				vs.Started, vs.PID = false, 0
			}
			vlog(vm.Name, "stopped")
			return nil
		}
		sleeper(200 * time.Millisecond)
	}
	if !force {
		return fmt.Errorf("%s did not stop within %s (use --force to kill)", vm.Name, grace)
	}
	vlog(vm.Name, "force-killing")
	out, err = runTool("quickemu", filepath.Dir(conf), nil, "--vm", conf, "--kill")
	if out != "" {
		vlog(vm.Name, "%s", strings.TrimSpace(out))
	}
	if vs := st.VMs[vm.Name]; vs != nil {
		vs.Started, vs.PID = false, 0
	}
	if err != nil {
		return fmt.Errorf("kill failed: %v", err)
	}
	return nil
}

// ---------- the commands ----------

// cmdUp implements `quickemu-env up`.
func cmdUp(e *Env, sel []string, rollback bool) int {
	vms, errs := e.selectVMs(sel)
	if len(errs) > 0 {
		return printErrs(e.File, errs)
	}
	st := e.loadState()
	e.orphanSweep(st, func(f string, a ...any) { plog("env: "+f, a...) })
	if err := e.ensurePorts(st, func(f string, a ...any) { plog("env: "+f, a...) }); err != nil {
		return errf("%v", err)
	}
	levels := e.order(vms)
	plog("env %s: %d vm(s), %d level(s)", e.Name, len(vms), len(levels))
	var started []*VM // for --rollback, in start order
	var mu sync.Mutex
	fail := false
	for li, level := range levels {
		if len(level) == 0 {
			continue
		}
		plog("== level %d: %s", li, vmNames(level))
		var wg sync.WaitGroup
		for _, vm := range level {
			wg.Add(1)
			go func(vm *VM) {
				defer wg.Done()
				if err := startVM(e, st, vm); err != nil {
					vlog(vm.Name, "FAILED: %v", err)
					mu.Lock()
					fail = true
					mu.Unlock()
					return
				}
				mu.Lock()
				started = append(started, vm)
				mu.Unlock()
				for _, h := range e.healthFor(vm.Name) {
					err := h.wait(
						func() string { return vmConf(e, st, vm) },
						func() vmRuntime { return inspect(vmConf(e, st, vm)) },
						func(f string, a ...any) { vlog(vm.Name, f, a...) })
					if err != nil {
						vlog(vm.Name, "FAILED healthcheck: %v", err)
						mu.Lock()
						fail = true
						mu.Unlock()
						return
					}
					vlog(vm.Name, "healthcheck %s ok", h.Type)
				}
				if rt := inspect(vmConf(e, st, vm)); rt.PID > 0 {
					vlog(vm.Name, "running (pid %d)", rt.PID)
				} else {
					vlog(vm.Name, "started")
				}
			}(vm)
		}
		wg.Wait()
		e.save(st)
		if fail {
			break
		}
	}
	e.save(st)
	if !fail {
		plog("env %s: up", e.Name)
		return 0
	}
	plog("env %s: FAILED", e.Name)
	if rollback {
		plog("env %s: --rollback — stopping %d vm(s)", e.Name, len(started))
		downVMs(e, st, reverseVMs(started), 10*time.Second, true)
		e.save(st)
	}
	return 1
}

func vmNames(l []*VM) string {
	var s []string
	for _, v := range l {
		s = append(s, v.Name)
	}
	return strings.Join(s, " ")
}

func reverseVMs(l []*VM) []*VM {
	out := make([]*VM, len(l))
	for i, v := range l {
		out[len(l)-1-i] = v
	}
	return out
}

// downVMs stops the given VMs in parallel — callers pre-order them
// (reverse level order for `down`, reverse start order for rollback).
func downVMs(e *Env, st *envState, vms []*VM, grace time.Duration, force bool) {
	var wg sync.WaitGroup
	for _, vm := range vms {
		wg.Add(1)
		go func(vm *VM) {
			defer wg.Done()
			if err := stopVM(e, st, vm, grace, force); err != nil {
				vlog(vm.Name, "stop error: %v", err)
			}
		}(vm)
	}
	wg.Wait()
}

// cmdDown implements `quickemu-env down` — reverse level order.
func cmdDown(e *Env, sel []string, grace time.Duration, force bool) int {
	vms, errs := e.selectVMs(sel)
	if len(errs) > 0 {
		return printErrs(e.File, errs)
	}
	st := e.loadState()
	levels := e.order(vms)
	for li := len(levels) - 1; li >= 0; li-- {
		if len(levels[li]) == 0 {
			continue
		}
		plog("== level %d: %s", li, vmNames(levels[li]))
		downVMs(e, st, levels[li], grace, force)
	}
	// Free the env's net reservations once their listeners are down.
	for _, n := range e.Nets {
		ns := st.Nets[n.Name]
		if ns == nil {
			continue
		}
		if selAllDown(e, st, n) {
			e.portsRelease(ns.Port)
			ns.Owner = 0
		}
	}
	e.save(st)
	plog("env %s: down", e.Name)
	return 0
}

// selAllDown reports whether every attached VM of a net is stopped —
// a partial `down vm2` only releases nets whose whole membership is
// down.
func selAllDown(e *Env, st *envState, n *Net) bool {
	for _, name := range n.Attach {
		v := e.vm(name)
		if v == nil {
			continue
		}
		rt := inspect(vmConf(e, st, v))
		if rt.PID > 0 && alive(rt.PID) {
			return false
		}
	}
	return true
}

// cmdStatus renders the per-VM table (or the v1 envelope with
// --format json): vm, state, pid and the resolved IP when known.
func cmdStatus(e *Env, sel []string, jsonOut bool) int {
	vms, errs := e.selectVMs(sel)
	if len(errs) > 0 {
		return printErrs(e.File, errs)
	}
	st := e.loadState()
	type row struct {
		name, state, pid, ip string
	}
	var rows []row
	var data []map[string]any
	for _, vm := range vms {
		conf := vmConf(e, st, vm)
		rt := inspect(conf)
		ip := guestIP(e, conf, rt)
		pid := ""
		if rt.PID > 0 {
			pid = strconv.Itoa(rt.PID)
		}
		rows = append(rows, row{vm.Name, rt.State, pid, ip})
		data = append(data, map[string]any{
			"name": vm.Name, "state": rt.State, "pid": rt.PID,
			"ip": ip, "conf": conf, "deps": vm.Deps,
			"ports": rt.Ports, "nets": vmNets(vm, e, st),
		})
	}
	if jsonOut {
		emitOK("env.status", map[string]any{"env": e.Name, "vms": data})
		return 0
	}
	fmt.Printf("%-20s %-9s %-8s %s\n", "VM", "STATE", "PID", "IP")
	for _, r := range rows {
		fmt.Printf("%-20s %-9s %-8s %s\n", r.name, r.state, r.pid, r.ip)
	}
	return 0
}

// cmdRestart = down + up on the same selection.
func cmdRestart(e *Env, sel []string, grace time.Duration, force, rollback bool) int {
	if rc := cmdDown(e, sel, grace, force); rc != 0 {
		return rc
	}
	return cmdUp(e, sel, rollback)
}

// cmdDestroy stops everything and removes artifacts the env created —
// state.json's created flag is the deletion whitelist: pre-existing
// confs and their data are never touched. Prompts unless --yes.
func cmdDestroy(e *Env, sel []string, yes bool, grace time.Duration, force bool) int {
	st := e.loadState()
	vms, errs := e.selectVMs(sel)
	if len(errs) > 0 {
		return printErrs(e.File, errs)
	}
	var doomed []*VM
	for _, vm := range vms {
		if vs := st.VMs[vm.Name]; vs != nil && vs.Created {
			doomed = append(doomed, vm)
		}
	}
	if !yes {
		fmt.Printf("Destroy env %s? %d created vm(s) will be DELETED (%s). [y/N] ",
			e.Name, len(doomed), vmNames(doomed))
		var line string
		fmt.Scanln(&line)
		if !strings.EqualFold(line, "y") && !strings.EqualFold(line, "yes") {
			fmt.Println("aborted")
			return 1
		}
	}
	if rc := cmdDown(e, sel, grace, force); rc != 0 {
		return rc
	}
	for _, vm := range doomed {
		vs := st.VMs[vm.Name]
		conf := vs.Conf
		if conf == "" {
			continue
		}
		if vs.VMdir != "" {
			vlog(vm.Name, "removing %s", vs.VMdir)
			os.RemoveAll(vs.VMdir)
		}
		vlog(vm.Name, "removing %s", conf)
		os.Remove(conf)
		delete(st.VMs, vm.Name)
	}
	for name, ns := range st.Nets {
		e.portsRelease(ns.Port)
		delete(st.Nets, name)
	}
	e.save(st)
	plog("env %s: destroyed", e.Name)
	return 0
}
