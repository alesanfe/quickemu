// shares_test.go — QE-060/061: multi-share parsing, validation and
// 9p emit; [[share]] TOML flattening.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShareEntriesFlatConf(t *testing.T) {
	c := Conf{
		"share_dirs":    "/a /b",
		"share_tags":    "tagA",
		"share_in_dirs": "/inbox",
	}
	got := shareEntries(c)
	if len(got) != 3 {
		t.Fatalf("entries %v", got)
	}
	if got[0].Tag != "tagA" || got[0].Dir != "out" {
		t.Fatalf("first entry %+v", got[0])
	}
	if got[1].Dir != "out" || got[1].Tag != "" {
		t.Fatalf("untagged entry %+v", got[1])
	}
	if got[2].Dir != "in" || got[2].Tag != "qe-inbox" {
		t.Fatalf("inbox entry %+v", got[2])
	}
}

func TestValidateShares(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	missing := filepath.Join(t.TempDir(), "nope")
	c := Conf{
		// bad tag skipped, dup tag skipped, missing dir skipped —
		// two good dirs survive.
		"share_dirs": d1 + " " + d2 + " " + missing + " " + d1,
		"share_tags": "t1  bad!tag",
	}
	got := validateShares(c)
	// d2 drops on its bad tag; the repeated d1 survives under its
	// basename (only the tag, not the path, must be unique).
	if len(got) != 2 {
		t.Fatalf("validated %v", got)
	}
	if got[0].Tag != "t1" || got[1].Tag != filepath.Base(d1) {
		t.Fatalf("tags %+v", got)
	}
}

func TestExtraShareArgsEmit(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	c := Conf{
		"share_dirs":    d1,
		"share_tags":    "media",
		"share_in_dirs": d2,
	}
	a := extraShareArgs(c)
	j := strings.Join(a, " ")
	if !strings.Contains(j, "id=fsdev1,path="+d1) ||
		!strings.Contains(j, "mount_tag=media") {
		t.Fatalf("share1 args: %s", j)
	}
	if !strings.Contains(j, "id=fsdev2,path="+d2) ||
		!strings.Contains(j, "mount_tag=qe-inbox") {
		t.Fatalf("inbox args: %s", j)
	}
	// Comma paths escape to ,, inside the QEMU option list — the
	// auto-derived tag would carry the comma too, so tag explicitly.
	c2 := Conf{"share_dirs": filepath.Join(d1, "sub,dir"), "share_tags": "valid"}
	os.Mkdir(filepath.Join(d1, "sub,dir"), 0o755)
	a = extraShareArgs(c2)
	if !strings.Contains(strings.Join(a, " "), "sub,,dir") {
		t.Fatalf("comma escape missing: %v", a)
	}
}

func TestFlattenSharesToml(t *testing.T) {
	c := Conf{}
	flattenShares(c, []any{
		map[string]any{"path": "/x", "tag": "media"},
		map[string]any{"path": "/y"},
		map[string]any{"path": "/in", "direction": "in"},
		map[string]any{"tag": "no-path"},
		"not-a-table",
	})
	if c["share_dirs"] != "/x /y" {
		t.Fatalf("share_dirs %q", c["share_dirs"])
	}
	if c["share_tags"] != "media" {
		t.Fatalf("share_tags %q", c["share_tags"])
	}
	if c["share_in_dirs"] != "/in" {
		t.Fatalf("share_in_dirs %q", c["share_in_dirs"])
	}
}

func TestParseTomlConfShares(t *testing.T) {
	f := filepath.Join(t.TempDir(), "vm.toml")
	os.WriteFile(f, []byte(`guest_os="linux"
[[share]]
path="/x"
tag="media"
[[share]]
path="/in"
direction="in"
`), 0o644)
	c := parseTomlConf(f)
	if c["share_dirs"] != "/x" || c["share_tags"] != "media" ||
		c["share_in_dirs"] != "/in" || c["guest_os"] != "linux" {
		t.Fatalf("conf %v", c)
	}
}
