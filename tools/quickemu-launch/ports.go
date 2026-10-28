// ports.go: the Bash port-allocation subsystem in Go —
// get_port + _port_registry_* + ports_record. Registry layout is
// identical to lifecycle.sh so Bash and Go instances dedup ports
// against each other.
package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// portRegistry: ${QE_PORT_REGISTRY:-$(qe_runtime_dir)/ports} —
// ${TMPDIR:-/tmp}/quickemu-$UID/ports.
func portRegistry() string {
	if v := os.Getenv("QE_PORT_REGISTRY"); v != "" {
		return v
	}
	return filepath.Join(os.TempDir(),
		fmt.Sprintf("quickemu-%d", os.Getuid()), "ports")
}

// portRegistryLock: mkdir spin-lock with dead-owner reclaim — 25
// tries — 200ms like _port_registry_lock.
func portRegistryLock() bool {
	reg := portRegistry()
	lock := reg + ".lock"
	os.MkdirAll(reg, 0755)
	for tries := 0; tries < 25; tries++ {
		if err := os.Mkdir(lock, 0755); err == nil {
			if os.WriteFile(filepath.Join(lock, "pid"),
				[]byte(fmt.Sprint(os.Getpid())), 0644) != nil {
				os.RemoveAll(lock) // no anonymous locks — they are
				continue           // reclaimable by the next acquirer
			}
			return true
		}
		if b, err := os.ReadFile(filepath.Join(lock, "pid")); err == nil {
			pid := 0
			fmt.Sscanf(string(b), "%d", &pid)
			// EPERM-aware: a foreign user's lock is alive, never stolen.
			if pid > 0 && !qelib.Alive(pid) {
				os.RemoveAll(lock) // stale lock owner — reclaim
				continue
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func portRegistryUnlock() { os.RemoveAll(portRegistry() + ".lock") }

// portRegistryTaken: reservation file exists AND owner alive; stale
// reservations are removed, matching _port_registry_taken.
func portRegistryTaken(port int) bool {
	f := filepath.Join(portRegistry(), fmt.Sprint(port))
	b, err := os.ReadFile(f)
	if err != nil {
		return false
	}
	pid := 0
	fmt.Sscanf(string(b), "%d", &pid)
	if pid > 0 && qelib.Alive(pid) {
		return true
	}
	os.Remove(f)
	return false
}

func portRegistryReserve(port, vmPid int, instance, vmname string) {
	reg := portRegistry()
	os.MkdirAll(reg, 0755)
	os.WriteFile(filepath.Join(reg, fmt.Sprint(port)),
		[]byte(fmt.Sprintf("%d %d %s %s\n", vmPid, time.Now().Unix(), instance, vmname)), 0644)
}

// portRegistryRelease: drop every reservation owned by pid —
// _port_registry_release parity (dead VMs hold no ports hostage).
func portRegistryRelease(owner int) {
	reg := portRegistry()
	entries, err := os.ReadDir(reg)
	if err != nil {
		return
	}
	for _, e := range entries {
		f := filepath.Join(reg, e.Name())
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		pid := 0
		fmt.Sscanf(string(b), "%d", &pid)
		if pid == owner {
			os.Remove(f)
		}
	}
}

// getPort: get_port(start,range) parity — lock, skip taken, probe a
// TCP connect to 127.0.0.1 (refused = free), reserve, unlock.
// Returns 0 when the range is exhausted (caller prints the Bash
// PORT_BUSY message).
func getPort(start, rng, vmPid int, vmname string) int {
	if !portRegistryLock() {
		return -1 // LOCK_TIMEOUT
	}
	defer portRegistryUnlock()
	for p := start; p <= start+rng; p++ {
		if portRegistryTaken(p) {
			continue
		}
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(p),
			300*time.Millisecond)
		if err == nil {
			c.Close()
			continue
		}
		portRegistryReserve(p, vmPid, "", vmname)
		return p
	}
	return 0 // PORT_BUSY
}

// portsRecord: append "name,port,host" to <vmdir>/<vm>.ports in run
// mode only — ports_record parity (skipped in plan mode).
func portsRecord(dir, vmname, rec string, runMode bool) {
	if !runMode {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, vmname+".ports"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, rec)
}

// spiceRecord: write the SPICE connect target (unix path or TCP port)
// to <vmdir>/<vm>.spice — spice_record parity; start_viewer reads it.
func spiceRecord(dir, vmname, val string, runMode bool) {
	if !runMode {
		return
	}
	os.WriteFile(filepath.Join(dir, vmname+".spice"), []byte(val+"\n"), 0644)
}
