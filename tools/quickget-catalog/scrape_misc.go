// scrape_misc.go — batch-3 one-off resolvers (case tables, checksum
// manifests, session pages) that don't fit the index/mirror/api
// families; each mirrors its get_<os> case block 1:1.
package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func scrapeMisc(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "bazzite":
		// get_bazzite: edition→iso map (names aren't prefix-consistent:
		// plasma loses its suffix, deck-* keeps only -deck).
		iso := map[string]string{
			"gnome":       "bazzite-gnome-stable-amd64.iso",
			"plasma":      "bazzite-stable-amd64.iso",
			"deck-gnome":  "bazzite-deck-gnome-stable-amd64.iso",
			"deck-plasma": "bazzite-deck-stable-amd64.iso",
		}[edition]
		if iso == "" {
			return "", "", nil
		}
		u := "https://download.bazzite.gg/" + iso
		sha := ""
		if b, err := fetchString(u + "-CHECKSUM"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return u, sha, nil
	case "bodhi":
		// get_bodhi: standard has no edition suffix; every other edition
		// appends -<edition> before .iso.
		iso := fmt.Sprintf("bodhi-%s-64.iso", release)
		if edition != "standard" && edition != "" {
			iso = fmt.Sprintf("bodhi-%s-64-%s.iso", release, edition)
		}
		u := fmt.Sprintf("https://sourceforge.net/projects/bodhilinux/files/%s/%s", release, iso)
		sha := ""
		if b, err := fetchString(u + ".sha256"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return u, sha, nil
	case "gnomeos":
		// get_gnomeos: nightly/3x live under a different host+name and
		// 46.0 drops the minor; the final URL comes from web_redirect.
		iso := fmt.Sprintf("gnome_os_installer_%s.iso", release)
		u := "https://download.gnome.org/gnomeos/" + release
		switch {
		case release == "nightly":
			iso, u = "gnome_os_installer.iso", "https://os.gnome.org/download/latest"
		case release == "46.0":
			iso = "gnome_os_installer_46.iso"
		case strings.HasPrefix(release, "3"):
			iso = "gnome_os_installer.iso"
		}
		fin, err := finalURL(u + "/" + iso)
		if err != nil {
			return "", "", err
		}
		return fin, "", nil
	case "mxlinux":
		// get_mxlinux: edition→iso map with mixed case (Fluxbox is
		// lowercase in the filename).
		iso := map[string]string{
			"Xfce":    fmt.Sprintf("MX-%s_Xfce_x64.iso", release),
			"KDE":     fmt.Sprintf("MX-%s_KDE_x64.iso", release),
			"Fluxbox": fmt.Sprintf("MX-%s_fluxbox_x64.iso", release),
		}[edition]
		if iso == "" {
			return "", "", nil
		}
		u := fmt.Sprintf("https://sourceforge.net/projects/mx-linux/files/Final/%s/%s", edition, iso)
		sha := ""
		if b, err := fetchString(u + ".sha256"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return u, sha, nil
	case "peppermint":
		// get_peppermint: edition picks both the iso name AND the
		// SourceForge subdirectory (XFCE vs Gnome_FlashBack).
		var iso, dir string
		switch edition {
		case "devuan-xfce":
			iso, dir = "PeppermintOS-devuan_64_xfce.iso", "XFCE"
		case "debian-xfce":
			iso, dir = "PeppermintOS-Debian-64.iso", "XFCE"
		case "devuan-gnome":
			iso, dir = "PeppermintOS-devuan_64_gfb.iso", "Gnome_FlashBack"
		case "debian-gnome":
			iso, dir = "PeppermintOS-Debian_64_gfb.iso", "Gnome_FlashBack"
		default:
			return "", "", nil
		}
		u := fmt.Sprintf("https://sourceforge.net/projects/peppermintos/files/isos/%s/%s", dir, iso)
		sha := ""
		if b, err := fetchString(u + "-sha512.checksum"); err == nil {
			sha = sumForISO(b, iso)
		}
		return u, sha, nil
	case "freedos":
		// get_freedos: per-release url/iso table; 1.3/1.4 hashes live in
		// verify.txt's sha256sum section (grep -A8 sha256sum | grep ISO).
		base := "https://www.ibiblio.org/pub/micro/pc-stuff/freedos/files/distributions/" + release
		var u, iso, hashFile string
		switch release {
		case "1.2":
			u, iso, hashFile = base+"/official", "FD12CD.iso", base+"/official/FD12.sha"
		case "1.3":
			u, iso, hashFile = base+"/official", "FD13-LiveCD.zip", base+"/official/verify.txt"
		case "1.4":
			u, iso, hashFile = base, "FD14-LiveCD.zip", base+"/verify.txt"
		default:
			return "", "", nil
		}
		sha := ""
		if b, err := fetchString(hashFile); err == nil {
			if release == "1.2" {
				sha = sumForISO(b, iso)
			} else {
				// Only lines under a "sha256sum" header count.
				in256 := false
				left := 0
				for _, l := range strings.Split(b, "\n") {
					if strings.Contains(l, "sha256sum") {
						in256, left = true, 8
						continue
					}
					if in256 && left > 0 {
						left--
						if strings.Contains(l, iso) {
							if f := strings.Fields(l); len(f) > 0 {
								sha = f[0]
							}
							break
						}
					}
				}
			}
		}
		return u + "/" + iso, sha, nil
	case "tinycore":
		// get_tinycore: arch derives from the EDITION (*Pure* → x86_64,
		// else x86) — not the guest arch; ISO is <ed>-<rel>.0.iso.
		arch := "x86"
		if strings.Contains(edition, "Pure") {
			arch = "x86_64"
		}
		iso := fmt.Sprintf("%s-%s.0.iso", edition, release)
		u := fmt.Sprintf("http://www.tinycorelinux.net/%s.x/%s/release", release, arch)
		sha := ""
		if b, err := fetchString(u + "/" + iso + ".md5.txt"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return u + "/" + iso, sha, nil
	case "garuda":
		// get_garuda: <edition>/latest.iso + sidecar .sha256 (hash-first).
		base := "https://iso.builds.garudalinux.org/iso/latest/garuda"
		iso := edition + "/latest.iso"
		sha := ""
		if b, err := fetchString(base + "/" + iso + ".sha256"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return base + "/" + iso, sha, nil
	case "kdeneon":
		// get_kdeneon: neon-<rel>-<ed>-current.sha256sum is a single
		// "hash iso" line (IFS read: first field hash, second iso).
		base := fmt.Sprintf("https://files.kde.org/neon/images/%s/%s/current", edition, release)
		iso, sha := "", ""
		if b, err := fetchString(base + "/neon-" + release + "-" + edition + "-current.sha256sum"); err == nil {
			f := strings.Fields(b)
			if len(f) > 0 {
				sha = f[0]
			}
			if len(f) > 1 {
				iso = f[1]
			}
		}
		return base + "/" + iso, sha, nil
	case "pclinuxos":
		// get_pclinuxos: newest pclinuxos64-<ed>-<ver>.iso in the index;
		// hash is the first 32 chars of <iso>.md5sum.
		base := "https://ftp.fau.de/pclinuxos/pclinuxos/iso"
		u, _, err := indexISO(base+"/", `pclinuxos64-`+regexp.QuoteMeta(edition)+`-[0-9.]+\.iso`)
		if err != nil || u == "" {
			return "", "", nil // Bash echoes " " when no ISO matches.
		}
		sha := ""
		if b, err := fetchString(u + ".md5sum"); err == nil && len(b) >= 32 {
			sha = b[:32]
		}
		return u, sha, nil
	case "redoxos":
		// get_redoxos: SHA256SUM grep livedisk tail -1 → f1 hash,
		// redox_*_livedisk.iso.zst token is the ISO name.
		base := "https://static.redox-os.org/releases/" + release + "/x86_64"
		iso, sha := "", ""
		if b, err := fetchString(base + "/SHA256SUM"); err == nil {
			var last string
			for _, l := range strings.Split(b, "\n") {
				if strings.Contains(l, "redox_"+edition+"_") && strings.HasSuffix(strings.TrimSpace(l), "livedisk.iso.zst") {
					last = l
				}
			}
			if last != "" {
				f := strings.Fields(last)
				sha = f[0]
				for _, t := range f {
					if strings.HasPrefix(t, "redox_") && strings.HasSuffix(t, "livedisk.iso.zst") {
						iso = t
					}
				}
			}
		}
		return base + "/" + iso, sha, nil
	case "slax":
		// get_slax: edition picks the Slax-1x.x dir; md5.txt 64bit-
		// rows give iso (f3) + md5 (f1), tail -1 each.
		dir := map[string]string{
			"debian":    "https://ftp.fi.muni.cz/pub/linux/slax/Slax-12.x",
			"slackware": "https://ftp.fi.muni.cz/pub/linux/slax/Slax-15.x",
		}[edition]
		if dir == "" {
			return "", "", nil
		}
		iso, sha := "", ""
		if b, err := fetchString(dir + "/md5.txt"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if !strings.Contains(l, "64bit-") {
					continue
				}
				if f := strings.Fields(l); len(f) >= 3 {
					sha, iso = f[0], f[2]
				}
			}
		}
		return dir + "/" + iso, sha, nil
	case "kdelinux":
		// get_kdelinux: newest kde-linux_<12digit>.iso + SHA256SUMS f1.
		base := "https://files.kde.org/kde-linux"
		u, _, err := indexISO(base+"/?C=M;O=D", `kde-linux_[0-9]{12}\.iso`)
		if err != nil || u == "" {
			return "", "", nil
		}
		iso := u[strings.LastIndex(u, "/")+1:]
		sha := ""
		if b, err := fetchString(base + "/SHA256SUMS"); err == nil {
			sha = sumForISO(b, iso)
		}
		return u, sha, nil
	case "kali":
		// get_kali: edition picks the installer suffix; newest
		// kali-linux-*-installer-<suffix> in the M-sorted index;
		// SHA256SUMS minus torrents, hash-first field.
		suffix := "amd64.iso"
		switch edition {
		case "purple":
			suffix = "purple-amd64.iso"
		case "netinst":
			suffix = "netinst-amd64.iso"
		}
		base := "https://cdimage.kali.org/" + release
		u, _, err := indexISO(base+"/?C=M;O=D", `kali-linux-[^>]*-installer-`+regexp.QuoteMeta(suffix))
		if err != nil || u == "" {
			return "", "", nil
		}
		iso := u[strings.LastIndex(u, "/")+1:]
		sha := ""
		if b, err := fetchString(base + "/SHA256SUMS"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if strings.Contains(l, iso) && !strings.Contains(l, "torrent") {
					if f := strings.Fields(l); len(f) > 0 {
						sha = f[0]
					}
					break
				}
			}
		}
		return u, sha, nil
	case "blissos":
		// get_blissos: newest Bliss-v<rel>-*.iso under the Official
		// Gapps/Generic path in the fosshub RSS feed; no checksum.
		body, err := fetchString("https://sourceforge.net/projects/blissos-x86/rss?path=/")
		if err != nil {
			return "", "", err
		}
		re := regexp.MustCompile(`Official/BlissOS[0-9]+/Gapps/Generic/Bliss-v` + regexp.QuoteMeta(release) + `-[^"<]*\.iso`)
		best := ""
		for _, m := range re.FindAllString(body, -1) {
			if versionLess(best, m) {
				best = m
			}
		}
		return "https://downloads.sourceforge.net/project/blissos-x86/" + best, "", nil
	case "nobara":
		// get_nobara: newest Nobara-<rel>-<ed>-<date>.iso on the
		// download page; images live on the nobara-images host.
		body, err := fetchString("https://nobaraproject.org/download-nobara/")
		if err != nil {
			return "", "", err
		}
		re := regexp.MustCompile(`Nobara-` + regexp.QuoteMeta(release) + `-` + regexp.QuoteMeta(edition) + `-[0-9-]+\.iso`)
		best := ""
		for _, m := range re.FindAllString(body, -1) {
			if versionLess(best, m) {
				best = m
			}
		}
		return "https://nobara-images.nobaraproject.org/" + best, "", nil
	case "q4os":
		// get_q4os: edition picks the suffix; rss grep newest
		// files/stable/q4os-<rel>-<sfx><n>.iso; URL drops "files/".
		sfx := map[string]string{"kde": `x64\.r`, "trinity": `x64-tde\.r`}[edition]
		if sfx == "" {
			return "", "", nil
		}
		body, err := fetchString("https://sourceforge.net/projects/q4os/rss?path=/stable")
		if err != nil {
			return "", "", err
		}
		re := regexp.MustCompile(`files/stable/q4os-` + regexp.QuoteMeta(release) + `-` + sfx + `[0-9]+\.iso`)
		best := ""
		for _, m := range re.FindAllString(body, -1) {
			if versionLess(best, m) {
				best = m
			}
		}
		return "https://downloads.sourceforge.net/project/q4os/" + strings.TrimPrefix(best, "files/"), "", nil
	case "android":
		// get_android: fosshub var settings JSON picks the iso+sha256
		// by name prefix android-<ed>-<rel>; the file itself is located
		// by walking OSDN 5-digit dirs newest-first.
		body, err := fetchString("https://www.fosshub.com/Android-x86-old.html")
		if err != nil {
			return "", "", err
		}
		iso, sha := "", ""
		if i := strings.Index(body, "var settings ="); i >= 0 {
			raw := body[i+len("var settings ="):]
			if j := strings.Index(raw, "</script>"); j >= 0 {
				raw = raw[:j]
			}
			raw = strings.TrimRight(strings.TrimSpace(raw), ";")
			var settings struct {
				Pool struct {
					F []struct {
						N    string `json:"n"`
						Hash struct {
							Sha256 string `json:"sha256"`
						} `json:"hash"`
					} `json:"f"`
				} `json:"pool"`
			}
			if json.Unmarshal([]byte(raw), &settings) == nil {
				want := "android-" + edition + "-" + release
				for _, f := range settings.Pool.F {
					if strings.HasPrefix(f.N, want) && strings.HasSuffix(f.N, ".iso") {
						iso, sha = f.N, f.Hash.Sha256
						break
					}
				}
			}
		}
		base := "https://mirrors.gigenet.com/OSDN/android-x86"
		dir, err := fetchString(base)
		if err != nil {
			return "", "", err
		}
		dirs := regexp.MustCompile(`[0-9]{5}`).FindAllString(dir, -1)
		sortDesc(dirs)
		for _, d := range dirs {
			if idx, err := fetchString(base + "/" + d); err == nil && strings.Contains(idx, iso) {
				return base + "/" + d + "/" + iso, sha, nil
			}
		}
		return base + "/" + iso, sha, nil
	case "gentoo":
		// get_gentoo: the <release>-iso.txt manifest names the ISO,
		// then <ISO>.DIGESTS carries the sha512 (line after the SHA512
		// header, iso-named, skipping the CONTENTS entry).
		base := "https://mirrors.kernel.org/gentoo/releases/amd64/autobuilds"
		want := "install"
		if edition == "livegui" {
			want = "livegui"
		}
		body, err := fetchString(base + "/" + release + "-iso.txt")
		if err != nil {
			return "", "", err
		}
		iso := ""
		for _, l := range strings.Split(body, "\n") {
			if f := strings.Fields(l); len(f) > 0 && strings.Contains(l, want) {
				iso = f[0]
				break
			}
		}
		if iso == "" {
			return "", "", fmt.Errorf("gentoo: no iso in %s-iso.txt", release)
		}
		sha := ""
		if b, err := fetchString(base + "/" + iso + ".DIGESTS"); err == nil {
			lines := strings.Split(b, "\n")
			for i, l := range lines {
				if !strings.Contains(l, "SHA512") || i+1 >= len(lines) {
					continue
				}
				nx := lines[i+1]
				if strings.Contains(nx, "iso") && !strings.Contains(nx, "CONTENTS") {
					if f := strings.Fields(nx); len(f) > 0 {
						sha = f[0]
						break
					}
				}
			}
		}
		return base + "/" + iso, sha, nil
	}
	return "", "", errScrapeSkip
}
