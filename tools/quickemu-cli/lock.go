// lock.go: VM locking — the legacy mkdir-exclusive lock extended to a
// shared/exclusive pair (QE-002).
//
// Exclusive writers hold <vmdir>/<vm>.lock (mkdir-atomic) with a bare
// "pid" file for legacy readers plus an "owner" file carrying the
// fingerprinted record "pid start-time op". Shared readers hold
// <vmdir>/<vm>.lock.shared/shared.<pid> files with the same record.
// Mutual exclusion is a two-phase check: the shared side creates its
// file then verifies no live exclusive dir; the exclusive side mkdirs
// then sweeps for live shared files — whichever sees the other backs
// off and retries, so neither can slip through the other's gap.
// Orphan reclaim verifies the fingerprint (start-time), not just
// liveness — a recycled pid can never hold a lock (QE-001).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func lockDir() string       { return filepath.Join(vmdir(), vmname()+".lock") }
func lockSharedDir() string { return filepath.Join(vmdir(), vmname()+".lock.shared") }

// exclDepth: nested exclusive holds by this process — a mutating
// sub-op dispatched under an already-held act lock must deepen
// instead of deadlocking on its own claim (same pid + start-time
// fingerprint in the owner record).
var exclDepth int

// lockMeta: "<pid> <start-time> <op>" — start-time is the fingerprint
// bit that survives PID recycling; op feeds the holder report.
func lockMeta(op string) string {
	st := procStartTimeJ(os.Getpid())
	if st == "" {
		st = "-"
	}
	return fmt.Sprintf("%d %s %s", os.Getpid(), st, op)
}

// lockHolder splits a lock-owner record into pid/op/liveness. The
// legacy "pid"-only layout yields no op and skips the fingerprint
// check (nothing recorded to check against).
func lockHolder(meta string) (pid int, op string, live bool) {
	f := strings.Fields(meta)
	if len(f) == 0 {
		return 0, "", false
	}
	pid, _ = strconv.Atoi(f[0])
	if pid <= 0 {
		return pid, "", false
	}
	if len(f) > 2 {
		op = f[2]
	}
	if !alive(pid) {
		return pid, op, false
	}
	// A recorded start-time that no longer matches means the pid was
	// recycled — the lock is stale even though kill(0) answers.
	if len(f) > 1 && f[1] != "" && f[1] != "-" {
		if cur := procStartTimeJ(pid); cur != "" && cur != f[1] {
			return pid, op, false
		}
	}
	return pid, op, true
}

