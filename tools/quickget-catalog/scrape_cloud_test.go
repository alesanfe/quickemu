// scrape_cloud_test.go — offline coverage for the *-cloud resolvers:
// alpine-cloud (sidecar .sha512, nocloud_/bare naming), debian-cloud
// (SHA512SUMS manifest), fedora-cloud (per-spin CHECKSUM manifest).
package main

import (
	"strings"
	"testing"
)

func TestScrapeAlpineCloud(t *testing.T) {
	// The real index mixes old nocloud_-prefixed builds with the newer
	// bare alpine-* names; version compare must pick 3.22.7 either way.
	idx := `<a href="nocloud_alpine-3.22.4-x86_64-bios-cloudinit-r0.qcow2">x</a>
		<a href="generic_alpine-3.22.6-x86_64-bios-cloudinit-r0.qcow2">x</a>
		<a href="generic_alpine-3.22.6-x86_64-bios-cloudinit-metal-r0.qcow2">x</a>
		<a href="alpine-3.22.7-x86_64-bios-cloudinit-r0.qcow2">x</a>
		<a href="alpine-3.22.7-x86_64-uefi-cloudinit-r0.qcow2">x</a>
		<a href="alpine-3.22.7-aarch64-uefi-cloudinit-r0.qcow2">x</a>`
	base := "https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/cloud/"
	iso := "alpine-3.22.7-x86_64-bios-cloudinit-r0.qcow2"
	fakeFetch(t, map[string]string{
		base:                   idx,
		base + iso + ".sha512": strings.Repeat("d", 128) + "  " + iso,
	})
	u, h, err := scrape("alpine-cloud", "v3.22", "", "amd64")
	if err != nil || u != base+iso {
		t.Fatalf("alpine-cloud url=%q err=%v", u, err)
	}
	if h != strings.Repeat("d", 128) {
		t.Fatalf("alpine-cloud hash=%q", h)
	}
}

func TestScrapeAlpineCloudARM(t *testing.T) {
	idx := `<a href="alpine-3.22.7-aarch64-uefi-cloudinit-r0.qcow2">x</a>
		<a href="alpine-3.22.7-x86_64-bios-cloudinit-r0.qcow2">x</a>`
	base := "https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/cloud/"
	fakeFetch(t, map[string]string{base: idx})
	u, _, err := scrape("alpine-cloud", "v3.22", "", "arm64")
	if err != nil || !strings.HasSuffix(u, "alpine-3.22.7-aarch64-uefi-cloudinit-r0.qcow2") {
		t.Fatalf("alpine-cloud arm64 url=%q err=%v", u, err)
	}
}

func TestScrapeDebianCloud(t *testing.T) {
	// genericcloud ships cloud-init; the "nocloud" variant (no init) must
	// never match — Debian's naming is inverted vs Alpine.
	idx := `<a href="debian-12-genericcloud-amd64.qcow2">x</a>
		<a href="debian-12-nocloud-amd64.qcow2">x</a>
		<a href="debian-12-genericcloud-arm64.qcow2">x</a>`
	base := "https://cloud.debian.org/images/cloud/bookworm/latest/"
	fakeFetch(t, map[string]string{
		base: idx,
		base + "SHA512SUMS": strings.Repeat("e", 128) + "  debian-12-genericcloud-amd64.qcow2\n" +
			strings.Repeat("f", 128) + "  debian-12-nocloud-amd64.qcow2\n",
	})
	u, h, err := scrape("debian-cloud", "12", "", "amd64")
	if err != nil || u != base+"debian-12-genericcloud-amd64.qcow2" {
		t.Fatalf("debian-cloud url=%q err=%v", u, err)
	}
	if h != strings.Repeat("e", 128) {
		t.Fatalf("debian-cloud hash=%q", h)
	}
	if _, _, err := scrape("debian-cloud", "10", "", "amd64"); err == nil {
		t.Fatal("expected unknown-release error")
	}
}

func TestScrapeFedoraCloud(t *testing.T) {
	idx := `<a href="Fedora-Cloud-Base-Generic-43-1.6.x86_64.qcow2">x</a>
		<a href="Fedora-Cloud-Base-Generic-43-1.6.aarch64.qcow2">x</a>
		<a href="Fedora-Cloud-43-1.6.x86_64-CHECKSUM">x</a>`
	base := "https://download.fedoraproject.org/pub/fedora/linux/releases/43/Cloud/x86_64/images/"
	fakeFetch(t, map[string]string{
		base: idx,
		base + "Fedora-Cloud-43-1.6.x86_64-CHECKSUM": "SHA256 (Fedora-Cloud-Base-Generic-43-1.6.x86_64.qcow2) = " +
			strings.Repeat("a", 64),
	})
	u, h, err := scrape("fedora-cloud", "43", "", "amd64")
	if err != nil || u != base+"Fedora-Cloud-Base-Generic-43-1.6.x86_64.qcow2" {
		t.Fatalf("fedora-cloud url=%q err=%v", u, err)
	}
	// hash slot carries the CHECKSUM manifest URL, not the digest.
	if h != base+"Fedora-Cloud-43-1.6.x86_64-CHECKSUM" {
		t.Fatalf("fedora-cloud sums=%q", h)
	}
}
