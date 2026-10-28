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
	case strings.Contains(url, "Athena-OS"):
		return `[{"browser_download_url":"https://gh/athenaos-live-x86_64.iso"},{"browser_download_url":"https://gh/athenaos-live-x86_64.iso.sha256"}]`, nil
	case strings.Contains(url, "install-media"):
		return `[{"browser_download_url":"https://gh/chimeraos-2025.04.21-x86_64.iso"},{"browser_download_url":"https://gh/sha256sum.txt"}]`, nil
	case strings.Contains(url, "harvester"):
		return `{"tag_name":"v1.9.0"}`, nil
	case strings.Contains(url, "api.github.com"):
		return `[{"browser_download_url":"https://gh/VanillaOS-2.0-oracular.iso"},
			{"browser_download_url":"https://gh/k3os-amd64.iso"},
			{"browser_download_url":"https://gh/haos_ova-14.0.ova"},
			{"browser_download_url":"https://gh/hello-0.9.0-amd64.iso"},
			{"browser_download_url":"https://gh/kairos-3.0-amd64.iso"},
			{"browser_download_url":"https://gh/metal-amd64.iso"},
			{"browser_download_url":"https://gh/image.iso"},
			{"browser_download_url":"https://gh/dahliaOS-24.04.img"},
			{"browser_download_url":"https://gh/zimaos-1.5.0.img"},
			{"browser_download_url":"https://gh/crystal-1.0.iso"}]`, nil
	case strings.Contains(url, "broadcom.com"):
		return `<a href="photon-5.0-dde71ec57.x86_64.iso">x</a><a href="photon-minimal-5.0-dde71ec57.x86_64.iso">y</a><a href="photon-5.0-dde71ec57.x86_64.debug.iso">z</a>`, nil
	case strings.Contains(url, "bandshed"):
		return `<a href="AVL_MXe-25.2_x64.iso">x</a><a href="AVL_MXe-25.2_x64.iso.sha256">s</a>`, nil
	case strings.Contains(url, "draugeros"):
		return `<a href="Drauger_OS-7.7-AMD64.iso">a</a><a href="Drauger_OS-7.8-AMD64.iso">b</a>`, nil
	case strings.Contains(url, "primeos"):
		return `"name":"PrimeOS-2.1.3-64-bit-20220719-BETA.iso" "name":"PrimeOS-2.1.2-64-bit-20220531-BETA.iso"`, nil
	case strings.Contains(url, "gparted-live-stable/1"):
		return `"name":"gparted-live-1.8.1-6-amd64.iso"`, nil
	case strings.Contains(url, "gparted"):
		return `"name":"1.8.1-6" "name":"1.8.1-3" "name":"gparted-live-stable"`, nil
	case strings.Contains(url, "mocaccino/files/v"):
		return `"name":"MocaccinoOS-MATE-0.20261019.iso" "name":"MocaccinoOS-KDE-0.20261019.iso" "name":"MocaccinoOS-MATE-0.20261019.iso.sha256"`, nil
	case strings.Contains(url, "mocaccino"):
		return `"name":"v26.10" "name":"v26.09"`, nil
	case strings.Contains(url, "loc-os"):
		return `"name":"Loc-OS-24-current_i686.iso"`, nil
	case strings.Contains(url, "release/2"):
		return `<a href="cldx-20261022-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "calculate-linux"):
		return `<a href="20261022/">d</a><a href="20261019/">d</a>`, nil
	case strings.Contains(url, "centos.org/8"):
		return `<a href="CentOS-8.5.2111-x86_64-dvd1.iso">x</a>`, nil
	case strings.Contains(url, "centos.org/5"):
		return `<a href="CentOS-5.11-x86_64-bin-1of9.iso">x</a>`, nil
	case strings.Contains(url, "centos"):
		return `<a href="CentOS-6.10-x86_64-bin-DVD1.iso">x</a>`, nil
	case strings.Contains(url, "core204"):
		return `<a href="ipfire-2.29-core204-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "freedif"):
		return `<a href="2.29-core204/">d</a><a href="2.29-core203/">d</a>`, nil
	case strings.Contains(url, "nluug"):
		return `<a href="KNOPPIX_V9.1CD-2021-01-25-EN.iso">x</a>`, nil
	case strings.Contains(url, "minix3"):
		return `<a href="minix_R3.3.0-588a35b.iso.bz2">x</a>`, nil
	case strings.Contains(url, "neptuneos"):
		return `<a href="Neptune9-20260314.iso">x</a>`, nil
	case strings.Contains(url, "openmamba"):
		return `<a href="https://cdn.openmamba.org/pub/openmamba/media/rolling/livecd/en/openmamba-livecd-en-snapshot-20261021.x86_64.iso">x</a>`, nil
	case strings.Contains(url, "yunohost"):
		return `<a href="yunohost-bookworm-12.0-amd64-stable.iso">x</a>`, nil
	case strings.Contains(url, "amazonlinux"):
		return `<a href="al2023-kvm-2023.12.20261026.1-kernel-6.1-x86_64.xfs.gpt.qcow2">x</a>`, nil
	case strings.Contains(url, "dietpi"):
		return `<a href="DietPi_NativePC-BIOS-x86_64-Trixie_Installer.img.xz">x</a>`, nil
	case strings.Contains(url, "euro-linux"):
		return `<a href="EL-9-x86_64-latest-appstream.iso">x</a>`, nil
	case strings.Contains(url, "hurd-amd64/2"):
		return `<a href="debian-sid-hurd-amd64-NETINST-1.iso">x</a>`, nil
	case strings.Contains(url, "hurd-amd64"):
		return `<a href="20260314/">d</a>`, nil
	case strings.Contains(url, "leaseweb"):
		return `<a href="grml-full-2026.09-amd64.iso">x</a>`, nil
	case strings.Contains(url, "omnios"):
		return `<a href="omnios-r151058.iso">x</a>`, nil
	case strings.Contains(url, "enterprise.proxmox"):
		return `<a href="proxmox-backup-server_4.0-1.iso">x</a><a href="proxmox-mailgateway_8.1.iso">y</a>`, nil
	case strings.Contains(url, "images/rpd"):
		return `<a href="2024-11-19-rpd-x86.iso">x</a>`, nil
	case strings.Contains(url, "rpd_x86"):
		return `<a href="rpd_x86-2024-11-19/">d</a>`, nil
	case strings.Contains(url, "tribblix"):
		return `<a href="https://iso.tribblix.org/iso/tribblix-0m41.1.iso">x</a>`, nil
	case strings.Contains(url, "xcp-ng"):
		return `<a href="xcp-ng-8.3-20250606.iso">x</a>`, nil
	case strings.Contains(url, "fyralabs"):
		return `<a href="ultramarine-42-x86_64-flagship.iso">x</a>`, nil
	case strings.Contains(url, "coreos.fedoraproject"):
		return `{"architectures":{"x86_64":{"artifacts":{"qemu":{"formats":{"qcow2.xz":{"disk":{"location":"https://fcos/qemu.qcow2.xz","sha256":"fcos"}}}}}}}}`, nil
	case strings.Contains(url, "xigmanas"):
		return `"name":"XigmaNAS-x64-13.0.0.5.iso"`, nil
	case strings.Contains(url, "salix"):
		return `"name":"salixlive64-xfce-15.0.iso"`, nil
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
		// second-expansion resolvers (ghISOsha/ghLatestTag/sfLatestDir/…).
		{"athenaos", "latest", "", "amd64", false},
		{"chimeraos", "latest", "", "amd64", false},
		{"harvester", "latest", "", "amd64", false},
		{"mocaccinoos", "latest", "mate", "amd64", false},
		{"mocaccinoos", "latest", "kde", "amd64", false},
		{"mocaccinoos", "latest", "bogus", "amd64", true},
		{"photonos", "5.0", "", "amd64", false},
		{"avlinux", "latest", "", "amd64", false},
		{"drauger-os", "latest", "", "amd64", false},
		{"primeos", "latest", "", "amd64", false},
		{"gparted-live", "latest", "", "amd64", false},
		{"loc-os", "23", "", "amd64", false},
		{"calculate-linux", "latest", "", "amd64", false},
		{"centos", "8.5.2111", "", "amd64", false},
		{"centos", "5.11", "", "amd64", false},
		{"ipfire", "latest", "", "amd64", false},
		{"knoppix", "9.1", "", "amd64", false},
		{"minix", "3.3.0", "", "amd64", false},
		{"neptune", "9", "", "amd64", false},
		{"openmamba", "latest", "", "amd64", false},
		{"umbrelos", "latest", "", "amd64", false},
		{"yunohost", "latest", "", "amd64", false},
		{"crystal-linux", "latest", "", "amd64", false},
		{"dahliaos", "latest", "", "amd64", false},
		{"haos", "latest", "", "amd64", false},
		{"hellosystem", "latest", "", "amd64", false},
		{"k3os", "latest", "", "amd64", false},
		{"kairos", "latest", "", "amd64", false},
		{"taloslinux", "latest", "", "amd64", false},
		{"toaruos", "latest", "", "amd64", false},
		{"zimaos", "latest", "", "amd64", false},
		{"bsdrp", "latest", "", "amd64", false},
		{"feren-os", "latest", "", "amd64", false},
		{"linuxfx", "latest", "", "amd64", false},
		{"4mlinux", "latest", "", "amd64", false},
		{"xigmanas", "latest", "", "amd64", false},
		{"salix", "15.0", "", "amd64", false},
		{"amazonlinux", "2023", "", "amd64", false},
		{"dietpi", "latest", "", "amd64", false},
		{"eurolinux", "9", "", "amd64", false},
		{"gnu-hurd", "latest", "", "amd64", false},
		{"grml", "latest", "", "amd64", false},
		{"omnios", "stable", "", "amd64", false},
		{"opensuse-microos", "latest", "", "amd64", false},
		{"proxmox-backup-server", "4.0-1", "", "amd64", false},
		{"proxmox-mail-gateway", "8.1", "", "amd64", false},
		{"raspberrypios-desktop", "latest", "", "amd64", false},
		{"steamos", "alchemist", "", "amd64", false},
		{"tribblix", "latest", "", "amd64", false},
		{"xcp-ng", "8.3", "", "amd64", false},
		{"ultramarine", "42", "", "amd64", false},
		{"fedora-coreos", "stable", "", "amd64", false},
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
	if name, err := ubuntuDailyCodename("ubuntu"); err == nil {
		// offline → error is fine, but a success must return a
		// non-empty codename — not just "did not panic".
		if name == "" {
			t.Error("ubuntuDailyCodename succeeded with empty codename")
		}
	}
}
