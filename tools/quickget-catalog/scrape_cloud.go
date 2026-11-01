// scrape_cloud.go — resolvers for pre-imaged cloud disk entries
// (*-cloud OSes): plain qcow2/img artifacts consumed by VMs directly,
// cloud-init/ignition do the first-boot setup. Each case resolves the
// distro's official image index and the version-sorted greatest
// matching artifact, plus a checksum when the publisher ships one.
package main

import (
	"fmt"
	"regexp"
	"strings"
)

// cloudArch maps the quickget arch token to the QEMU-style token used
// by most cloud-image filenames (amd64→x86_64, arm64→aarch64). Alpine,
// Debian and Ubuntu name files with the quickget token itself.
func cloudArch(arch string) string {
	if arch == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

// cloudSum tries the usual per-file checksum sidecars, then falls back
// to a manifest lookup — publishers disagree on both the convention
// (.sha256 vs .SHA256SUM) and the file layout (sidecar vs sums file).
func cloudSum(u, base, sumsName string) string {
	for _, ext := range []string{".sha256", ".SHA256SUM", ".sha256sum"} {
		if s := sumAt(u + ext); s != "" {
			return s
		}
	}
	if sumsName != "" {
		if b, err := fetchString(strings.TrimRight(base, "/") + "/" + sumsName); err == nil {
			return sumForISO(b, u[strings.LastIndex(u, "/")+1:])
		}
	}
	return ""
}

// cloudLatest is the shared index-scan: fetch base, return the
// version-sorted greatest href/filename match joined onto base.
func cloudLatest(base, re string) (string, error) {
	u, _, err := indexISO(base, re)
	return u, err
}

// scrapeCloud resolves cloud-disk OSes. Each case lists a directory
// index and picks the version-sorted greatest matching artifact.
func scrapeCloud(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "alpine-cloud":
		// releases/cloud index carries <ver>/<arch>/<firmware>/cloudinit
		// qcow2 builds; the filename prefix shifted across releases —
		// nocloud_ (≤3.19), generic_ (3.20), bare (3.21+) — so match all
		// three and keep the whole name for the URL. x86_64 ships
		// bios+uefi variants, aarch64 uefi only.
		qarch, fw := "x86_64", "bios"
		if arch == "arm64" {
			qarch, fw = "aarch64", "uefi"
		}
		base := fmt.Sprintf("https://dl-cdn.alpinelinux.org/alpine/%s/releases/cloud/", release)
		body, err := fetchString(base)
		if err != nil {
			return "", "", err
		}
		re := regexp.MustCompile(fmt.Sprintf(
			`(?:nocloud_|generic_)?alpine-([0-9.]+)-%s-%s-cloudinit-r[0-9]+\.qcow2`, qarch, fw))
		best, bestVer := "", ""
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			if best == "" || versionLess(bestVer, m[1]) {
				best, bestVer = m[0], m[1]
			}
		}
		if best == "" {
			return "", "", fmt.Errorf("no iso matching %s in %s", re, base)
		}
		u := strings.TrimRight(base, "/") + "/" + best
		return u, sumAt(u + ".sha512"), nil
	case "ubuntu-cloud":
		// cloud-images.ubuntu.com publishes per-release "release"
		// dirs; the plain -server-cloudimg-<arch>.img is the generic
		// qcow2 (arch/disk-kvm/azure variants end differently and are
		// never matched). Filenames keep the amd64/arm64 token.
		base := fmt.Sprintf("https://cloud-images.ubuntu.com/releases/%s/release/", release)
		u, err := cloudLatest(base,
			`ubuntu-[0-9][0-9.]*-server-cloudimg-`+regexp.QuoteMeta(arch)+`\.img`)
		if err != nil {
			return "", "", err
		}
		return u, cloudSum(u, base, "SHA256SUMS"), nil
	case "debian-cloud":
		// cloud.debian.org publishes per-codename dirs with a "latest"
		// alias. Editions pick the build flavor: genericcloud =
		// cloud-init included (the default), nocloud = the build
		// WITHOUT cloud integration — naming is inverted vs Alpine.
		edition = map[string]string{"": "genericcloud", "genericcloud": "genericcloud",
			"nocloud": "nocloud"}[edition]
		if edition == "" {
			return "", "", fmt.Errorf("debian-cloud: unknown edition")
		}
		codenames := map[string]string{
			"13": "trixie", "12": "bookworm", "11": "bullseye",
		}
		codename, ok := codenames[release]
		if !ok {
			return "", "", fmt.Errorf("debian-cloud: unknown release %s", release)
		}
		base := fmt.Sprintf("https://cloud.debian.org/images/cloud/%s/latest/", codename)
		re := fmt.Sprintf(`debian-%s-%s-%s\.qcow2`,
			regexp.QuoteMeta(release), edition, regexp.QuoteMeta(arch))
		return indexISOsums(base, re, "SHA512SUMS")
	case "fedora-cloud":
		// releases/<ver>/Cloud/<arch>/images/ carries the Generic qcow2
		// plus a per-spin CHECKSUM manifest; 45_Beta lives under test/.
		qarch := "x86_64"
		if arch == "arm64" {
			qarch = "aarch64"
		}
		dir := "releases"
		if strings.Contains(release, "Beta") {
			dir = "releases/test"
		}
		base := fmt.Sprintf("https://download.fedoraproject.org/pub/fedora/linux/%s/%s/Cloud/%s/images/", dir, release, qarch)
		u, _, err := indexISO(base, `Fedora-Cloud-Base-Generic-[^"]+\.qcow2`)
		if err != nil {
			return "", "", err
		}
		// CHECKSUM filename embeds the spin version too — resolve it
		// from the same index and hand it back as a sums URL.
		body, err := fetchString(base)
		if err != nil {
			return u, "", nil
		}
		if sum := latestMatch(body, `Fedora-Cloud-[^"/<> ]+`+qarch+`-CHECKSUM`); sum != "" {
			return u, strings.TrimRight(base, "/") + "/" + sum, nil
		}
		return u, "", nil
	case "arch-cloud":
		// pkgbuild.com images: latest/ carries the dated cloudimg
		// qcow2 plus a .SHA256 sidecar. x86_64 only upstream.
		if arch == "arm64" {
			return "", "", fmt.Errorf("arch-cloud: no official arm64 cloud image")
		}
		base := "https://geo.mirror.pkgbuild.com/images/latest/"
		u, err := cloudLatest(base, `Arch-Linux-x86_64-cloudimg-[0-9.]+\.qcow2`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(u + ".SHA256"), nil
	case "rocky-cloud":
		// dl.rockylinux.org/pub/rocky/<ver>/images/<arch>/ carries
		// GenericCloud-Base qcow2 builds plus a CHECKSUM manifest
		// (BSD-style "SHA256 (name) = hex" lines).
		rel := regexp.QuoteMeta(strings.SplitN(release, ".", 2)[0])
		base := fmt.Sprintf("https://dl.rockylinux.org/pub/rocky/%s/images/%s/",
			rel, cloudArch(arch))
		u, err := cloudLatest(base,
			fmt.Sprintf(`Rocky-%s-GenericCloud-Base[^"/<> ]*\.%s\.qcow2`, rel, cloudArch(arch)))
		if err != nil {
			return "", "", err
		}
		return u, cloudSum(u, base, "CHECKSUM"), nil
	case "alma-cloud":
		// repo.almalinux.org/<ver>/cloud/<arch>/images/ publishes
		// GenericCloud qcow2s (dated builds + a "latest" alias) and a
		// CHECKSUM manifest.
		rel := regexp.QuoteMeta(strings.SplitN(release, ".", 2)[0])
		base := fmt.Sprintf("https://repo.almalinux.org/almalinux/%s/cloud/%s/images/",
			rel, cloudArch(arch))
		u, err := cloudLatest(base,
			fmt.Sprintf(`AlmaLinux-%s-GenericCloud[^"/<> ]*\.%s\.qcow2`, rel, cloudArch(arch)))
		if err != nil {
			return "", "", err
		}
		return u, cloudSum(u, base, "CHECKSUM"), nil
	case "opensuse-cloud":
		// Leap images live under distribution/leap/<ver>/appliances/;
		// Tumbleweed under tumbleweed/appliances/. aarch64 builds are
		// published from the ports tree. Minimal-VM *-Cloud.qcow2 is
		// the cloud-init image.
		rel := release
		// Leap ≥16 renamed the image prefix openSUSE-Leap- → Leap-.
		variant := "(?:openSUSE-)?Leap-" + regexp.QuoteMeta(rel)
		base := fmt.Sprintf("https://download.opensuse.org/distribution/leap/%s/appliances/", rel)
		if rel == "tumbleweed" {
			variant = "openSUSE-Tumbleweed"
			base = "https://download.opensuse.org/tumbleweed/appliances/"
		}
		if arch == "arm64" {
			if rel == "tumbleweed" {
				base = "https://download.opensuse.org/ports/aarch64/tumbleweed/appliances/"
			} else {
				base = fmt.Sprintf("https://download.opensuse.org/ports/aarch64/distribution/leap/%s/appliances/", rel)
			}
		}
		u, err := cloudLatest(base,
			fmt.Sprintf(`%s-Minimal-VM\.%s-Cloud\.qcow2`, variant, cloudArch(arch)))
		if err != nil {
			return "", "", err
		}
		return u, cloudSum(u, base, "SHA256SUMS"), nil
	case "centos-stream-cloud":
		// cloud.centos.org/<ver>-stream/<arch>/images/ carries dated
		// GenericCloud qcow2s plus a "latest" alias; checksums come as
		// .SHA256SUM sidecars or a CHECKSUM manifest.
		rel := regexp.QuoteMeta(strings.SplitN(release, ".", 2)[0])
		base := fmt.Sprintf("https://cloud.centos.org/centos/%s-stream/%s/images/",
			rel, cloudArch(arch))
		u, err := cloudLatest(base,
			fmt.Sprintf(`CentOS-Stream-GenericCloud-%s[^"/<> ]*\.%s\.qcow2`, rel, cloudArch(arch)))
		if err != nil {
			return "", "", err
		}
		return u, cloudSum(u, base, "CHECKSUM"), nil
	}
	return "", "", errScrapeSkip
}
