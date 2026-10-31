//go:build windows

package main

// statfsFree has no syscall.Statfs on Windows; reporting "huge"
// skips the free-space gate on a platform quickemu does not target.
func statfsFree(dir string) (int64, error) { return 1 << 62, nil }
