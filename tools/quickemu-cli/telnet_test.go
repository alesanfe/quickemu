package main

import (
	"bytes"
	"testing"
)

func feedTelnet(t *testing.T, in []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	telnetCopy(&out, bytes.NewReader(in))
	return out.Bytes()
}

// Regression: WILL (0xFB) and friends take an option byte — the
// off-by-one let that byte leak into the terminal stream.
func TestTelnetCopyStripsIACCommands(t *testing.T) {
	in := []byte{
		'h', 'i',
		0xFF, 0xFB, 0x01, // IAC WILL ECHO
		'h', 'o',
		0xFF, 0xFD, 0x03, // IAC DO SGA
		0xFF, 0xFE, 0x05, // IAC DONT
		0xFF, 0xFC, 0x22, // IAC WONT
		'!',
	}
	got := feedTelnet(t, in)
	if !bytes.Equal(got, []byte("hiho!")) {
		t.Fatalf("option bytes leaked: %q", got)
	}
}

// Escaped IAC (IAC IAC) is a literal 0xFF in the data stream.
func TestTelnetCopyEscapedIAC(t *testing.T) {
	got := feedTelnet(t, []byte{'a', 0xFF, 0xFF, 'b'})
	want := []byte{'a', 0xFF, 'b'}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Subnegotiation blocks (SB ... IAC SE) are consumed whole.
func TestTelnetCopySubnegotiation(t *testing.T) {
	in := []byte{
		'x',
		0xFF, 0xFA, 0x18, 't', 'e', 'r', 'm', 0xFF, 0xF0, // IAC SB TTYPE term IAC SE
		'y',
	}
	got := feedTelnet(t, in)
	if !bytes.Equal(got, []byte("xy")) {
		t.Fatalf("subnegotiation leaked: %q", got)
	}
}

// Two-byte commands (GA etc., no option byte) end the command
// immediately — the next byte is data again.
func TestTelnetCopyTwoByteCommand(t *testing.T) {
	got := feedTelnet(t, []byte{0xFF, 0xF9, 'z'}) // IAC GA
	if !bytes.Equal(got, []byte("z")) {
		t.Fatalf("two-byte command ate data: %q", got)
	}
}

func TestTelnetCopyEOFMidCommand(t *testing.T) {
	// Truncated IAC at EOF must not spin or crash.
	got := feedTelnet(t, []byte{'a', 0xFF})
	if !bytes.Equal(got, []byte("a")) {
		t.Fatalf("got %q", got)
	}
	got = feedTelnet(t, []byte{'a', 0xFF, 0xFB}) // truncated WILL
	if !bytes.Equal(got, []byte("a")) {
		t.Fatalf("got %q", got)
	}
}
