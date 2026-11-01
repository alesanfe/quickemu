// ssh_parse.go: the `quickemu ssh|scp|sftp|ssh-config|ssh-forget`
// pre-parse — QE-030/031/032/033. Same shape as guest_parse.go: an
// intercept at the top of parseArgs owns the verb table and fills
// sshVerbState; resolution against the live VM happens at act time.
package main

import (
	"fmt"
	"os"
	"strings"
)

// sshVerbState: parse output for one queued ssh-family action.
var sshVerbState struct {
	user     string   // -l/--user or user@ prefix
	port     string   // -p/--port override (forwarded port on host)
	jump     string   // -J/--jump ProxyJump
	agent    bool     // -A agent forwarding
	verbose  bool     // -v
	ansible  bool     // ssh-config --ansible
	alias    string   // ssh-config --host <name>
	pos      []string // positionals (scp src/dst, sftp target, ssh host tail)
	passthru []string // everything after `--` — verbatim ssh args
}

// sshIntercept: `ssh`, `scp`, `sftp`, `ssh-config`, `ssh-forget` in
// args[0] — or misplaced deeper in argv, which earns a real error
// rather than a silent fall-through into the default launch.
func sshIntercept(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "ssh":
		sshParseSSH(args[1:])
		return true
	case "scp":
		sshParseCopy(args[1:], "vm_scp")
		return true
	case "sftp":
		sshParseCopy(args[1:], "vm_sftp")
		return true
	case "ssh-config":
		sshParseConfig(args[1:])
		return true
	case "ssh-forget":
		sshParseForget(args[1:])
		return true
	}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "ssh", "scp", "sftp", "ssh-config", "ssh-forget":
			if guestValueFlags[args[i-1]] {
				continue // a flag value, not the subcommand
			}
			if jsonRequested() {
				emitFail("ssh.usage", "usage",
					fmt.Sprintf("%q must be the first argument", args[i]))
				os.Exit(1)
			}
			fmt.Printf("ERROR! '%s' must be the first argument — try 'quickemu %s ... --vm <conf>'.\n",
				args[i], args[i])
			os.Exit(1)
		}
	}
	return false
}

// sshUsage: help for the ssh verb family (stdout contract).
func sshUsage() {
	if jsonRequested() {
		emitFail("ssh.usage", "usage", "ssh verb required")
		return
	}
	fmt.Println("quickemu ssh commands:")
	fmt.Println("  ssh [-l user] [-p port] [-A] [-J jump] [-v] [-- <ssh args>]")
	fmt.Println("  scp [-P port] <src> <dst> [-- <scp args>]   (':path' marks the guest side)")
	fmt.Println("  sftp [-P port] [user@]host [-- <sftp args>]")
	fmt.Println("  ssh-config [--host alias] [--ansible]       OpenSSH host block / Ansible inventory")
	fmt.Println("  ssh-forget                                drop the VM's recorded host key")
	fmt.Println("  all verbs accept --vm <conf> and --format json")
}

// sshShared: --vm/--format plus the per-verb value flags — consumed
// in place; returns unconsumed tokens (positionals or errors).
func sshShared(tail []string) []string {
	var rest []string
	for i := 0; i < len(tail); i++ {
		a := tail[i]
		val := func() string {
			if i+1 >= len(tail) {
				sshFlagErr("%s needs a value.", a)
			}
			i++
			return tail[i]
		}
		switch {
		case a == "--":
			sshVerbState.passthru = tail[i+1:]
			return rest
		case a == "--vm":
			vmConf = val()
		case a == "--format":
			f := val()
			if f != "json" && f != "human" && f != "text" {
				sshFlagErr("'--format' must be 'human' or 'json'.")
			}
			outFmt = f
			if outFmt == "human" {
				outFmt = "text"
			}
		case a == "-l" || a == "--user":
			sshVerbState.user = val()
		case a == "-p" || a == "-P" || a == "--port":
			// ssh spells it -p, scp/sftp -P — accept both, the wrapper
			// always translates to the client binary's own spelling.
			sshVerbState.port = val()
		case a == "-J" || a == "--jump":
			sshVerbState.jump = val()
		case a == "-A":
			sshVerbState.agent = true
		case a == "-v" || a == "--verbose":
			sshVerbState.verbose = true
		case a == "-h" || a == "--help":
			sshUsage()
			os.Exit(0)
		default:
			rest = append(rest, a)
		}
	}
	return rest
}

// sshFlagErr: shared flag-error line (stdout contract).
func sshFlagErr(format string, a ...any) {
	if jsonRequested() {
		emitFail("ssh.usage", "usage", fmt.Sprintf(format, a...))
		os.Exit(1)
	}
	fmt.Printf("ERROR! "+format+"\n", a...)
	os.Exit(1)
}

// sshParseSSH: ssh [-l u] [-p n] [-A] [-J h] [-v] [-- args]
func sshParseSSH(tail []string) {
	rest := sshShared(tail)
	// Non-flag leftovers are ssh args forwarded verbatim (QE-030:
	// "pasar argumentos adicionales al cliente SSH") — a user@host
	// positional is an error though: the wrapper owns the target.
	for _, a := range rest {
		if !strings.HasPrefix(a, "-") && strings.Contains(a, "@") {
			sshFlagErr("the target is the VM — user goes in -l, not %q.", a)
		}
	}
	sshVerbState.passthru = append(sshVerbState.passthru, rest...)
	actions = append(actions, "vm_ssh")
}

// sshParseCopy: scp/sftp — positionals pass through (src/dst or the
// sftp target); bare remote paths get user@host: at act time.
func sshParseCopy(tail []string, act string) {
	rest := sshShared(tail)
	for _, a := range rest {
		if strings.HasPrefix(a, "-") {
			sshFlagErr("%q is not a %s flag — pass client options after --.", a, act[3:])
		}
	}
	sshVerbState.pos = rest
	actions = append(actions, act)
}

// sshParseConfig: ssh-config [--host name] [--ansible]
func sshParseConfig(tail []string) {
	for i := 0; i < len(tail); i++ {
		a := tail[i]
		val := func() string {
			if i+1 >= len(tail) {
				sshFlagErr("%s needs a value.", a)
			}
			i++
			return tail[i]
		}
		switch a {
		case "--vm":
			vmConf = val()
		case "--format":
			f := val()
			if f != "json" && f != "human" && f != "text" {
				sshFlagErr("'--format' must be 'human' or 'json'.")
			}
			outFmt = f
			if outFmt == "human" {
				outFmt = "text"
			}
		case "--host":
			sshVerbState.alias = val()
		case "--ansible":
			sshVerbState.ansible = true
		default:
			sshFlagErr("%q is not an ssh-config flag.", a)
		}
	}
	actions = append(actions, "vm_ssh_config")
}

// sshParseForget: ssh-forget — drop the HostKeyAlias entry.
func sshParseForget(tail []string) {
	if rest := sshShared(tail); len(rest) > 0 {
		sshFlagErr("%q is not an ssh-forget flag.", rest[0])
	}
	actions = append(actions, "vm_ssh_forget")
}
