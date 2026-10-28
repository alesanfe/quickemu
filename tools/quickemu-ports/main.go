// quickemu-ports: ADR-0001 phase 4 — the port-reservation registry in
// Go. Same on-disk layout as lifecycle.sh: <runtimeDir>/ports/<port>
// marker files holding "pid <ts> <instance>", guarded by a mkdir lock.
package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const version = "0.1.0"

func die(c, m string) int {
	qelib.Emit(qelib.CmdName("ports"), "quickemu-ports/"+version, false, nil,
		[]qelib.QEError{{Code: c, Message: m, Severity: "error", Retryable: false}})
	return 1
}
func ok(data map[string]any) int {
	qelib.EmitOK(qelib.CmdName("ports"), "quickemu-ports/"+version, data)
	return 0
}

func stale(meta string) bool { // "pid ts instance [lease]" -> owner dead or lease out
	f := strings.Fields(meta)
	if len(f) == 0 {
		return true
	}
	if len(f) >= 4 { // lease expiry: field 4 = epoch deadline
		if dl, err := strconv.ParseInt(f[3], 10, 64); err == nil &&
			time.Now().Unix() > dl {
			return true
		}
	}
	p, _ := strconv.Atoi(f[0])
	// EPERM = alive: a foreign owner's lease must never be reclaimed.
	// An unparseable/zero pid is a corrupt record — stale, not alive.
	return p <= 0 || !qelib.Alive(p)
}

func lock(reg string) func() {
	lk := reg + ".lock"
	for i := 0; i < 50; i++ {
		if os.Mkdir(lk, 0o700) == nil {
			os.WriteFile(filepath.Join(lk, "pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
			return func() { os.RemoveAll(lk) }
		}
		if b, err := os.ReadFile(filepath.Join(lk, "pid")); err == nil && stale(string(b)) {
			os.RemoveAll(lk)
			continue
		}
		time.Sleep(100 * time.Millisecond)
	}
	die("LOCK_BUSY", "could not acquire port registry lock")
	return nil
}

func main() { os.Exit(run()) }

func run() int {
	os.Args = qelib.NormArgs(os.Args)
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-ports <reserve|release|list|check|find> --registry <dir> [--port N] [--pid P] [--instance I] [--lo N --hi N]")
		return 2
	}
	sub := os.Args[1]
	var reg, port, pidS, inst, proto, bind, lease string
	var lo, hi int
	for i := 2; i+1 < len(os.Args); i += 2 {
		switch os.Args[i] {
		case "--registry":
			reg = os.Args[i+1]
		case "--port":
			port = os.Args[i+1]
		case "--pid":
			pidS = os.Args[i+1]
		case "--instance":
			inst = os.Args[i+1]
		case "--lo":
			lo, _ = strconv.Atoi(os.Args[i+1])
		case "--hi":
			hi, _ = strconv.Atoi(os.Args[i+1])
		case "--proto":
			proto = os.Args[i+1] // tcp|udp, default tcp
		case "--bind":
			bind = os.Args[i+1] // 127.0.0.1, 0.0.0.0, ::1, ::
		case "--lease":
			lease = os.Args[i+1] // seconds; reservation auto-expires
		}
	}
	if reg == "" {
		return die("BAD_ARGS", "--registry required")
	}
	os.MkdirAll(reg, 0o700)
	unlock := lock(reg)
	defer unlock()

	// prune stale reservations first
	ents, _ := os.ReadDir(reg)
	for _, e := range ents {
		if e.IsDir() || e.Name() == "lock" || !strings.HasSuffix(e.Name(), ".port") && !isNum(e.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(reg, e.Name()))
		if err == nil && stale(string(b)) {
			os.Remove(filepath.Join(reg, e.Name()))
		}
	}

	switch sub {
	case "reserve":
		if port == "" || pidS == "" {
			return die("BAD_ARGS", "reserve needs --port and --pid")
		}
		f := filepath.Join(reg, port)
		if _, err := os.Stat(f); err == nil {
			return die("PORT_TAKEN", "port "+port+" reserved")
		}
		rec := pidS + " " + strconv.FormatInt(time.Now().Unix(), 10) + " " + inst
		if lease != "" {
			if ls, err := strconv.ParseInt(lease, 10, 64); err == nil && ls > 0 {
				rec += " " + strconv.FormatInt(time.Now().Unix()+ls, 10)
			}
		}
		if err := os.WriteFile(f, []byte(rec), 0o600); err != nil {
			return die("PORT_PERSIST", err.Error())
		}
		return ok(map[string]any{"port": port})
	case "release":
		os.Remove(filepath.Join(reg, port))
		return ok(map[string]any{"port": port})
	case "check":
		if proto == "" {
			proto = "tcp"
		}
		if bind == "" {
			bind = ":"
		}
		addr := bind + port
		var err error
		if proto == "udp" {
			var pc net.PacketConn
			pc, err = net.ListenPacket("udp", addr)
			if err == nil {
				pc.Close()
			}
		} else {
			var ln net.Listener
			ln, err = net.Listen("tcp", addr)
			if err == nil {
				ln.Close()
			}
		}
		if err != nil {
			return die("PORT_BOUND", err.Error())
		}
		return ok(map[string]any{"port": port, "proto": proto, "bind": bind, "free": true})
	case "find":
		for p := lo; p <= hi; p++ {
			f := filepath.Join(reg, strconv.Itoa(p))
			if _, err := os.Stat(f); err == nil {
				continue
			}
			if proto == "" {
				proto = "tcp"
			}
			b := bind
			if b == "" {
				b = ":"
			}
			free := false
			if proto == "udp" {
				pc, err := net.ListenPacket("udp", b+strconv.Itoa(p))
				if err == nil {
					pc.Close()
					free = true
				}
			} else {
				ln, err := net.Listen("tcp", b+strconv.Itoa(p))
				if err == nil {
					ln.Close()
					free = true
				}
			}
			if !free {
				continue
			}
			fmt.Println(p)
			return 0
		}
		return die("PORT_RANGE_EXHAUSTED", "no free port")
	case "list":
		parts := []map[string]any{}
		for _, e := range ents {
			if b, err := os.ReadFile(filepath.Join(reg, e.Name())); err == nil && isNum(e.Name()) {
				parts = append(parts, map[string]any{
					"port": e.Name(), "owner": strings.TrimSpace(string(b))})
			}
		}
		return ok(map[string]any{"reservations": parts})
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
		return 2
	}
}

func isNum(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
