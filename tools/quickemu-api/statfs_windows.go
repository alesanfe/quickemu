//go:build windows

package main

// statfsAvailKB has no syscall.Statfs on Windows; reporting
// "unknown" skips the free-space stat on a platform quickemu does not
// target.
func statfsAvailKB(dir string) (int64, bool) { return 0, false }
