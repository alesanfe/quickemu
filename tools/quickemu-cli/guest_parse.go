// guest_parse.go: the `quickemu guest <verb> ...` and `quickemu ip`
// pre-parse. One hook at the top of parseArgs hands argv here before
// the normal flag scan; this file owns the verb table and fills the
// same shared state (actions, vmConf, outFmt) the runActions flow
// consumes — no parser machinery is duplicated.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/quickemu-qga"
)

// guestValueFlags: flags in the main parse surface that consume the
// NEXT token as a value — a "guest"/"ip" sitting there is data
// (`--type guest`), not the subcommand.
var guestValueFlags = map[string]bool{
	"--vm": true, "--format": true, "--type": true, "--monitor-cmd": true,
	"--send-keys": true, "--clone": true, "--linked-clone": true,
	"--rename": true, "--move": true, "--export": true, "--import": true,
	"--backup-verify": true, "--backup-restore": true, "--backup-prune": true,
	"--trash-restore": true, "--disk-resize": true, "--snapshot": true,
	"--snapshot-create": true, "--snapshot-apply": true, "--snapshot-delete": true,
	"--hotplug-usb": true, "--hotplug-disk": true, "--hotunplug": true,
	"--op-status": true, "--operation-status": true, "--op-cancel": true,
	"--operation-cancel": true, "--op-watch": true, "--operation-watch": true,
	"--width": true, "--height": true, "--ssh-access": true, "--ssh-port": true,
	"--spice-port": true, "--monitor-telnet-host": true, "--monitor-telnet-port": true,
	"--serial-telnet-host": true, "--serial-telnet-port": true,
	"--keyboard": true, "--keyboard-layout": true, "--keyboard_layout": true,
	"--mouse": true, "--usb-controller": true, "--usb-passthrough": true,
	"--sound-card": true, "--sound-duplex": true, "--display": true,
	"--access": true, "--public-dir": true, "--monitor": true,
	"--serial": true, "--viewer": true, "--cpu-pinning": true,
	"--extra_args": true, "--extra-args": true, "--profile": true,
	"--plan-id": true, "--plugin": true, "--shortcut": true,
	"--screenshot": true, "--events": true,
}

// guestIntercept is the parseArgs hook: returns true when argv was
// consumed here (guest path fully handled — verb parsed, action
// queued). args[0]=="guest"|"ip" is the documented position; a bare
// guest/ip word deeper in argv is either a flag value (left alone)
// or a misplaced subcommand — which gets a real error rather than a
// silent fall-through into the default launch.
func guestIntercept(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "guest":
		guestParseVerb(args[1:])
		return true
	case "ip":
		guestParseVerb(append([]string{"ip"}, args[1:]...))
		return true
	}
	for i := 1; i < len(args); i++ {
		if args[i] != "guest" && args[i] != "ip" {
			continue
		}
		if guestValueFlags[args[i-1]] {
			continue // a value token, not the subcommand
		}
		if jsonRequested() {
			emitFail("guest.usage", "usage",
				fmt.Sprintf("%q must be the first argument", args[i]))
			os.Exit(1)
		}
		fmt.Printf("ERROR! '%s' must be the first argument — try 'quickemu %s ... --vm <conf>'.\n",
			args[i], args[i])
		os.Exit(1)
	}
	return false
}

// guestUsage: the guest-verb help — printed on `guest` with no verb
// or an unrecognised one (stdout, like the main usage()).
func guestUsage() {
	if jsonRequested() {
		emitFail("guest.usage", "usage", "guest verb required")
		return
	}
	fmt.Println("quickemu guest commands (QEMU Guest Agent):")
	fmt.Println("  guest ping                                agent liveness check")
	fmt.Println("  guest info                                agent/os/hostname/time summary")
	fmt.Println("  guest exec [--shell] [--env K=V] [--timeout 2m] [--max-output N] -- <argv>")
	fmt.Println("  guest push <local> <guest> [--verify] [--unsafe] [--max-size N]")
	fmt.Println("  guest pull <guest> <local> [--verify] [--restricted] [--force] [--max-size N]")
	fmt.Println("  guest shutdown [--timeout 60s] [--force] [--no-wait]")
	fmt.Println("  guest reboot | guest halt [--timeout 60s] [--force]")
	fmt.Println("  guest fsfreeze status|freeze|thaw [--hold D]")
	fmt.Println("  quickemu ip [--iface NAME] [-4|-6]        guest network addresses")
	fmt.Println("  all verbs accept --vm <conf> and --format json")
}

