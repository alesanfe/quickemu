//lint:file-ignore ST1005 Bash parity error strings keep capitalization/punctuation.

// scrape_html — HTML/regex page-scrape resolvers, alphabetical band a–l
// (split of the original scrapeHTML dispatch). Package-private; the
// scrape() chain calls scrapeHTML, which fans out to both bands.
package main

import (
	"errors"
	"fmt"
	"strings"
)

func scrapeHTMLAM(osName, release, edition, arch string) (string, string, error) {
	switch osName {
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
	case "9front":
		// get_9front: index page, latest 9front-*.amd64.iso.gz
		// (upstream ships the ISO gzip-compressed; imageNormalize unpacks).
		body, err := fetchString("https://9front.org/iso/")
		if err != nil {
			return "", "", err
		}
		iso := latestMatch(body, `9front-[0-9]+\.amd64\.iso\.gz`)
		if iso == "" {
			return "", "", fmt.Errorf("9front: no iso in index")
		}
		return "https://9front.org/iso/" + iso, "", nil
	case "archcraft":
		// get_archcraft: sourceforge /download redirect, strip ?query.
		u, err := finalURL("https://sourceforge.net/projects/archcraft/files/" + release + "/download")
		if err != nil {
			return "", "", err
		}
		return strings.SplitN(u, "?", 2)[0], "", nil
	case "azurelinux":
		// get_azurelinux: aka.ms redirect → final URL is the ISO.
		qarch := "x86_64"
		if arch == "arm64" {
			qarch = "aarch64"
		}
		u, err := finalURL(fmt.Sprintf("https://aka.ms/azurelinux-%s-%s.iso", release, qarch))
		if err != nil {
			return "", "", err
		}
		return u, "", nil
	case "backbox":
		// get_backbox: download page first .iso href; mirror fallback.
		body, err := fetchString("https://linux.backbox.org/download/")
		if err != nil {
			return "", "", err
		}
		iso := findLine(body, `href="([^"]*\.iso)`, 1)
		if iso == "" {
			iso = fmt.Sprintf("https://backbox.mirror.garr.it/backbox-%s-desktop-amd64.iso", release)
		}
		return iso, "", nil
	case "batocera":
		// get_batocera: stable/<rel> dir, batocera*.img.gz name.
		u := "https://mirrors.o2switch.fr/batocera/x86_64/stable/" + release
		body, err := fetchString(u + "/")
		if err != nil {
			return "", "", err
		}
		iso := findLine(body, `"(batocera[^"]*\.img\.gz)"`, 1)
		if iso == "" {
			iso = findLine(body, `(batocera[^\s"]*\.img\.gz)`, 1)
		}
		return u + "/" + iso, "", nil
	case "endeavouros":
		// get_endeavouros: mirror ISO list, name matching release
		// (case-insensitive), excluding x86_64 entries; .sha512sum.
		u := "https://mirror.alpix.eu/endeavouros/iso"
		body, err := fetchString(u + "/")
		if err != nil {
			return "", "", err
		}
		iso := ""
		for _, m := range matchAll(body, `<a href="([^"]*\.iso)">`) {
			if strings.Contains(strings.ToLower(m), strings.ToLower(release)) && !strings.Contains(m, "x86_64") {
				iso = m
				break
			}
		}
		// fallback: any href ending in .iso
		if iso == "" {
			for _, m := range matchAll(body, `href="([^"]*)"`) {
				if strings.Contains(strings.ToLower(m), strings.ToLower(release)) && strings.HasSuffix(m, ".iso") && !strings.Contains(m, "x86_64") {
					iso = m
					break
				}
			}
		}
		if iso == "" {
			return "", "", fmt.Errorf("endeavouros: no iso for %s", release)
		}
		sha := ""
		if b, err := fetchString(u + "/" + iso + ".sha512sum"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return u + "/" + iso, sha, nil
	case "kicksecure":
		// get_kicksecure: newest version dir → Kicksecure-CLI-*.qcow2.libvirt.xz
		base := "https://download.kicksecure.com/libvirt"
		body, err := fetchString(base + "/")
		if err != nil {
			return "", "", err
		}
		ver := latestMatch(body, `href="([0-9.]+)/"`)
		if ver == "" {
			return "", "", fmt.Errorf("kicksecure: no version dir")
		}
		body, err = fetchString(base + "/" + ver + "/")
		if err != nil {
			return "", "", err
		}
		iso := findLine(body, `(Kicksecure-CLI-[^" ]*Intel_AMD64\.qcow2\.libvirt\.xz)`, 1)
		return fmt.Sprintf("%s/%s/%s", base, ver, iso), "", nil
	case "chimeralinux":
		// get_chimeralinux: DATE from sha256sums.txt line1 field5; hash
		// is the x86_64-LIVE <edition> line field1.
		u := "https://repo.chimera-linux.org/live/" + release
		body, err := fetchString(u + "/sha256sums.txt")
		if err != nil {
			return "", "", err
		}
		first := strings.SplitN(body, "\n", 2)[0]
		date := ""
		if f := strings.Split(first, "-"); len(f) >= 5 {
			date = f[4]
		}
		iso := fmt.Sprintf("chimera-linux-x86_64-LIVE-%s-%s.iso", date, edition)
		sha := ""
		for _, l := range strings.Split(body, "\n") {
			if strings.Contains(l, "x86_64-LIVE") && strings.Contains(l, edition) {
				if f := strings.Fields(l); len(f) > 0 {
					sha = f[0]
				}
				break
			}
		}
		return u + "/" + iso, sha, nil
	case "antix":
		// get_antix: edition→iso suffix + runit url/readme variants.
		u := fmt.Sprintf("https://sourceforge.net/projects/antix-linux/files/Final/antiX-%s", release)
		readme := "README"
		iso := fmt.Sprintf("antiX-%s", release)
		if strings.Contains(edition, "runit") {
			iso += "-runit"
			readme = "README2"
			if release == "21" {
				u += "/runit-bullseye"
			} else {
				u += "/runit-antiX-" + release
			}
		}
		switch {
		case strings.HasPrefix(edition, "base-"):
			iso += "_x64-base.iso"
		case strings.HasPrefix(edition, "core-"):
			iso += "_x64-core.iso"
		case strings.HasPrefix(edition, "full-"):
			iso += "_x64-full.iso"
		case strings.HasPrefix(edition, "net-"):
			iso += "-net_x64-net.iso"
		}
		sha := ""
		if b, err := fetchString(u + "/" + readme + ".txt"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if strings.Contains(l, iso) {
					if f := strings.Fields(l); len(f) > 0 {
						sha = f[0]
					}
					break
				}
			}
		}
		return u + "/" + iso, sha, nil
	case "crunchbang++":
		// get_crunchbang++: GH releases API (CBPP/cbpp), first amd64
		// browser_download_url matching the release; no hash upstream.
		body, err := fetchString("https://api.github.com/repos/CBPP/cbpp/releases")
		if err != nil {
			return "", "", err
		}
		for _, m := range matchAll(body, `"browser_download_url": ?"([^"]+)"`) {
			if !strings.Contains(m, "amd64") {
				continue
			}
			if release == "" || release == "latest" || strings.Contains(m, "cbpp-"+release+"-") {
				return m, "", nil
			}
		}
		return "", "", fmt.Errorf("crunchbang++: no amd64 iso for %s", release)
	case "debian":
		// get_debian: archive base; current release → debian-cd; netinst
		// rewrites -live/hybrid→cd; arm64 is netinst-only.
		darch := arch
		if darch == "arm64" && edition != "netinst" {
			return "", "", fmt.Errorf("Debian %s is not available for ARM64. Use 'netinst' edition.", edition)
		}
		u := fmt.Sprintf("https://cdimage.debian.org/cdimage/archive/%s-live/%s/iso-hybrid", release, darch)
		if body, err := fetchString("https://cdimage.debian.org/debian-cd/"); err == nil {
			if cur := findLine(body, `>([0-9]+\.[0-9]+(\.[0-9]+)?)/`, 1); cur == release {
				u = fmt.Sprintf("https://cdimage.debian.org/debian-cd/%s-live/%s/iso-hybrid", release, darch)
			}
		}
		iso := fmt.Sprintf("debian-live-%s-%s-%s.iso", release, darch, edition)
		if edition == "netinst" {
			u = strings.ReplaceAll(strings.ReplaceAll(u, "-live", ""), "hybrid", "cd")
			iso = strings.ReplaceAll(iso, "-live", "")
		}
		sha := ""
		if b, err := fetchString(u + "/SHA512SUMS"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if strings.Contains(l, iso) {
					if f := strings.Fields(l); len(f) > 0 {
						sha = f[0]
					}
					break
				}
			}
		}
		return u + "/" + iso, sha, nil
	case "deepin":
		// get_deepin: 20.9 lacks the /amd64 dir; newer releases have it.
		u := "https://cdimage.deepin.com/releases/" + release
		if release != "20.9" {
			u += "/amd64"
		}
		iso := fmt.Sprintf("deepin-desktop-community-%s-amd64.iso", release)
		sha := ""
		if b, err := fetchString(u + "/SHA256SUMS"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if strings.Contains(l, iso) {
					if f := strings.Fields(l); len(f) > 0 {
						sha = f[0]
					}
					break
				}
			}
		}
		return u + "/" + iso, sha, nil
	case "easyos":
		// get_easyos: probe the two newest year dirs for the release.
		body, err := fetchString("https://distro.ibiblio.org/easyos/amd64/releases/kirkstone/")
		if err != nil {
			return "", "", err
		}
		iso := fmt.Sprintf("easy-%s-amd64.img", release)
		years := matchAll(body, `([0-9]{4})/`)
		for i, j := 0, len(years)-1; i < j; i, j = i+1, j-1 {
			years[i], years[j] = years[j], years[i]
		}
		for n, year := range years {
			if n >= 2 {
				break
			}
			u := fmt.Sprintf("https://distro.ibiblio.org/easyos/amd64/releases/kirkstone/%s/%s", year, release)
			if webCheck(u + "/") {
				sha := ""
				if b, err := fetchString(u + "/md5.sum.txt"); err == nil {
					if f := strings.Fields(b); len(f) > 0 {
						sha = f[0]
					}
				}
				return u + "/" + iso, sha, nil
			}
		}
		return "", "", fmt.Errorf("easyos: no year dir for %s", release)
	case "endless":
		// get_endless: per-edition FILE_TS constants.
		ts := map[string]string{
			"base": "241023-183516", "en": "241023-200926",
			"es": "241023-184649", "fr": "241023-191212", "pt_BR": "241023-191427",
		}[edition]
		u := fmt.Sprintf("https://images-dl.endlessm.com/release/%s/eos-amd64-amd64/%s", release, edition)
		rel3 := release
		if len(rel3) > 3 {
			rel3 = rel3[:3]
		}
		iso := fmt.Sprintf("eos-eos%s-amd64-amd64.%s.%s.iso", rel3, ts, edition)
		return u + "/" + iso, "", nil
	case "hamonikr":
		// get_hamonikr: edition→iso naming + static hash table.
		hashes := map[string]string{
			"lts":  "580687cc7274f6a2f2bdf81d0e4b11512ad5c37cee019df748b43e7c61d950e1",
			"hwe":  "e781f32da41b654db16406737ea5aa96271d4e461cbe2b3a58f536dfeaa095bf",
			"lite": "7133b53dc211e37827ea29268216de6a11f8308b442ba143ec85f809833348db",
		}
		names := map[string]string{"lts": "LTS", "hwe": "HWE", "lite": "Lite"}
		iso := fmt.Sprintf("hamonikr-kumkang-%s-%s-amd64.iso", release, names[edition])
		return "https://mirror.hamonikr.org/hamonikr/" + iso, hashes[edition], nil
	case "altlinux":
		// get_altlinux: official FTP mirror, per-edition naming.
		base := "https://ftp.altlinux.org/pub/distributions/ALTLinux/images/p" + release + "/" + edition + "/x86_64"
		iso := map[string]string{
			"workstation": "alt-workstation-11.2-x86_64.iso",
			"simply":      "slinux-11.1-x86_64.iso",
			"server":      "alt-server-11.1-x86_64.iso",
			"education":   "alt-education-11.2-x86_64.iso",
		}[edition]
		if edition == "kworkstation" {
			iso = "alt-kworkstation-11-live-x86_64.iso"
			base = "https://ftp.altlinux.org/pub/distributions/ALTLinux/images/p" + release + "/kworkstation"
		}
		if iso == "" {
			return "", "", fmt.Errorf("altlinux: no iso for edition %s", edition)
		}
		return base + "/" + iso, "", nil
	case "blankon":
		return "https://jahitan.blankonlinux.id/releases/current/blankon-sinambung-26.09-verbeek-amd64.iso", "", nil
	case "bosslinux":
		return "https://sourceforge.net/projects/archiveos/files/b/boss/boss-8.0-amd64-DVD-301019.iso/download", "", nil
	case "canaima":
		// get_canaima: CNTI download host; 8.4 carries a pinned md5.
		if release == "9.0" {
			return "https://descargas.softwarelibre.gob.ve/canaima/iso/wakupe/canaima-9.0-" + edition + "-beta_amd64.iso", "", nil
		}
		return "https://descargas.softwarelibre.gob.ve/canaima/iso/kavanayen/canaima-8.4-1-ISO-FULL_amd64.iso", "99c1820f7412d9b20cf81843f898ec1c", nil
	case "educaandos":
		// get_educaandos: cica.es mirror, per-release filename.
		iso := map[string]string{
			"v2":               "EducaAndosV2-offline.iso",
			"20.04-2021122201": "educaandos-20.04-2021122201.iso",
			"20.04-2021060801": "educaandos-20.04-instalador-centros-2021060801.iso",
			"20.04-2024013001": "educaandos-20.04-instalador-centros-2024013001.iso",
			"20.04-20241107":   "educaandos-20.04-instalador-centros-20241107.iso",
			"20.04-2025021001": "educaandos-20.04-instalador-centros-2025021001.iso",
		}[release]
		if iso == "" {
			iso = "educaandos-20.04-instalador-centros.iso"
		}
		return "http://guadalinex-edu.cica.es/centros/" + iso, "", nil
	case "esfera":
		return "https://archive.org/download/guadalinex-v9-32B/esfera-live-" + release + ".iso", "", nil
	case "fedoracore":
		iso := "FC" + release + "-i386-disc1.iso"
		if release == "1" {
			iso = "yarrow-i386-disc1.iso"
		}
		return "https://archives.fedoraproject.org/pub/archive/fedora/linux/core/" + release + "/i386/iso/" + iso, "", nil
	case "gecos":
		iso := "GECOS-v2-32B-latest.iso"
		if edition == "lite" {
			iso = "GECOS-v2-lite-32B-20141017.iso"
		}
		return "https://archive.org/download/guadalinex-v9-32B/" + iso, "", nil
	case "kurumin":
		iso := "kurumin-ng_" + release + ".iso"
		if release == "7.0" {
			iso = "kurumin-7.0.iso"
			if edition == "light" {
				iso = "kurumin-light-7.0.iso"
			}
		}
		return "https://sourceforge.net/projects/archiveos/files/k/kurumin/" + iso + "/download", "", nil
	case "lernstick":
		iso := "lernstick_debian" + release + "_" + edition + "_latest.iso"
		if edition == "exam" {
			iso = "lernstick_pruefungsumgebung_debian" + release + "_latest.iso"
		}
		return "https://releases.lernstick.ch/" + iso, "", nil
	case "life":
		iso := "Life-" + release + ".amd64.iso"
		if release == "22.04" {
			iso = "Li-f-e-22.04.3.amd64.iso"
		}
		return "https://sourceforge.net/projects/cyberorg-home/files/Li-f-e/" + iso + "/download", "", nil
	case "lliurex":
		base := "http://lliurex.net/isos/" + release
		iso := "LliureX-home-desktop_" + release + "_latest.iso"
		switch release {
		case "23":
			iso = "LliureX-" + edition + "_64bits_23_latest.iso"
		case "21.07":
			if e := map[string]string{
				"desktop-fp": "LliureX-desktop-fp_64bits_21_20220406.iso",
				"client-fp":  "LliureX-client-fp_64bits_21_20220406.iso",
				"server-fp":  "LliureX-server-fp_64bits_21_20220406.iso",
			}[edition]; e != "" {
				iso = e
			} else {
				iso = "LliureX-" + edition + "_64bits_21_20220406.iso"
			}
			base = "http://lliurex.net/isos/21.07_64bits/20220406"
		case "19.07":
			if e := map[string]string{
				"escriptori-fp": "lliurex-Escriptori-fp_64bits_19_latest.iso",
				"client-fp":     "lliurex-Client-fp_64bits_19_latest.iso",
				"servidor-fp":   "lliurex-Servidor-fp_64bits_19_latest.iso",
			}[edition]; e != "" {
				iso = e
			} else {
				iso = "lliurex-" + edition + "_64bits_19_latest.iso"
			}
			base = "http://lliurex.net/isos/19.07_64bits"
		case "16.06":
			iso = "lliurex-" + edition + "_64bits_16_latest.iso"
			base = "http://lliurex.net/isos/16.06_64bits"
		}
		return base + "/" + iso, "", nil
	}
	return "", "", errScrapeSkip
}

// scrapeHTML fans out to the two alphabetical bands; kept as the
// single entry in the scrape() resolver chain.
func scrapeHTML(osName, release, edition, arch string) (string, string, error) {
	for _, resolve := range []func(string, string, string, string) (string, string, error){
		scrapeHTMLAM, scrapeHTMLNZ,
	} {
		u, h, err := resolve(osName, release, edition, arch)
		if !errors.Is(err, errScrapeSkip) {
			return u, h, err
		}
	}
	return "", "", errScrapeSkip
}
