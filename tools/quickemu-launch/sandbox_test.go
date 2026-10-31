// sandbox_test.go — QE-100: seccomp sandbox emit + resume conflict.
package main

import (
	"strings"
	"testing"
)

func TestSandboxArgs(t *testing.T) {
	// off/empty → nothing.
	for _, v := range []string{"", "off"} {
		if a := sandboxArgs(Conf{"sandbox": v}); a != nil {
			t.Fatalf("sandbox=%q → %v", v, a)
		}
	}
	// on → strict minus spawn.
	a := sandboxArgs(Conf{"sandbox": "on"})
	j := strings.Join(a, " ")
	if !strings.Contains(j, "-sandbox on,") || strings.Contains(j, "spawn=deny") {
		t.Fatalf("on args: %v", a)
	}
	// strict → spawn=deny included.
	a = sandboxArgs(Conf{"sandbox": "strict"})
	if !strings.Contains(strings.Join(a, " "), "spawn=deny") {
		t.Fatalf("strict args: %v", a)
	}
	// strict + resume_from relaxes spawn (exec: transport).
	a = sandboxArgs(Conf{"sandbox": "strict", "resume_from": "/x.state.gz"})
	if strings.Contains(strings.Join(a, " "), "spawn=deny") {
		t.Fatalf("strict+resume args: %v", a)
	}
	// unknown → nil + warning.
	if a := sandboxArgs(Conf{"sandbox": "bogus"}); a != nil {
		t.Fatalf("bogus → %v", a)
	}
}
