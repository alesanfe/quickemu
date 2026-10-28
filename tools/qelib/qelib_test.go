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
