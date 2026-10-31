// scrape_cloud_extra_test.go — offline coverage for the QE-041 cloud
// catalog resolvers (ubuntu/arch/rocky/alma/opensuse/centos-stream) and
// the debian-cloud edition split, plus catalog-entry metadata checks
// for the new qcow2 image_type entries. No network: fakeFetch serves
// fixture index pages and checksum manifests.
package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestScrapeUbuntuCloud(t *testing.T) {
	base := "https://cloud-images.ubuntu.com/releases/24.04/release/"
	fakeFetch(t, map[string]string{
		base: `<a href="ubuntu-24.04-server-cloudimg-amd64.img">x</a>
			<a href="ubuntu-24.04-server-cloudimg-amd64-azure.img">x</a>
			<a href="ubuntu-24.04-server-cloudimg-amd64-disk-kvm.img">x</a>
			<a href="ubuntu-24.04.1-server-cloudimg-amd64.img">x</a>
			<a href="ubuntu-24.04-server-cloudimg-arm64.img">x</a>`,
		base + "SHA256SUMS": strings.Repeat("a", 64) + " *ubuntu-24.04.1-server-cloudimg-amd64.img\n" +
			strings.Repeat("b", 64) + " *ubuntu-24.04-server-cloudimg-amd64.img\n",
	})
	u, h, err := scrape("ubuntu-cloud", "24.04", "", "amd64")
	if err != nil {
		t.Fatalf("ubuntu-cloud err=%v", err)
	}
	// Version-sorted greatest wins: 24.04.1 > 24.04, flavour variants
	// (-azure/-disk-kvm) never match the generic qcow2 name.
	if u != base+"ubuntu-24.04.1-server-cloudimg-amd64.img" {
		t.Fatalf("ubuntu-cloud url=%q", u)
	}
	if h != strings.Repeat("a", 64) {
		t.Fatalf("ubuntu-cloud hash=%q", h)
	}
}

func TestScrapeUbuntuCloudArm(t *testing.T) {
	base := "https://cloud-images.ubuntu.com/releases/22.04/release/"
	fakeFetch(t, map[string]string{
		base: `<a href="ubuntu-22.04-server-cloudimg-amd64.img">x</a>
			<a href="ubuntu-22.04-server-cloudimg-arm64.img">x</a>`,
	})
	u, _, err := scrape("ubuntu-cloud", "22.04", "", "arm64")
	if err != nil || !strings.HasSuffix(u, "-arm64.img") {
		t.Fatalf("ubuntu-cloud arm64 url=%q err=%v", u, err)
	}
}

func TestScrapeArchCloud(t *testing.T) {
	base := "https://geo.mirror.pkgbuild.com/images/latest/"
	img := "Arch-Linux-x86_64-cloudimg-20251101.441932.qcow2"
	fakeFetch(t, map[string]string{
		base: `<a href="Arch-Linux-x86_64-basic-20251101.441932.qcow2">x</a>
			<a href="Arch-Linux-x86_64-cloudimg-20251001.440000.qcow2">x</a>
			<a href="` + img + `">x</a>`,
		base + img + ".SHA256": strings.Repeat("c", 64) + "  " + img,
	})
	u, h, err := scrape("arch-cloud", "latest", "", "amd64")
	if err != nil || u != base+img {
		t.Fatalf("arch-cloud url=%q err=%v", u, err)
	}
	if h != strings.Repeat("c", 64) {
		t.Fatalf("arch-cloud hash=%q", h)
	}
	// No official aarch64 build exists — must fail, not silently amd64.
	if _, _, err := scrape("arch-cloud", "latest", "", "arm64"); err == nil {
		t.Fatal("arch-cloud arm64 must fail (no official image)")
	}
}

