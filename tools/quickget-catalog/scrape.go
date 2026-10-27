// Scraper handling: OSes whose get_ needs live page/JSON resolution.
// Implements the common patterns (json API, html links, yaml) natively
// — no bash delegation for these.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// scrape resolves URL+hash for scrape-type OSes. Each case mirrors its
// get_<os> logic exactly.
func scrape(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "alpine":
		// get_alpine: fetch latest-releases.yaml, extract version+sha256
		qarch := "x86_64"
		if arch == "arm64" {
			qarch = "aarch64"
		}
		u := fmt.Sprintf("https://dl-cdn.alpinelinux.org/alpine/%s/releases/%s", release, qarch)
		body, err := fetchString(u + "/latest-releases.yaml")
		if err != nil {
			return "", "", err
		}
		// extract Virtual-flavor version + sha256
		version := findLine(body, `flavor: alpine-virt[\s\S]*?version: ([\d.]+)`, 1)
		if version == "" {
			version = findLine(body, `version: ([\d.]+)`, 1)
		}
		sha := findLine(body, `flavor: alpine-virt[\s\S]*?sha256: ([a-f0-9]{64})`, 1)
		iso := fmt.Sprintf("alpine-virt-%s-%s.iso", version, qarch)
		return u + "/" + iso, sha, nil
	case "archlinux":
		// get_archlinux: fetch releng/releases/json, take releases[0]
		body, err := fetchString("https://archlinux.org/releng/releases/json/")
		if err != nil {
			return "", "", err
		}
		var j struct {
			Releases []struct {
				IsoURL string `json:"iso_url"`
				Sha256 string `json:"sha256_sum"`
			} `json:"releases"`
		}
		if json.Unmarshal([]byte(body), &j) != nil || len(j.Releases) == 0 {
			return "", "", fmt.Errorf("arch json parse")
		}
		return "https://geo.mirror.pkgbuild.com/" + j.Releases[0].IsoURL, j.Releases[0].Sha256, nil
	case "artixlinux":
		// get_artixlinux: sha256sums file, grep for the ISO name
		u := "https://iso.artixlinux.org/iso"
		iso := fmt.Sprintf("artix-%s-%s-x86_64.iso", edition, release)
		body, err := fetchString(u + "/sha256sums")
		if err != nil {
			return "", "", err
		}
		sha := findLine(body, `([a-f0-9]{64})`, 1)
		for _, line := range strings.Split(body, "\n") {
			if strings.Contains(line, iso) {
				sha = findLine(line, `([a-f0-9]{64})`, 1)
				break
			}
		}
		return u + "/" + iso, sha, nil
	default:
		return "", "", fmt.Errorf("scrape not implemented for %s", osName)
	}
}
