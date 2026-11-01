// suspend.go: QE-020 — suspend-to-disk via QMP `migrate exec:` and
// resume via `-incoming exec:`. The FSM owns the arcs:
//
//	running|paused → saving → suspended → starting → running
//
// The image is gzip-compressed vmstate in the VM dir; a sidecar
// .meta records the guest/arch it was saved under so resume refuses
// an incompatible conf instead of booting a corrupt guest.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// suspendImage / suspendMeta: the state image name suspendImageExists
// probes for, plus the compatibility sidecar.
func suspendImage() string { return filepath.Join(vmdir(), vmname()+".state.gz") }
func suspendMeta() string  { return suspendImage() + ".meta" }

// suspendMetaRec: resume-compatibility facts recorded at save time —
// a conf pointing at a different guest/arch rejects the resume.
type suspendMetaRec struct {
	GuestOS string `json:"guest_os"`
	Arch    string `json:"arch"`
	Cores   string `json:"cpu_cores,omitempty"`
	SavedAt string `json:"saved_at"`
	Version string `json:"quickemu_version"`
}

// suspendTimeout: migrate can move GBs of RAM — bound by
// QE_SUSPEND_TIMEOUT (seconds), default 10 minutes.
func suspendTimeout() time.Duration {
	d := 600.0
	if v := os.Getenv("QE_SUSPEND_TIMEOUT"); v != "" {
		fmt.Sscan(v, &d)
	}
	return time.Duration(d * float64(time.Second))
}