func TestScrapeRockyCloud(t *testing.T) {
	base := "https://dl.rockylinux.org/pub/rocky/9/images/x86_64/"
	img := "Rocky-9-GenericCloud-Base.latest.x86_64.qcow2"
	fakeFetch(t, map[string]string{
		base: `<a href="Rocky-9-GenericCloud-Base-9.5.20241201.0.x86_64.qcow2">x</a>
			<a href="` + img + `">x</a>
			<a href="Rocky-9-GenericCloud-LVM.latest.x86_64.qcow2">x</a>`,
		base + "CHECKSUM": "SHA256 (" + img + ") = " + strings.Repeat("d", 64) + "\n",
	})
	u, h, err := scrape("rocky-cloud", "9", "", "amd64")
	if err != nil || u != base+img {
		t.Fatalf("rocky-cloud url=%q err=%v", u, err)
	}
	if h != strings.Repeat("d", 64) {
		t.Fatalf("rocky-cloud hash=%q", h)
	}
}

func TestScrapeAlmaCloud(t *testing.T) {
	base := "https://repo.almalinux.org/almalinux/9/cloud/aarch64/images/"
	img := "AlmaLinux-9-GenericCloud-latest.aarch64.qcow2"
	fakeFetch(t, map[string]string{
		base: `<a href="AlmaLinux-9-GenericCloud-9.4-20240805.aarch64.qcow2">x</a>
			<a href="` + img + `">x</a>`,
		base + "CHECKSUM": strings.Repeat("e", 64) + "  " + img + "\n",
	})
	u, h, err := scrape("alma-cloud", "9", "", "arm64")
	if err != nil || u != base+img {
		t.Fatalf("alma-cloud url=%q err=%v", u, err)
	}
	if h != strings.Repeat("e", 64) {
		t.Fatalf("alma-cloud hash=%q", h)
	}
}

func TestScrapeOpensuseCloud(t *testing.T) {
	// Leap release.
	lbase := "https://download.opensuse.org/distribution/leap/15.6/appliances/"
	limg := "openSUSE-Leap-15.6-Minimal-VM.x86_64-Cloud.qcow2"
	fakeFetch(t, map[string]string{
		lbase: `<a href="` + limg + `">x</a>
			<a href="openSUSE-Leap-15.6-Minimal-VM.x86_64-kvm-and-xen.qcow2">x</a>`,
		lbase + limg + ".sha256": strings.Repeat("f", 64) + "  " + limg,
	})
	u, h, err := scrape("opensuse-cloud", "15.6", "", "amd64")
	if err != nil || u != lbase+limg {
		t.Fatalf("opensuse-cloud leap url=%q err=%v", u, err)
	}
	if h != strings.Repeat("f", 64) {
		t.Fatalf("opensuse-cloud hash=%q", h)
	}
	// Tumbleweed.
	tbase := "https://download.opensuse.org/tumbleweed/appliances/"
	timg := "openSUSE-Tumbleweed-Minimal-VM.x86_64-Cloud.qcow2"
	fakeFetch(t, map[string]string{
		tbase: `<a href="` + timg + `">x</a>`,
	})
	u, _, err = scrape("opensuse-cloud", "tumbleweed", "", "amd64")
	if err != nil || u != tbase+timg {
		t.Fatalf("opensuse-cloud tw url=%q err=%v", u, err)
	}
	// Tumbleweed aarch64 comes from the ports tree.
	abase := "https://download.opensuse.org/ports/aarch64/tumbleweed/appliances/"
	aimg := "openSUSE-Tumbleweed-Minimal-VM.aarch64-Cloud.qcow2"
	fakeFetch(t, map[string]string{
		abase: `<a href="` + aimg + `">x</a>`,
	})
	u, _, err = scrape("opensuse-cloud", "tumbleweed", "", "arm64")
	if err != nil || u != abase+aimg {
		t.Fatalf("opensuse-cloud tw arm64 url=%q err=%v", u, err)
	}
}

func TestScrapeCentosStreamCloud(t *testing.T) {
	base := "https://cloud.centos.org/centos/9-stream/x86_64/images/"
	img := "CentOS-Stream-GenericCloud-9-latest.x86_64.qcow2"
	fakeFetch(t, map[string]string{
		base: `<a href="CentOS-Stream-GenericCloud-9-20250407.0.x86_64.qcow2">x</a>
			<a href="` + img + `">x</a>`,
		base + img + ".SHA256SUM": strings.Repeat("a", 64) + "  " + img,
	})
	u, h, err := scrape("centos-stream-cloud", "9", "", "amd64")
	if err != nil || u != base+img {
		t.Fatalf("centos-stream-cloud url=%q err=%v", u, err)
	}
	if h != strings.Repeat("a", 64) {
		t.Fatalf("centos-stream-cloud hash=%q", h)
	}
}

