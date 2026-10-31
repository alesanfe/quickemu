// ssh.go: quickemu ssh/scp/sftp/ssh-config/ssh-forget — QE-030–033.
// Resolution: the live .ports ssh record wins, then conf ssh_port,
// then QGA ip discovery (bridge/virtio-net VMs with no forward — the
// guest answers on its own address:22). Keys live in a per-VM
// known_hosts bound to a HostKeyAlias derived from the VM identity,
// never the ephemeral forwarded port (QE-031).
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/quickemu-qga"
)

// sshTarget: the resolved endpoint for one invocation.
type sshTarget struct {
	host   string // connect address (127.0.0.1 for a hostfwd, guest IP otherwise)
	port   string // TCP port (forwarded port or 22 for direct)
	user   string
	alias  string // HostKeyAlias — keys bind to the VM, not the port
	kh     string // per-VM known_hosts path
	viaQGA bool   // port came from QGA ip discovery, not a forward
}

// sshForward: the "ssh,<port>[,<bind>]" record in <vmdir>/<vm>.ports.
// The third field is the bind address the launcher recorded; old
// launchers wrote two fields only (fine — loopback is the default).
func sshForward() (port, bind string) {
	b, err := os.ReadFile(filepath.Join(vmdir(), vmname()+".ports"))
	if err != nil {
		return "", ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.SplitN(l, ",", 3)
		if len(f) >= 2 && f[0] == "ssh" {
			port = f[1]
			if len(f) == 3 {
				bind = f[2]
			}
			return
		}
	}
	return "", ""
}

// sshResolve: flag port → .ports record → conf ssh_port → QGA ip.
// An unreachable/absent VM fails with the actionable message, not a
// bare "connection refused" from a stale forward.
func sshResolve() (*sshTarget, error) {
	t := &sshTarget{alias: "quickemu-" + vmname()}
	if sshVerbState.port != "" && !sshPortValid(sshVerbState.port) {
		return nil, fmt.Errorf("invalid port %q — expected 1-65535", sshVerbState.port)
	}
	port, bind := sshForward()
	switch {
	case sshVerbState.port != "":
		t.port = sshVerbState.port
		t.host = "127.0.0.1"
	case port != "":
		t.port = port
		// A wildcard/specific bind still means *we* reach it on
		// loopback; a non-loopback bind address is the address itself.
		if bind != "" && bind != "0.0.0.0" && bind != "*" && bind != "localhost" {
			t.host = bind
		} else {
			t.host = "127.0.0.1"
		}
	case confKV()["ssh_port"] != "":
		t.port = confKV()["ssh_port"]
		t.host = "127.0.0.1"
	default:
		// No forward — bridged/virtio-net guests answer on their own
		// address. QGA ip discovery (QE-016) finds it when the agent
		// is up; the typed error keeps "not running" clear.
		ctx, cancel := guestCtx(5 * time.Second)
		cl, err := guestDial(5 * time.Second)
		if err != nil {
			cancel()
			return nil, err
		}
		v4, v6, ierr := guestIPs(ctx, cl)
		cancel()
		if ierr != nil || (len(v4) == 0 && len(v6) == 0) {
			return nil, errors.New("no ssh forward and no guest IP — is the VM running with a reachable network?")
		}
		if len(v4) > 0 {
			t.host = v4[0]
		} else {
			t.host = v6[0]
		}
		t.port, t.viaQGA = "22", true
	}
	switch {
	case sshVerbState.user != "":
		t.user = sshVerbState.user
	case confKV()["ssh_user"] != "":
		t.user = confKV()["ssh_user"]
	case os.Getenv("USER") != "":
		t.user = os.Getenv("USER")
	default:
		t.user = "user"
	}
	t.kh = filepath.Join(vmdir(), vmname()+".ssh_known_hosts")
	return t, nil
}

// sshKnownHosts: the per-VM known_hosts — created 0600, never a
// symlink (QE-031: a symlink here redirects trust writes outside the
// VM dir; the post-open Fstat closes the lstat→open window).
func sshKnownHosts(t *sshTarget) error {
	if fi, err := os.Lstat(t.kh); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file — refusing to use it", t.kh)
	}
	f, err := os.OpenFile(t.kh, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", t.kh)
	}
	if fi.Mode().Perm() != 0600 {
		f.Chmod(0600) // group/world-readable trust store gets tightened
	}
	return nil
}

// sshOpts: the options every ssh-family client shares — isolated
// trust store, VM-bound key alias, accept-new (still verifies key
// changes; never off — QE-031).
func sshOpts(t *sshTarget) []string {
	return []string{
		"-o", "UserKnownHostsFile=" + t.kh,
		"-o", "HostKeyAlias=" + t.alias,
		"-o", "StrictHostKeyChecking=accept-new",
	}
}

