// scrape_test.go — scraper paths with fetchString/finalURL replaced by a
// URL→body fixture map (no network). Exercises the alpine YAML, archlinux
// JSON, vanillaos releases, ubuntu SHA256/MD5 sum parsing, releases/
// editions scrapers and the pure version-sort helpers.
package main

import (
	"errors"
	"strings"
	"testing"
)

// fakeFetch installs a URL-keyed fetcher and restores on cleanup.
func fakeFetch(t *testing.T, pages map[string]string) {
	t.Helper()
	old := fetchString
	fetchString = func(u string) (string, error) {
		if b, ok := pages[u]; ok {
			return b, nil
		}
		return "", errors.New("no such url: " + u)
	}
	t.Cleanup(func() { fetchString = old })
}

func TestScrapeAlpine(t *testing.T) {
	yaml := `
- flavor: alpine-virt
  arch: x86_64
  iso: alpine-virt-3.20.0-x86_64.iso
  sha256: ` + strings.Repeat("a", 64) + `
- flavor: alpine-standard
  arch: x86_64
  iso: alpine-standard-3.20.0-x86_64.iso
  sha256: ` + strings.Repeat("b", 64)
	pages := map[string]string{
		"https://dl-cdn.alpinelinux.org/alpine/3.20/releases/x86_64/latest-releases.yaml": yaml,
	}
	fakeFetch(t, pages)
	u, h, err := scrape("alpine", "3.20", "", "amd64")
	if err != nil || !strings.HasSuffix(u, "alpine-virt-3.20.0-x86_64.iso") || len(h) != 64 {
		t.Fatalf("alpine u=%q h=%q err=%v", u, h, err)
	}
	// Missing flavor → error path.
	pages["https://dl-cdn.alpinelinux.org/alpine/3.20/releases/x86_64/latest-releases.yaml"] = "- flavor: none\n"
	if _, _, err = scrape("alpine", "3.20", "", "amd64"); err == nil {
		t.Fatal("expected flavor error")
	}
	if _, _, err = scrape("bogus-os", "", "", ""); err == nil {
		t.Fatal("expected unimplemented error")
	}
}

func TestScrapeArchlinux(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://archlinux.org/releng/releases/json/": `{"releases":[
		  {"iso_url":"https://geo.mirror/arch-2025.iso","sha256_sum":"` + strings.Repeat("c", 64) + `"}]}`,
	})
	u, h, err := scrape("archlinux", "", "", "amd64")
	if err != nil || u == "" || len(h) != 64 {
		t.Fatalf("archlinux %q %q %v", u, h, err)
	}
}

func TestScrapeVanillaos(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://api.github.com/repos/Vanilla-OS/live-iso/releases": `[
		  {"browser_download_url":"https://dl/vanilla-2.0.iso"}]`,
	})
	u, _, err := scrape("vanillaos", "2.0", "", "amd64")
	if err != nil || !strings.Contains(u, "vanilla-2.0") {
		t.Fatalf("vanillaos %q %v", u, err)
	}
}

func TestScrapeUbuntuFlavour(t *testing.T) {
	sums := "aaa *ubuntu-24.04-desktop-amd64.iso\n" +
		"bbb *ubuntu-24.04-desktop-amd64+mac.iso\n"
	fakeFetch(t, map[string]string{
		"https://releases.ubuntu.com/24.04/SHA256SUMS": sums,
	})
	u, h, err := scrape("ubuntu", "24.04", "", "amd64")
	if err != nil || !strings.HasSuffix(u, "ubuntu-24.04-desktop-amd64.iso") || h != "aaa" {
		t.Fatalf("ubuntu %q %q %v", u, h, err)
	}
	// Server via MD5SUMS fallback (field-3 iso, field-1 hash).
	fakeFetch(t, map[string]string{
		"https://releases.ubuntu.com/16.04/MD5SUMS": "xyz abc ubuntu-16.04-server-amd64.iso\n",
	})
	u, h, err = scrape("ubuntu-server", "16.04", "", "amd64")
	if err != nil || !strings.Contains(u, "server-amd64") {
		t.Fatalf("server %q %q %v", u, h, err)
	}
	// No sums reachable → unavailable error.
	fakeFetch(t, map[string]string{})
	if _, _, err = scrape("ubuntu", "99.99", "", "amd64"); err == nil {
		t.Fatal("expected unavailable")
	}
}

func TestScrapeReleasesEditions(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://www.pureos.net/download/": `"https://a-downloads.puri.sm-x-y-v10"`,
		"https://ftp.halifax.rwth-aachen.de/blackarch/iso/": `
		  <a href="blackarch-linux-full-2023-x86_64.iso">x</a>
		  <a href="blackarch-linux-slim-2023-x86_64.iso">y</a>`,
	})
	if r := scrapeReleases("pureos"); len(r) == 0 {
		t.Fatal("pureos releases empty")
	}
	if r := scrapeEditions("blackarch", "2023"); len(r) < 2 {
		t.Fatalf("blackarch editions %v", r)
	}
	if r := scrapeEditions("blackarch", ""); r != nil {
		t.Fatal("empty release must be nil")
	}
	if r := scrapeReleases("no-such"); r != nil {
		t.Fatal("unknown releases must be nil")
	}
}

func TestVersionHelpers(t *testing.T) {
	if got := latestMatch("v1.2 v1.10 v1.9", `v[0-9.]+`); got != "v1.10" {
		t.Fatalf("latestMatch %q", got)
	}
	if !versionLess("1.9", "1.10") || versionLess("1.10", "1.9") {
		t.Fatal("versionLess numeric")
	}
	if !versionLess("a", "b") || versionLess("1", "a") == versionLess("a", "1") {
		t.Fatal("versionLess lexicographic")
	}
	if !isNum("12") || isNum("1a") || atoi("42") != 42 {
		t.Fatal("isNum/atoi")
	}
	if got := matchAll("a1 b2 c3", `([a-z][0-9])`); len(got) != 3 {
		t.Fatalf("matchAll %v", got)
	}
}
