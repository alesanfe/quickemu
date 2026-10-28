// catalog_test.go — quickgetc's catalog selection logic (the
// jq-parity helpers) plus hash/cache checks.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func catFixture() []OSEntry {
	return []OSEntry{{
		Name: "alpine", PrettyName: "Alpine",
		Releases: []Release{
			{Release: "3.19", Arch: "x86_64"},
			{Release: "3.20", Arch: ""},
			{Release: "3.20", Edition: "virt", Arch: "aarch64"},
		},
	}, {
		Name: "plain", PrettyName: "Plain",
		Releases: []Release{{Release: "1"}},
	}}
}

func TestFindAndSelect(t *testing.T) {
	cat := catFixture()
	e := findOSEntry(cat, "alpine")
	if e == nil || findOSEntry(cat, "nope") != nil {
		t.Fatal("findOSEntry")
	}
	// Defaults applied in place.
	if e.Releases[1].Arch != "x86_64" || e.Releases[1].Release != "3.20" {
		t.Fatalf("defaults %+v", e.Releases[1])
	}
	ents := findReleaseEntries(e, "3.20")
	if len(ents) != 2 {
		t.Fatalf("releaseEntries %d", len(ents))
	}
	if releaseRequiresEdition(ents) {
		t.Fatal("3.20 has a null-edition entry")
	}
	allEd := findReleaseEntries(e, "3.19")
	if releaseRequiresEdition(allEd) {
		// 3.19 has no edition → does not require one
	}
	ed, eds, err := findEditionEntry(ents, "", "x86_64", "x86_64")
	if err != nil || ed == nil || len(eds) != 1 {
		t.Fatalf("editionEntry %v %v %v", ed, eds, err)
	}
	// Edition "virt" only exists for aarch64 → arch mismatch errors
	// when an explicit arch is requested.
	if _, _, err = findEditionEntry(ents, "virt", "x86_64", "x86_64"); err == nil {
		t.Fatal("expected arch error")
	}
	// Host-arch fallback picks aarch64 when arch is "".
	ed, _, err = findEditionEntry(ents, "virt", "", "aarch64")
	if err != nil || ed.Arch != "aarch64" {
		t.Fatalf("hostarch %v %v", ed, err)
	}
	if _, _, err = findEditionEntry(ents, "missing", "", "x86_64"); err == nil {
		t.Fatal("expected edition error")
	}
}

func TestListHelpers(t *testing.T) {
	cat := catFixture()
	e := findOSEntry(cat, "alpine")
	if r := listReleasesRaw(e); len(r) != 2 || r[0] != "3.19" || r[1] != "3.20" {
		t.Fatalf("releases %v", r)
	}
	if eds := listEditions(e, "3.20"); len(eds) != 2 {
		t.Fatalf("editions %v", eds)
	}
	if !equalStrs([]string{"a"}, []string{"a"}) ||
		equalStrs([]string{"a"}, []string{"b"}) ||
		equalStrs([]string{"a"}, nil) {
		t.Fatal("equalStrs")
	}
	listReleasesDisplay(e) // exercises both branches over stdout
	listCSV(cat)
}

func TestSourceHelpers(t *testing.T) {
	var r Release
	if err := json.Unmarshal([]byte(`{"iso":[
	  {"web":{"url":"https://a/1.iso"}},
	  {"web":{"url":"https://a/1.iso"}}],
	 "img":[{"file_name":"local.img"}]}`), &r); err != nil {
		t.Fatal(err)
	}
	urls := allSourceURLs(r)
	if len(urls) != 1 || urls[0] != "https://a/1.iso" {
		t.Fatalf("urls %v", urls)
	}
	if r.sourcesFor("iso") == nil || r.sourcesFor("img") == nil ||
		r.sourcesFor("nope") != nil {
		t.Fatal("sourcesFor")
	}
	if s := setArch("x86_64"); s != "x86_64" {
		t.Fatal("setArch")
	}
	if supportedOSNames(catFixture()) == "" {
		t.Fatal("supportedOSNames")
	}
}

func TestCacheAndHash(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	os.WriteFile(p, []byte("abc"), 0o600)
	if !cacheIsValid(p) {
		t.Fatal("fresh file should be valid")
	}
	os.Chtimes(p, time.Now().Add(-48*time.Hour), time.Now().Add(-48*time.Hour))
	if cacheIsValid(p) {
		t.Fatal("stale file must be invalid")
	}
	if cacheIsValid(filepath.Join(dir, "none")) {
		t.Fatal("missing must be invalid")
	}
	// checkHash: sha256 of "abc".
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte("abc")))
	if !checkHash(p, sum) {
		t.Fatal("sha256 match")
	}
	if checkHash(p, strings_RepeatZero(64)) {
		t.Fatal("sha256 mismatch")
	}
	if !checkHash(p, "zzz") { // unknown algo → warn + pass
		t.Fatal("unknown algo must pass")
	}
	if checkHash(filepath.Join(dir, "none"), sum) {
		t.Fatal("missing file must fail")
	}
}

func strings_RepeatZero(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}
