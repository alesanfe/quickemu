//go:build !windows

package main

import "syscall"

// statfsFree returns free bytes available to an unprivileged user on
// the filesystem holding dir.
func statfsFree(dir string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * st.Bsize, nil
}