// guestParseVerb: args after the "guest" word (or after "ip",
// rewritten to the ip verb). The first token is the verb; the tail
// carries verb flags plus the shared --vm/--format.
func guestParseVerb(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		guestUsage()
		os.Exit(2)
	}
	verb, tail := args[0], args[1:]
	switch verb {
	case "ping":
		if rest := guestSharedFlags(tail); len(rest) > 0 {
			guestFlagErr("%q is not a guest ping flag.", rest[0])
		}
		actions = append(actions, "guest_ping")
	case "info", "status":
		if rest := guestSharedFlags(tail); len(rest) > 0 {
			guestFlagErr("%q is not a guest info flag.", rest[0])
		}
		actions = append(actions, "guest_info")
	case "exec", "run":
		guestParseExec(tail)
		actions = append(actions, "guest_exec")
	case "push":
		guestParseXfer(tail, true)
		actions = append(actions, "guest_push")
	case "pull", "get":
		guestParseXfer(tail, false)
		actions = append(actions, "guest_pull")
	case "shutdown", "poweroff":
		guestParsePower(tail)
		actions = append(actions, "guest_shutdown")
	case "reboot":
		guestParsePower(tail)
		actions = append(actions, "guest_reboot")
	case "halt":
		guestParsePower(tail)
		actions = append(actions, "guest_halt")
	case "fsfreeze", "freeze":
		guestParseFreeze(tail)
		actions = append(actions, "guest_fsfreeze")
	case "ip":
		guestParseIP(tail)
		actions = append(actions, "guest_ip")
	default:
		if jsonRequested() {
			emitFail("guest.usage", "usage",
				fmt.Sprintf("%q is not a guest command", verb))
			os.Exit(1)
		}
		fmt.Printf("ERROR! \"%s\" is not a guest command.\n", verb)
		guestUsage()
		os.Exit(1)
	}
}

// guestFlagErr: shared flag-error line (stdout contract).
func guestFlagErr(format string, a ...any) {
	if jsonRequested() {
		emitFail("guest.usage", "usage", fmt.Sprintf(format, a...))
		os.Exit(1)
	}
	fmt.Printf("ERROR! "+format+"\n", a...)
	os.Exit(1)
}

// guestDuration accepts "60s"/"2m" duration strings or a bare
// seconds count.
func guestDuration(v string) time.Duration {
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	guestFlagErr("invalid duration %q (e.g. 30s, 2m, or seconds).", v)
	return 0
}

// guestSize accepts byte counts with optional K/M/G suffix.
func guestSize(v string) int64 {
	mult := int64(1)
	s := strings.ToUpper(strings.TrimSpace(v))
	for _, suf := range []struct {
		s string
		m int64
	}{{"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}} {
		if strings.HasSuffix(s, suf.s) {
			mult = suf.m
			s = strings.TrimSuffix(s, suf.s)
			break
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		guestFlagErr("invalid size %q (e.g. 1048576, 512K, 10M, 1G).", v)
	}
	return n * mult
}

// guestSharedFlags: the --vm/--format subset honoured by every
// guest verb — returns the tail minus consumed flags.
func guestSharedFlags(args []string) []string {
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--vm":
			if i+1 < len(args) {
				i++
				vmConf = args[i]
			} else {
				guestFlagErr("--vm needs a conf path.")
			}
		case "--format":
			if i+1 < len(args) {
				i++
				if args[i] == "json" || args[i] == "human" || args[i] == "text" {
					outFmt = args[i]
					if outFmt == "human" {
						outFmt = "text"
					}
				} else {
					guestFlagErr("'--format' must be 'human' or 'json'.")
				}
			} else {
				guestFlagErr("--format needs a value.")
			}
		default:
			rest = append(rest, args[i])
		}
	}
	return rest
}

