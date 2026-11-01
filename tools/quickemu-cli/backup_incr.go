// backup_incr.go — QE-102: incremental backups through QMP dirty
// bitmaps (block-dirty-bitmap-* + drive-backup), for RUNNING VMs.
// The full-export path (quickemu-vm backup) stays for stopped VMs.
//
//	baseline: block-dirty-bitmap-add qe-incr → drive-backup sync=full
//	next:     drive-backup sync=incremental bitmap=qe-incr
//	after:    block-dirty-bitmap-clear qe-incr (fresh tracking epoch)
//
// Artifacts land in <vmdir>/backups/incr/<utc>.qcow2 with a
// registry.json sidecar recording the chain order.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const incrBitmapName = "qe-incr"

// incrDir/incrRegistry: artifact + chain-metadata locations.
func incrDir() string      { return filepath.Join(vmdir(), "backups", "incr") }
func incrRegistry() string { return filepath.Join(incrDir(), "registry.json") }

type incrRec struct {
	At   string `json:"at"`
	File string `json:"file"`
	Mode string `json:"mode"` // full | incremental
	Size int64  `json:"size"`
}

func incrRegistryRead() []incrRec {
	b, err := os.ReadFile(incrRegistry())
	if err != nil {
		return nil
	}
	var arr []incrRec
	if json.Unmarshal(b, &arr) != nil {
		return nil
	}
	return arr
}

func incrRegistryAppend(r incrRec) {
	arr := append(incrRegistryRead(), r)
	b, _ := json.MarshalIndent(arr, "", "  ")
	tmp := incrRegistry() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		if err := os.Rename(tmp, incrRegistry()); err != nil {
			fmt.Fprintf(os.Stderr, " - WARNING! backup registry update failed: %v\n", err)
		}
	}
}

// incrHasBaseline: any recorded "full" artifact means the bitmap
// already tracks an epoch — later backups go incremental.
func incrHasBaseline() bool {
	for _, r := range incrRegistryRead() {
		if r.Mode == "full" {
			return true
		}
	}
	return false
}

// qmpBlock returns .data.return for block jobs/bitmaps queries.
func qmpBlock(cmd string, args map[string]any) (json.RawMessage, error) {
	out, err := qmpRaw(qmpCmd(cmd, args))
	if err != nil {
		return nil, err
	}
	var env struct {
		Data struct {
			Return json.RawMessage `json:"return"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(out), &env) != nil {
		return nil, fmt.Errorf("bad reply: %s", out)
	}
	return env.Data.Return, nil
}

// incrWaitJob: drive-backup is a block job — poll query-block-jobs
// until it drains (or the deadline passes).
func incrWaitJob(deadline time.Duration) error {
	end := time.Now().Add(deadline)
	for {
		raw, err := qmpBlock("query-block-jobs", nil)
		if err != nil {
			return err
		}
		var jobs []map[string]any
		if raw != nil && json.Unmarshal(raw, &jobs) == nil && len(jobs) == 0 {
			return nil
		}
		if time.Now().After(end) {
			return fmt.Errorf("block job did not finish in %s", deadline)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// actBackupIncr: --backup-incr — baseline full or dirty-incremental.
func actBackupIncr() int {
	fail := func(code, msg string) int {
		if outFmt == "json" {
			emitFail("vm.backup", code, msg)
		} else {
			fmt.Printf(" - backup: %s\n", msg)
		}
		return 1
	}
	if !requireRunningEmit("vm.backup") {
		return 1
	}
	if err := os.MkdirAll(incrDir(), 0o755); err != nil {
		return fail("backup_dir_failed", err.Error())
	}
	// Ensure the tracking bitmap exists BEFORE the backup starts —
	// it must record dirties from the baseline epoch onward.
	raw, err := qmpBlock("block-dirty-bitmap-add", map[string]any{
		"node": "SystemDisk", "name": incrBitmapName})
	if err != nil {
		return fail("bitmap_failed", err.Error())
	}
	if len(raw) > 2 && string(raw) != "{}" {
		// QEMU returns {"error":...} inside return for bad commands.
		var e struct {
			Error struct {
				Desc string `json:"desc"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error.Desc != "" {
			return fail("bitmap_refused", e.Error.Desc)
		}
	}
	mode := "incremental"
	if !incrHasBaseline() {
		mode = "full"
	}
	target := filepath.Join(incrDir(),
		time.Now().UTC().Format("20060102T150405Z")+".qcow2")
	args := map[string]any{
		"device": "SystemDisk", "target": target,
		"format": "qcow2", "sync": mode,
	}
	if mode == "incremental" {
		args["bitmap"] = incrBitmapName
	}
	if _, err := qmpBlock("drive-backup", args); err != nil {
		return fail("backup_failed", err.Error())
	}
	if err := incrWaitJob(5 * time.Minute); err != nil {
		return fail("job_failed", err.Error())
	}
	// Backup landed — clear the bitmap so the next epoch starts
	// clean. A clear failure keeps tracking (safe: next backup just
	// copies more blocks than strictly needed).
	qmpBlock("block-dirty-bitmap-clear", map[string]any{
		"node": "SystemDisk", "name": incrBitmapName})
	var sz int64
	if st, err := os.Stat(target); err == nil {
		sz = st.Size()
	}
	incrRegistryAppend(incrRec{
		At:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		File: target, Mode: mode, Size: sz})
	qeEvent("vm.backup", fmt.Sprintf(`{"file":%q,"mode":%q}`, target, mode))
	if outFmt == "json" {
		emitOK("vm.backup", map[string]any{
			"file": target, "mode": mode, "size": sz})
	} else {
		fmt.Printf(" - %s backup written to %s\n", mode, target)
	}
	return 0
}

// actBackupIncrList: --backup-incr-list — the recorded chain.
func actBackupIncrList() int {
	arr := incrRegistryRead()
	if outFmt == "json" {
		emitOK("vm.backup.list", map[string]any{"increments": arr})
		return 0
	}
	fmt.Println("Incremental backups in " + incrDir() + ":")
	for _, r := range arr {
		fmt.Printf(" - %s  %-11s %8d B  %s\n", r.At, r.Mode, r.Size, filepath.Base(r.File))
	}
	if len(arr) == 0 {
		fmt.Println(" - (none yet)")
	}
	return 0
}
