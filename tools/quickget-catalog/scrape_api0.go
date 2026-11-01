// scrape_api0.go — JSON/YAML API resolvers (original scrape switch).
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"strconv"
	"strings"
	"time"
)

// api0Resolvers maps OS name → JSON/YAML API resolver; the bodies are
// the original scrapeAPI0 switch cases, verbatim.
var api0Resolvers = map[string]func(release, edition, arch string) (string, string, error){
	"alpine": func(release, edition, arch string) (string, string, error) {
		// get_alpine: fetch latest-releases.yaml, extract the
		// alpine-virt flavor's version + sha256 (real YAML parse —
		// replaces the fragile regex over the document).
		qarch := "x86_64"
		if arch == "arm64" {
			qarch = "aarch64"
		}
		u := fmt.Sprintf("https://dl-cdn.alpinelinux.org/alpine/%s/releases/%s", release, qarch)
		body, err := fetchString(u + "/latest-releases.yaml")
		if err != nil {
			return "", "", err
		}
		var doc []struct {
			Flavor string `yaml:"flavor"`
			Arch   string `yaml:"arch"`
			Iso    string `yaml:"iso"`
			Sha256 string `yaml:"sha256"`
		}
		if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
			return "", "", err
		}
		for _, f := range doc {
			if f.Flavor == "alpine-virt" && (f.Arch == qarch || f.Arch == "") {
				return u + "/" + f.Iso, f.Sha256, nil
			}
		}
		return "", "", fmt.Errorf("alpine-virt flavor not found in latest-releases.yaml")
	},
	"archlinux": func(release, edition, arch string) (string, string, error) {
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
	},
	"manjaro": func(release, edition, arch string) (string, string, error) {
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
	},
	"elementary": func(release, edition, arch string) (string, string, error) {
		// get_elementary: per-release stamp; URL embeds base64(unix ts).
		stamp := map[string]string{
			"7.0": ".20230129rc", "7.1": ".20230926rc",
			"8.0": ".20241122rc", "8.1": "-amd64.20260219",
		}[release]
		iso := fmt.Sprintf("elementaryos-%s-stable%s.iso", release, stamp)
		ts := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().Unix())))
		return "https://dl.elementaryos.org/" + ts + "/" + iso, "", nil
	},
	"fedora": func(release, edition, arch string) (string, string, error) {
		// get_fedora: releases.json select by variant/subvariant/arch/version.
		farch := "x86_64"
		if arch == "arm64" {
			farch = "aarch64"
		}
		if edition == "Everything" {
			// The Everything netinst is not in releases.json; it is
			// indexed under releases/<v>/Everything/<arch>/iso. This
			// is the installer medium anaconda kickstart can drive.
			if strings.HasSuffix(release, "_Beta") {
				return "", "", fmt.Errorf("fedora: Everything not published for %s", release)
			}
			return indexISO(fmt.Sprintf(
				"https://dl.fedoraproject.org/pub/fedora/linux/releases/%s/Everything/%s/iso/",
				release, farch),
				`Fedora-Everything-netinst-`+farch+`-[^"]*\.iso`)
		}
		variant := edition
		switch edition {
		case "Server", "Kinoite", "Onyx", "Silverblue", "Sericea", "Workstation", "KDE":
		default:
			variant = "Spins"
		}
		if n, _ := strconv.Atoi(strings.TrimSuffix(release, "_Beta")); variant == "KDE" && n < 42 {
			variant = "Spins"
		}
		rel := strings.ReplaceAll(release, "_", " ")
		body, err := fetchString("https://getfedora.org/releases.json")
		if err != nil {
			return "", "", err
		}
		var builds []struct {
			Variant, Subvariant, Arch, Version, Link, Sha256 string
		}
		if err := json.Unmarshal([]byte(body), &builds); err != nil {
			return "", "", err
		}
		for _, b := range builds {
			if (b.Variant == variant || b.Variant == edition) &&
				b.Subvariant == edition && b.Arch == farch &&
				b.Version == rel && strings.HasSuffix(b.Link, ".iso") {
				return b.Link, b.Sha256, nil
			}
		}
		return "", "", fmt.Errorf("fedora: no iso for %s %s %s", release, edition, arch)
	},
	"popos": func(release, edition, arch string) (string, string, error) {
		// get_popos: api.pop-os.org JSON {url, sha_sum}.
		body, err := fetchString("https://api.pop-os.org/builds/" + release + "/" + edition)
		if err != nil {
			return "", "", err
		}
		var v struct {
			URL    string `json:"url"`
			ShaSum string `json:"sha_sum"`
		}
		if err := json.Unmarshal([]byte(body), &v); err != nil || v.URL == "" {
			return "", "", fmt.Errorf("popos: no build for %s %s", release, edition)
		}
		return v.URL, v.ShaSum, nil
	},
	"rebornos": func(release, edition, arch string) (string, string, error) {
		// get_rebornos: meta.cdn JSON {url, md5}.
		body, err := fetchString("https://meta.cdn.soulharsh007.dev/RebornOS-ISO?format=json")
		if err != nil {
			return "", "", err
		}
		var v struct {
			URL, Md5 string
		}
		if err := json.Unmarshal([]byte(body), &v); err != nil || v.URL == "" {
			return "", "", fmt.Errorf("rebornos: no iso metadata")
		}
		return v.URL, v.Md5, nil
	},
	"tails": func(release, edition, arch string) (string, string, error) {
		// get_tails: latest.json installations[0] iso target-files[0].
		body, err := fetchString("https://tails.boum.org/install/v2/Tails/amd64/" + release + "/latest.json")
		if err != nil {
			return "", "", err
		}
		var j struct {
			Installations []struct {
				Paths []struct {
					Type  string `json:"type"`
					Files []struct {
						URL, Sha256 string
					} `json:"target-files"`
				} `json:"installation-paths"`
			} `json:"installations"`
		}
		if err := json.Unmarshal([]byte(body), &j); err != nil {
			return "", "", err
		}
		for _, inst := range j.Installations {
			for _, p := range inst.Paths {
				if p.Type == "iso" && len(p.Files) > 0 {
					return p.Files[0].URL, p.Files[0].Sha256, nil
				}
			}
		}
		return "", "", fmt.Errorf("tails: no iso in latest.json for %s", release)
	},
}

func scrapeAPI0(osName, release, edition, arch string) (string, string, error) {
	if fn := api0Resolvers[osName]; fn != nil {
		return fn(release, edition, arch)
	}
	return "", "", errScrapeSkip
}
