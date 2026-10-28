package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// telnetAttach runs a minimal RFC-854 client against QEMU's
// telnet: monitor/serial endpoints — strips inbound IAC negotiation,
// doubles outbound 0xFF. Replaces the external `telnet` binary.
func telnetAttach(host, port string) int {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! "+err.Error())
		return 1
	}
	defer c.Close()
	done := make(chan struct{})
	go func() { // QEMU → stdout, IAC-filtered
		telnetCopy(os.Stdout, c)
		close(done)
	}()
	go func() { // stdin → QEMU, IAC-escaped
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			for i := 0; i < n; i++ {
				if buf[i] == 0xFF {
					c.Write([]byte{0xFF, 0xFF})
				} else {
					c.Write(buf[i : i+1])
				}
			}
			if err != nil {
				return
			}
		}
	}()
	<-done
	return 0
}

// telnetCopy filters IAC command sequences from the inbound stream:
// IAC IAC → literal 0xFF; IAC <cmd> <opt> (3 bytes); IAC SB … IAC SE.
func telnetCopy(w io.Writer, r io.Reader) {
	const (
		iac = 0xFF
		sb  = 0xFA
		se  = 0xF0
	)
	state := 0 // 0=data, 1=saw-IAC, 2=in-subnegotiation, 3=SB+saw-IAC
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		for i := 0; i < n; i++ {
			b := buf[i]
			switch state {
			case 0:
				if b == iac {
					state = 1
				} else {
					w.Write(buf[i : i+1])
				}
			case 1:
				switch {
				case b == iac:
					w.Write([]byte{iac})
					state = 0
				case b == sb:
					state = 2
				case b >= se && b < 0xFB:
					state = 0 // 2-byte commands (SE..GA 0xF0-0xFA) — WILL/WONT/DO/DONT (0xFB+) take an option byte
				default:
					state = 4 // WILL/DO/DONT/WONT — skip option byte
				}
			case 2: // in subnegotiation
				if b == iac {
					state = 3
				}
			case 3:
				state = 2
				if b == se {
					state = 0
				}
			case 4: // option byte after WILL/DO/DONT/WONT
				state = 0
			}
		}
		if err != nil {
			return
		}
	}
}