// sshRun: exec with inherited stdio — signals reach the client in
// the shared process group; the exit code propagates verbatim.
func sshRun(bin string, argv []string) int {
	if _, err := exec.LookPath(bin); err != nil {
		fmt.Printf("ERROR! '%s' is not installed — install openssh-client.\n", bin)
		return 1
	}
	c := execCommand(bin, argv...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := c.Run()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
		return 1 // killed by signal — 1, like Bash's non-zero propagates
	}
	fmt.Printf("ERROR! could not start %s: %v\n", bin, err)
	return 1
}

// actSSH: quickemu ssh — build the client argv and exec it.
func actSSH() int {
	t, err := sshResolve()
	if err != nil {
		return sshFail("ssh", err)
	}
	if err := sshKnownHosts(t); err != nil {
		return sshFail("ssh", err)
	}
	argv := []string{"-p", t.port}
	argv = append(argv, sshOpts(t)...)
	if sshVerbState.agent {
		argv = append(argv, "-A")
	}
	if sshVerbState.jump != "" {
		argv = append(argv, "-J", sshVerbState.jump)
	}
	if sshVerbState.verbose {
		argv = append(argv, "-v")
	}
	argv = append(argv, t.user+"@"+t.host)
	argv = append(argv, sshVerbState.passthru...)
	if sshVerbState.verbose {
		fmt.Fprintf(os.Stderr, "+ ssh %s\n", strings.Join(argv, " "))
	}
	return sshRun("ssh", argv)
}

// sshRemote: re-shape one scp positional — a leading ':' marks the
// guest side explicitly (`scp file :dest` — the unambiguous form);
// an arg that already carries a host target (x:y) stays verbatim; a
// bare path is remote when `remote` picks this side, else local.
func (t *sshTarget) sshRemote(a string, remote bool) string {
	if strings.HasPrefix(a, ":") {
		return t.user + "@" + t.host + a
	}
	if strings.Contains(a, ":") {
		return a
	}
	if !remote {
		return a
	}
	return t.user + "@" + t.host + ":" + a
}

// scpRemoteSide: which positional is the guest side? An explicit
// ':' marker decides first; else an existing local src means push
// (dst remote), otherwise the src is the guest path (pull).
func scpRemoteSide(src, dst string) (srcRemote, dstRemote bool) {
	if strings.HasPrefix(src, ":") {
		return true, false
	}
	if strings.HasPrefix(dst, ":") {
		return false, true
	}
	if _, err := os.Stat(src); err == nil {
		return false, true
	}
	return true, false
}

// actSCP: quickemu scp <src> <dst> — one side is the guest; a bare
// path on the remote side gets user@host:, explicit forms pass
// through (compat with native scp syntax — QE-032).
func actSCP() int {
	pos := sshVerbState.pos
	if len(pos) != 2 {
		fmt.Println("ERROR! quickemu scp takes exactly two paths — the remote one is the guest's.")
		return 1
	}
	t, err := sshResolve()
	if err != nil {
		return sshFail("scp", err)
	}
	if err := sshKnownHosts(t); err != nil {
		return sshFail("scp", err)
	}
	srcRemote, dstRemote := scpRemoteSide(pos[0], pos[1])
	argv := []string{"-P", t.port}
	argv = append(argv, sshOpts(t)...)
	if sshVerbState.verbose {
		argv = append(argv, "-v")
	}
	argv = append(argv, t.sshRemote(pos[0], srcRemote), t.sshRemote(pos[1], dstRemote))
	argv = append(argv, sshVerbState.passthru...)
	if sshVerbState.verbose {
		fmt.Fprintf(os.Stderr, "+ scp %s\n", strings.Join(argv, " "))
	}
	return sshRun("scp", argv)
}

// actSFTP: quickemu sftp [target] — bare target/none → user@host.
func actSFTP() int {
	t, err := sshResolve()
	if err != nil {
		return sshFail("sftp", err)
	}
	if err := sshKnownHosts(t); err != nil {
		return sshFail("sftp", err)
	}
	argv := []string{"-P", t.port}
	argv = append(argv, sshOpts(t)...)
	if sshVerbState.verbose {
		argv = append(argv, "-v")
	}
	target := t.user + "@" + t.host
	for _, a := range sshVerbState.pos {
		if strings.Contains(a, "@") || strings.Contains(a, ":") {
			target = a
		}
	}
	argv = append(argv, target)
	argv = append(argv, sshVerbState.passthru...)
	if sshVerbState.verbose {
		fmt.Fprintf(os.Stderr, "+ sftp %s\n", strings.Join(argv, " "))
	}
	return sshRun("sftp", argv)
}