// lockTries: attempt count for one acquire. QE_LOCK_TIMEOUT (seconds,
// fractional ok) maps onto the fixed 200ms poll; VM_LOCK_TRIES stays
// the Bash-compatible override (default 1 for writers like Bash's
// VM_LOCK_TRIES=1, 5 for readers — a beat of patience for reads).
func lockTries(def int) int {
	if v := os.Getenv("QE_LOCK_TIMEOUT"); v != "" {
		if sec, err := strconv.ParseFloat(v, 64); err == nil && sec > 0 {
			if n := int(sec*5 + 0.5); n > 0 {
				return n
			}
			return 1
		}
	}
	if t := os.Getenv("VM_LOCK_TRIES"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// exclusiveOwner reads the exclusive lock's owner: the fingerprinted
// "owner" file when present, the bare "pid" file otherwise.
func exclusiveOwner(hd string) (pid int, op string, live bool) {
	if b, err := os.ReadFile(filepath.Join(hd, "owner")); err == nil {
		if p, o, l := lockHolder(string(b)); p > 0 {
			return p, o, l
		}
	}
	if b, err := os.ReadFile(filepath.Join(hd, "pid")); err == nil {
		return lockHolder(string(b))
	}
	return 0, "", false
}

// exclusiveReclaimable: the legacy staleness rules extended with the
// fingerprint — dead owner, fingerprint-mismatched (recycled) owner,
// or corrupt/pid-less content older than the 2s creation window.
func exclusiveReclaimable(hd string) bool {
	pid, _, live := exclusiveOwner(hd)
	if pid > 0 {
		return !live
	}
	st, err := os.Stat(hd)
	return err == nil && time.Since(st.ModTime()) > 2*time.Second
}

// sharedHolders scans <vm>.lock.shared for shared.<pid> claims and
// returns the verified-live owners (pid→op). With sweep set, dead,
// fingerprint-mismatched, forged (name/content pid mismatch) and
// over-age corrupt files are unlinked — fresh corrupt ones stay, they
// may be a writer mid-create.
func sharedHolders(sweep bool) map[int]string {
	holders := map[int]string{}
	ents, _ := os.ReadDir(lockSharedDir())
	for _, e := range ents {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "shared.") {
			continue
		}
		p := filepath.Join(lockSharedDir(), e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		pid, op, live := lockHolder(string(b))
		nameOK := e.Name() == fmt.Sprintf("shared.%d", pid)
		switch {
		case live && nameOK:
			holders[pid] = op
		case pid > 0 && !live:
			if sweep {
				os.Remove(p)
			}
		default:
			// Unparseable or forged — sweep only once past the
			// creation window (a torn write can't be mid-flight then).
			if sweep {
				if st, serr := os.Stat(p); serr != nil || time.Since(st.ModTime()) > 2*time.Second {
					os.Remove(p)
				}
			}
		}
	}
	return holders
}

// lockFail prints the locked error with the holder report appended —
// same first line as Bash, the "held by" detail after it.
func lockFail(holder string) {
	msg := vmname() + " is locked by another operation."
	if holder != "" {
		msg += " Held by " + holder + "."
	}
	if outFmt == "json" {
		emitFail("vm.lock", "VM_LOCKED", msg)
	} else {
		fmt.Fprintf(os.Stderr, "ERROR! %s\n", msg)
	}
}

// exclusiveHolder describes the current exclusive owner for errors.
func exclusiveHolder(hd string) string {
	if pid, op, _ := exclusiveOwner(hd); pid > 0 {
		if op != "" {
			return fmt.Sprintf("%s (pid %d)", op, pid)
		}
		return fmt.Sprintf("pid %d", pid)
	}
	return ""
}

// sharedHolderDesc lists live shared claimants for the error report.
func sharedHolderDesc() string {
	parts := []string{}
	for pid, op := range sharedHolders(false) {
		if op != "" {
			parts = append(parts, fmt.Sprintf("%s (pid %d)", op, pid))
		} else {
			parts = append(parts, fmt.Sprintf("pid %d", pid))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// vmLockAcquire/vmLockRelease: vm_lock_* parity — exclusive lock at
// <vmdir>/<vm>.lock with a pid file for stale-owner reclaim. Bash
// defaults VM_LOCK_TRIES=1: one attempt, then LOCK_BUSY.
func vmLockAcquire() bool { return vmLockAcquireOp("") }

// vmLockAcquireOp is vmLockAcquire with the operation name recorded —
// the "held by" detail in lock errors.
func vmLockAcquireOp(op string) bool {
	if err := os.MkdirAll(vmdir(), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! Cannot create VM dir %s: %v\n", vmdir(), err)
		return false
	}
	hd := lockDir()
	// Re-entrant: the verified owner is this very process (same pid +
	// start-time fingerprint — a foreign pid can never match both) —
	// deepen the hold instead of deadlocking on our own claim.
	if pid, _, live := exclusiveOwner(hd); pid == os.Getpid() && live {
		exclDepth++
		return true
	}
	tries := lockTries(1)
	writeOwner := func() bool {
		// A lock without a readable owner is reclaimable by the next
		// acquirer — on write failure drop the lock rather than hold
		// an anonymous one.
		if os.WriteFile(filepath.Join(hd, "pid"),
			[]byte(strconv.Itoa(os.Getpid())), 0644) != nil {
			os.RemoveAll(hd)
			return false
		}
		// The fingerprinted owner record rides alongside the bare pid
		// file legacy readers consume.
		if os.WriteFile(filepath.Join(hd, "owner"),
			[]byte(lockMeta(op)), 0644) != nil {
			os.RemoveAll(hd)
			return false
		}
		return true
	}
	reclaim := func() bool {
		os.RemoveAll(hd)
		return os.Mkdir(hd, 0755) == nil && writeOwner()
	}
	for i := 0; i < tries; i++ {
		acquired := false
		if err := os.Mkdir(hd, 0755); err == nil {
			acquired = writeOwner()
		} else if exclusiveReclaimable(hd) {
			// Dead/reused/aged owner — safe to take over (crash-safe,
			// like Bash's kill -0 check), then retry the mkdir inline:
			// a reclaimed lock is ours to take, not a failed attempt.
			acquired = reclaim()
		}
		if acquired {
			// Phase two: a shared reader may have landed its file
			// between our mkdir and now — that reader wins; drop the
			// exclusive claim and retry.
			if len(sharedHolders(true)) == 0 {
				exclDepth = 0
				return true
			}
			os.RemoveAll(hd)
		}
		if i+1 < tries {
			time.Sleep(200 * time.Millisecond)
		}
	}
	lockFail(exclusiveHolder(hd))
	return false
}

func vmLockRelease() {
	if exclDepth > 0 {
		exclDepth--
		return
	}
	os.RemoveAll(lockDir())
}

// vmLockAcquireShared: shared read lock — a shared.<pid> file inside
// <vm>.lock.shared, refused only while a verified-live exclusive
// owner holds <vm>.lock. Dead or recycled claims are reclaimed.
func vmLockAcquireShared(op string) bool {
	if err := os.MkdirAll(vmdir(), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! Cannot create VM dir %s: %v\n", vmdir(), err)
		return false
	}
	sd := lockSharedDir()
	if err := os.MkdirAll(sd, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! Cannot create shared lock dir %s: %v\n", sd, err)
		return false
	}
	mine := filepath.Join(sd, fmt.Sprintf("shared.%d", os.Getpid()))
	tries := lockTries(5)
	for i := 0; i < tries; i++ {
		sharedHolders(true) // sweep dead/reused/forged shared claims
		if f, err := os.OpenFile(mine, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644); err == nil {
			fmt.Fprint(f, lockMeta(op))
			f.Close()
		} else if b, rerr := os.ReadFile(mine); rerr == nil {
			if p, _, _ := lockHolder(string(b)); p == os.Getpid() {
				// Nested acquire by this same process — refresh the
				// record (idempotent), no second claim needed.
				os.WriteFile(mine, []byte(lockMeta(op)), 0644)
			} else {
				// shared.<ourpid> claiming another owner: forged —
				// sweep it and retry rather than trust its content.
				os.Remove(mine)
				if i+1 < tries {
					time.Sleep(200 * time.Millisecond)
				}
				continue
			}
		} else {
			// Created-but-unreadable slot — can't verify, back off.
			if i+1 < tries {
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}
		// Phase two: no verified-live exclusive owner may hold the
		// dir — dead or aged remnants are reclaimed here, live ones
		// force us to drop our file and retry.
		hd := lockDir()
		if _, err := os.Stat(hd); err == nil {
			selfHeld := false
			if ep, _, live := exclusiveOwner(hd); live {
				if ep == os.Getpid() {
					// Our own exclusive hold already serializes us —
					// a shared claim underneath it is always
					// consistent.
					selfHeld = true
				} else {
					os.Remove(mine)
				}
			} else if exclusiveReclaimable(hd) && os.RemoveAll(hd) == nil {
				// Orphaned exclusive remnant reclaimed — fall through.
			} else {
				// Fresh exclusive in mid-creation — wait a beat.
				os.Remove(mine)
			}
			if !selfHeld {
				if _, err := os.Stat(hd); err == nil {
					if i+1 < tries {
						time.Sleep(200 * time.Millisecond)
					}
					continue
				}
			}
		}
		return true
	}
	lockFail(exclusiveHolder(lockDir()))
	return false
}

func vmLockReleaseShared() {
	sd := lockSharedDir()
	os.Remove(filepath.Join(sd, fmt.Sprintf("shared.%d", os.Getpid())))
	os.Remove(sd) // rmdir — only succeeds once the last reader is gone
}
