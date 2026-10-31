package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// FuzzSafeMember: the tar-slip predicate is a security boundary — it
// must never panic, and any name it accepts combined with
// filepath.IsLocal must resolve inside the destination root.
func FuzzSafeMember(f *testing.F) {
	seeds := []string{
		"a/b/c.qcow2", "../escape", "..\\win32", "/etc/passwd",
		"a/../../b", "C:evil", "....//x", ".", "..", "x/./y",
		"a/../b", "deep/" + strings.Repeat("a/", 20) + "f",
		"\x00nul", "name\nwith\nnewline", "trail/",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	root := "/vmroot"
	f.Fuzz(func(t *testing.T, name string) {
		ok := safeMember(name)
		if !ok {
			return
		}
		if !filepath.IsLocal(name) {
			t.Fatalf("safeMember accepted non-local %q", name)
		}
		// Defense in depth: the joined path must stay under root.
		joined := filepath.Join(root, name)
		rel, err := filepath.Rel(root, joined)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("member %q escapes root -> %q", name, joined)
		}
	})
}