// actSSHConfig: QE-033 — an OpenSSH Host block, Ansible inventory
// line, or the JSON field set Quickgui/automation consumes.
func actSSHConfig() int {
	t, err := sshResolve()
	if err != nil {
		return sshFail("ssh-config", err)
	}
	host := sshVerbState.alias
	if host == "" {
		host = t.alias
	}
	if outFmt == "json" {
		emitOK("vm.ssh_config", map[string]any{
			"host": host, "hostname": t.host, "port": t.port,
			"user": t.user, "known_hosts": t.kh, "host_key_alias": t.alias,
			"command": fmt.Sprintf("ssh -p %s %s@%s", t.port, t.user, t.host),
		})
		return 0
	}
	if sshVerbState.ansible {
		fmt.Printf("%s ansible_host=%s ansible_port=%s ansible_user=%s "+
			"ansible_ssh_extra_args='-o UserKnownHostsFile=%s -o HostKeyAlias=%s'\n",
			host, t.host, t.port, t.user, t.kh, t.alias)
		return 0
	}
	fmt.Printf("Host %s\n", host)
	fmt.Printf("    HostName %s\n", t.host)
	fmt.Printf("    Port %s\n", t.port)
	fmt.Printf("    User %s\n", t.user)
	fmt.Printf("    UserKnownHostsFile %s\n", t.kh)
	fmt.Printf("    HostKeyAlias %s\n", t.alias)
	fmt.Printf("    StrictHostKeyChecking accept-new\n")
	return 0
}

// actSSHForget: QE-031 — drop the VM's recorded host key. ssh-keygen
// -R rewrites safely; without it we filter the alias lines ourselves
// (the file is small, ours, and per-VM).
func actSSHForget() int {
	t := &sshTarget{alias: "quickemu-" + vmname(),
		kh: filepath.Join(vmdir(), vmname()+".ssh_known_hosts")}
	b, err := os.ReadFile(t.kh)
	if err != nil {
		fmt.Printf(" - ssh:      No known_hosts for %s — nothing to forget.\n", vmname())
		return 0
	}
	if _, err := exec.LookPath("ssh-keygen"); err == nil {
		if c := execCommand("ssh-keygen", "-R", t.alias, "-f", t.kh); c.Run() == nil {
			fmt.Printf(" - ssh:      Forgot host key for %s.\n", vmname())
			return 0
		}
	}
	var kept []string
	dropped := 0
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, t.alias+" ") {
			dropped++
			continue
		}
		kept = append(kept, l)
	}
	if dropped == 0 {
		fmt.Printf(" - ssh:      No recorded key for %s.\n", vmname())
		return 0
	}
	if err := os.WriteFile(t.kh, []byte(strings.Join(kept, "\n")), 0600); err != nil {
		return sshFail("ssh-forget", err)
	}
	fmt.Printf(" - ssh:      Forgot %d host key(s) for %s.\n", dropped, vmname())
	return 0
}

// sshFail: user-facing ssh errors (stdout contract; JSON envelope
// maps the QGA resolution failures to their typed codes).
func sshFail(verb string, err error) int {
	if outFmt == "json" {
		code := "ssh_error"
		var ae *qga.AgentError
		switch {
		case errors.Is(err, qga.ErrSocketMissing):
			code = "channel_missing"
		case errors.Is(err, qga.ErrAgentUnreachable):
			code = "channel_unreachable"
		case errors.Is(err, qga.ErrAgentNotReady):
			code = "agent_not_ready"
		case errors.Is(err, qga.ErrTimeout):
			code = "timeout"
		case errors.As(err, &ae):
			code = "agent_error:" + ae.Class
		}
		emitFail("vm."+verb, code, err.Error())
		return 1
	}
	fmt.Printf(" - %s: %v\n", verb, err)
	return 1
}

// dispatchSSH: the ssh-family slot in the dispatch table.
func dispatchSSH(act string) func() int {
	switch act {
	case "vm_ssh":
		return actSSH
	case "vm_scp":
		return actSCP
	case "vm_sftp":
		return actSFTP
	case "vm_ssh_config":
		return actSSHConfig
	case "vm_ssh_forget":
		return actSSHForget
	}
	return nil
}

// sshPortValid: a flag-supplied port must parse 1-65535 — a bogus
// value would land verbatim in the client argv as an option-shaped
// string.
func sshPortValid(p string) bool {
	n, err := strconv.Atoi(p)
	return err == nil && n > 0 && n <= 65535
}