// shellQ: single-quote a path for the migrate exec: URI — QEMU runs
// `exec:<cmd>` through /bin/sh, and a VM dir with spaces or quotes
// must not split the redirect target.
func shellQ(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// migrateStatus: query-migrate → the status word ("active",
// "completed", "failed", "cancelled", ...). "" on channel failure.
func migrateStatus() (status string, pct float64) {
	out, err := qmp("query-migrate")
	if err != nil {
		return "", 0
	}
	var rep struct {
		Data struct {
			Status string `json:"status"`
			RAM    struct {
				Total       float64 `json:"total"`
				Transferred float64 `json:"transferred"`
			} `json:"ram"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(out), &rep) != nil {
		return "", 0
	}
	if rep.Data.RAM.Total > 0 {
		pct = rep.Data.RAM.Transferred / rep.Data.RAM.Total * 100
	}
	return rep.Data.Status, pct
}

// actSuspend: vm_suspend — pause the machine, stream vmstate to the
// gzipped image, then quit. A failed/cancelled migrate returns the
// VM to its prior state; the partial image is removed.
func actSuspend() int {
	fail := func(code, msg string) int {
		machineTransition(stRunning, "suspend-failed", "quickemu-cli")
		if outFmt == "json" {
			emitFail("vm.suspend", code, msg)
		} else {
			fmt.Printf(" - suspend: %s\n", msg)
		}
		return 1
	}
	if !requireRunningEmit("vm.suspend") {
		return 1
	}
	if _, err := os.Stat(qmpSock()); err != nil {
		return fail("no_qmp", "no QMP channel — the VM must run with monitor=socket.")
	}
	if !machineTransition(stSaving, "suspend", "quickemu-cli") {
		return fail("bad_state", "cannot suspend from the current state.")
	}
	// Freeze vcpus before migrating so the state image is consistent —
	// QEMU accepts migrate on a running VM but the saved stream is
	// cleaner (and faster to settle) with the guest stopped.
	st, _ := state()
	wasPaused := st == "paused"
	if !wasPaused {
		vmControl("stop", qmpCmd("stop", nil))
	}
	img, meta := suspendImage(), suspendMeta()
	uri := "exec:gzip -c > " + shellQ(img)
	if out, err := qmpRaw(qmpCmd("migrate", map[string]any{"uri": uri})); err != nil ||
		strings.Contains(out, `"error"`) {
		return fail("migrate_refused", "migrate request refused — "+strings.TrimSpace(out))
	}
	if outFmt != "json" {
		fmt.Printf(" - suspend: saving state to %s ...\n", img)
	}
	deadline := time.Now().Add(suspendTimeout())
	lastPct := -1
	for {
		st, pct := migrateStatus()
		switch st {
		case "completed":
			goto saved
		case "failed", "cancelled":
			os.Remove(img)
			if !wasPaused {
				vmControl("cont", qmpCmd("cont", nil))
			}
			return fail("migration_"+st, "migration "+st)
		case "":
			// channel hiccup — keep polling until the deadline
		}
		if int(pct) > lastPct && pct > 0 && outFmt != "json" {
			fmt.Printf("\r - suspend: %d%%", int(pct))
			lastPct = int(pct)
		}
		if time.Now().After(deadline) {
			qmpRaw(qmpCmd("migrate_cancel", nil))
			if !wasPaused {
				vmControl("cont", qmpCmd("cont", nil))
			}
			return fail("timeout", "timed out — migrate_cancel sent")
		}
		time.Sleep(500 * time.Millisecond)
	}
saved:
	// Stop QEMU so the state file is the only live copy; the pidfile
	// disappears with the process and resume cold-boots from the
	// image. A graceful quit is still orderly at this point.
	pid := readPID()
	qmpRaw(qmpCmd("quit", nil))
	for i := 0; i < 100 && alive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if alive(pid) {
		os.Remove(img)
		return fail("quit_failed", fmt.Sprintf("qemu pid %d did not exit after migrate", pid))
	}
	rec := suspendMetaRec{
		GuestOS: confKV()["guest_os"], Arch: confKV()["arch"],
		Cores:   confKV()["cpu_cores"],
		SavedAt: time.Now().UTC().Format(time.RFC3339),
		Version: qeVersion,
	}
	if b, err := json.Marshal(rec); err == nil {
		if err := os.WriteFile(meta, b, 0600); err != nil {
			fmt.Fprintf(os.Stderr, " - WARNING! suspend metadata not written: %v\n", err)
		}
	}
	// The record lands BEFORE cleanup: reconcile reads stale+suspended
	// + image as the expected end of a suspend, never a crash.
	machineTransition(stSuspended, "suspend-complete", "quickemu-cli")
	runtimeCleanup()
	qeEvent("vm.suspended", fmt.Sprintf(`{"image":%q}`, img))
	if outFmt == "json" {
		emitOK("vm.suspend", map[string]any{"vm": vmname(), "image": img})
	} else {
		fmt.Printf("\n - %s suspended (state saved to %s).\n", vmname(), img)
	}
	return 0
}

// suspendedState: FSM says suspended AND the image survives — the
// only combination resume may act on.
func suspendedState() bool {
	r := machineState()
	return r != nil && r.State == stSuspended && suspendImageExists()
}

// resumeCompatible: the saved guest/arch must match the conf — a
// mismatched resume boots garbage. Missing meta refuses too (an
// unlabelled image is an unknown compatibility claim).
func resumeCompatible() error {
	b, err := os.ReadFile(suspendMeta())
	if err != nil {
		return fmt.Errorf("no suspend metadata — cannot verify compatibility")
	}
	var m suspendMetaRec
	if json.Unmarshal(b, &m) != nil {
		return fmt.Errorf("corrupt suspend metadata")
	}
	if g := confKV()["guest_os"]; m.GuestOS != "" && g != "" && m.GuestOS != g {
		return fmt.Errorf("saved guest_os %q != conf %q", m.GuestOS, g)
	}
	if a := confKV()["arch"]; m.Arch != "" && a != "" && m.Arch != a {
		return fmt.Errorf("saved arch %q != conf %q", m.Arch, a)
	}
	return nil
}

// actResumeSuspended: resume_vm's suspended branch — cold-boot QEMU
// with -incoming reading the saved image. confOpts["resume_from"]
// rides the normal applyOverrides channel into the launch conf.
func actResumeSuspended() int {
	fail := func(code, msg string) int {
		if outFmt == "json" {
			emitFail("vm.resume", code, msg)
		} else {
			fmt.Println(" - resume: " + msg)
		}
		return 1
	}
	if err := resumeCompatible(); err != nil {
		return fail("incompatible", fmt.Sprintf("%v — fix the conf or delete %s.", err, suspendImage()))
	}
	img := suspendImage()
	if !machineTransition(stStarting, "resume-suspend", "quickemu-cli") {
		return fail("bad_state", "cannot resume from the current state.")
	}
	confOpts["resume_from"] = img
	out, err := tool("quickemu-launch", applyOverrides(), "--run")
	if outFmt != "json" {
		fmt.Print(out)
	}
	if err != nil {
		machineTransition(stSuspended, "resume-failed", "quickemu-cli")
		return fail("launch_failed", strings.TrimSpace(out))
	}
	machineTransition(stRunning, "resumed", "quickemu-cli")
	qeEvent("vm.resumed", `{"from":"suspend"}`)
	// The image is single-use: a stale copy resumed over a modified
	// disk is silent corruption (QE-020 modified-device guard).
	os.Remove(img)
	os.Remove(suspendMeta())
	if outFmt == "json" {
		emitOK("vm.resume", map[string]any{"vm": vmname(), "from": "suspend"})
	} else {
		fmt.Printf(" - %s resumed from suspend.\n", vmname())
	}
	return 0
}
