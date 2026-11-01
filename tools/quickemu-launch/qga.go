// qga.go: the universal QEMU guest-agent channel. The channel used
// to live inside the SPICE emit, so guests only got an agent socket
// on SPICE displays — every display mode now carries it (the guest-*)
// CLI verbs talk to the agent socket regardless), and qga="off"
// disables the emission entirely.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// qgaEnabled: the channel is on unless qga=off|false|no — presence is
// the default so existing confs get the agent socket for free.
func qgaEnabled(c Conf) bool {
	switch c.get("qga", "on") {
	case "off", "false", "no":
		return false
	}
	return true
}

// qgaSockPath: the agent channel endpoint (the CLI's qgaSock resolves
// the identical path via qelib.SockPath).
func qgaSockPath(dir, vmname string) string {
	return qelib.SockPath(dir, vmname, qelib.SockAgent)
}

// qgaArgs: virtio-serial bus + chardev socket + the
// org.qemu.guest_agent.0 virtserialport — identical emission to the
// old SPICE-block lines, now independent of the display choice.
// The bus is emitted here exactly once: spiceArgs skips its own
// virtio-serial-pci when the channel is enabled so the vdagent/
// webdav ports share this bus.
func qgaArgs(c Conf, dir, vmname string) []string {
	if !qgaEnabled(c) {
		return nil
	}
	sock := qemuOptEsc(qelib.SockBind(dotdir(dir), qelib.SockAgent))
	return []string{
		"-device", "virtio-serial-pci",
		"-chardev", fmt.Sprintf("socket,id=agent0,path=%s,server=on,wait=off", sock),
		"-device", "virtserialport,chardev=agent0,name=org.qemu.guest_agent.0",
	}
}

// chmodAgentSock: QEMU binds the chardev socket with the process
// umask (world-accessible) — the agent channel is privileged (exec,
// file I/O, freeze), so tighten it to owner-only once QEMU has
// created the file. Best-effort: a stubborn filesystem leaves the
// socket usable, only noisier.
func chmodAgentSock(dir, vmname string) {
	sock := qgaSockPath(dir, vmname)
	for i := 0; i < 30; i++ {
		if st, err := os.Stat(sock); err == nil && st.Mode()&os.ModeSocket != 0 {
			if err := os.Chmod(sock, 0o600); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: could not chmod 0600 %s: %v\n", sock, err)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	// QEMU may legitimately never bind the socket (qga=off raced, or a
	// build without virtserialport) — stay silent, the port exists or
	// it does not.
}
