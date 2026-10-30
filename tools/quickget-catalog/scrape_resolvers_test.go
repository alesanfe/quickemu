// scrape_resolvers_test.go — per-OS resolver coverage, batch-2 get_* ports (split of scrape2_test.go)
package main

import (
	"strings"
	"testing"
)

// --- Batch-2 resolver coverage (get_* ports) ---

func TestScrapeGaruda(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://iso.builds.garudalinux.org/iso/latest/garuda/dr460nized/latest.iso.sha256": "deadbeefcafe  garuda-dr460nized-linux-zen-240101.iso",
	})
	u, h, err := scrape("garuda", "", "dr460nized", "x86_64")
	if err != nil || u != "https://iso.builds.garudalinux.org/iso/latest/garuda/dr460nized/latest.iso" {
		t.Fatalf("garuda url=%q err=%v", u, err)
	}
	if h != "deadbeefcafe" {
		t.Fatalf("garuda hash=%q", h)
	}
}

func TestScrapeKdeneon(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://files.kde.org/neon/images/user/6.0/current/neon-6.0-user-current.sha256sum": "aaaabbbb  neon-user-6.0-20240101-1234.iso",
	})
	u, h, err := scrape("kdeneon", "6.0", "user", "x86_64")
	if err != nil || u != "https://files.kde.org/neon/images/user/6.0/current/neon-user-6.0-20240101-1234.iso" {
		t.Fatalf("kdeneon url=%q err=%v", u, err)
	}
	if h != "aaaabbbb" {
		t.Fatalf("kdeneon hash=%q", h)
	}
}

func TestScrapePclinuxos(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://ftp.fau.de/pclinuxos/pclinuxos/iso/":                                    `<a href="pclinuxos64-kde5-2024.01.iso">x</a><a href="pclinuxos64-kde5-2024.05.iso">y</a>`,
		"https://ftp.fau.de/pclinuxos/pclinuxos/iso/pclinuxos64-kde5-2024.05.iso.md5sum": "0123456789abcdef0123456789abcdef  pclinuxos64-kde5-2024.05.iso",
	})
	u, h, err := scrape("pclinuxos", "", "kde5", "x86_64")
	if err != nil || !strings.HasSuffix(u, "pclinuxos64-kde5-2024.05.iso") {
		t.Fatalf("pclinuxos url=%q err=%v", u, err)
	}
	if h != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("pclinuxos hash=%q", h)
	}
}

func TestScrapeRedoxos(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://static.redox-os.org/releases/0.9.0/x86_64/SHA256SUM": "hash111  redox_desktop_0.9.0_a_livedisk.iso.zst\nhash222  redox_desktop_0.9.0_b_livedisk.iso.zst",
	})
	u, h, err := scrape("redoxos", "0.9.0", "desktop", "x86_64")
	// tail -1 picks the LAST livedisk row.
	if err != nil || !strings.HasSuffix(u, "redox_desktop_0.9.0_b_livedisk.iso.zst") || h != "hash222" {
		t.Fatalf("redoxos u=%q h=%q err=%v", u, h, err)
	}
}

func TestScrapeSlax(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://ftp.fi.muni.cz/pub/linux/slax/Slax-12.x/md5.txt": "md5a  32bit-something\nmd5b  other file\nmd5c xx slax-64bit-12.2.iso",
	})
	u, h, err := scrape("slax", "", "debian", "x86_64")
	if err != nil || !strings.HasSuffix(u, "slax-64bit-12.2.iso") || h != "md5c" {
		t.Fatalf("slax u=%q h=%q err=%v", u, h, err)
	}
	// Unknown edition → no URL (map default branch).
	if u, _, _ := scrape("slax", "", "bogus", "x86_64"); u != "" {
		t.Fatalf("slax bogus edition url=%q", u)
	}
}

func TestScrapeKdelinux(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://files.kde.org/kde-linux/?C=M;O=D":   `<a href="kde-linux_202401011200.iso">a</a><a href="kde-linux_202403150800.iso">b</a>`,
		"https://files.kde.org/kde-linux/SHA256SUMS": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  kde-linux_202401011200.iso\nbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  kde-linux_202403150800.iso",
	})
	u, h, err := scrape("kdelinux", "", "", "x86_64")
	// indexISO picks the version-greatest match (sort -Vr | head -1
	// parity — the 12-digit stamps are version-comparable).
	if err != nil || !strings.HasSuffix(u, "kde-linux_202403150800.iso") {
		t.Fatalf("kdelinux u=%q err=%v", u, err)
	}
	if h != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("kdelinux h=%q", h)
	}
}

