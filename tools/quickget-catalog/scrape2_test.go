// scrape2_test.go — offline scrape() coverage: fetchString/finalURL/
// webCheck seams serve canned bodies shaped like each upstream feed.
package main

import (
	"fmt"
	"strings"
	"testing"
)

const shaX = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// bodyFor returns a canned page keyed on URL shape, so every scraper
// branch exercises its real parsing path.
func bodyFor(url string) (string, error) {
	switch {
	case strings.Contains(url, "latest-releases.yaml"):
		return fmt.Sprintf("- flavor: alpine-virt\n  arch: x86_64\n  iso: alpine-virt-3.19.0-x86_64.iso\n  sha256: %s\n- flavor: alpine-virt\n  arch: aarch64\n  iso: alpine-virt-3.19.0-aarch64.iso\n  sha256: %s\n- flavor: alpine-standard\n  iso: other.iso\n", shaX, shaX), nil
	case strings.Contains(url, "releng/releases/json"):
		return fmt.Sprintf(`{"releases":[{"iso_url":"archlinux-2024.01.01-x86_64.iso","sha256_sum":"%s"}]}`, shaX), nil
	case strings.Contains(url, "file-info.json"):
		return `{"official":{"kde":{"image":"https://m/manjaro-kde.iso"},"kde.minimal":{"image":"https://m/m.iso"}},"community":{"cinnamon":{"image":"https://m/c.iso"}}}`, nil
	case strings.Contains(url, "manjaro-sway"):
		return `[{"name":"manjaro-sway-1.0.iso","url":"https://m/sway.iso"}]`, nil
	case strings.Contains(url, "rss"):
		return `<rss><link>https://x/Release/ISO/nitrux-2.0.iso</link>
			<link>https://x/files/release/ROME/omv-plasma6x11.x86_64.iso</link></rss>`, nil
	case strings.Contains(url, "api.github.com"):
		return `[{"browser_download_url":"https://gh/VanillaOS-2.0-oracular.iso"}]`, nil
	case strings.Contains(url, "SHA"), strings.Contains(url, "sha256"),
		strings.Contains(url, "md5"), strings.Contains(url, "MD5"),
		strings.Contains(url, "SHASUMS"), strings.Contains(url, "sha512"),
		strings.Contains(url, "checksums"), strings.Contains(url, "sha256sum"),
		strings.Contains(url, "sha256sums"):
		h := shaX + "  "
		return h + "artix-xfce-2024-x86_64.iso\n" + h + "void-live-x86_64-1-base.iso a b " + shaX +
			"\na-b-c-d-20240101-x x86_64-LIVE\n" + h + "chimera linux x86_64-LIVE gnome\n" + h +
			"siduction-x.iso.md5\nx y z " + shaX + "\n" + h + "debian-live-12.5.0-amd64-xfce.iso\n" +
			h + "Zorin-OS-17.1-Core-64-bit.iso\n" + shaX + "\n", nil
	case strings.Contains(url, "README"):
		return shaX + "  antiX-23-runit_x64-full.iso\n", nil
	case strings.Contains(url, "debian-cd") && strings.HasSuffix(url, "/"):
		return `<a href="12.5.0/">x</a>`, nil
	case strings.Contains(url, "rocky"):
		return `<a class="link" href="9.3/">9.3/</a><a class="link" href="8.9/">8.9/</a><a class="link" href="full/">full/</a>`, nil
	case strings.Contains(url, "slitaz"):
		return `<a class='iso' href='slitaz-rolling-core64.iso'>slitaz-rolling-core64.iso</a><a class='iso'>slitaz-rolling-core.iso</a>`, nil
	case strings.Contains(url, "pureos"):
		return `"https://downloads.puri.sm/pureos-10.3-librem-2024-x/"`, nil
	case strings.Contains(url, "princeton"), strings.Contains(url, "dotsrc"):
		return `<a class="folder" href="siduction-24.01/">x</a><a href="x-1.0-y-z-20240101.iso.md5">m</a>`, nil
	case strings.Contains(url, "kirkstone"):
		return `<a href="2025/">x</a><a href="2024/">y</a>`, nil
	case strings.Contains(url, "truenas"), strings.Contains(url, "sys.truenas"):
		return `<a href="24.04.1/">v</a><a href="TrueNAS-SCALE-24.04.1.iso">i</a>`, nil
	case strings.Contains(url, "kicksecure"):
		return `<a href="17.9.9/">v</a><a href="Kicksecure-CLI-17.9.9.Intel_AMD64.qcow2.libvirt.xz">i</a>`, nil
	default:
		// generic index page: iso links + version dirs + current.iso
		return `<a href="backbox-8-desktop-amd64.iso">b</a>
			<a href="batocera-x86_64-40.img.gz">bt</a>
			<a href="EndeavourOS-mercury.iso">e</a>
			<a href="9front-10000.amd64.iso">9</a>
			<a href="9front-9000.amd64.iso">9</a>
			<a href="pentoo-full-2024x.iso">p</a>
			<a href="tsurugi_linux_2.0.iso">t</a>
			<a href="vitalinux-3.5-2024-desktop-amd64.iso">v</a>
			<a href="TUXEDO-OS-2-20240101-current.iso">tx</a>
			<a href="Zorin-OS-17.1-Core-64-bit.iso">z</a>
			<a href="MAX-13.0.iso">mx</a>
			<a href="cachyos-x86_64.iso">c</a>
			&ISO/desktop;x;https://m/cachyos-desktop.iso;y
			`, nil
	}
}

