package qelib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsWSL(t *testing.T) {
	old := ProcOSRelease
	defer func() { ProcOSRelease = old }()
	dir := t.TempDir()

	mk := func(content string) {
		p := filepath.Join(dir, "osrelease")
		os.WriteFile(p, []byte(content), 0o644)
		ProcOSRelease = p
	}
	// Env marker wins regardless of procfs fixture.
	t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
	ProcOSRelease = filepath.Join(dir, "missing")
	if !IsWSL() {
		t.Fatal("WSL_DISTRO_NAME should detect WSL")
	}
	t.Setenv("WSL_DISTRO_NAME", "")
	t.Setenv("WSL_INTEROP", "/run/WSL")
	if !IsWSL() {
		t.Fatal("WSL_INTEROP should detect WSL")
	}
	os.Unsetenv("WSL_INTEROP")
	mk("6.18.54.1-microsoft-standard-WSL2")
	if !IsWSL() {
		t.Fatal("microsoft osrelease should detect WSL")
	}
	mk("6.9.3-arch1-1")
	if IsWSL() {
		t.Fatal("vanilla kernel must not detect WSL")
	}
	ProcOSRelease = filepath.Join(dir, "gone")
	if IsWSL() {
		t.Fatal("unreadable procfs must not detect WSL")
	}
}
