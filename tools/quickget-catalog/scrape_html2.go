//lint:file-ignore ST1005 Bash parity error strings keep capitalization/punctuation.

// scrape_html2 — HTML/regex page-scrape resolvers, alphabetical band m–z
// (split of the original scrapeHTML dispatch). Package-private; the
// scrape() chain calls scrapeHTML, which fans out to both bands.
package main

import (
	"fmt"
	"regexp"
	"strings"
)

// htmlNZResolvers maps OS name → HTML/regex page-scrape resolver; the
// bodies are the original scrapeHTMLNZ switch cases, verbatim.
var htmlNZResolvers = map[string]func(release, edition, arch string) (string, string, error){
	"reactos": func(release, edition, arch string) (string, string, error) {
		// get_reactos: sourceforge latest-download redirect, no hash.
		u, err := finalURL("https://sourceforge.net/projects/reactos/files/latest/download")
		if err != nil {
			return "", "", err
		}
		return u, "", nil
	},
	"maboxlinux": func(release, edition, arch string) (string, string, error) {
		// get_maboxlinux: sourceforge /download redirect, strip ?query.
		u, err := finalURL("https://sourceforge.net/projects/mabox-linux/files/" + release + "/download")
		if err != nil {
			return "", "", err
		}
		return strings.SplitN(u, "?", 2)[0], "", nil
	},
	"maxlinux": func(release, edition, arch string) (string, string, error) {
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
	},
	"pentoo": func(release, edition, arch string) (string, string, error) {
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
	},
	"tsurugi": func(release, edition, arch string) (string, string, error) {
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
	},
	"tuxedo-os": func(release, edition, arch string) (string, string, error) {
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
	},
	"vitalinux": func(release, edition, arch string) (string, string, error) {
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
	},
	"siduction": func(release, edition, arch string) (string, string, error) {
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
	},
	"nitrux": func(release, edition, arch string) (string, string, error) {
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
	},
	"nyarch": func(release, edition, arch string) (string, string, error) {
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
	},
	"openmandriva": func(release, edition, arch string) (string, string, error) {
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
	},
	"opensuse": func(release, edition, arch string) (string, string, error) {
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
	},
	"os2warp": func(release, edition, arch string) (string, string, error) {
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
	},
	"recreomatic": func(release, edition, arch string) (string, string, error) {
		// get_recreomatic: fixed archive.org ISO.
		return "https://archive.org/download/guadalinex-v9-32B/recreomatic.iso", "", nil
	},
	"pardus": func(release, edition, arch string) (string, string, error) {
		// get_pardus: ED_UPPER edition in iso; no hash.
		iso := fmt.Sprintf("Pardus-%s-%s-latest-amd64.iso", release, strings.ToUpper(edition))
		return "https://indir.pardus.org.tr/ISO/Parduslatest/" + iso, "", nil
	},
	"pureos": func(release, edition, arch string) (string, string, error) {
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
	},
	"rosalinux": func(release, edition, arch string) (string, string, error) {
		// get_rosalinux: ED upper in iso; edition lower in path.
		ed := strings.ToUpper(edition)
		iso := fmt.Sprintf("ROSA.FRESH.%s.13.3.x86_64.iso", ed)
		return fmt.Sprintf("https://mirror.rosa.ru/rosa/rosa13/iso/ROSA.FRESH.13/%s/%s", edition, iso), "", nil
	},
	"slint": func(release, edition, arch string) (string, string, error) {
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
	},
	"truenas-scale": func(release, edition, arch string) (string, string, error) {
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
	},
	"vanillaos": func(release, edition, arch string) (string, string, error) {
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
	},
	"void": func(release, edition, arch string) (string, string, error) {
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
	},
	"whonix": func(release, edition, arch string) (string, string, error) {
		// get_whonix: pinned VER constant.
		ver := "18.2.4.2"
		iso := fmt.Sprintf("Whonix-LXQt-%s.Intel_AMD64.qcow2.libvirt.xz", ver)
		return "https://download.whonix.org/libvirt/" + ver + "/" + iso, "", nil
	},
	"zorin": func(release, edition, arch string) (string, string, error) {
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
	},
	"mageia": func(release, edition, arch string) (string, string, error) {
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
	},
	"mangaka": func(release, edition, arch string) (string, string, error) {
		return "https://archive.org/download/linux-mangaka/linux-mangaka-" + release + "-v2.iso", "", nil
	},
	"metamorphose": func(release, edition, arch string) (string, string, error) {
		return "https://sourceforge.net/projects/metamorphose/files/MetamorphosePuma/Metamorphose_8.5.7-Panther-x64.iso/download", "", nil
	},
	"nova": func(release, edition, arch string) (string, string, error) {
		return "https://sourceforge.net/projects/archiveos/files/n/nova/nova-" + edition + "-" + release + "-amd64.iso/download", "", nil
	},
	"plan9": func(release, edition, arch string) (string, string, error) {
		iso := "plan9-" + release + ".iso"
		if release == "4e" {
			iso = "plan9-4e-latest.iso"
		}
		return "https://archive.org/download/plan9isosUS/" + iso, "", nil
	},
	"securityonion": func(release, edition, arch string) (string, string, error) {
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
	},
	"uwuntu": func(release, edition, arch string) (string, string, error) {
		return "https://archive.org/download/uw-untu-22.10-desktop-amd-64_202212/UwUntu-22.10-desktop-amd64.iso", "", nil
	},
	"wifislax": func(release, edition, arch string) (string, string, error) {
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
	},
}

func scrapeHTMLNZ(osName, release, edition, arch string) (string, string, error) {
	if fn := htmlNZResolvers[osName]; fn != nil {
		return fn(release, edition, arch)
	}
	return "", "", errScrapeSkip
}
