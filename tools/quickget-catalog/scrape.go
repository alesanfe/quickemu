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
	case "cachyos":
		// get_cachyos: download page, token after & containing
		// ISO/<edition>, cut ; field2; hash from <url>.sha256
		body, err := fetchString("https://cachyos.org/download/")
		if err != nil {
			return "", "", err
		}
		url := ""
		for _, tok := range strings.Split(strings.ReplaceAll(body, "&", "\n"), "\n") {
			if strings.Contains(tok, "ISO/"+edition) && !strings.Contains(tok, "iso.sha") && !strings.Contains(tok, "iso.sig") {
				if f := strings.SplitN(tok, ";", 3); len(f) >= 2 {
					url = f[1]
					break
				}
			}
		}
		if url == "" {
			return "", "", fmt.Errorf("cachyos: no ISO/%s link", edition)
		}
		sha := ""
		if b, err := fetchString(url + ".sha256"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return url, sha, nil
	case "manjaro":
		// get_manjaro: file-info.json manifest; sway uses its own feed;
		// cinnamon|i3 are community. minimal edition is a .-suffix.
		typ := "official"
		manifest := "https://gitlab.manjaro.org/web/iso-info/-/raw/master/file-info.json"
		if release == "sway" {
			manifest = "https://mirror.manjaro-sway.download/manjaro-sway/release.json"
			typ = "sway"
		} else if release == "cinnamon" || release == "i3" {
			typ = "community"
		}
		body, err := fetchString(manifest)
		if err != nil {
			return "", "", err
		}
		url := ""
		if release == "sway" {
			var arr []struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			}
			if json.Unmarshal([]byte(body), &arr) == nil {
				for _, it := range arr {
					if strings.HasPrefix(it.Name, "manjaro-sway-") && strings.HasSuffix(it.Name, ".iso") {
						url = it.URL
						break
					}
				}
			}
		} else {
			suffix := ""
			if edition == "minimal" {
				suffix = ".minimal"
			}
			var doc map[string]any
			if json.Unmarshal([]byte(body), &doc) == nil {
				if sec, ok := doc[typ].(map[string]any); ok {
					if rel, ok := sec[release+suffix].(map[string]any); ok {
						url, _ = rel["image"].(string)
					}
				}
			}
		}
		if url == "" {
			return "", "", fmt.Errorf("manjaro: no image for %s/%s", release, edition)
		}
		sha := ""
		if b, err := fetchString(url + ".sha512"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return url, sha, nil
	case "reactos":
		// get_reactos: sourceforge latest-download redirect, no hash.
		u, err := finalURL("https://sourceforge.net/projects/reactos/files/latest/download")
		if err != nil {
			return "", "", err
		}
		return u, "", nil
	default:
		return "", "", fmt.Errorf("scrape not implemented for %s", osName)
	}
}
