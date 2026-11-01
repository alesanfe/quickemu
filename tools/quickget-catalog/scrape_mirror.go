// scrape_mirror.go — Mirror/index-template resolvers (original scrape switch).
package main

import (
	"fmt"
	"regexp"
)

func scrapeMirror(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "devuan":
		// Live images live under devuan_<codename>/desktop-live/; the
		// netinst edition resolves the d-i ISO under installer-iso/.
		// Digests are a shared SHA256SUMS.txt or a per-file .sha256
		// sidecar (excalibur).
		subdir, suffix := "desktop-live", "desktop-live"
		if edition == "netinst" {
			subdir, suffix = "installer-iso", "netinstall"
		}
		base := "https://files.devuan.org/devuan_" + release + "/" + subdir + "/"
		re := `devuan_` + regexp.QuoteMeta(release) + `_[0-9.-]+_amd64_` + suffix + `\.iso`
		u, sha, err := indexISOsums(base, re, "SHA256SUMS.txt")
		if err != nil {
			return "", "", err
		}
		if sha == "" {
			sha = sumAt(u + ".sha256")
		}
		return u, sha, nil
	case "openkylin":
		// cdimage index per release; build-date suffixes churn, so scrape.
		kind := map[string]string{"desktop": "Desktop", "server": "Server"}[edition]
		if kind == "" {
			return "", "", fmt.Errorf("openkylin: unknown edition %q", edition)
		}
		u, _, err := indexISO("https://cdimage.openkylin.top/"+release+"/",
			`openKylin-`+kind+`-V`+regexp.QuoteMeta(release)+`-(?:[0-9][^"/]*)?x86_64\.iso`)
		return u, "", err
	}
	return "", "", errScrapeSkip
}
