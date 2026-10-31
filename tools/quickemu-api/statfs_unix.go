//go:build !windows

package main

import "syscall"

// statfsAvailKB returns KiB available to an unprivileged user on the
// filesystem holding dir; second return reports the probe succeeded.
func statfsAvailKB(dir string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * st.Bsize / 1024, true
}
