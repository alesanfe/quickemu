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

func TestScrapeCrunchbangPP(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://api.github.com/repos/CBPP/cbpp/releases": `[
		  {"browser_download_url":"https://github.com/CBPP/cbpp/releases/download/v12.0/cbpp-12.0-amd64-20230611.iso"},
		  {"browser_download_url":"https://github.com/CBPP/cbpp/releases/download/v12.0/cbpp-12.0-i386-20230611.iso"},
		  {"browser_download_url":"https://github.com/CBPP/cbpp/releases/download/v10/cbpp-10.1-amd64-20190713.iso"}]`,
	})
	u, h, err := scrape("crunchbang++", "10.1", "", "amd64")
	if err != nil || !strings.HasSuffix(u, "cbpp-10.1-amd64-20190713.iso") || h != "" {
		t.Fatalf("crunchbang++ %q %q %v", u, h, err)
	}
	if u, _, err = scrape("crunchbang++", "latest", "", "amd64"); err != nil ||
		!strings.Contains(u, "cbpp-12.0-amd64") {
		t.Fatalf("crunchbang++ latest %q %v", u, err)
	}
	if _, _, err = scrape("crunchbang++", "999", "", "amd64"); err == nil {
		t.Fatal("crunchbang++ unknown release must fail")
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

// rescued_test cases — the 25 OSes whose catalog entries had no url and
// no scrape flag (unresolvable); each asserts its get_<os> parity URL.
func TestScrapeRescuedStatic(t *testing.T) {
	cases := []struct{ os, rel, ed, want string }{
		{"altlinux", "11", "workstation", "ftp.altlinux.org/pub/distributions/ALTLinux/images/p11/workstation/x86_64/alt-workstation-11.2-x86_64.iso"},
		{"altlinux", "11", "kworkstation", "kworkstation/alt-kworkstation-11-live-x86_64.iso"},
		{"blankon", "sinambung", "", "jahitan.blankonlinux.id/releases/current/blankon-sinambung-26.09-verbeek-amd64.iso"},
		{"bosslinux", "8.0", "", "archiveos/files/b/boss/boss-8.0-amd64-DVD-301019.iso/download"},
		{"canaima", "9.0", "full", "iso/wakupe/canaima-9.0-full-beta_amd64.iso"},
		{"educaandos", "v2", "", "EducaAndosV2-offline.iso"},
		{"educaandos", "20.04", "", "educaandos-20.04-instalador-centros.iso"},
		{"educaandos", "20.04-2025021001", "", "educaandos-20.04-instalador-centros-2025021001.iso"},
		{"esfera", "1.1.0", "", "guadalinex-v9-32B/esfera-live-1.1.0.iso"},
		{"fedoracore", "1", "", "core/1/i386/iso/yarrow-i386-disc1.iso"},
		{"fedoracore", "5", "", "core/5/i386/iso/FC5-i386-disc1.iso"},
		{"gecos", "v2", "lite", "GECOS-v2-lite-32B-20141017.iso"},
		{"gecos", "v2", "standard", "GECOS-v2-32B-latest.iso"},
		{"kurumin", "7.0", "", "kurumin/kurumin-7.0.iso/download"},
		{"kurumin", "7.0", "light", "kurumin-light-7.0.iso/download"},
		{"kurumin", "8.06", "", "kurumin-ng_8.06.iso/download"},
		{"lernstick", "13", "exam", "lernstick_pruefungsumgebung_debian13_latest.iso"},
		{"lernstick", "13", "edu", "lernstick_debian13_edu_latest.iso"},
		{"life", "22.04", "", "Li-f-e-22.04.3.amd64.iso"},
		{"life", "24.04", "", "Life-24.04.amd64.iso"},
		{"lliurex", "23", "desktop", "isos/23/LliureX-desktop_64bits_23_latest.iso"},
		{"lliurex", "21.07", "desktop-fp", "21.07_64bits/20220406/LliureX-desktop-fp_64bits_21_20220406.iso"},
		{"lliurex", "19.07", "escriptori-fp", "19.07_64bits/lliurex-Escriptori-fp_64bits_19_latest.iso"},
		{"lliurex", "16.06", "mini", "16.06_64bits/lliurex-mini_64bits_16_latest.iso"},
		{"lliurex", "25", "", "isos/25/LliureX-home-desktop_25_latest.iso"},
		{"mangaka", "chu", "", "linux-mangaka/linux-mangaka-chu-v2.iso"},
		{"metamorphose", "8.5.7", "", "MetamorphosePuma/Metamorphose_8.5.7-Panther-x64.iso/download"},
		{"nova", "7.0", "escritorio", "nova/nova-escritorio-7.0-amd64.iso/download"},
		{"openkylin", "3.0", "server", "openKylin-Server-V3.0-20260916-x86_64.iso"},
		{"openkylin", "3.0", "desktop", "openKylin-Desktop-V3.0-20260918.1-x86_64.iso"},
		{"plan9", "4e", "", "plan9isosUS/plan9-4e-latest.iso"},
		{"plan9", "2e", "", "plan9isosUS/plan9-2e.iso"},
		{"uwuntu", "22.10", "", "uw-untu-22.10-desktop-amd-64_202212/UwUntu-22.10-desktop-amd64.iso"},
		{"wifislax", "4.12", "", "wifislax-4-9-final/wifislax-4-12-final%20%5BPIXIESCRIPT%5D.iso"},
		{"wifislax", "4.11.1", "", "Wifislax4.11.1/Wifislax-4.11.1.iso"},
		{"wifislax", "4.3", "", "wifislax-4-3-fin/wifislax-4-3-fin.iso"},
		{"wifislax", "4.10.1", "", "wifislax-4-10-1-final_202406/wifislax-4-10-1-final.iso"},
	}
	for _, c := range cases {
		u, _, err := scrape(c.os, c.rel, c.ed, "amd64")
		if err != nil || !strings.HasSuffix(u, c.want) && !strings.Contains(u, c.want) {
			t.Fatalf("%s %s %s → %q %v (want contains %q)", c.os, c.rel, c.ed, u, err, c.want)
		}
	}
	if _, _, err := scrape("altlinux", "11", "bogus", "amd64"); err == nil {
		t.Fatal("altlinux bogus edition must fail")
	}
	// canaima 8.4 carries its pinned md5.
	_, h, err := scrape("canaima", "8.4", "", "amd64")
	if err != nil || h != "99c1820f7412d9b20cf81843f898ec1c" {
		t.Fatalf("canaima 8.4 md5 %q %v", h, err)
	}
}

func TestScrapeFedora(t *testing.T) {
	body := `[
	  {"variant":"Workstation","subvariant":"Workstation","arch":"x86_64","version":"44","link":"https://dl/f44-ws.iso","sha256":"ws"},
	  {"variant":"Spins","subvariant":"LXQt","arch":"x86_64","version":"43","link":"https://dl/f43-lxqt.iso","sha256":"sp"},
	  {"variant":"KDE","subvariant":"KDE","arch":"x86_64","version":"44","link":"https://dl/f44-kde.iso","sha256":"kde"},
	  {"variant":"Workstation","subvariant":"Workstation","arch":"aarch64","version":"44","link":"https://dl/f44-ws-arm.iso","sha256":"arm"}
	]`
	fakeFetch(t, map[string]string{"https://getfedora.org/releases.json": body})
	u, h, err := scrape("fedora", "44", "Workstation", "amd64")
	if err != nil || u != "https://dl/f44-ws.iso" || h != "ws" {
		t.Fatalf("fedora ws %q %q %v", u, h, err)
	}
	// Spins variant resolution.
	if u, _, err = scrape("fedora", "43", "LXQt", "amd64"); err != nil || !strings.HasSuffix(u, "f43-lxqt.iso") {
		t.Fatalf("fedora spin %q %v", u, err)
	}
	// KDE is an edition ≥42.
	if u, _, err = scrape("fedora", "44", "KDE", "amd64"); err != nil || !strings.HasSuffix(u, "f44-kde.iso") {
		t.Fatalf("fedora kde %q %v", u, err)
	}
	// arm64 maps to aarch64.
	if u, _, err = scrape("fedora", "44", "Workstation", "arm64"); err != nil || !strings.HasSuffix(u, "ws-arm.iso") {
		t.Fatalf("fedora arm %q %v", u, err)
	}
	// _Beta unescapes to a space for the version match.
	fakeFetch(t, map[string]string{"https://getfedora.org/releases.json": `[{"variant":"Workstation","subvariant":"Workstation","arch":"x86_64","version":"45 Beta","link":"https://dl/b.iso","sha256":"b"}]`})
	if u, _, err = scrape("fedora", "45_Beta", "Workstation", "amd64"); err != nil || !strings.HasSuffix(u, "b.iso") {
		t.Fatalf("fedora beta %q %v", u, err)
	}
	if _, _, err = scrape("fedora", "99", "Workstation", "amd64"); err == nil {
		t.Fatal("fedora 99 must fail")
	}
}

func TestScrapeAPIScrapers(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://www.mageia.org/en/downloads/get/?q=Mageia-9-Live-Plasma-x86_64.iso":          `<a href="https://mirror/Mageia-9-Live-Plasma-x86_64.iso">click here</a>`,
		"https://mirror/Mageia-9-Live-Plasma-x86_64.iso.sha512":                               strings.Repeat("c", 128) + "  iso",
		"https://api.pop-os.org/builds/22.04/intel":                                           `{"url":"https://iso/pop.iso","sha_sum":"popsha"}`,
		"https://meta.cdn.soulharsh007.dev/RebornOS-ISO?format=json":                          `{"url":"https://cdn/rb.iso","md5":"rbmd5"}`,
		"https://api.github.com/repos/Security-Onion-Solutions/securityonion/releases/latest": `{"tag_name": "3.3.0"}`,
		"https://tails.boum.org/install/v2/Tails/amd64/stable/latest.json":                    `{"installations":[{"installation-paths":[{"type":"iso","target-files":[{"url":"https://dl/tails.iso","sha256":"tsha"}]}]}]}`,
	})
	cases := []struct{ os, rel, ed, want string }{
		{"mageia", "9", "Plasma", "mirror/Mageia-9-Live-Plasma-x86_64.iso"},
		{"popos", "22.04", "intel", "iso/pop.iso"},
		{"rebornos", "latest", "", "cdn/rb.iso"},
		{"securityonion", "latest", "", "securityonion/securityonion-3.3.0.iso"},
		{"tails", "stable", "", "tails.iso"},
	}
	for _, c := range cases {
		u, _, err := scrape(c.os, c.rel, c.ed, "amd64")
		if err != nil || !strings.Contains(u, c.want) {
			t.Fatalf("%s → %q %v", c.os, u, err)
		}
	}
	// error paths
	fakeFetch(t, map[string]string{})
	for _, c := range cases {
		if _, _, err := scrape(c.os, c.rel, c.ed, "amd64"); err == nil {
			t.Fatalf("%s without fixtures must fail", c.os)
		}
	}
}
