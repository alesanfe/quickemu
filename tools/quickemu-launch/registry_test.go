package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func regEnv(t *testing.T) string {
	t.Helper()
	reg := filepath.Join(t.TempDir(), "ports")
	t.Setenv("QE_PORT_REGISTRY", reg)
	return reg
}

func TestRegistryReserveTakenRelease(t *testing.T) {
	reg := regEnv(t)
	me := os.Getpid()

	portRegistryReserve(55900, me, "i1", "vm-a")
	if !portRegistryTaken(55900) {
		t.Fatal("reserved port must be taken")
	}
	b, _ := os.ReadFile(filepath.Join(reg, "55900"))
	var pid int
	fmt.Sscanf(string(b), "%d", &pid)
	if pid != me {
		t.Fatalf("record pid=%d want %d", pid, me)
	}

	portRegistryRelease(me)
	if portRegistryTaken(55900) {
		t.Fatal("released port must be free")
	}
}

// A record owned by a dead pid is treated as free AND removed —
// stale leases must not hold ports hostage.
func TestRegistryReclaimsDeadOwner(t *testing.T) {
	reg := regEnv(t)
	os.MkdirAll(reg, 0o755)
	os.WriteFile(filepath.Join(reg, "55901"),
		[]byte("1073741824 1 i vm-b\n"), 0o644)
	if portRegistryTaken(55901) {
		t.Fatal("dead owner must not hold the port")
	}
	if _, err := os.Stat(filepath.Join(reg, "55901")); !os.IsNotExist(err) {
		t.Fatal("stale record must be removed")
	}
}

// Garbage in the record (no parseable pid) is the same as dead.
func TestRegistryGarbageRecordReclaimed(t *testing.T) {
	reg := regEnv(t)
	os.MkdirAll(reg, 0o755)
	os.WriteFile(filepath.Join(reg, "55902"), []byte("junk\n"), 0o644)
	if portRegistryTaken(55902) {
		t.Fatal("garbage record must not hold the port")
	}
}

// Release only drops records owned by the pid — records of other
// owners are left for Taken() to reclaim.
func TestRegistryReleaseOwnerScoped(t *testing.T) {
	regEnv(t)
	me := os.Getpid()
	portRegistryReserve(55903, me, "i1", "a")
	reg := portRegistry()
	os.WriteFile(filepath.Join(reg, "55904"),
		[]byte("1073741824 1 i b\n"), 0o644)
	portRegistryRelease(me)
	if portRegistryTaken(55903) {
		t.Fatal("own reservation must be released")
	}
	if _, err := os.Stat(filepath.Join(reg, "55904")); err != nil {
		t.Fatal("foreign record must survive release")
	}
}
