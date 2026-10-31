// scrape_fetch.go — shared fetch/resolve helpers for the scrape
// resolvers: GitHub releases API, SourceForge, directory indexes and
// checksum-manifest lookups. Also hosts errScrapeSkip, the sentinel
// sub-resolvers return for OS names outside their mechanism group.
package main

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ghISO fetches a GitHub releases API page and returns the first
// browser_download_url matching re (releases arrive newest-first).
func ghISO(repo, re string) (string, string, error) {
	body, err := fetchString("https://api.github.com/repos/" + repo + "/releases")
	if err != nil {
		return "", "", err
	}
	r := regexp.MustCompile(re)
	for _, m := range matchAll(body, `"browser_download_url": ?"([^"]+)"`) {
		if r.MatchString(m) {
			return m, "", nil
		}
	}
	return "", "", fmt.Errorf("%s: no asset matching %s", repo, re)
}

// ghISOsha is ghISO plus a sibling checksum asset (e.g. .iso.sha256 or
// sha256sum.txt); the checksum returned is the digest inside the asset.
func ghISOsha(repo, isoRe, sumRe string) (string, string, error) {
	body, err := fetchString("https://api.github.com/repos/" + repo + "/releases")
	if err != nil {
		return "", "", err
	}
	ir := regexp.MustCompile(isoRe)
	sr := regexp.MustCompile(sumRe)
	var iso, sum string
	for _, m := range matchAll(body, `"browser_download_url": ?"([^"]+)"`) {
		if iso == "" && ir.MatchString(m) {
			iso = m
		}
		if sum == "" && sr.MatchString(m) {
			sum = m
		}
	}
	if iso == "" {
		return "", "", fmt.Errorf("%s: no asset matching %s", repo, isoRe)
	}
	if sum != "" {
		sum = sumAt(sum)
	}
	return iso, sum, nil
}

// ghISOsums is ghISOsha for manifest checksum assets (SHA256SUMS): the
// sha is extracted for the specific matched ISO filename.
func ghISOsums(repo, isoRe, sumRe string) (string, string, error) {
	body, err := fetchString("https://api.github.com/repos/" + repo + "/releases")
	if err != nil {
		return "", "", err
	}
	ir := regexp.MustCompile(isoRe)
	sr := regexp.MustCompile(sumRe)
	var iso, sums string
	for _, m := range matchAll(body, `"browser_download_url": ?"([^"]+)"`) {
		if iso == "" && ir.MatchString(m) {
			iso = m
		}
		if sums == "" && sr.MatchString(m) {
			sums = m
		}
	}
	if iso == "" {
		return "", "", fmt.Errorf("%s: no asset matching %s", repo, isoRe)
	}
	if sums == "" {
		return iso, "", nil
	}
	b, err := fetchString(sums)
	if err != nil {
		return iso, "", nil
	}
	return iso, sumForISO(b, iso[strings.LastIndex(iso, "/")+1:]), nil
}

// ghLatestTag returns the tag_name of a repo's latest stable release.
func ghLatestTag(repo string) (string, error) {
	body, err := fetchString("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return "", err
	}
	m := matchAll(body, `"tag_name": ?"([^"]+)"`)
	if len(m) == 0 {
		return "", fmt.Errorf("%s: no latest tag", repo)
	}
	return m[0], nil
}

