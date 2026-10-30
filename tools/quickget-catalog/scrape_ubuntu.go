// scrape_ubuntu.go — Ubuntu-family resolvers + shared flavour helpers.
// scrapeUbuntuFamily serves the original scrape switch; scrapeUbuntu
// serves the batch-2 scrapeRescued chain (ubuntusway).
package main

import (
	"fmt"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func scrapeUbuntuFamily(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "ubuntu", "edubuntu", "kubuntu", "lubuntu", "ubuntu-budgie",
		"ubuntu-mate", "ubuntucinnamon", "ubuntukylin", "ubuntustudio",
		"ubuntu-unity", "xubuntu":
		return getUbuntuFlavour(osName, release, arch, false)
	case "ubuntu-server":
		return getUbuntuFlavour(osName, release, arch, true)
	}
	return "", "", errScrapeSkip
}

// getUbuntuFlavour mirrors get_ubuntu/get_ubuntu-server: pick the
// release dir (daily codename via launchpad when daily*/dvd), then
// grep SHA256SUMS (or MD5SUMS) for the desktop|dvd|install arch iso.
func getUbuntuFlavour(osName, release, arch string, server bool) (string, string, error) {
	var u string
	if strings.HasPrefix(release, "daily") || release == "dvd" {
		codename, err := ubuntuDailyCodename(osName)
		if err != nil || codename == "" {
			return "", "", fmt.Errorf("%s %s is currently unavailable", osName, release)
		}
		u = fmt.Sprintf("https://cdimage.ubuntu.com/%s/%s/daily-live/current", osName, codename)
	} else if server {
		if arch == "arm64" {
			u = fmt.Sprintf("https://cdimage.ubuntu.com/releases/%s/release", release)
		} else {
			u = fmt.Sprintf("https://releases.ubuntu.com/%s", release)
		}
	} else if osName == "ubuntu" && arch == "arm64" {
		u = fmt.Sprintf("https://cdimage.ubuntu.com/releases/%s/release", release)
	} else if osName == "ubuntu" {
		u = fmt.Sprintf("https://releases.ubuntu.com/%s", release)
	} else {
		u = fmt.Sprintf("https://cdimage.ubuntu.com/%s/releases/%s/release", osName, release)
	}
	name := "live-server"
	if server && (strings.HasPrefix(release, "14") || strings.HasPrefix(release, "16")) {
		name = "server"
	}
	// SHA256SUMS preferred, MD5SUMS fallback. Desktop greps
	// desktop|dvd|install + arch, excludes +mac, tail -1, iso = *field2.
	// Server greps NAME + arch; MD5 branch uses space field 3.
	for _, sums := range []string{"SHA256SUMS", "MD5SUMS"} {
		body, err := fetchString(u + "/" + sums)
		if err != nil {
			continue
		}
		var line string
		for _, l := range strings.Split(body, "\n") {
			if !strings.Contains(l, "iso") {
				continue
			}
			if server {
				if !strings.Contains(l, name) {
					continue
				}
			} else {
				if !strings.Contains(l, "desktop") && !strings.Contains(l, "dvd") && !strings.Contains(l, "install") {
					continue
				}
				if strings.Contains(l, "+mac") {
					continue
				}
			}
			if !strings.Contains(l, arch) {
				continue
			}
			line = l
		}
		if line == "" {
			continue
		}
		iso, sha := "", ""
		if server && sums == "MD5SUMS" {
			if f := strings.Fields(line); len(f) >= 3 {
				iso = f[2]
			}
		} else if i := strings.LastIndex(line, "*"); i >= 0 {
			iso = line[i+1:]
		}
		if f := strings.Fields(line); len(f) > 0 {
			sha = f[0]
		}
		if iso == "" || sha == "" {
			continue
		}
		return u + "/" + iso, sha, nil
	}
	return "", "", fmt.Errorf("%s %s is currently unavailable", osName, release)
}

// ubuntuDailyCodename mirrors ubuntu_daily_codename: launchpad series
// newest-first; first name whose daily-live SHA256SUMS exists.
func ubuntuDailyCodename(osName string) (string, error) {
	return qelib.UbuntuDailyCodename(osName)
}

// scrapeUbuntu: batch-2 Ubuntu-derivative resolvers (scrapeRescued chain).
func scrapeUbuntu(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "ubuntusway":
		base := "https://downloads.ubuntusway.com/" + release + "/"
		body, err := fetchString(base)
		if err != nil {
			return "", "", err
		}
		// Newest version dir may exist before its ISOs land (e.g. 26.04
		// while empty) — walk dirs newest-first until one yields an ISO.
		dirs := matchAll(body, `href="[^"]*?/?([0-9]{2}\.[0-9]{2})/"`)
		for i := 1; i < len(dirs); i++ {
			for j := i; j > 0 && versionLess(dirs[j-1], dirs[j]); j-- {
				dirs[j-1], dirs[j] = dirs[j], dirs[j-1]
			}
		}
		isoRe := map[string]string{
			"stable":  `ubuntusway-[0-9.]+-desktop-amd64\.iso`,
			"beta":    `ubuntusway-[0-9.]+-beta-desktop-amd64\.iso`,
			"testing": `ubuntusway-[0-9.]+-dev-[0-9]+-desktop-amd64\.iso`,
		}[release]
		for _, d := range dirs {
			if u, sha, err := indexISOsum(base+d+"/", isoRe, ".sha256.txt"); err == nil {
				return u, sha, nil
			}
		}
		return "", "", fmt.Errorf("ubuntusway: no iso in %s", base)
	}
	return "", "", errScrapeSkip
}
