package main

import (
	"os"

	"github.com/alesanfe/quickemu/tools/qelib"
)

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
