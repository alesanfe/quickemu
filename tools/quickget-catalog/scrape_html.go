//lint:file-ignore ST1005 Bash parity error strings keep capitalization/punctuation.

// scrape_html.go — HTML/regex page-scrape resolvers (original scrape switch).
package main

import (
	"fmt"
	"regexp"
	"strings"
)

func scrapeHTML(osName, release, edition, arch string) (string, string, error) {
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
	case "reactos":
		// get_reactos: sourceforge latest-download redirect, no hash.
		u, err := finalURL("https://sourceforge.net/projects/reactos/files/latest/download")
		if err != nil {
			return "", "", err
		}
		return u, "", nil
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
	case "maboxlinux":
		// get_maboxlinux: sourceforge /download redirect, strip ?query.
		u, err := finalURL("https://sourceforge.net/projects/mabox-linux/files/" + release + "/download")
		if err != nil {
			return "", "", err
		}
		return strings.SplitN(u, "?", 2)[0], "", nil
	case "maxlinux":
		// get_maxlinux: MAX-<rel> dir on RedIRIS, first MAX-*.iso; .md5.
		base := "https://ftp.rediris.es/mirror/MAX-interno"
		u := fmt.Sprintf("%s/MAX-%s", base, release)
		body, err := fetchString(u + "/")
		if err != nil {
			return "", "", err
		}
		var iso string
		for _, m := range matchAll(body, `(MAX-[^"<> ]*\.iso)`) {
			if !strings.Contains(m, "md5") {
				iso = m
				break
			}
		}
		if iso == "" {
			return "", "", fmt.Errorf("maxlinux: no iso in MAX-%s", release)
		}
		sha := ""
		if b, err := fetchString(u + "/" + iso + ".md5"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return u + "/" + iso, sha, nil
	case "pentoo":
		// get_pentoo: isos index, latest pentoo-<edition>-*.iso.
		body, err := fetchString("https://www.pentoo.ch/isos/")
		if err != nil {
			return "", "", err
		}
		iso := latestMatch(body, fmt.Sprintf(`pentoo-%s-[a-zA-Z0-9_-]*\.iso`, regexp.QuoteMeta(edition)))
		if iso == "" {
			return "", "", fmt.Errorf("pentoo: no %s iso", edition)
		}
		return "https://www.pentoo.ch/isos/" + iso, "", nil
	case "tsurugi":
		// get_tsurugi: nluug index, latest tsurugi_linux_*.iso; fallback.
		u := "https://ftp.nluug.nl/pub/security/DigitalForensics/tsurugi"
		body, err := fetchString(u + "/")
		if err != nil {
			return "", "", err
		}
		iso := latestMatch(body, `tsurugi_linux_[0-9.]*\.iso`)
		if iso == "" {
			iso = fmt.Sprintf("tsurugi_linux_%s.iso", release)
		}
		return u + "/" + iso, "", nil
	case "tuxedo-os":
		// get_tuxedo-os: current.iso link on homepage → checksums/<iso>.sha256.
		body, err := fetchString("https://os.tuxedocomputers.com/")
		if err != nil {
			return "", "", err
		}
		iso := ""
		for _, m := range matchAll(body, `["'=]([^"'=]*current\.iso)"`) {
			iso = m
			break
		}
		if iso == "" {
			iso = findLine(body, `(TUXEDO-OS-[^"'=]*current\.iso)`, 1)
		}
		if iso == "" {
			return "", "", fmt.Errorf("tuxedo-os: no current.iso link")
		}
		sha := ""
		if b, err := fetchString("https://os.tuxedocomputers.com/checksums/" + iso + ".sha256"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return "https://os.tuxedocomputers.com/" + iso, sha, nil
	case "vitalinux":
		// get_vitalinux: educa.aragon.es index, vitalinux-<rel>*desktop-amd64.iso + .md5.
		u := "https://mirror.vitalinux.educa.aragon.es/isos"
		body, err := fetchString(u + "/")
		if err != nil {
			return "", "", err
		}
		iso := latestMatch(body, fmt.Sprintf(`vitalinux-%s[^"<> ]*desktop-amd64\.iso`, regexp.QuoteMeta(release)))
		if iso == "" {
			iso = "vitalinux-3.5-2026.05.12-desktop-amd64.iso"
		}
		sha := ""
		if b, err := fetchString(u + "/" + iso + ".md5"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return u + "/" + iso, sha, nil
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
	case "siduction":
		// get_siduction: folder name from princeton index, then parse the
		// dotsrc dir for version/date; hash from <iso>.md5.
		body, err := fetchString("https://mirror.math.princeton.edu/pub/siduction/iso/")
		if err != nil {
			return "", "", err
		}
		name := findLine(body, `folder" href="([^"]*)/"`, 1)
		if name == "" {
			name = findLine(body, `>([^<>/]+)/</a>`, 1)
		}
		u := fmt.Sprintf("https://mirrors.dotsrc.org/siduction/iso/%s/%s", name, edition)
		page, err := fetchString(u)
		if err != nil {
			return "", "", err
		}
		md5line := ""
		for _, l := range strings.Split(page, "\n") {
			if strings.Contains(l, ".iso.md5") {
				md5line = l
				break
			}
		}
		version, date := "", ""
		if f := strings.Split(md5line, "-"); len(f) >= 6 {
			version = f[1]
			date = strings.SplitN(f[5], ".", 2)[0]
		}
		iso := fmt.Sprintf("siduction-%s-%s-%s-amd64-%s.iso", version, name, edition, date)
		sha := ""
		if b, err := fetchString(u + "/" + iso + ".md5"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
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
	case "nitrux":
		// get_nitrux: sourceforge RSS, latest .iso filename; .md5sum hash.
		body, err := fetchString("https://sourceforge.net/projects/nitruxos/rss?path=/Release/ISO")
		if err != nil {
			return "", "", err
		}
		iso := findLine(body, `/Release/ISO/([^\]<]*\.iso)`, 1)
		if iso == "" {
			return "", "", fmt.Errorf("nitrux: no iso in rss")
		}
		base := "https://sourceforge.net/projects/nitruxos/files/Release"
		sha := ""
		if b, err := fetchString(base + "/MD5/" + iso + ".md5sum"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if strings.Contains(l, iso) {
					if f := strings.Fields(l); len(f) > 0 {
						sha = f[0]
					}
					break
				}
			}
		}
		return base + "/ISO/" + iso, sha, nil
	case "nyarch":
		// get_nyarch: nv-* prefix → Nv-; kde→KDE else Gnome.
		prefix, ed := "", edition
		if strings.HasPrefix(edition, "nv-") {
			prefix, ed = "Nv-", strings.TrimPrefix(edition, "nv-")
		}
		if ed == "kde" {
			ed = "KDE"
		} else {
			ed = "Gnome"
		}
		iso := fmt.Sprintf("Nyarch-%s%s-%s.iso", prefix, ed, release)
		return fmt.Sprintf("https://sourceforge.net/projects/nyarchlinux/files/%s/download", iso), "", nil
	case "openmandriva":
		// get_openmandriva: edition→suffix; RSS feed, last -<suffix>.x86_64.iso;
		// %28/%29 unescape; hash from downloads.sf.net .sha256sum.
		suffix := map[string]string{
			"plasma": "plasma6x11", "plasma-wayland": "plasma6wayland",
			"gnome": "gnome3", "lxqt": "lxqt",
		}[edition]
		body, err := fetchString("https://sourceforge.net/projects/openmandriva/rss?path=/release/ROME")
		if err != nil {
			return "", "", err
		}
		var iso string
		for _, m := range matchAll(body, fmt.Sprintf(`(files/release/ROME/[^"<]*-%s\.x86_64\.iso)`, suffix)) {
			iso = m // sort -V | tail -1 → last match
		}
		if iso == "" {
			return "", "", fmt.Errorf("openmandriva: no %s iso", edition)
		}
		iso = strings.TrimPrefix(strings.NewReplacer("%28", "(", "%29", ")").Replace(iso), "files/")
		sha := ""
		if b, err := fetchString("https://downloads.sourceforge.net/project/openmandriva/" + iso + ".sha256sum"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return "https://downloads.sourceforge.net/project/openmandriva/" + iso, sha, nil
	case "opensuse":
		// get_opensuse: release→product URL/iso map; hash = .sha256 line 4.
		var u, iso string
		switch release {
		case "tumbleweed":
			u, iso = "https://download.opensuse.org/tumbleweed/iso", "openSUSE-Tumbleweed-DVD-x86_64-Current.iso"
		case "microos":
			u, iso = "https://download.opensuse.org/tumbleweed/iso", "openSUSE-MicroOS-DVD-x86_64-Current.iso"
		case "15.0", "15.1":
			u = "https://download.opensuse.org/distribution/leap/" + release + "/iso"
			iso = fmt.Sprintf("openSUSE-Leap-%s-DVD-x86_64.iso", release)
		case "16.0":
			u = "https://download.opensuse.org/distribution/leap/" + release + "/offline"
			iso = fmt.Sprintf("Leap-%s-offline-installer-x86_64.install.iso", release)
		default:
			u = "https://download.opensuse.org/distribution/leap/" + release + "/iso"
			iso = fmt.Sprintf("openSUSE-Leap-%s-DVD-x86_64-Current.iso", release)
		}
		sha := ""
		if b, err := fetchString(u + "/" + iso + ".sha256"); err == nil {
			lines := strings.Split(b, "\n")
			if len(lines) >= 4 {
				if f := strings.Fields(lines[3]); len(f) > 0 {
					sha = f[0]
				}
			}
		}
		return u + "/" + iso, sha, nil
	case "os2warp":
		// get_os2warp: archive.org per-release ISO map, warp4 default.
		switch release {
		case "3":
			return "https://archive.org/download/ibm-os2-warp-3.0-connect-max-german/CDINST/OS2_Warp3_Connect_Max_de_CD_ROM.iso", "", nil
		case "2.1":
			return "https://archive.org/download/ibm-os2-2.1-german/CD-Inst/OS2_CD_ROM.iso", "", nil
		case "2.0":
			return "https://archive.org/download/ibm-os2-2.0-max-cd-german/OS2Reg20Max/CDINST/OS2_CD_ROM_20_Max.iso", "", nil
		}
		return "https://archive.org/download/IBMOS2Warp4Collection/OS2WARP4-EN.iso", "", nil
	case "recreomatic":
		// get_recreomatic: fixed archive.org ISO.
		return "https://archive.org/download/guadalinex-v9-32B/recreomatic.iso", "", nil
	case "pardus":
		// get_pardus: ED_UPPER edition in iso; no hash.
		iso := fmt.Sprintf("Pardus-%s-%s-latest-amd64.iso", release, strings.ToUpper(edition))
		return "https://indir.pardus.org.tr/ISO/Parduslatest/" + iso, "", nil
	case "pureos":
		// get_pureos: live PureName/PureDate from download page.
		body, err := fetchString("https://www.pureos.net/download/")
		if err != nil {
			return "", "", err
		}
		pname := findLine(body, `downloads\.puri\.sm/([^/"]+)/`, 1)
		pdate := findLine(body, `downloads\.puri\.sm/[^/"]+/[^/"]+/([^/"]+)`, 1)
		ed := strings.ToLower(edition)
		squashed := strings.ReplaceAll(pdate, "-", "")
		u := fmt.Sprintf("https://downloads.puri.sm/%s/%s/%s", pname, ed, pdate)
		iso := fmt.Sprintf("pureos-%s-%s-live-%s_amd64.iso", release, ed, squashed)
		sha := ""
		trimmed := strings.TrimSuffix(iso, ".iso")
		if b, err := fetchString(u + "/" + trimmed + ".checksums_sha256.txt"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if strings.Contains(l, ".iso") {
					sha = strings.SplitN(l, ".", 2)[0]
					break
				}
			}
		}
		return u + "/" + iso, sha, nil
	case "rosalinux":
		// get_rosalinux: ED upper in iso; edition lower in path.
		ed := strings.ToUpper(edition)
		iso := fmt.Sprintf("ROSA.FRESH.%s.13.3.x86_64.iso", ed)
		return fmt.Sprintf("https://mirror.rosa.ru/rosa/rosa13/iso/ROSA.FRESH.13/%s/%s", edition, iso), "", nil
	case "slint":
		// get_slint: MAJ_VER unused for url; hash = <iso>.sha256 field 4.
		u := "https://slackware.uk/slint/x86_64/slint-" + strings.SplitN(release, "-", 2)[0] + "/iso"
		iso := fmt.Sprintf("slint-%s-x86_64.iso", release)
		sha := ""
		if b, err := fetchString(u + "/" + iso + ".sha256"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				f := strings.Fields(l)
				if len(f) >= 4 {
					sha = f[3]
					break
				}
			}
		}
		return u + "/" + iso, sha, nil
	case "truenas-scale":
		// get_truenas-scale: version dir scrape (latest x.y.z) → iso scrape.
		if release == "" {
			return "", "", fmt.Errorf("truenas-scale: empty release")
		}
		base := "https://download.sys.truenas.net"
		dir := "TrueNAS-SCALE-" + strings.ToUpper(release[:1]) + release[1:]
		body, err := fetchString(base + "/" + dir + "/")
		if err != nil {
			return "", "", err
		}
		vdir := latestMatch(body, `([0-9]+\.[0-9]+\.[0-9]+)`)
		if vdir == "" {
			return "", "", fmt.Errorf("truenas-scale: no version dir")
		}
		body, err = fetchString(fmt.Sprintf("%s/%s/%s/", base, dir, vdir))
		if err != nil {
			return "", "", err
		}
		iso := ""
		for _, m := range matchAll(body, `(TrueNAS[^"<]*\.iso)`) {
			if !strings.Contains(m, "gpg") && !strings.Contains(m, "sha") && !strings.Contains(m, "mtree") && !strings.Contains(m, "wrap") {
				iso = m
				break
			}
		}
		if iso == "" {
			return "", "", fmt.Errorf("truenas-scale: no iso")
		}
		sha := ""
		if b, err := fetchString(fmt.Sprintf("%s/%s/%s/%s.sha256", base, dir, vdir, iso)); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return fmt.Sprintf("%s/%s/%s/%s", base, dir, vdir, iso), sha, nil
	case "vanillaos":
		// get_vanillaos: GH releases API, first download_url matching release.
		body, err := fetchString("https://api.github.com/repos/Vanilla-OS/live-iso/releases")
		if err != nil {
			return "", "", err
		}
		iso := ""
		for _, m := range matchAll(body, `"browser_download_url": ?"([^"]+)"`) {
			if strings.Contains(m, release) {
				iso = m
				break
			}
		}
		if iso == "" {
			for _, m := range matchAll(body, `"(https://[^"]*download_url[^"]*|https://github\.com/[^"]*\.iso)"`) {
				if strings.Contains(m, release) {
					iso = m
					break
				}
			}
		}
		if iso == "" {
			return "", "", fmt.Errorf("vanillaos: no iso for %s", release)
		}
		sha := ""
		if b, err := fetchString(strings.TrimSuffix(iso, ".iso") + ".sha256.txt"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return iso, sha, nil
	case "void":
		// get_void: edition→iso variant under live/<release>/.
		var iso string
		switch edition {
		case "glibc":
			iso = fmt.Sprintf("void-live-x86_64-%s-base.iso", release)
		case "musl":
			iso = fmt.Sprintf("void-live-x86_64-musl-%s-base.iso", release)
		case "xfce-glibc":
			iso = fmt.Sprintf("void-live-x86_64-%s-xfce.iso", release)
		case "xfce-musl":
			iso = fmt.Sprintf("void-live-x86_64-musl-%s-xfce.iso", release)
		}
		u := "https://repo-default.voidlinux.org/live/" + release
		sha := ""
		if b, err := fetchString("https://repo-default.voidlinux.org/live/sha256sum.txt"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if strings.Contains(l, iso) {
					if f := strings.Fields(l); len(f) >= 4 {
						sha = f[3]
					}
					break
				}
			}
		}
		return u + "/" + iso, sha, nil
	case "whonix":
		// get_whonix: pinned VER constant.
		ver := "18.2.4.2"
		iso := fmt.Sprintf("Whonix-LXQt-%s.Intel_AMD64.qcow2.libvirt.xz", ver)
		return "https://download.whonix.org/libvirt/" + ver + "/" + iso, "", nil
	case "zorin":
		// get_zorin: edition→name map; index latest non-Beta iso; SHA256SUMS.txt.
		edName := map[string]string{
			"core64": "Core-64-bit", "lite64": "Lite-64-bit",
			"education64": "Education-64-bit",
		}[edition]
		u := "https://plug-mirror.rcac.purdue.edu/zorin-iso/" + release
		body, err := fetchString(u + "/")
		if err != nil {
			return "", "", err
		}
		iso := ""
		for _, m := range matchAll(body, fmt.Sprintf(`(Zorin-OS-[0-9.]*-%s[^"]*\.iso)`, edName)) {
			if !strings.Contains(m, "Beta") && versionLess(iso, m) {
				iso = m
			}
		}
		if iso == "" {
			return "", "", fmt.Errorf("zorin: no %s iso", edition)
		}
		sha := ""
		if b, err := fetchString(u + "/SHA256SUMS.txt"); err == nil {
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
	case "mageia":
		// get_mageia: downloads page 'click here' href → iso; .sha512 sidecar.
		body, err := fetchString("https://www.mageia.org/en/downloads/get/?q=Mageia-" + release + "-Live-" + edition + "-x86_64.iso")
		if err != nil {
			return "", "", err
		}
		iso := ""
		for _, m := range matchAll(body, `href="([^"]*\.iso)"`) {
			iso = m
			break
		}
		if iso == "" {
			return "", "", fmt.Errorf("mageia: no live iso for %s %s", release, edition)
		}
		sha := ""
		if b, err := fetchString(iso + ".sha512"); err == nil {
			if f := strings.Fields(b); len(f) > 0 {
				sha = f[0]
			}
		}
		return iso, sha, nil
	case "mangaka":
		return "https://archive.org/download/linux-mangaka/linux-mangaka-" + release + "-v2.iso", "", nil
	case "metamorphose":
		return "https://sourceforge.net/projects/metamorphose/files/MetamorphosePuma/Metamorphose_8.5.7-Panther-x64.iso/download", "", nil
	case "nova":
		return "https://sourceforge.net/projects/archiveos/files/n/nova/nova-" + edition + "-" + release + "-amd64.iso/download", "", nil
	case "plan9":
		iso := "plan9-" + release + ".iso"
		if release == "4e" {
			iso = "plan9-4e-latest.iso"
		}
		return "https://archive.org/download/plan9isosUS/" + iso, "", nil
	case "securityonion":
		// get_securityonion: GH releases latest tag → download host ISO.
		body, err := fetchString("https://api.github.com/repos/Security-Onion-Solutions/securityonion/releases/latest")
		if err != nil {
			return "", "", err
		}
		ver := ""
		if m := regexp.MustCompile(`"tag_name": ?"([^"]+)"`).FindStringSubmatch(body); len(m) > 1 {
			ver = m[1]
		}
		if ver == "" {
			return "", "", fmt.Errorf("securityonion: no release tag")
		}
		return "https://download.securityonion.net/file/securityonion/securityonion-" + ver + ".iso", "", nil
	case "uwuntu":
		return "https://archive.org/download/uw-untu-22.10-desktop-amd-64_202212/UwUntu-22.10-desktop-amd64.iso", "", nil
	case "wifislax":
		// get_wifislax: archive.org items, per-release iso+item.
		iso, item := "wifislax-4-10-1-final.iso", "wifislax-4-10-1-final_202406"
		switch release {
		case "4.12":
			iso, item = "wifislax-4-12-final%20%5BPIXIESCRIPT%5D.iso", "wifislax-4-9-final"
		case "4.11.1":
			iso, item = "Wifislax-4.11.1.iso", "Wifislax4.11.1"
		case "4.9":
			iso, item = "wifislax-4-9-final.iso", "wifislax-4-9-final"
		case "4.7.2":
			iso, item = "wifislax-4-7-2-final.iso", "wifislax-4-9-final"
		case "4.3":
			iso, item = "wifislax-4-3-fin.iso", "wifislax-4-3-fin"
		}
		return "https://archive.org/download/" + item + "/" + iso, "", nil
	}
	return "", "", errScrapeSkip
}