// guestParseExec: flags until the first positional (or a literal
// "--") — everything after is the guest argv, verbatim.
func guestParseExec(tail []string) {
	e := &guestVerbState.exec
	e.timeout = 2 * time.Minute
	e.maxOutput = 1 << 20
	var pos []string
	for i := 0; i < len(tail); i++ {
		a := tail[i]
		if a == "--" {
			pos = append(pos, tail[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, tail[i:]...)
			break
		}
		val := func() string {
			if i+1 >= len(tail) {
				guestFlagErr("%s needs a value.", a)
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
				guestFlagErr("'--format' must be 'human' or 'json'.")
			}
			outFmt = f
			if outFmt == "human" {
				outFmt = "text"
			}
		case "--shell":
			e.shell = true
		case "--env":
			e.env = append(e.env, val())
		case "--timeout":
			e.timeout = guestDuration(val())
		case "--max-output":
			e.maxOutput = int(guestSize(val()))
		default:
			guestFlagErr("%q is not a guest exec flag.", a)
		}
	}
	e.argv = pos
	if len(e.argv) == 0 {
		guestFlagErr("guest exec needs a command (e.g. guest exec -- uname -a).")
	}
}

// guestParseXfer: push/pull — two positionals (src, dst), plus the
// verification/restriction/overwrite flags.
func guestParseXfer(tail []string, push bool) {
	x := &guestVerbState.xfer
	x.restricted = push // writes default to restricted mode
	x.maxSize = qga.DefaultMaxTransfer
	var pos []string
	for i := 0; i < len(tail); i++ {
		a := tail[i]
		val := func() string {
			if i+1 >= len(tail) {
				guestFlagErr("%s needs a value.", a)
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
				guestFlagErr("'--format' must be 'human' or 'json'.")
			}
			outFmt = f
			if outFmt == "human" {
				outFmt = "text"
			}
		case "--verify":
			x.verify = true
		case "--restricted":
			x.restricted = true
		case "--unsafe":
			x.restricted = false
		case "--force":
			x.force = true
		case "--max-size":
			x.maxSize = guestSize(val())
		default:
			if strings.HasPrefix(a, "-") {
				guestFlagErr("%q is not a guest transfer flag.", a)
			}
			pos = append(pos, a)
		}
	}
	if len(pos) != 2 {
		verb := "pull <guest-path> <local-path>"
		if push {
			verb = "push <local-path> <guest-path>"
		}
		guestFlagErr("guest %s takes exactly two paths.", verb)
	}
	x.src, x.dst = pos[0], pos[1]
}

// guestParsePower: shutdown/reboot/halt flags.
func guestParsePower(tail []string) {
	pw := &guestVerbState.power
	pw.timeout = 60 * time.Second
	for i := 0; i < len(tail); i++ {
		a := tail[i]
		val := func() string {
			if i+1 >= len(tail) {
				guestFlagErr("%s needs a value.", a)
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
				guestFlagErr("'--format' must be 'human' or 'json'.")
			}
			outFmt = f
			if outFmt == "human" {
				outFmt = "text"
			}
		case "--timeout":
			pw.timeout = guestDuration(val())
		case "--force":
			pw.force = true
		case "--no-wait":
			pw.noWait = true
		default:
			guestFlagErr("%q is not a guest power flag.", a)
		}
	}
}

// guestParseFreeze: fsfreeze status|freeze|thaw [--hold D].
func guestParseFreeze(tail []string) {
	f := &guestVerbState.freeze
	var pos []string
	for i := 0; i < len(tail); i++ {
		a := tail[i]
		val := func() string {
			if i+1 >= len(tail) {
				guestFlagErr("%s needs a value.", a)
			}
			i++
			return tail[i]
		}
		switch a {
		case "--vm":
			vmConf = val()
		case "--format":
			fm := val()
			if fm != "json" && fm != "human" && fm != "text" {
				guestFlagErr("'--format' must be 'human' or 'json'.")
			}
			outFmt = fm
			if outFmt == "human" {
				outFmt = "text"
			}
		case "--hold":
			f.hold = guestDuration(val())
		default:
			if strings.HasPrefix(a, "-") {
				guestFlagErr("%q is not a guest fsfreeze flag.", a)
			}
			pos = append(pos, a)
		}
	}
	switch {
	case len(pos) == 0:
		f.sub = "status"
	case len(pos) == 1:
		switch pos[0] {
		case "status", "freeze", "thaw":
			f.sub = pos[0]
		default:
			guestFlagErr("guest fsfreeze %q — expected status|freeze|thaw.", pos[0])
		}
	default:
		guestFlagErr("guest fsfreeze takes one sub-command (status|freeze|thaw).")
	}
}

// guestParseIP: --iface, -4/-6 selection.
func guestParseIP(tail []string) {
	ip := &guestVerbState.ip
	for i := 0; i < len(tail); i++ {
		a := tail[i]
		val := func() string {
			if i+1 >= len(tail) {
				guestFlagErr("%s needs a value.", a)
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
				guestFlagErr("'--format' must be 'human' or 'json'.")
			}
			outFmt = f
			if outFmt == "human" {
				outFmt = "text"
			}
		case "--iface":
			ip.iface = val()
		case "-4", "--ipv4":
			ip.v4 = true
		case "-6", "--ipv6":
			ip.v6 = true
		default:
			guestFlagErr("%q is not an ip flag.", a)
		}
	}
}