func TestScrapeKali(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://cdimage.kali.org/kali-2024.1/?C=M;O=D":   `<a href="kali-linux-2024.1-installer-amd64.iso">x</a>`,
		"https://cdimage.kali.org/kali-2024.1/SHA256SUMS": "torrhash  kali-linux-2024.1-installer-amd64.iso.torrent\nrealhash  kali-linux-2024.1-installer-amd64.iso",
	})
	u, h, err := scrape("kali", "kali-2024.1", "", "x86_64")
	if err != nil || !strings.HasSuffix(u, "kali-linux-2024.1-installer-amd64.iso") {
		t.Fatalf("kali u=%q err=%v", u, err)
	}
	if h != "realhash" {
		t.Fatalf("kali h=%q (torrent line must be skipped)", h)
	}
}

func TestScrapeBlissos(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://sourceforge.net/projects/blissos-x86/rss?path=/": `x<y>Official/BlissOS15/Gapps/Generic/Bliss-v15.9-alpha1.iso</y>` +
			`<y>Official/BlissOS16/Gapps/Generic/Bliss-v16.0-beta2.iso</y>`,
	})
	u, h, err := scrape("blissos", "16.0", "", "x86_64")
	if err != nil || u != "https://downloads.sourceforge.net/project/blissos-x86/Official/BlissOS16/Gapps/Generic/Bliss-v16.0-beta2.iso" || h != "" {
		t.Fatalf("blissos u=%q h=%q err=%v", u, h, err)
	}
}

func TestScrapeNobara(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://nobaraproject.org/download-nobara/": `href=Nobara-39-Official-2024-01-05.iso href=Nobara-39-Official-2024-03-01.iso`,
	})
	u, _, err := scrape("nobara", "39", "Official", "x86_64")
	if err != nil || u != "https://nobara-images.nobaraproject.org/Nobara-39-Official-2024-03-01.iso" {
		t.Fatalf("nobara u=%q err=%v", u, err)
	}
}

func TestScrapeQ4os(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://sourceforge.net/projects/q4os/rss?path=/stable": `<x>files/stable/q4os-5.6-x64.r12.iso</x><x>files/stable/q4os-5.6-x64.r15.iso</x>`,
	})
	u, _, err := scrape("q4os", "5.6", "kde", "x86_64")
	// "files/" prefix is stripped from the sourceforge download path.
	if err != nil || u != "https://downloads.sourceforge.net/project/q4os/stable/q4os-5.6-x64.r15.iso" {
		t.Fatalf("q4os u=%q err=%v", u, err)
	}
}

func TestScrapeBazzite(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://download.bazzite.gg/bazzite-stable-amd64.iso-CHECKSUM": "cafebabe  bazzite-stable-amd64.iso",
	})
	// plasma maps to bazzite-stable (edition name is NOT in the file).
	u, h, err := scrape("bazzite", "latest", "plasma", "x86_64")
	if err != nil || u != "https://download.bazzite.gg/bazzite-stable-amd64.iso" || h != "cafebabe" {
		t.Fatalf("bazzite u=%q h=%q err=%v", u, h, err)
	}
}

func TestScrapeBodhi(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://sourceforge.net/projects/bodhilinux/files/7.0.0/bodhi-7.0.0-64-hwe.iso.sha256": "deadbeef  bodhi-7.0.0-64-hwe.iso",
	})
	u, h, err := scrape("bodhi", "7.0.0", "hwe", "x86_64")
	if err != nil || u != "https://sourceforge.net/projects/bodhilinux/files/7.0.0/bodhi-7.0.0-64-hwe.iso" || h != "deadbeef" {
		t.Fatalf("bodhi hwe u=%q h=%q err=%v", u, h, err)
	}
	// standard drops the edition suffix entirely.
	u, _, _ = scrape("bodhi", "7.0.0", "standard", "x86_64")
	if !strings.HasSuffix(u, "bodhi-7.0.0-64.iso") {
		t.Fatalf("bodhi standard u=%q", u)
	}
}

func TestScrapeGnomeos(t *testing.T) {
	// 46.0 drops the minor in the iso name; the final URL comes from
	// web_redirect (finalURL seam).
	oldF := finalURL
	finalURL = func(u string) (string, error) { return u + "?dl", nil }
	t.Cleanup(func() { finalURL = oldF })
	u, h, err := scrape("gnomeos", "46.0", "", "x86_64")
	if err != nil || u != "https://download.gnome.org/gnomeos/46.0/gnome_os_installer_46.iso?dl" || h != "" {
		t.Fatalf("gnomeos u=%q h=%q err=%v", u, h, err)
	}
	u, _, _ = scrape("gnomeos", "nightly", "", "x86_64")
	if u != "https://os.gnome.org/download/latest/gnome_os_installer.iso?dl" {
		t.Fatalf("gnomeos nightly u=%q", u)
	}
}

