// scrape_lists.go — scrapeReleases / scrapeEditions — live release/edition lists for scrape families.
package main

import (
	"regexp"
	"strings"
)

// scrapeEditions resolves editions for OSes whose editions_<os>() is a
// live scrape keyed on ${RELEASE} — the static catalog lists them as
// empty. Mirrors the Bash function output (page order, deduplicated).
// scrapeReleases: releases_<os> parity for OSes whose release list is
// live (empty in the static catalog). Each case mirrors the Bash
// web_pipe pipeline.
func scrapeReleases(osName string) []string {
	switch osName {
	case "pureos":
		// releases_pureos: download page, first puri.sm link, 4th '-' field
		body, err := fetchString("https://www.pureos.net/download/")
		if err != nil {
			return nil
		}
		for _, m := range regexp.MustCompile(`"([^"]*downloads\.puri[^"]*)"`).FindAllStringSubmatch(body, -1) {
			f := strings.Split(m[1], "-")
			if len(f) > 4 {
				return []string{f[4]}
			}
		}
	case "rockylinux":
		// releases_rockylinux: vault index links minus full/RC/ISO,
		// reversed (newest first).
		body, err := fetchString("https://dl.rockylinux.org/vault/rocky/")
		if err != nil {
			return nil
		}
		var rels []string
		for _, m := range regexp.MustCompile(`class="link"[^>]*>([^<"]+)/?<`).FindAllStringSubmatch(body, -1) {
			n := strings.Trim(m[1], "/")
			if n == "" || strings.ContainsAny(n, " ") {
				continue
			}
			low := strings.ToLower(n)
			if strings.Contains(low, "full") || strings.Contains(low, "rc") ||
				strings.Contains(low, "iso") || strings.Contains(low, "parent") {
				continue
			}
			rels = append(rels, n)
		}
		for i, j := 0, len(rels)-1; i < j; i, j = i+1, j-1 {
			rels[i], rels[j] = rels[j], rels[i]
		}
		return rels
	case "slitaz":
		// releases_slitaz: rolling index iso filenames, unique versions.
		body, err := fetchString("https://mirror.slitaz.org/iso/rolling/")
		if err != nil {
			return nil
		}
		seen := map[string]bool{}
		var rels []string
		for _, m := range regexp.MustCompile(`class='iso'[^>]*>'?([^'<>]+)`).FindAllStringSubmatch(body, -1) {
			f := strings.TrimSuffix(m[1], "'")
			parts := strings.SplitN(f, "-", 3)
			if len(parts) < 3 || !strings.HasSuffix(f, ".iso") {
				continue
			}
			ver := strings.TrimSuffix(parts[2], ".iso")
			if !seen[ver] {
				seen[ver] = true
				rels = append(rels, ver)
			}
		}
		return rels
	}
	return nil
}
func scrapeEditions(osName, release string) []string {
	if release == "" {
		return nil
	}
	switch osName {
	case "blackarch":
		// editions_blackarch: mirror listing → blackarch-linux-<ed>-<rel>-x86_64
		body, err := fetchString("https://ftp.halifax.rwth-aachen.de/blackarch/iso/")
		if err != nil {
			return nil
		}
		re := regexp.MustCompile(`blackarch-linux-([a-z]+)-` + regexp.QuoteMeta(release) + `-x86_64`)
		var eds []string
		seen := map[string]bool{}
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				eds = append(eds, m[1])
			}
		}
		return eds
	}
	return nil
}
