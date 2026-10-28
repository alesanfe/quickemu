package qelib

import (
	"os"
	"reflect"
	"testing"
)

func TestNormArgs(t *testing.T) {
	in := []string{"tool", "--port=2222", "--pid", "9", "-x", "pos"}
	want := []string{"tool", "--port", "2222", "--pid", "9", "-x", "pos"}
	if got := NormArgs(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("NormArgs = %v, want %v", got, want)
	}
	// In-place reuse would clobber unread elements — ensure the
	// original slice is untouched.
	if in[1] != "--port=2222" || in[2] != "--pid" {
		t.Fatalf("input mutated: %v", in)
	}
}

func TestAlive(t *testing.T) {
	if Alive(0) || Alive(-1) {
		t.Fatal("non-positive pid reported alive")
	}
	if !Alive(os.Getpid()) {
		t.Fatal("own pid reported dead")
	}
	if Alive(1 << 30) {
		t.Fatal("impossible pid reported alive")
	}
}

func TestFindTool(t *testing.T) {
	if _, err := FindTool("definitely-not-a-real-tool-xyz"); err == nil {
		t.Fatal("missing tool resolved")
	}
	if p, err := FindTool("go"); err != nil || p == "" {
		t.Fatalf("PATH lookup failed: %v %v", p, err)
	}
}

func TestMemMB(t *testing.T) {
	for in, want := range map[string]int{"4G": 4096, "512M": 512, "256K": 0, "128": 128, "junk": 0, "": 0} {
		if got := MemMB(in); got != want {
			t.Fatalf("MemMB(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestHostArch(t *testing.T) {
	if a := HostArch(); a != "x86_64" && a != "aarch64" {
		t.Fatalf("HostArch = %q", a)
	}
}

func TestQMPCmd(t *testing.T) {
	if got := QMPCmd("query-status", nil); got != `{"execute":"query-status"}` {
		t.Fatalf("QMPCmd nil = %s", got)
	}
	got := QMPCmd("device_add", map[string]any{"driver": "usb-host", "id": "x"})
	if got != `{"arguments":{"driver":"usb-host","id":"x"},"execute":"device_add"}` {
		t.Fatalf("QMPCmd args = %s", got)
	}
}

func TestCmdName(t *testing.T) {
	os.Args = []string{"prog", "reserve"}
	if got := CmdName("ports"); got != "ports.reserve" {
		t.Fatalf("CmdName = %q", got)
	}
	os.Args = []string{"prog"}
	if got := CmdName("ports"); got != "ports" {
		t.Fatalf("CmdName = %q", got)
	}
}
