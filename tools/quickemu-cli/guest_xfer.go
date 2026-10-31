// guest_xfer.go: guest push/pull + the power verbs — orchestration
// over the qga client's file transfer and guest-shutdown, plus the
// host-side fallbacks (ACPI via the monitor channel) and force
// escalation.
package main

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/alesanfe/quickemu/tools/quickemu-qga"
)

// ---------- guest push / pull ----------

// actGuestXfer runs one file transfer. push=true reads local→guest,
// false guest→local. The guest path is policy-checked (restricted
// mode), and a pull destination refuses to clobber without --force
// (the write itself is atomic inside the client: temp + rename).
func actGuestXfer(push bool) int {
	x := guestVerbState.xfer
	verb, cmd := "guest pull", "guest.pull"
	if push {
		verb, cmd = "guest push", "guest.push"
	}
	// The guest-side path — push writes it, pull reads it.
	guestPath := x.dst
	if !push {
		guestPath = x.src
	}
	if x.restricted && !qga.SafeGuestPath(guestPath) {
		fmt.Printf(" - %s: %q is a restricted guest path (system/traversal/device); rerun with --unsafe if intended.\n",
			verb, guestPath)
		return 1
	}
	local := x.src
	if !push {
		local = x.dst
	}
	if push {
		if st, err := os.Stat(local); err != nil {
			fmt.Printf(" - %s: local file %s not found.\n", verb, local)
			return 1
		} else if st.IsDir() {
			fmt.Printf(" - %s: %s is a directory — single-file transfers only.\n", verb, local)
			return 1
		} else if st.Size() > x.maxSize {
			fmt.Printf(" - %s: %s exceeds --max-size (%d > %d bytes).\n",
				verb, local, st.Size(), x.maxSize)
			return 1
		}
	} else {
		// Pull refuses to overwrite a real file without --force.
		if _, err := os.Stat(local); err == nil && !x.force {
			fmt.Printf(" - %s: %s exists — use --force to overwrite.\n", verb, local)
			return 1
		}
	}
	cl, err := guestDial(8 * time.Second)
	if err != nil {
		return guestEmitErr(cmd, verb, err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	o := qga.TransferOpts{MaxBytes: x.maxSize, Restricted: x.restricted}
	var res *qga.TransferResult
	if push {
		res, err = cl.Push(ctx, local, guestPath, o)
	} else {
		res, err = cl.Pull(ctx, guestPath, local, o)
	}
	if err != nil {
		return guestEmitErr(cmd, verb, err)
	}
	// Optional hash verification: re-read the remote copy so a
	// silent corruption/short-write surfaces as a mismatch.
	verify := ""
	if x.verify {
		got, verr := cl.VerifyPull(ctx, guestPath, o)
		if verr != nil {
			return guestEmitErr(cmd, verb, verr)
		}
		if got != res.SHA256 {
			fmt.Fprintf(os.Stderr, "WARNING: %s verify mismatch (sent %s, remote %s) — retransferring is advised.\n",
				verb, res.SHA256[:12], got[:12])
			if outFmt == "json" {
				emitFail(cmd, "verify_mismatch", "remote copy hash differs")
			}
			return 1
		}
		verify = res.SHA256
	}
	qeEvent("vm.guest-transfer", fmt.Sprintf(`{"dir":%q,"bytes":%d}`,
		map[bool]string{true: "push", false: "pull"}[push], res.Bytes))
	if outFmt == "json" {
		emitOK(cmd, map[string]any{"vm": vmname(), "bytes": res.Bytes,
			"sha256": res.SHA256, "verified": verify != "",
			"guestPath": guestPath, "localPath": local,
			"truncated": res.Trunc})
		return 0
	}
	fmt.Printf(" - %s: %d bytes -> %s", verb, res.Bytes, guestPath)
	if !push {
		fmt.Printf(" (saved %s)", local)
	}
	if verify != "" {
		fmt.Printf(" [sha256 %s verified]", verify[:12])
	}
	if res.Trunc {
		fmt.Printf(" [truncated at --max-size]")
	}
	fmt.Println()
	return 0
}

// ---------- guest shutdown / reboot / halt ----------

// actGuestPower: QGA first (guest-shutdown is the cleanest path —
// the agent orchestrates the OS shutdown), monitor ACPI as the
// fallback, and --force as the only escalation to hard power off.
// Confirmation is state-driven: the VM must actually leave/keep the
// running state, not just ACK the request.
func actGuestPower(mode qga.ShutdownMode) int {
	pw := guestVerbState.power
	name := map[qga.ShutdownMode]string{
		qga.ShutdownPowerdown: "shutdown", qga.ShutdownHalt: "halt",
		qga.ShutdownReboot: "reboot"}[mode]
	cmd := "guest." + name
	if !vmRunning() {
		fmt.Printf(" - %s is not running.\n", vmname())
		return 1
	}
	// Try the agent first — a missing channel/agent falls straight
	// to the monitor path instead of erroring.
	cl, derr := guestDial(5 * time.Second)
	graceful := false
	if derr == nil {
		ctx, cancel := guestCtx(15 * time.Second)
		err := cl.Shutdown(ctx, mode)
		cancel()
		cl.Close()
		if err == nil {
			graceful = true
		} else {
			fmt.Fprintf(os.Stderr, "WARNING: guest-agent %s failed (%v) — falling back to monitor.\n", name, err)
		}
	}
	if !graceful {
		// ACPI fallback: system_powerdown for shutdown/halt (the OS
		// still gets a clean request), system_reset for reboot.
		switch mode {
		case qga.ShutdownReboot:
			if vmControl("system_reset", qmpCmd("system_reset", nil)) != 0 {
				if pw.force {
					return guestForceStop(name, pw.timeout)
				}
				return guestEmitErr(cmd, "guest "+name,
					fmt.Errorf("no monitor channel and no agent"))
			}
		default:
			if vmControl("system_powerdown", qmpCmd("system_powerdown", nil)) != 0 {
				if pw.force {
					return guestForceStop(name, pw.timeout)
				}
				return guestEmitErr(cmd, "guest "+name,
					fmt.Errorf("agent unavailable and no monitor channel"))
			}
		}
		qeEvent("vm."+name+"-requested", `{"via":"monitor"}`)
	} else {
		qeEvent("vm."+name+"-requested", `{"via":"qga"}`)
	}
	// --no-wait reports the request; a reboot stays running by
	// definition, so waiting would only burn the timeout.
	if pw.noWait || mode == qga.ShutdownReboot {
		fmt.Printf(" - %s (%d) %s requested.\n", vmname(), readPID(), name)
		return 0
	}
	// Wait for the state to actually leave "running" — the request
	// being ACK'd is not the contract, the VM stopping is.
	deadline := time.Now().Add(pw.timeout)
	for time.Now().Before(deadline) {
		st, _ := state()
		if st == "stopped" || st == "stale" || st == "stopping" || !vmPidAlive() {
			if outFmt == "json" {
				emitOK(cmd, map[string]any{"vm": vmname(), "state": "stopped",
					"via": map[bool]string{true: "qga", false: "monitor"}[graceful]})
			} else {
				fmt.Printf(" - %s %s complete.\n", vmname(), name)
			}
			return 0
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Timeout: the request went out but the guest never left running.
	if pw.force {
		return guestForceStop(name, pw.timeout)
	}
	fmt.Printf(" - %s: %s requested but guest still running after %s (use --force to hard-stop).\n",
		vmname(), name, pw.timeout)
	return 1
}

// guestForceStop: the explicit-flag escalation — monitor "quit"
// first (still orderly for QEMU), SIGKILL only as the last resort.
// Only reachable under --force: never a default path.
func guestForceStop(name string, timeout time.Duration) int {
	pid := readPID()
	machineTransition(stStopping, "quickemu-guest-force-"+name, "quickemu-cli")
	if monitorSendCmd("quit") != 0 {
		hmp("quit")
	}
	deadline := time.Now().Add(timeout)
	for alive(pid) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if alive(pid) {
		if p, err := os.FindProcess(pid); err == nil {
			p.Signal(syscall.SIGKILL)
		}
		for i := 0; i < 40 && alive(pid); i++ {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if alive(pid) {
		fmt.Printf(" - %s (%d) did not stop.\n", vmname(), pid)
		return 1
	}
	machineTransition(stStopped, "force-"+name, "quickemu-cli")
	runtimeCleanup()
	fmt.Printf(" - %s (%d) stopped (forced).\n", vmname(), pid)
	return 0
}
