// scrape_cloud.go — resolvers for pre-imaged cloud disk entries
// (*-cloud OSes): plain qcow2 artifacts consumed by VMs directly,
// cloud-init/ignition do the first-boot setup.
package main

import (
	"fmt"
	"regexp"
	"strings"
)

// scrapeCloud resolves cloud-disk OSes. Each case lists a directory
// index and picks the version-sorted greatest matching artifact.
func scrapeCloud(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "alpine-cloud":
		// releases/cloud index carries <ver>/<arch>/<firmware>/cloudinit
		// qcow2 builds; newer releases dropped the "nocloud_" prefix, so
		// accept both. x86_64 ships bios+uefi variants, aarch64 uefi only.
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
			`(?:nocloud_)?alpine-([0-9.]+)-%s-%s-cloudinit-r[0-9]+\.qcow2`, qarch, fw))
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
	case "debian-cloud":
		// cloud.debian.org publishes per-codename dirs with a "latest"
		// alias; genericcloud = cloud-init included ("nocloud" is the
		// build WITHOUT it — naming is inverted vs Alpine).
		codenames := map[string]string{
			"13": "trixie", "12": "bookworm", "11": "bullseye",
		}
		codename, ok := codenames[release]
		if !ok {
			return "", "", fmt.Errorf("debian-cloud: unknown release %s", release)
		}
		base := fmt.Sprintf("https://cloud.debian.org/images/cloud/%s/latest/", codename)
		re := fmt.Sprintf(`debian-%s-genericcloud-%s\.qcow2`, release, arch)
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
		if sum := latestMatch(body, qarch+`-CHECKSUM`); sum != "" {
			return u, strings.TrimRight(base, "/") + "/" + sum, nil
		}
		return u, "", nil
	}
	return "", "", errScrapeSkip
}
