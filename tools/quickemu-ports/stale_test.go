package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// Regression: a record whose pid field is unparseable or zero used
// to survive stale() forever (p<=0 was "unknown", not "dead"), so a
// corrupt record leaked the port reservation permanently.
func TestStaleCorruptPidReclaimed(t *testing.T) {
	for _, meta := range []string{
		"abc 1700000000 vm1", // non-numeric pid
		"0 1700000000 vm1",   // zero pid
		"-5 1700000000 vm1",  // negative pid
		"garbage",            // single field
		"  ",                 // whitespace only
	} {
		if !stale(meta) {
			t.Errorf("corrupt record %q must be stale", meta)
		}
	}
}

// A live owner (this test process) with an unexpired lease is NOT
// stale even if the record is otherwise odd.
func TestStaleLivePidWithFutureLease(t *testing.T) {
	future := time.Now().Add(time.Hour).Unix()
	meta := fmt.Sprintf("%d %d vm1 %d", os.Getpid(), time.Now().Unix(), future)
	if stale(meta) {
		t.Error("live owner + unexpired lease must not be stale")
	}
}

// A live owner whose lease already expired IS stale.
func TestStaleLivePidExpiredLease(t *testing.T) {
	past := time.Now().Add(-time.Hour).Unix()
	meta := fmt.Sprintf("%d %d vm1 %d", os.Getpid(), time.Now().Unix(), past)
	if !stale(meta) {
		t.Error("expired lease must be stale even with a live owner")
	}
}
