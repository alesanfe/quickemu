// sockpath.go: per-VM unix channel paths — bound by QEMU/companion
// daemons under launch, connected to by every reader (cli, env, api,
// qmp, provision). One implementation so the on-disk layout is decided
// in exactly one place.
//
// Layout: <vmdir>/.s/<kind> — fixed short basenames keep the path
// under sockaddr_un's ~107 usable bytes even with long VM names and
// deep workdirs (<vm>-agent.sock overflowed it for e.g.
// debian-cloud-13-genericcloud under a prov-e2e tree, and QEMU only
// reports the overflow deep in <vm>.log). Readers still resolve
// sockets bound by pre-.s launches through SockPath's legacy
// fallback, so a VM started before the upgrade stays controllable.
package qelib

import (
	"os"
	"path/filepath"
	"strings"
)

// Socket kinds — the .s/ basename for each channel.
const (
	SockAgent     = "agent"     // qemu-guest-agent virtserialport chardev
	SockQMP       = "qmp"       // -qmp machine-control channel
	SockMonitor   = "monitor"   // -monitor human-monitor channel
	SockSerial    = "serial"    // -serial console chardev
	SockSpice     = "spice"     // SPICE unix endpoint (display=spice local)
	SockVirtiofsd = "virtiofsd" // virtiofsd --socket-path
	SockSwtpm     = "swtpm"     // swtpm --ctrl type=unixio
)

// SockSubdir is the hidden dir inside the VM dir carrying every bound
// channel.
const SockSubdir = ".s"

// sockLegacy: the <vm><suffix> basenames bound before the .s layout —
// kept so clients still reach VMs launched by older binaries.
var sockLegacy = map[string]string{
	SockAgent:     "-agent.sock",
	SockQMP:       "-qmp.socket",
	SockMonitor:   "-monitor.socket",
	SockSerial:    "-serial.socket",
	SockSpice:     ".sock",
	SockVirtiofsd: ".virtiofsd-sock",
	SockSwtpm:     ".swtpm-sock",
}

// SockBind: the path the launcher binds for a channel — always the
// .s/ layout. Binders unlink stale sockets (QEMU unix_listen, and the
// daemon spawns remove explicitly), so nothing cleans before re-use.
func SockBind(dir, kind string) string {
	return filepath.Join(dir, SockSubdir, kind)
}

// SockPath: the path a client connects to — the .s/ socket when it is
// live, else a live legacy <vm><suffix> socket (a VM still running
// from a pre-.s binary), else the .s/ path so a dial error names the
// file the next launch creates.
func SockPath(dir, vm, kind string) string {
	p := SockBind(dir, kind)
	if isSocket(p) {
		return p
	}
	if suf, ok := sockLegacy[kind]; ok && vm != "" {
		if lp := filepath.Join(dir, vm+suf); isSocket(lp) {
			return lp
		}
	}
	return p
}

// SockOwner: the VM a socket dir belongs to — the <dir>/.s/vm sidecar
// the launcher writes, else the vmdir basename (covers .s trees from
// before the sidecar, where the per-VM dir convention names the VM).
func SockOwner(dir string) string {
	if b, err := os.ReadFile(filepath.Join(dir, SockSubdir, "vm")); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	return filepath.Base(filepath.Clean(dir))
}

func isSocket(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode()&os.ModeSocket != 0
}
