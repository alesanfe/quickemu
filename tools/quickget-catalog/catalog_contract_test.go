// catalog_contract_test.go: adapter_contract_spec.sh parity — the
// data catalog is the releases_*/get_* function table now, so every
// OS must carry a non-empty release list and a resolvable URL source
// (template, iso or live scrape), with no duplicate entries.
package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := loadCatalog(filepath.Join("..", "..", "catalog", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return c
}

func TestEveryOSHasReleases(t *testing.T) {
	c := testCatalog(t)
	// Live-scraped release lists (scrapeReleases) are legitimately
	// empty in the static catalog — every other OS must carry a
	// static list (releases_<os> parity).
	live := map[string]bool{"pureos": true, "rockylinux": true, "slitaz": true}
	for _, e := range c.Entries {
		if len(e.Releases) == 0 && !live[e.OS] {
			t.Errorf("%s: no releases and no scraper", e.OS)
		}
	}
}

func TestEveryOSHasURLSource(t *testing.T) {
	c := testCatalog(t)
	// Native downloaders live in quickget-vm (ubuntu pipeline,
	// Windows MSDL, macOS chunklists) — the same exceptions the Bash
	// contract spec made for *ubuntu*/windows-server/macos.
	native := func(os string) bool {
		return strings.HasPrefix(os, "ubuntu") || os == "windows-server" ||
			os == "windows" || os == "macos"
	}
	for _, e := range c.Entries {
		if e.Scrape || native(e.OS) {
			continue
		}
		if e.URL == "" && e.ISO == "" && e.HashURL == "" {
			t.Errorf("%s: no url/iso/hash_url and not a scraper", e.OS)
		}
	}
}

func TestNoDuplicateOS(t *testing.T) {
	c := testCatalog(t)
	seen := map[string]bool{}
	for _, e := range c.Entries {
		if seen[e.OS] {
			t.Errorf("duplicate catalog entry: %s", e.OS)
		}
		seen[e.OS] = true
	}
}

func TestListShapes(t *testing.T) {
	c := testCatalog(t)
	for _, e := range c.Entries {
		for _, l := range append(append([]string{}, e.Releases...), e.Editions...) {
			if strings.ContainsAny(l, "\r\n") {
				t.Errorf("%s: list item %q contains CR/LF", e.OS, l)
			}
		}
		if len(e.Arch) == 0 {
			t.Errorf("%s: no arch", e.OS)
		}
	}
}
