package main

import (
	"fmt"
	"net"
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

// A live lock owner can never be stolen: the second acquirer retries
// (~5s) then reports the lock timeout, matching get_port's -1.
func TestRegistryLockTimeout(t *testing.T) {
	regEnv(t)
	if !portRegistryLock() {
		t.Fatal("first lock failed")
	}
	defer portRegistryUnlock()
	if portRegistryLock() {
		portRegistryUnlock()
		t.Fatal("live lock must not be stolen")
	}
	if p := getPort(5900, 3, os.Getpid(), "vm"); p != -1 {
		t.Fatalf("getPort under held lock: %d, want -1", p)
	}
}

// get_port reservations are global: a port taken via getPort must be
// skipped by firstFreePort's scan, and a fully-reserved range reads
// as exhausted.
func TestFirstFreePortSkipsRegistryTaken(t *testing.T) {
	regEnv(t)
	oldListen := netListen
	netListen = func(_, _ string) (net.Listener, error) {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	defer func() { netListen = oldListen }()
	me := os.Getpid()
	portRegistryLock()
	portRegistryReserve(22220, me, "i", "other")
	portRegistryUnlock()
	if p := firstFreePort("", 22220, 22229, "vm"); p != "22221" {
		t.Fatalf("firstFreePort must skip registry-taken port: %q", p)
	}
	portRegistryRelease(me)
}