func TestScrapeDebianCloudEditions(t *testing.T) {
	base := "https://cloud.debian.org/images/cloud/bookworm/latest/"
	fakeFetch(t, map[string]string{
		base: `<a href="debian-12-genericcloud-amd64.qcow2">x</a>
			<a href="debian-12-nocloud-amd64.qcow2">x</a>`,
		base + "SHA512SUMS": strings.Repeat("e", 128) + "  debian-12-genericcloud-amd64.qcow2\n" +
			strings.Repeat("f", 128) + "  debian-12-nocloud-amd64.qcow2\n",
	})
	// Empty edition → genericcloud (cloud-init present).
	u, h, err := scrape("debian-cloud", "12", "", "amd64")
	if err != nil || !strings.Contains(u, "genericcloud") {
		t.Fatalf("debian-cloud default url=%q err=%v", u, err)
	}
	if h != strings.Repeat("e", 128) {
		t.Fatalf("debian-cloud genericcloud hash=%q", h)
	}
	// Explicit nocloud edition resolves the no-cloud-init build.
	u, h, err = scrape("debian-cloud", "12", "nocloud", "amd64")
	if err != nil || !strings.Contains(u, "nocloud-amd64") {
		t.Fatalf("debian-cloud nocloud url=%q err=%v", u, err)
	}
	if h != strings.Repeat("f", 128) {
		t.Fatalf("debian-cloud nocloud hash=%q", h)
	}
	// Unknown edition fails loudly.
	if _, _, err := scrape("debian-cloud", "12", "bogus", "amd64"); err == nil {
		t.Fatal("debian-cloud bogus edition must fail")
	}
}

// Metadata-only assertions for the new cloud catalog entries: every
// *-cloud OS is scrape-driven, qcow2-typed, amd64(+arm64 where the
// publisher ships it), and carries releases + default-user info.
func TestCloudCatalogEntries(t *testing.T) {
	c, err := loadCatalog(filepath.Join("..", "..", "catalog", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	wantArch := map[string][]string{
		"ubuntu-cloud":        {"amd64", "arm64"},
		"debian-cloud":        {"amd64", "arm64"},
		"fedora-cloud":        {"amd64", "arm64"},
		"alpine-cloud":        {"amd64", "arm64"},
		"arch-cloud":          {"amd64"},
		"rocky-cloud":         {"amd64", "arm64"},
		"alma-cloud":          {"amd64", "arm64"},
		"opensuse-cloud":      {"amd64", "arm64"},
		"centos-stream-cloud": {"amd64", "arm64"},
	}
	info, err := loadInfo(filepath.Join("..", "..", "catalog", "data", "info.json"))
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	for osName, archs := range wantArch {
		e := find(c, osName)
		if e == nil {
			t.Errorf("%s: missing catalog entry", osName)
			continue
		}
		if !e.Scrape {
			t.Errorf("%s: not scrape-driven", osName)
		}
		if e.ImageType != "qcow2" {
			t.Errorf("%s: image_type=%q want qcow2", osName, e.ImageType)
		}
		if len(e.Releases) == 0 {
			t.Errorf("%s: empty releases", osName)
		}
		if strings.Join(e.Arch, ",") != strings.Join(archs, ",") {
			t.Errorf("%s: arch=%v want %v", osName, e.Arch, archs)
		}
		// Cloud images are unusable without cloud-init and carry a
		// baked-in default user — recorded in info credentials.
		inf, ok := info[osName]
		if !ok {
			t.Errorf("%s: missing info.json entry", osName)
			continue
		}
		if inf.Credentials == "" || inf.Credentials == "-" {
			t.Errorf("%s: no default user in credentials", osName)
		}
		if !strings.Contains(strings.ToLower(inf.Info), "cloud-init") {
			t.Errorf("%s: info must mention cloud-init", osName)
		}
	}
}