// indexISO fetches a directory-index/page and returns the version-sorted
// greatest href/filename matching re, joined onto base.
func indexISO(base, re string) (string, string, error) {
	body, err := fetchString(base)
	if err != nil {
		return "", "", err
	}
	iso := latestMatch(body, re)
	if iso == "" {
		return "", "", fmt.Errorf("no iso matching %s in %s", re, base)
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(iso, "/"), "", nil
}

// latestDir returns the version-sorted greatest subdirectory of base
// matching re (e.g. release folders on a mirror). The match is anchored
// to `href="...name/"` so stray numbers in page text (years, sizes)
// are never mistaken for directories.
func latestDir(base, re string) (string, error) {
	body, err := fetchString(base)
	if err != nil {
		return "", err
	}
	var best string
	for _, m := range regexp.MustCompile(`href="[^"]*?/?(`+re+`)/"`).FindAllStringSubmatch(body, -1) {
		if best == "" || versionLess(best, m[1]) {
			best = m[1]
		}
	}
	if best == "" {
		return "", fmt.Errorf("no dir matching %s in %s", re, base)
	}
	return strings.TrimRight(base, "/") + "/" + strings.Trim(best, "/"), nil
}

// sfLatest resolves a SourceForge project's newest uploaded artifact
// (files/latest/download redirect).
func sfLatest(project string) (string, string, error) {
	u, err := finalURL("https://sourceforge.net/projects/" + project + "/files/latest/download")
	return u, "", err
}

// sfISO lists a SourceForge files directory and returns a direct
// /download URL for the greatest match of re.
func sfISO(project, dir, re string) (string, string, error) {
	return sfISOat(fmt.Sprintf("https://sourceforge.net/projects/%s/files/%s", project, dir), re)
}

// sfISOat is sfISO on an already-resolved files URL (nested dirs).
func sfISOat(base, re string) (string, string, error) {
	iso, _, err := indexISO(base, re)
	if err != nil {
		return "", "", err
	}
	return strings.TrimRight(iso, "/") + "/download", "", nil
}

// sfLatestDir returns the newest "name"-listed subdirectory of a
// SourceForge files/ listing (version-sorted, trailing slash kept).
func sfLatestDir(project, sub, re string) (string, error) {
	base := fmt.Sprintf("https://sourceforge.net/projects/%s/files/%s", project, sub)
	body, err := fetchString(base)
	if err != nil {
		return "", err
	}
	var best string
	for _, m := range matchAll(body, `"name":"(`+re+`)"`) {
		if best == "" || versionLess(best, m) {
			best = m
		}
	}
	if best == "" {
		return "", fmt.Errorf("sf:%s: no dir matching %s", project, re)
	}
	return strings.TrimRight(base, "/") + "/" + url.PathEscape(best) + "/", nil
}

// indexISOsha is indexISO plus the "<file>.sha256" sidecar digest
// (Bandshed-style mirror layout).
func indexISOsha(base, re string) (string, string, error) {
	return indexISOsum(base, re, ".sha256")
}

// indexISOsum is indexISO plus a "<file><ext>" sidecar digest
// (e.g. .sha256, .md5).
func indexISOsum(base, re, ext string) (string, string, error) {
	u, _, err := indexISO(base, re)
	if err != nil {
		return "", "", err
	}
	return u, sumAt(u + ext), nil
}

// indexISOsums is indexISO plus a digest lookup for the resolved file
// inside a multi-file sums manifest (SHA256SUMS, CHECKSUM, MANIFEST).
func indexISOsums(base, isoRe, sumsName string) (string, string, error) {
	u, _, err := indexISO(base, isoRe)
	if err != nil {
		return "", "", err
	}
	sha := ""
	if b, err := fetchString(strings.TrimRight(base, "/") + "/" + sumsName); err == nil {
		sha = sumForISO(b, u[strings.LastIndex(u, "/")+1:])
	}
	return u, sha, nil
}

var hexRe = regexp.MustCompile(`^[a-fA-F0-9]{32}$|^[a-fA-F0-9]{40}$|^[a-fA-F0-9]{64}$|^[a-fA-F0-9]{128}$`)

// sumAt fetches a checksum file/URL and returns the first hex digest.
func sumAt(u string) string {
	body, err := fetchString(u)
	if err != nil {
		return ""
	}
	return regexp.MustCompile(`[a-fA-F0-9]{128}|[a-fA-F0-9]{64}|[a-fA-F0-9]{40}|[a-fA-F0-9]{32}`).FindString(body)
}

// sumForISO finds iso's digest inside a sums manifest body; supports
// "<hex> <name>" and BSD-style "SHA256 (<name>) = <hex>" lines.
func sumForISO(body, iso string) string {
	for _, l := range strings.Split(body, "\n") {
		if !strings.Contains(l, iso) {
			continue
		}
		for _, f := range strings.Fields(l) {
			f = strings.Trim(f, "()= \t")
			if hexRe.MatchString(f) {
				return strings.ToLower(f)
			}
		}
	}
	return ""
}

// errScrapeSkip marks an OS name outside a sub-resolver's mechanism
// group; the dispatcher tries the next group on errors.Is match.
var errScrapeSkip = errors.New("scrape: unhandled family")