func stubNet(t *testing.T) {
	t.Helper()
	oldF, oldU, oldW := fetchString, finalURL, webCheck
	fetchString = bodyFor
	finalURL = func(u string) (string, error) { return u + "?download=1", nil }
	webCheck = func(string) bool { return true }
	t.Cleanup(func() { fetchString, finalURL, webCheck = oldF, oldU, oldW })
}

func TestScrapeAll(t *testing.T) {
	stubNet(t)
	cases := []struct {
		os, rel, ed, arch string
		wantErr           bool
	}{
		{"alpine", "3.19", "", "amd64", false},
		{"alpine", "3.19", "", "arm64", false},
		{"archlinux", "latest", "", "amd64", false},
		{"artixlinux", "2024", "xfce", "amd64", false},
		{"cachyos", "1", "desktop", "amd64", false},
		{"cachyos", "1", "nope", "amd64", true},
		{"manjaro", "kde", "", "amd64", false},
		{"manjaro", "kde", "minimal", "amd64", false},
		{"manjaro", "sway", "", "amd64", false},
		{"manjaro", "cinnamon", "", "amd64", false},
		{"reactos", "latest", "", "amd64", false},
		{"9front", "latest", "", "amd64", false},
		{"archcraft", "v1", "", "amd64", false},
		{"azurelinux", "3.0", "", "amd64", false},
		{"azurelinux", "3.0", "", "arm64", false},
		{"backbox", "8", "", "amd64", false},
		{"batocera", "40", "", "amd64", false},
		{"endeavouros", "mercury", "", "amd64", false},
		{"kicksecure", "latest", "", "amd64", false},
		{"maboxlinux", "v1", "", "amd64", false},
		{"maxlinux", "13.0", "", "amd64", false},
		{"pentoo", "latest", "full", "amd64", false},
		{"pentoo", "latest", "nope", "amd64", true},
		{"tsurugi", "1.0", "", "amd64", false},
		{"tuxedo-os", "2", "", "amd64", false},
		{"vitalinux", "3.5", "", "amd64", false},
		{"chimeralinux", "latest", "gnome", "amd64", false},
		{"siduction", "24.01", "kde", "amd64", false},
		{"antix", "23", "full-runit", "amd64", false},
		{"antix", "21", "runit-base", "amd64", false},
		{"debian", "12.5.0", "xfce", "amd64", false},
		{"debian", "12.5.0", "netinst", "amd64", false},
		{"debian", "12.5.0", "xfce", "arm64", true},
		{"debian", "12.5.0", "netinst", "arm64", false},
		{"deepin", "23", "", "amd64", false},
		{"deepin", "20.9", "", "amd64", false},
		{"devuan", "daedalus", "", "amd64", false},
		{"easyos", "5.0", "", "amd64", false},
		{"elementary", "8.1", "", "amd64", false},
		{"elementary", "7.0", "", "amd64", false},
		{"endless", "5.1", "en", "amd64", false},
		{"endless", "5.1", "base", "amd64", false},
		{"hamonikr", "8", "lts", "amd64", false},
		{"hamonikr", "8", "hwe", "amd64", false},
		{"nitrux", "latest", "", "amd64", false},
		{"nyarch", "24", "kde", "amd64", false},
		{"nyarch", "24", "nv-gnome", "amd64", false},
		{"openmandriva", "rome", "plasma", "amd64", false},
		{"openmandriva", "rome", "gnome", "amd64", true},
		{"opensuse", "tumbleweed", "", "amd64", false},
		{"opensuse", "microos", "", "amd64", false},
		{"opensuse", "15.1", "", "amd64", false},
		{"opensuse", "16.0", "", "amd64", false},
		{"opensuse", "15.6", "", "amd64", false},
		{"os2warp", "3", "", "amd64", false},
		{"os2warp", "2.1", "", "amd64", false},
		{"os2warp", "4", "", "amd64", false},
		{"recreomatic", "9", "", "amd64", false},
		{"pardus", "23", "xfce", "amd64", false},
		{"pureos", "10", "gnome", "amd64", false},
		{"rosalinux", "13", "kde", "amd64", false},
		{"slint", "15.0-5", "", "amd64", false},
		{"truenas-scale", "cobia", "", "amd64", false},
		{"ubuntu-server", "24.04", "", "amd64", true},
		{"vanillaos", "2.0-oracular", "", "amd64", false},
		{"vanillaos", "nope", "", "amd64", true},
		{"void", "20240314", "glibc", "amd64", false},
		{"void", "20240314", "musl", "amd64", false},
		{"void", "20240314", "xfce-glibc", "amd64", false},
		{"void", "20240314", "xfce-musl", "amd64", false},
		{"whonix", "latest", "", "amd64", false},
		{"zorin", "17", "core64", "amd64", false},
		{"zorin", "17", "lite64", "amd64", true},
		{"notaos", "1", "", "amd64", true},
	}
	for _, c := range cases {
		t.Run(c.os+"/"+c.rel+"/"+c.ed, func(t *testing.T) {
			u, sha, err := scrape(c.os, c.rel, c.ed, c.arch)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got %s", u)
				}
				return
			}
			if err != nil || u == "" {
				t.Fatalf("err=%v url=%q sha=%q", err, u, sha)
			}
		})
	}
}

func TestScrapeReleasesAndEditions(t *testing.T) {
	stubNet(t)
	if r := scrapeReleases("pureos"); len(r) == 0 || r[0] != "x/" {
		t.Fatalf("pureos rels: %v", r)
	}
	if r := scrapeReleases("rockylinux"); len(r) != 2 || r[0] != "8.9" {
		t.Fatalf("rocky rels: %v", r)
	}
	if r := scrapeReleases("slitaz"); len(r) == 0 {
		t.Fatal("slitaz rels empty")
	}
	if r := scrapeReleases("nope"); r != nil {
		t.Fatalf("unknown: %v", r)
	}
	if e := scrapeEditions("x", ""); e != nil {
		t.Fatal("empty release → nil")
	}
	if _, err := ubuntuDailyCodename("ubuntu"); err == nil {
		// offline → error is fine, but no panic
		t.Log("ubuntuDailyCodename ok")
	}
}
