// ports.go: the Bash port-allocation subsystem in Go —
// get_port + _port_registry_* + ports_record. Registry layout is
// identical to lifecycle.sh so Bash and Go instances dedup ports
// against each other.
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// registryLock: the _port_registry_lock mkdir spin-lock generalized
// over the registry directory — the vsock CID registry uses the same
// protocol (25 tries — 200ms, dead-owner reclaim) against its own dir.
func registryLock(reg string) bool {
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

func registryUnlock(reg string) { os.RemoveAll(reg + ".lock") }

func portRegistryLock() bool { return registryLock(portRegistry()) }
func portRegistryUnlock()    { registryUnlock(portRegistry()) }

// registryTaken: reservation file exists AND owner alive; stale
// reservations are removed, matching _port_registry_taken.
func registryTaken(reg string, key int) bool {
	f := filepath.Join(reg, fmt.Sprint(key))
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

func portRegistryTaken(port int) bool { return registryTaken(portRegistry(), port) }

// registryReserve records key -> "pid ts instance vm" in reg.
func registryReserve(reg string, key, vmPid int, instance, vmname string) {
	if err := os.MkdirAll(reg, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "registry:", err)
		return
	}
	// An unrecorded reservation is a double-allocation waiting to
	// happen — never report success silently.
	if err := os.WriteFile(filepath.Join(reg, fmt.Sprint(key)),
		[]byte(fmt.Sprintf("%d %d %s %s\n", vmPid, time.Now().Unix(), instance, vmname)), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "registry: cannot record "+fmt.Sprint(key)+":", err)
	}
}

func portRegistryReserve(port, vmPid int, instance, vmname string) {
	registryReserve(portRegistry(), port, vmPid, instance, vmname)
}

// registryRelease: drop every reservation owned by pid —
// _port_registry_release parity (dead VMs hold no ports hostage).
func registryRelease(reg string, owner int) {
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

func portRegistryRelease(owner int) { registryRelease(portRegistry(), owner) }

// getPort: get_port(start,range) parity — lock, skip taken, probe a
// TCP connect to 127.0.0.1 (refused = free), reserve, unlock.
// Returns 0 when the range is exhausted (caller prints the Bash
// PORT_BUSY message).
func getPort(start, rng, vmPid int, vmname string) int {
	if !portRegistryLock() {
		fmt.Fprintln(os.Stderr, "ERROR! Could not acquire the port allocation lock.")
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
	fmt.Fprintf(os.Stderr, "ERROR! No free host port in range %d-%d.\n",
		start, start+rng)
	return 0 // PORT_BUSY
}

// portsRecord: append "name,port,host" to <vmdir>/<vm>.ports in run
// mode only — ports_record parity (skipped in plan mode).
func portsRecord(dir, vmname, rec string, runMode bool) {
	if !runMode {
		return
	}
	// .ports is parsed line-by-line by other tools — a record carrying
	// a newline injects a fake entry (e.g. "ssh,22" from a hostile
	// monitor_telnet_host).
	if strings.ContainsAny(rec, "\r\n") {
		fmt.Fprintf(os.Stderr, "WARNING: refusing malformed ports record %q\n", rec)
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, vmname+".ports"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not record port assignment: %v\n", err)
		return
	}
	// A dropped record means the next VM can be handed the same port —
	// the write failing is worth a warning, like mv's stderr in Bash.
	if _, err := fmt.Fprintln(f, rec); err != nil {
		f.Close()
		fmt.Fprintf(os.Stderr, "WARNING: could not record port assignment: %v\n", err)
		return
	}
	f.Close()
}

// spiceRecord: write the SPICE connect target (unix path or TCP port)
// to <vmdir>/<vm>.spice — spice_record parity; start_viewer reads it.
func spiceRecord(dir, vmname, val string, runMode bool) {
	if !runMode {
		return
	}
	if strings.ContainsAny(val, "\r\n") {
		fmt.Fprintf(os.Stderr, "WARNING: refusing malformed spice record %q\n", val)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, vmname+".spice"), []byte(val+"\n"), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not write spice record: %v\n", err)
	}
}

// telnetChan renders the shared telnet channel setup for monitor and
// serial — the conf-key prefix, default port, flag name and aligned
// label are the only differences (Bash kept both blocks open-coded).
func telnetChan(args []string, conf Conf, vmdir, vmname string, runMode bool,
	kind, label, exhaust string, defPort int) []string {
	host := conf.get(kind+"_telnet_host", "localhost")
	warnTelnetExposure(kind, host)
	portStr := conf.get(kind+"_telnet_port", strconv.Itoa(defPort))
	start, _ := strconv.Atoi(portStr)
	if start <= 0 {
		start = defPort
	}
	port := getPort(start, 9, os.Getpid(), vmname)
	if port <= 0 {
		fmt.Printf("%sAll %s ports have been exhausted.\n", label, exhaust)
		return args
	}
	args = append(args, "-"+kind,
		fmt.Sprintf("telnet:%s:%d,server,nowait", qemuOptEsc(host), port))
	fmt.Printf("%sOn host:  telnet %s %d\n", label, host, port)
	portsRecord(vmdir, vmname, fmt.Sprintf("%s-telnet,%d,%s", kind, port, host), runMode)
	return args
}

// sockHint prints the socat/nc connect hint for a unix channel socket.
func sockHint(label, sockPath string) {
	if _, err := exec.LookPath("socat"); err == nil {
		fmt.Printf("%sOn host:  socat -,echo=0,icanon=0 unix-connect:%s\n", label, sockPath)
	} else if _, err := exec.LookPath("nc"); err == nil {
		fmt.Printf("%sOn host:  nc -U \"%s\"\n", label, sockPath)
	}
}
