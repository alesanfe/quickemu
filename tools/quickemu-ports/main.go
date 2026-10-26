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
	"syscall"
	"time"
)

const version = "0.1.0"

func die(c, m string) { fmt.Printf(`{"success":false,"errors":[{"code":"%s","message":"%s"}]}`+"\n", c, m); os.Exit(1) }
func ok(d string)   { fmt.Printf(`{"success":true,"data":%s}`+"\n", d); os.Exit(0) }

func stale(meta string) bool { // "pid ts instance" -> owner dead?
	f := strings.Fields(meta)
	if len(f) == 0 {
		return true
	}
	p, _ := strconv.Atoi(f[0])
	return p > 0 && syscall.Kill(p, 0) != nil
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

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-ports <reserve|release|list|check|find> --registry <dir> [--port N] [--pid P] [--instance I] [--lo N --hi N]")
		os.Exit(2)
	}
	sub := os.Args[1]
	var reg, port, pidS, inst, proto, bind string
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
		}
	}
	if reg == "" {
		die("BAD_ARGS", "--registry required")
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
			die("BAD_ARGS", "reserve needs --port and --pid")
		}
		f := filepath.Join(reg, port)
		if _, err := os.Stat(f); err == nil {
			die("PORT_TAKEN", "port "+port+" reserved")
		}
		os.WriteFile(f, []byte(pidS+" "+strconv.FormatInt(time.Now().Unix(), 10)+" "+inst), 0o600)
		ok(`{"port":` + port + `}`)
	case "release":
		os.Remove(filepath.Join(reg, port))
		ok(`{"port":` + port + `}`)
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
			die("PORT_BOUND", err.Error())
		}
		ok(`{"port":` + port + `,"proto":"` + proto + `","bind":"` + bind + `","free":true}`)
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
			return
		}
		die("PORT_RANGE_EXHAUSTED", "no free port")
	case "list":
		var parts []string
		for _, e := range ents {
			if b, err := os.ReadFile(filepath.Join(reg, e.Name())); err == nil && isNum(e.Name()) {
				parts = append(parts, fmt.Sprintf(`{"port":%s,"owner":"%s"}`, e.Name(), strings.TrimSpace(string(b))))
			}
		}
		ok(`{"reservations":[` + strings.Join(parts, ",") + `]}`)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
		os.Exit(2)
	}
}

func isNum(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
