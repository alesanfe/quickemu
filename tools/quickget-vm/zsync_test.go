package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression: the original inline rewrite replaced the FIRST
// occurrence of "https" anywhere in the URL — a mirror path like
// ".../https/mirror.iso" got mangled. Only the scheme may change.
func TestZsyncURLSchemeOnlyRewrite(t *testing.T) {
	cases := map[string]string{
		"https://releases.ubuntu.com/24.04/x.iso":   "http://releases.ubuntu.com/24.04/x.iso.zsync",
		"https://mirror.example/https/backup/x.iso": "http://mirror.example/https/backup/x.iso.zsync",
		"http://mirror.example/x.iso":               "http://mirror.example/x.iso.zsync",
		"https://cdn.example/a?b=https://evil":      "http://cdn.example/a?b=https://evil.zsync",
	}
	for in, want := range cases {
		if got := zsyncURL(in); got != want {
			t.Errorf("zsyncURL(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

// cryptSalt must produce exactly n random characters from a safe
// alphabet — it lands in autoinstall YAML unquoted.
func TestCryptSaltAlphabetAndLen(t *testing.T) {
	const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./"
	for _, n := range []int{0, 1, 16, 64} {
		s := cryptSalt(n)
		if len(s) != n {
			t.Fatalf("cryptSalt(%d) len=%d", n, len(s))
		}
		for _, c := range s {
			ok := false
			for _, a := range alpha {
				if c == a {
					ok = true
					break
				}
			}
			if !ok {
				t.Fatalf("cryptSalt emitted %q outside the crypt alphabet", c)
			}
		}
	}
	if cryptSalt(16) == cryptSalt(16) {
		t.Fatal("cryptSalt must not repeat")
	}
}

// findISO removes `old` (the consumed source image) then returns the
// first *.iso left in the dir — or `old` when nothing remains.
func TestFindISOReplacesOld(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"old.iso", "new.iso"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := findISO(dir, "old.iso"); got != "new.iso" {
		t.Fatalf("findISO=%q want new.iso", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.iso")); !os.IsNotExist(err) {
		t.Fatal("old.iso must be removed")
	}
	// No *.iso left → the old name comes back for messaging parity.
	if got := findISO(dir, "new.iso"); got != "new.iso" {
		t.Fatalf("findISO empty-dir=%q want new.iso", got)
	}
}
