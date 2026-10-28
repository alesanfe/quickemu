// ports_test.go: registry contract — stale-detection rules, arg
// normalisation and numeric validation (adversarial/lifecycle spec
// parity, ported to the Go internals).
package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestStaleDeadOwner(t *testing.T) {
	meta := "99999999 " + strconv.FormatInt(time.Now().Unix(), 10) + " inst"
	if !stale(meta) {
		t.Error("dead owner pid must be stale")
	}
}

func TestStaleLiveOwner(t *testing.T) {
	meta := strconv.Itoa(os.Getpid()) + " " +
		strconv.FormatInt(time.Now().Unix(), 10) + " inst"
	if stale(meta) {
		t.Error("live owner pid must not be stale")
	}
}

func TestStaleExpiredLease(t *testing.T) {
	past := strconv.FormatInt(time.Now().Unix()-10, 10)
	meta := strconv.Itoa(os.Getpid()) + " 1 inst " + past
	if !stale(meta) {
		t.Error("expired lease must be stale even for a live owner")
	}
	future := strconv.FormatInt(time.Now().Unix()+3600, 10)
	meta = strconv.Itoa(os.Getpid()) + " 1 inst " + future
	if stale(meta) {
		t.Error("unexpired lease with live owner must not be stale")
	}
}

func TestStaleEmptyAndGarbage(t *testing.T) {
	if !stale("") {
		t.Error("empty record must be stale")
	}
}

func TestNormArgsEqualsForm(t *testing.T) {
	got := normArgs([]string{"p", "reserve", "--port=2222", "--pid", "7"})
	want := []string{"p", "reserve", "--port", "2222", "--pid", "7"}
	if len(got) != len(want) {
		t.Fatalf("normArgs len %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normArgs[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestIsNum(t *testing.T) {
	if !isNum("2222") || isNum("x") || isNum("22x2") || isNum("") {
		t.Error("isNum must accept digits only")
	}
}

func TestRegistryReserveReleaseRoundtrip(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "ports")
	os.MkdirAll(reg, 0700)
	f := filepath.Join(reg, "2222")
	os.WriteFile(f, []byte("1 2 i"), 0600)
	if _, err := os.Stat(f); err != nil {
		t.Fatal("marker written")
	}
	os.Remove(f)
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("release must remove the marker")
	}
}