func TestScrapeMxlinux(t *testing.T) {
	fakeFetch(t, map[string]string{
		"https://sourceforge.net/projects/mx-linux/files/Final/Fluxbox/MX-25.3_fluxbox_x64.iso.sha256": "aabbccdd  MX-25.3_fluxbox_x64.iso",
	})
	u, h, err := scrape("mxlinux", "25.3", "Fluxbox", "x86_64")
	// Fluxbox is lowercase in the filename (not %EDITION%).
	if err != nil || u != "https://sourceforge.net/projects/mx-linux/files/Final/Fluxbox/MX-25.3_fluxbox_x64.iso" || h != "aabbccdd" {
		t.Fatalf("mxlinux u=%q h=%q err=%v", u, h, err)
	}
}

func TestScrapePeppermint(t *testing.T) {
	sha512 := strings.Repeat("ab", 64)
	fakeFetch(t, map[string]string{
		"https://sourceforge.net/projects/peppermintos/files/isos/Gnome_FlashBack/PeppermintOS-Debian_64_gfb.iso-sha512.checksum": sha512 + "  PeppermintOS-Debian_64_gfb.iso",
	})
	u, h, err := scrape("peppermint", "latest", "debian-gnome", "x86_64")
	if err != nil || u != "https://sourceforge.net/projects/peppermintos/files/isos/Gnome_FlashBack/PeppermintOS-Debian_64_gfb.iso" || h != sha512 {
		t.Fatalf("peppermint u=%q h=%q err=%v", u, h, err)
	}
}

func TestScrapeFreedos(t *testing.T) {
	// 1.4: verify.txt has multiple hash sections; only the sha256sum
	// block counts (grep -A8 sha256sum | grep ISO).
	verify := "md5sum:\nmd5aaa  FD14-LiveCD.zip\nsha1sum:\nsha111  FD14-LiveCD.zip\n" +
		"sha256sum:\nline filler\nmore filler\npad\npad2\npad3\npad4\n256hex  FD14-LiveCD.zip"
	fakeFetch(t, map[string]string{
		"https://www.ibiblio.org/pub/micro/pc-stuff/freedos/files/distributions/1.4/verify.txt": verify,
	})
	u, h, err := scrape("freedos", "1.4", "", "x86_64")
	if err != nil || u != "https://www.ibiblio.org/pub/micro/pc-stuff/freedos/files/distributions/1.4/FD14-LiveCD.zip" {
		t.Fatalf("freedos 1.4 u=%q err=%v", u, err)
	}
	if h != "256hex" {
		t.Fatalf("freedos hash must come from sha256sum section, got %q", h)
	}
	// Unknown release → empty (case table has no default).
	if u, _, _ := scrape("freedos", "9.9", "", "x86_64"); u != "" {
		t.Fatalf("freedos unknown rel u=%q", u)
	}
}

func TestScrapeTinycore(t *testing.T) {
	fakeFetch(t, map[string]string{
		"http://www.tinycorelinux.net/15.x/x86_64/release/TinyCorePure64-15.0.iso.md5.txt": "md5hash  TinyCorePure64-15.0.iso",
	})
	u, h, err := scrape("tinycore", "15", "TinyCorePure64", "x86_64")
	// *Pure* edition → x86_64 path even though the guest arch is amd64.
	if err != nil || u != "http://www.tinycorelinux.net/15.x/x86_64/release/TinyCorePure64-15.0.iso" || h != "md5hash" {
		t.Fatalf("tinycore u=%q h=%q err=%v", u, h, err)
	}
	u, _, _ = scrape("tinycore", "15", "Core", "x86_64")
	if !strings.Contains(u, "/x86/release/") {
		t.Fatalf("non-Pure edition must stay x86: %q", u)
	}
}

func TestScrapeAndroid(t *testing.T) {
	settings := `x var settings = {"pool":{"f":[
		{"n":"android-x86_64-9.0-r2.iso","hash":{"sha256":"andsha"}},
		{"n":"other.iso","hash":{"sha256":"nope"}}
	]}}; </script>`
	fakeFetch(t, map[string]string{
		"https://www.fosshub.com/Android-x86-old.html":       settings,
		"https://mirrors.gigenet.com/OSDN/android-x86":       `<a href="71930">d</a><a href="80000">e</a>`,
		"https://mirrors.gigenet.com/OSDN/android-x86/80000": "contains android-x86_64-9.0-r2.iso here",
	})
	u, h, err := scrape("android", "9.0-r2", "x86_64", "x86_64")
	if err != nil || u != "https://mirrors.gigenet.com/OSDN/android-x86/80000/android-x86_64-9.0-r2.iso" || h != "andsha" {
		t.Fatalf("android u=%q h=%q err=%v", u, h, err)
	}
}
