package main

import "testing"

// safeMember is the first tar-slip gate: absolute paths, drive
// letters, .. and runaway depth are rejected before IsLocal runs.
func TestSafeMember(t *testing.T) {
	bad := []string{
		"/etc/passwd", `\abs`, "c:/x", "C:\\x", "a:b",
		"..", "../x", "a/../../x", "a/../..",
		"1/2/3/4/5/6/7/8/9/10/11/12/13/14/15/16/17", // depth > maxDepth
	}
	for _, n := range bad {
		if safeMember(n) {
			t.Errorf("unsafe member accepted: %q", n)
		}
	}
	good := []string{
		"vm.conf", "a/b/c.qcow2", "a/../b", "./x",
		"1/2/3/4/5/6/7/8/9/10/11/12/13/14/15", // just under maxDepth
	}
	for _, n := range good {
		if !safeMember(n) {
			t.Errorf("safe member rejected: %q", n)
		}
	}
}
