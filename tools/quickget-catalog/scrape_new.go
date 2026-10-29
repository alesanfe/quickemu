// scrape_new.go — additional scrape resolvers (second catalog-expansion batch).
// Shared helpers keep each case a few lines; every resolver mirrors the
// upstream mirror layout it scrapes.
package main

import (
	"encoding/json"
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

var hexRe = regexp.MustCompile(`^[a-fA-F0-9]{32}$|^[a-fA-F0-9]{64}$`)

// sumAt fetches a checksum file/URL and returns the first hex digest.
func sumAt(u string) string {
	body, err := fetchString(u)
	if err != nil {
		return ""
	}
	return regexp.MustCompile(`[a-fA-F0-9]{64}|[a-fA-F0-9]{32}`).FindString(body)
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

// scrapeRescued handles the second expansion batch; called from scrape's
// default branch.
func scrapeRescued(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	// --- GitHub releases API ---
	case "athenaos":
		return ghISOsha("Athena-OS/athena", `athenaos-live-x86_64\.iso`, `athenaos-live-x86_64\.iso\.sha256`)
	case "chimeraos":
		return ghISOsha("ChimeraOS/install-media", `chimeraos-[^"]*x86_64\.iso`, `sha256sum\.txt`)
	case "crystal-linux":
		return ghISO("crystal-linux/iso", `\.iso$`)
	case "dahliaos":
		return ghISO("dahliaOS/releases", `\.(img|zip)$`)
	case "haos":
		return ghISO("home-assistant/operating-system", `haos_ova-.*\.ova`)
	case "harvester":
		// ISOs are hosted on releases.rancher.com keyed by release tag.
		tag, err := ghLatestTag("harvester/harvester")
		if err != nil {
			return "", "", err
		}
		return fmt.Sprintf("https://releases.rancher.com/harvester/%s/harvester-%s-amd64.iso", tag, tag), "", nil
	case "hellosystem":
		return ghISO("helloSystem/ISO", `hello-.*-amd64\.iso`)
	case "k3os":
		return ghISO("rancher/k3os", `k3os-amd64\.iso`)
	case "kairos":
		return ghISO("kairos-io/kairos", `kairos-.*amd64.*\.iso`)
	case "mocaccinoos":
		// SF: files/<ver>/MocaccinoOS-<EDITION>-<ver>.iso + .sha256.
		ed := map[string]string{"mate": "MATE", "cosmic": "Cosmic", "gnome": "GNOME", "kde": "KDE", "xfce": "XFCE"}[edition]
		if ed == "" {
			return "", "", fmt.Errorf("mocaccinoos: unknown edition %q", edition)
		}
		dir, err := sfLatestDir("mocaccino", "", `v[0-9]+\.[0-9]+`)
		if err != nil {
			return "", "", err
		}
		u, _, err := indexISO(dir, `MocaccinoOS-`+ed+`-[^"]*\.iso`)
		if err != nil {
			return "", "", err
		}
		u = strings.TrimRight(u, "/")
		return u + "/download", u + ".sha256/download", nil
	case "photonos":
		// 5.0 names carry an arch infix; 1.0-4.0 GA isos are
		// photon-<rel>-<hexhash>.iso (hex guard drops .debug.iso).
		re := `photon-[0-9][^"]*x86_64\.iso`
		if release != "5.0" {
			re = `photon-[0-9][0-9.]*-[a-f0-9]+\.iso`
		}
		return indexISO(fmt.Sprintf("https://packages.broadcom.com/photon/%s/GA/iso/", release), re)
	case "taloslinux":
		return ghISO("siderolabs/talos", `metal-amd64\.iso`)
	case "toaruos":
		return ghISO("klange/toaruos", `\.iso$`)
	case "aerynos":
		// cdn.aerynos.dev/isos/: AerynOS-<ver>-GNOME-live-x86_64.iso + .sha256sum.
		return indexISOsum("https://cdn.aerynos.dev/isos/",
			`AerynOS-`+regexp.QuoteMeta(release)+`-GNOME-live-x86_64\.iso`, ".sha256sum")
	case "zimaos":
		return ghISO("IceWhaleTech/ZimaOS", `zimaos.*\.(img|iso)`)
	case "instantos":
		return ghISOsha("instantOS/instantOS", `instantos-[^"]*x86_64\.iso`, `instantos-[^"]*x86_64\.iso\.sha256`)
	case "minios":
		ed := map[string]string{"standard": "standard", "toolbox": "toolbox", "ultra": "ultra"}[edition]
		if ed == "" {
			return "", "", fmt.Errorf("minios: unknown edition %q", edition)
		}
		return ghISOsha("minios-linux/minios-live", `minios-[^"]*-`+ed+`-amd64-[^"]*\.iso`, `minios-[^"]*-`+ed+`-amd64-[^"]*\.iso\.sha256`)
	case "rescuezilla":
		return ghISOsha("rescuezilla/rescuezilla", `rescuezilla-[0-9.]+-64bit\.`+regexp.QuoteMeta(edition)+`\.iso`, `SHA256SUM`)
	case "rancheros":
		return ghISO("rancher/os", `rancheros\.iso`)
	case "shredos":
		return ghISO("PartialVolume/shredos.x86_64", `shredos-[0-9._]+_x86-64_v[0-9.]+_[0-9]+\.iso`)

	// --- SourceForge (batch 3) ---
	case "robolinux":
		// robolinux64-<ed>-vR<ver>.iso and newer robolinux-<ed>-<ver>.iso.
		return sfISO("robolinux", "", `robolinux(64)?-`+regexp.QuoteMeta(edition)+`-(vR)?[0-9.]+[^"/]*\.iso`)
	case "voyager":
		stream := "debian"
		if edition == "lts" {
			stream = "LTS"
		}
		return sfISO("voyagerlive", "", `Voyager-[0-9.]+-`+stream+`-amd64\.iso`)
	case "ubuntuce":
		return sfISO("ubuntuce", "", `ubuntuce-[^"/]*desktop-amd64\.iso`)
	case "smoothwall":
		return sfISO("smoothwall", "SmoothWall/3.1%20SP6", `Express-3\.1-SP6-x86_64\.iso`)
	case "endian":
		return sfISO("efw", "Development/EFW-3.3.2", `community[^"/]*x86-64[^"/]*\.iso`)
	case "t1n1wall":
		return sfISO("t1n1wall", "snapshots/2.11-beta/i386", `generic-pc-[^"/]*\.iso`)
	case "jnode":
		dir, err := sfLatestDir("jnode", "JNode-x86/", `[0-9.]+`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `jnode-x86-[^"/]*\.iso\.gz`)
	case "ultimate-edition":
		return sfISO("ultimateedition", "", `ultimate-arch-server-[0-9.]+-x86_64-[a-z]+\.iso`)
	case "nethserver":
		return sfISO("nethserver", "", `nethserver-[0-9.]+-x86_64\.iso`)
	case "archbang":
		return sfISO("archbang", "ArchBANG", `archbang-[0-9]+-x86_64\.iso`)
	case "geckolinux":
		sub := map[string]string{"rolling": "Rolling", "static": "Static", "next": "NEXT"}[release]
		if sub == "" {
			return "", "", fmt.Errorf("geckolinux: unknown release %q", release)
		}
		ed := map[string]string{
			"cinnamon": "Cinnamon", "xfce": "XFCE", "gnome": "Gnome",
			"kde": "Plasma", "mate": "Mate", "lxqt": "LXQt",
			"budgie": "Budgie", "barebones": "BareBones",
		}[edition]
		if ed == "" {
			return "", "", fmt.Errorf("geckolinux: unknown edition %q", edition)
		}
		dir, err := sfLatestDir("geckolinux", sub+"/", `[0-9][0-9.]+`)
		if err != nil {
			return "", "", err
		}
		// The ISO name embeds the build dir's version: 999.220820.0.
		ver := strings.Trim(strings.TrimRight(dir, "/")[strings.LastIndex(strings.TrimRight(dir, "/"), "/")+1:], "/")
		return sfISOat(dir, `GeckoLinux_`+strings.ToUpper(sub)+`[-_]`+ed+`\.x86_64-`+regexp.QuoteMeta(ver)+`[^"/]*\.iso`)
	case "risios":
		dir, err := sfLatestDir("risios", "", `[0-9]+-[0-9]{8}`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `risiOS-Live-[^"/]*\.iso`)
	case "makulu":
		return sfISO("makulu", "downloads/AIOS", `MakuluLinux-[^"/]*\.iso`)
	case "ezarcher":
		return sfISO("ezarch", "Testing_ISO", `ezarcher-kde-[^"/]*\.iso`)
	case "boot-repair":
		return sfISO("boot-repair-cd", "", `boot-repair-disk-64bit\.iso`)
	case "rescatux":
		dir, err := sfLatestDir("rescatux", "", `rescatux-[0-9.]+`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `rescatux-[^"/]*\.iso`)
	case "lxle":
		return sfISO("lxle", "Final/OS/Focal-64", `LXLE-[^"/]*\.iso`)
	case "furybsd":
		dir, err := sfLatestDir("furybsd", "", `[^"]*-`+regexp.QuoteMeta(strings.ToUpper(edition)))
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `FuryBSD-[^"/]*\.iso`)
	case "caelinux":
		dir, err := sfLatestDir("caelinux", "", `[A-Za-z]*`+regexp.QuoteMeta(release))
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `caelinux`+regexp.QuoteMeta(release)+`lite\.iso`)
	case "wubuntu":
		dir, err := sfLatestDir("windows-linux", "", `[0-9][0-9.]+-[^"/]+`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `winux-[0-9]+\.[0-9][^"/]*\.iso`)
	case "supergrub2":
		arch := "i386_pc"
		if edition == "efi" {
			arch = "i386_efi"
		}
		dir := fmt.Sprintf("https://sourceforge.net/projects/supergrub2/files/%s/super_grub2_disk_%s", release, release)
		u, _, err := sfISOat(dir, `supergrub2-classic-[^"/]*-`+arch+`-CD\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(strings.TrimSuffix(u, "/download") + ".sha256/download"), nil
	case "refracta":
		u, _, err := sfISO("refracta", "isohybrid", `refracta[0-9.]+_xfce_amd64[^"/]*\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt("https://sourceforge.net/projects/refracta/files/isohybrid/SHA256SUMS/download"), nil

	// --- batch 4: SF (cont.) ---
	case "rhino-linux":
		dir, err := sfLatestDir("rhino-linux-builder", "", `[0-9]{4}\.[0-9]+`)
		if err != nil {
			return "", "", err
		}
		u, _, err := sfISOat(dir, `Rhino-Linux-[0-9.]+-amd64\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(strings.TrimSuffix(u, "/download") + ".sha256/download"), nil
	case "ubuntudde":
		dir, err := sfLatestDir("ubuntudde", "", `[0-9.]+`)
		if err != nil {
			return "", "", err
		}
		u, _, err := sfISOat(dir, `ubuntudde-[^"/]*amd64-desktop\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(strings.TrimSuffix(u, "/download") + ".sha256.txt/download"), nil
	case "regataos":
		ed := map[string]string{"en-us": "en-US", "pt-br": "pt-BR", "nv": "NV_en-US"}[edition]
		if ed == "" {
			return "", "", fmt.Errorf("regataos: unknown edition %q", edition)
		}
		dir, err := sfLatestDir("regataos", "", `regataos-[0-9.]+`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `Regata_OS_[0-9]+_`+ed+`\.x86_64-[^"/]*\.iso`)
	case "archman":
		dir, err := sfLatestDir("archman-os", "stable-iso/", `[0-9]{4}-[0-9]{2}`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir+"xfce/", `Archman_Xfce_[0-9]+\.iso`)
	case "kamarada":
		if edition == "gnome" {
			u, _, err := sfISO("kamarada", "manjaro", `kamarada-gnome-[^"/]*\.iso`)
			if err != nil {
				return "", "", err
			}
			return u, sumAt(strings.TrimSuffix(u, "/download") + ".sha256/download"), nil
		}
		return sfISO("kamarada", "15.6", `Linux_Kamarada-15\.6-x86_64-Build[^"/]*\.iso`)
	case "slackel":
		return sfISO("slackel", edition, `slackellive64-`+regexp.QuoteMeta(edition)+`-[0-9.]+\.iso`)
	case "easynas":
		return sfISO("easynas", "EasyNAS%20v1.1", `EasyNAS\.x86_64-[^"/]*install\.iso`)
	case "elastix":
		return sfISO("elastix", "Elastix%20PBX%20Appliance%20Software/5.0.0", `debian-amd64-netinst-elastix\.iso`)
	case "m0n0wall":
		// Dirs: 1.3,1.31..1.33 are stable; the 1.23x betas are older — the
		// 1-2 digit pattern keeps the stable series.
		dir, err := sfLatestDir("m0n0wall", "m0n0wall/", `1\.[0-9]{1,2}`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `cdrom-[^"/]*\.iso`)
	case "dban":
		return sfISO("dban", "dban/dban-2.3.0", `dban-2\.3\.0_i586\.iso`)
	case "drbl":
		dir, err := sfLatestDir("drbl", "drbl_live_stable/", `[0-9.-]+`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `drbl-live-xfce-[^"/]*amd64\.iso`)
	case "septor":
		u, _, err := sfISO("septor", "download_", `Septor-[0-9]+-amd64\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(strings.TrimSuffix(u, "/download") + ".md5.txt/download"), nil
	case "hefftorlinux":
		dir, err := sfLatestDir("hefftorlinux", "", `v[0-9-]+`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `hefftorlinux-`+regexp.QuoteMeta(edition)+`-v[^"/]*x86_64\.iso`)
	case "ctlos":
		dir, err := sfLatestDir("ctlos", "", `v[0-9.]+`)
		if err != nil {
			return "", "", err
		}
		u, _, err := sfISOat(dir, `ctlos_v[^"/]*\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(strings.TrimSuffix(u, "/download") + ".sha256/download"), nil

	// --- batch 4: mirrors/direct ---
	case "adelie":
		// adelie-{inst,live-<de>}-x86_64-<rel>-<build>.iso + SHA512SUMS.
		ed := "inst"
		if edition != "" && edition != "install" {
			ed = "live-" + edition
		}
		return indexISOsums("https://distfiles.adelielinux.org/adelie/"+release+"/iso/",
			`adelie-`+ed+`-x86_64-`+regexp.QuoteMeta(release)+`-[^"/]*\.iso`, "SHA512SUMS")
	case "anolis":
		ed := "minimal"
		if edition != "" {
			ed = edition
		}
		base := "https://mirrors.openanolis.cn/anolis/" + release + "/isos/GA/x86_64/"
		// 8.6+/23.x is AnolisOS-<rel>-x86_64-<ed>.iso; older drops are
		// AnolisOS-<rel>-(GA-|Minimal-)x86_64-dvd.iso word orders.
		if u, s, err := indexISOsum(base, `AnolisOS-`+regexp.QuoteMeta(release)+`-x86_64-`+regexp.QuoteMeta(ed)+`\.iso`, ".md5sum"); err == nil {
			return u, s, nil
		}
		if ed == "dvd" || ed == "boot" {
			return indexISOsum(base, `AnolisOS-`+regexp.QuoteMeta(release)+`-(?:GA-)?x86_64-dvd\.iso`, ".md5sum")
		}
		return indexISOsum(base, `AnolisOS-`+regexp.QuoteMeta(release)+`-(?i:`+regexp.QuoteMeta(ed)+`)-x86_64(?:-dvd)?\.iso`, ".md5sum")
	case "openeuler":
		ed := "netinst"
		if edition != "" {
			ed = edition
		}
		return indexISOsum("https://repo.openeuler.org/openEuler-"+release+"/ISO/x86_64/", `openEuler-`+regexp.QuoteMeta(release)+`-`+regexp.QuoteMeta(ed)+`-x86_64-dvd\.iso`, ".sha256sum")
	case "astralinux":
		return indexISO("https://download.astralinux.ru/astra/stable/orel/iso/", `alce-current\.iso`)
	case "helenos":
		return indexISO("https://www.helenos.org/releases/", `HelenOS-[0-9.]+-amd64\.iso`)
	case "9legacy":
		return "https://9legacy.org/download/9legacy.iso.bz2", "", nil
	case "galliumos":
		// 2.x is galliumos-<ed>-<rel>.iso; 3.x flips the order.
		re := `galliumos-(?:` + regexp.QuoteMeta(edition) + `-` + regexp.QuoteMeta(release) +
			`|` + regexp.QuoteMeta(release) + `-` + regexp.QuoteMeta(edition) + `)\.iso`
		return indexISO("https://galliumos.org/releases/"+release+"/", re)
	case "ventoy":
		return ghISOsha("ventoy/Ventoy", `ventoy-[0-9.]+-livecd\.iso`, `sha256\.txt`)
	case "minikube":
		return ghISOsha("kubernetes/minikube", `minikube-v[0-9.]+-amd64\.iso`, `minikube-v[0-9.]+-amd64\.iso\.sha256`)
	case "crux":
		dir, err := latestDir("https://mirror.freedif.org/crux/", `crux-[0-9.]+`)
		if err != nil {
			return "", "", err
		}
		u, _, err := indexISO(dir+"/iso/", `crux-[0-9.]+\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(u + ".sha256"), nil
	case "sortix":
		dir, err := latestDir("https://pub.sortix.org/sortix/release/", `[0-9]+\.[0-9]+`)
		if err != nil {
			return "", "", err
		}
		return indexISO(dir+"/builds/", `sortix-[^"/]*x86_64\.iso`)
	case "postmarketos":
		base := "https://images.postmarketos.org/bpo/edge/generic-x86_64/" + edition + "/"
		dir, err := latestDir(base, `[0-9]{8}-[0-9]+`)
		if err != nil {
			return "", "", err
		}
		return indexISO(dir+"/", `[0-9]{8}-[0-9]+-postmarketOS-edge-`+regexp.QuoteMeta(edition)+`-generic-x86_64[^"/]*\.img\.xz`)
	case "ovirt-node":
		return indexISO("https://resources.ovirt.org/pub/ovirt-4.5/iso/ovirt-node-ng-installer/", `ovirt-node-ng-installer-latest-`+regexp.QuoteMeta(edition)+`\.iso`)
	case "ploplinux":
		dir, err := latestDir("https://download.plop.at/ploplinux/", `[0-9]{2}\.[0-9]+`)
		if err != nil {
			return "", "", err
		}
		return indexISO(dir+"/live/", `ploplinux-[0-9.]+-x86_64\.iso`)
	case "t2sde":
		u, _, err := indexISO("https://dl.t2sde.org/binary/2024/", `t2-`+regexp.QuoteMeta(release)+`-x86-64-base-wayland-glibc-gcc-nocona\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(strings.TrimSuffix(u, ".iso") + ".sha256"), nil
	case "fiwix":
		body, err := fetchString("https://www.fiwix.org/downloads.html")
		if err != nil {
			return "", "", err
		}
		m := latestMatch(body, `FiwixOS-[0-9.]+-i386\.iso`)
		if m == "" {
			return "", "", fmt.Errorf("fiwix: no iso")
		}
		return "https://www.fiwix.org/" + m, "", nil
	case "sculpt-os":
		body, err := fetchString("https://genode.org/download/sculpt")
		if err != nil {
			return "", "", err
		}
		u := latestMatch(body, `https://genode\.org/files/sculpt/sculpt-[0-9-]+\.img`)
		if u == "" {
			return "", "", fmt.Errorf("sculpt-os: no img")
		}
		return u, "", nil
	case "mikrotik-chr":
		body, err := fetchString("https://upgrade.mikrotik.com/routeros/NEWEST7.stable")
		if err != nil {
			return "", "", err
		}
		f := strings.Fields(body)
		if len(f) == 0 {
			return "", "", fmt.Errorf("mikrotik-chr: no version")
		}
		return fmt.Sprintf("https://download.mikrotik.com/routeros/%s/chr-%s.img.zip", f[0], f[0]), "", nil

	// --- batch 5: SourceForge (cont.) ---
	case "issabel":
		dir, err := sfLatestDir("issabelpbx", "", `Issabel [0-9]+`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `issabel[0-9]-USB-DVD-x86_64-[0-9]+\.iso`)
	case "syslinuxos":
		dir, err := sfLatestDir("syslinuxos", "", `SysLinuxOS-[0-9]+`)
		if err != nil {
			return "", "", err
		}
		ed := map[string]string{"gnome": "GNOME", "mate": "MATE"}[edition]
		if ed == "" {
			return "", "", fmt.Errorf("syslinuxos: unknown edition %q", edition)
		}
		return sfISOat(dir, `SysLinuxOS-[0-9.]+-`+ed+`\.iso`)
	case "cyborghawk":
		return sfISO("cyborghawk", "Cyborg%20Hawk%20v1.0", `Cyborg-Hawk-Linux\.iso`)
	case "cuerdos":
		ed := map[string]string{"xfce": "Xfce", "sway": "Sway", "shell": "Shell", "plasma": "Plasma"}[edition]
		if ed == "" {
			return "", "", fmt.Errorf("cuerdos: unknown edition %q", edition)
		}
		return sfISO("cuerdos", "CuerdOS-2.1.1-"+ed, `CuerdOS-2\.1\.1-`+ed+`\.iso`)
	case "openfiler":
		return sfISO("openfiler", "", `openfileresa-[0-9.]+-x86_64-disc1\.iso`)
	case "redo-rescue":
		u, _, err := sfISO("redobackup", "", `redorescue-[0-9.]+\.iso`)
		if err != nil {
			return "", "", err
		}
		body, err := fetchString("https://sourceforge.net/projects/redobackup/files/SHA256SUMS/download")
		if err != nil {
			return u, "", nil
		}
		iso := strings.TrimSuffix(u, "/download")
		return u, sumForISO(body, iso[strings.LastIndex(iso, "/")+1:]), nil
	case "fuduntu":
		ed := map[string]string{"live": "LiveDVD", "lite": "LiteDVD"}[edition]
		if ed == "" {
			return "", "", fmt.Errorf("fuduntu: unknown edition %q", edition)
		}
		return sfISO("fuduntu", "", `Fuduntu-[0-9.]+-x86_64-`+ed+`\.iso`)
	case "apodio":
		return sfISO("apodio", "", `Apodio[0-9]+-[0-9.]+\.iso`)
	case "openartist":
		return sfISO("openartist", "", `openArtist\.iso`)
	case "pinguy":
		ed := map[string]string{"full": "Full", "mini": "Mini"}[edition]
		if ed == "" {
			return "", "", fmt.Errorf("pinguy: unknown edition %q", edition)
		}
		return sfISO("pinguy-os", "Pinguy_OS_18.04_LTS/"+ed, `Pinguy_OS_[0-9.]+_`+ed+`-LTS_x86-64\.iso`)
	case "blackbuntu":
		return sfISO("blackbuntu", "Community%20Edition/Community%20Edition%200.3", `bb[0-9]+-amd64[^"]*\.iso`)
	case "nodezero":
		return sfISO("nodezero", "NodeZero", `NodeZero\.iso`)
	case "matriux":
		return sfISO("matriux", "matriux_leandros_RC1", `Matriux_Leandros[0-9]+\.iso`)
	case "antergos":
		if edition == "minimal" {
			return sfISO("antergos", "mirror/iso/release", `antergos-minimal-[0-9.]+-x86_64\.iso`)
		}
		return sfISO("antergos", "mirror/iso/release", `antergos-[0-9.]+-x86_64\.iso`)
	case "academix":
		return sfISO("academix", "iso", `AcademiX_[0-9.]+-stable_64bit\.iso`)
	case "arch-linux-gui":
		return sfISO("arch-linux-gui", "", `alg-`+edition+`-[0-9.]+-x86_64\.iso`)

	// --- batch 5: mirrors/direct ---
	case "freepbx":
		pat := `SNGDEB-PBX` + release + `-amd64-[0-9.-]+\.iso`
		if release == "15" {
			pat = `SNG7-FPBX-64bit-[0-9-]+\.iso`
		} else if release == "16" {
			pat = `SNG7-PBX16-64bit-[0-9-]+\.iso`
		}
		return indexISOsum("https://downloads.freepbxdistro.org/ISO/", pat, ".sha256sum")
	case "truenas-core":
		return indexISO("https://download.freenas.org/"+release+"/STABLE/latest/x64/", `TrueNAS-`+regexp.QuoteMeta(release)+`-U[0-9.]+\.iso`)
	case "remixos":
		b := "B2016112101"
		if edition == "32bit" {
			b = "B2016112201"
		}
		return "https://sourceforge.net/projects/remix-os/files/Remix_OS_for_PC_Android_M_" + edition + "_" + b + ".zip/download", "", nil
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
	case "mythbuntu":
		return indexISO("https://cdimage.ubuntu.com/mythbuntu/releases/"+release+"/release/", `mythbuntu-[0-9.]+-desktop-amd64\.iso`)
	case "finnix":
		dir, err := latestDir("https://www.finnix.org/releases/", `[0-9]{1,3}`)
		if err != nil {
			return "", "", err
		}
		return indexISO(dir+"/", `finnix-[0-9]+\.iso`)
	case "kaspersky-rescue":
		return "https://rescuedisk.s.kaspersky-labs.com/updatable/2018/krd.iso", "", nil
	case "ipxe":
		return "https://boot.ipxe.org/ipxe.iso", "", nil

	// --- index/mirror (batch 3) ---
	case "pfsense":
		base := "https://atxfiles.netgate.com/mirror/downloads/"
		u, _, err := indexISO(base, `pfSense-CE-`+regexp.QuoteMeta(release)+`-RELEASE-amd64\.iso\.gz`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(u + ".sha256"), nil
	case "ubuntu-core":
		return indexISOsums("https://cdimage.ubuntu.com/ubuntu-core/"+release+"/stable/current/", `ubuntu-core-`+regexp.QuoteMeta(release)+`-amd64\.img\.xz`, "SHA256SUMS")
	case "opencloudos":
		base := "https://mirrors.opencloudos.tech/opencloudos/" + release + "/isos/x86_64/"
		ed := "minimal"
		if edition != "" {
			ed = edition
		}
		// 9.x nests a dated build dir; 8.x is a flat listing where
		// "everything" is published as dvd1.
		if dir, err := latestDir(base, `[0-9]{8}\.[0-9]+`); err == nil {
			return indexISOsums(dir+"/", `(?i)opencloudos-`+regexp.QuoteMeta(release)+`-[^"/]*-x86_64-`+regexp.QuoteMeta(ed)+`\.iso`, "CHECKSUM")
		}
		// 9.x flat listings put the edition before x86_64; on 8.x
		// "everything" is published as dvd1 after it. 8.6 is all
		// lowercase: opencloudos-8.6-x86_64-<ed>.iso.
		if u, s, err := indexISOsums(base, `(?i)opencloudos-`+regexp.QuoteMeta(release)+`-`+regexp.QuoteMeta(ed)+`-x86_64\.iso`, "CHECKSUM"); err == nil {
			return u, s, nil
		}
		if ed == "everything" {
			ed = "dvd1"
		}
		if u, s, err := indexISOsums(base, `(?i)opencloudos-`+regexp.QuoteMeta(release)+`-x86_64-`+regexp.QuoteMeta(ed)+`\.iso`, "CHECKSUM"); err == nil {
			return u, s, nil
		}
		// 8.10 names carry a build date: OpenCloudOS-8.10-20241210.0-x86_64-<ed>.iso
		return indexISOsums(base, `(?i)opencloudos-`+regexp.QuoteMeta(release)+`-[0-9]{8}\.[0-9]+-x86_64-`+regexp.QuoteMeta(ed)+`\.iso`, "CHECKSUM")
	case "tencentos":
		base := "https://mirrors.tencent.com/tencentos/" + release + "/isos/x86_64/"
		ed := "minimal"
		if edition != "" {
			ed = edition
		}
		// 4.2+ nests a dated build dir; 4.0 is a flat listing where
		// the netinst image replaces boot.
		if dir, err := latestDir(base, `[0-9]{8}\.[0-9]+`); err == nil {
			return indexISOsums(dir+"/", `TencentOS-Server-`+regexp.QuoteMeta(release)+`-[^"/]*-x86_64-`+regexp.QuoteMeta(ed)+`\.iso`, "CHECKSUM")
		}
		if ed == "boot" {
			ed = "netinst"
		}
		return indexISOsums(base, `TencentOS-Server-`+regexp.QuoteMeta(release)+`-`+regexp.QuoteMeta(ed)+`-x86_64\.iso`, "CHECKSUM")
	case "grommunio":
		file := "grommunio.x86_64-latest"
		if edition == "offline" {
			file = "grommunio-offline-latest"
		}
		u := "https://download.grommunio.com/appliance/" + file + ".install.iso"
		return u, sumAt(u + ".sha256"), nil
	case "turnkeylinux":
		ed := "core"
		if edition != "" {
			ed = edition
		}
		u, _, err := indexISO("https://mirror.turnkeylinux.org/turnkeylinux/images/iso/", `turnkey-`+regexp.QuoteMeta(ed)+`-`+regexp.QuoteMeta(release)+`-[a-z]+-amd64\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(u + ".hash"), nil
	case "netrunner":
		return indexISO("https://pub.netrunner.com/", `netrunner-[0-9]+-64-bit\.iso`)
	case "daphile":
		suffix := ""
		if edition == "rt" {
			suffix = "-rt"
		}
		return indexISO("https://www.daphile.com/firmware/stable/", `daphile-[0-9.]+-x86_64`+suffix+`\.iso`)
	case "debian-edu":
		return indexISO("https://get.debian.org/cdimage/release/current/amd64/iso-cd/", `debian-edu-[0-9.]+-amd64-netinst\.iso`)
	case "opensuse-slowroll":
		return indexISO("https://download.opensuse.org/slowroll/iso/", `openSUSE-Slowroll-DVD-x86_64-Build[0-9.]+-Media\.iso`)
	case "hardenedbsd":
		return indexISOsums("https://installers.hardenedbsd.org/pub/"+release+"/amd64/amd64/installer/LATEST/", `disc1\.iso`, "MANIFEST")
	case "zenwalk":
		return indexISOsum("https://download.zenwalk.org/x86_64/"+release+"/", `zenwalk-`+regexp.QuoteMeta(release)+`(-[0-9]+)?\.iso`, ".md5")
	case "freedombox":
		return indexISO("https://ftp.freedombox.org/pub/freedombox/hardware/amd64/"+release+"/", `freedombox-[^"/]*_all-amd64\.img\.xz`)
	case "parabola":
		return indexISO("https://repo.parabola.nu/iso/x86_64-experimental-lxde-2019.06-pre/", `parabola-x86_64-systemd-lxde-[^"/]*complete\.iso`)

	// --- SourceForge ---
	case "avlinux":
		return indexISOsha("https://downloads.bandshed.net/AVL_MXe_25/", `AVL_MXe-[^"]*x64\.iso`)
	case "bsdrp":
		return sfLatest("bsdrp")
	case "drauger-os":
		return indexISO("https://download.draugeros.org/ISOs/", `Drauger_OS-[^"]*AMD64\.iso`)
	case "feren-os":
		return sfLatest("ferenos")
	case "linuxfx":
		return sfLatest("linuxfxdevil")
	case "loc-os":
		// "Loc-OS <rel>" subdirs; releases carry DE variants, 24 is
		// "current". The i686 build must never win over x86_64/amd64.
		base := "Loc-OS%20" + release
		if edition != "" {
			re := `Loc-OS-` + regexp.QuoteMeta(release) + `-(?i:` + regexp.QuoteMeta(edition) + `)[^"]*`
			if u, _, err := sfISO("loc-os", base, re+`(amd64|x86_64)\.iso`); err == nil {
				return u, "", nil
			}
			return sfISO("loc-os", base, re+`\.iso`)
		}
		return sfISO("loc-os", base, `Loc-OS-`+regexp.QuoteMeta(release)+`-[^"]*(amd64|x86_64)\.iso`)
	case "pisilinux":
		// SF "2.4" dir has KDE6/Minimal; lxqt lives in its own dir.
		if edition == "lxqt" {
			return sfISO("pisilinux", "lxqt", `Pisi-Linux-lxqt-[0-9.]+-x86_64\.iso`)
		}
		ed := map[string]string{"kde6": "KDE6", "minimal": "Minimal"}[edition]
		if ed == "" {
			return "", "", fmt.Errorf("pisilinux: unknown edition %q", edition)
		}
		return sfISO("pisilinux", release,
			`Pisi-Linux-`+regexp.QuoteMeta(release)+`\.[0-9]+-`+ed+`-x86_64\.iso`)
	case "vicibox":
		// ViciBox_V<maj>.x86_64-<rel>.iso (+ optional -md minimal variant).
		suf := ""
		if edition == "md" {
			suf = "-md"
		}
		maj := strings.SplitN(release, ".", 2)[0]
		return indexISO("https://download.vicidial.com/iso/vicibox/server/",
			`ViciBox_V`+regexp.QuoteMeta(maj)+`\.x86_64-`+regexp.QuoteMeta(release)+suf+`\.iso`)
	case "primeos":
		return sfISO("primeos", "64-bit", `PrimeOS-[^"]*\.iso`)
	case "xigmanas":
		return sfISO("xigmanas", "", `XigmaNAS-x64-.*\.iso`)
	case "4mlinux":
		return sfLatest("linux4m")
	case "salix":
		return sfISO("salix", release, `salix.*\.iso`)
	case "gparted-live":
		dir, err := sfLatestDir("gparted", "gparted-live-stable/", `[0-9.]+-[0-9]+`)
		if err != nil {
			return "", "", err
		}
		return sfISOat(dir, `gparted-live-[0-9][^"/]*amd64\.iso`)

	// --- directory indexes ---
	case "amazonlinux":
		// os-images/latest/kvm redirects to the newest versioned dir.
		base := "https://cdn.amazonlinux.com/al2023/os-images/latest/kvm/"
		u, _, err := indexISO(base, `al2023-kvm-[^"]*x86_64\.xfs\.gpt\.qcow2`)
		if err != nil {
			return "", "", err
		}
		sha := ""
		if b, err := fetchString(base + "/SHA256SUMS"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if f := strings.Fields(l); len(f) > 0 {
					sha = f[0]
					break
				}
			}
		}
		return u, sha, nil
	case "calculate-linux":
		// mirror.calculate-linux.org/release/<yyyymmdd>/cldx-*-x86_64.iso
		d, err := latestDir("https://mirror.calculate-linux.org/release/", `[0-9]{8}`)
		if err != nil {
			return "", "", err
		}
		u, _, err := indexISO(d+"/", `cldx-[^"]*-x86_64\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, u[:strings.LastIndex(u, "/")] + "/SHA256SUMS", nil
	case "centos":
		// 5.x ships a 9-disc bin set; 6+ uses DVD1; 7 uses DVD-<date>.
		return indexISO("https://vault.centos.org/"+release+"/isos/x86_64/", `CentOS-[^"]*x86_64-([Bb]in-[Dd][Vv][Dd]1|[Dd][Vv][Dd]1|[Dd][Vv][Dd]-[0-9.-]+|[0-9]+-[Dd][Vv][Dd]1|[Bb]in-?1of[0-9]+|[Dd][Vv][Dd])\.iso`)
	case "dietpi":
		return indexISO("https://dietpi.com/downloads/images/", `DietPi_NativePC-BIOS-x86_64[^"]*\.img\.xz`)
	case "eurolinux":
		// vault.cdn flat listing: EL-<major>-x86_64-latest-appstream.iso.
		return indexISO("https://vault.cdn.euro-linux.com/isos/", `EL-`+regexp.QuoteMeta(release)+`-x86_64-latest-appstream\.iso`)
	case "gnu-hurd":
		d, err := latestDir("https://cdimage.debian.org/cdimage/ports/latest/hurd-amd64/", `([0-9]+)`)
		if err != nil {
			return "", "", err
		}
		return indexISO(d, `debian-sid-hurd-amd64-NETINST-1\.iso`)
	case "grml":
		return indexISO("https://mirror.us.leaseweb.net/grml/", `grml-full-[^"]*amd64\.iso`)
	case "ipfire":
		// downloads.ipfire.org index is gone; freedif mirrors the tree.
		d, err := latestDir("https://mirror.freedif.org/ipfire/releases/ipfire-2.x/", `[0-9]+\.[0-9]+-core[0-9]+`)
		if err != nil {
			return "", "", err
		}
		return indexISO(d+"/", `ipfire-[^"]*x86_64\.iso`)
	case "knoppix":
		return indexISO("https://ftp.nluug.nl/pub/os/Linux/distr/knoppix/", `KNOPPIX_V`+regexp.QuoteMeta(release)+`CD-[^"]*EN\.iso`)
	case "minix":
		return indexISO("http://download.minix3.org/iso/", `minix_R`+regexp.QuoteMeta(release)+`[^"]*\.iso\.bz2`)
	case "neptune":
		return indexISO("https://download.neptuneos.com/download/", `Neptune`+regexp.QuoteMeta(release)+`[^"/]*\.iso`)
	case "omnios":
		return indexISO("https://downloads.omnios.org/media/stable/", `omnios-r[^"]*\.iso`)
	case "openmamba":
		body, err := fetchString("https://openmamba.org/en/downloads/")
		if err != nil {
			return "", "", err
		}
		u := latestMatch(body, `https://cdn\.openmamba\.org[^"]*livecd-en-snapshot-[^"]*\.iso`)
		if u == "" {
			return "", "", fmt.Errorf("openmamba: no livecd iso")
		}
		return u, "", nil
	case "opensuse-microos":
		return "https://download.opensuse.org/tumbleweed/iso/openSUSE-MicroOS-DVD-x86_64-Current.iso", "", nil
	case "proxmox-backup-server":
		// release-aware: proxmox-backup-server_4.2-1.iso etc.
		return indexISO("https://enterprise.proxmox.com/iso/", `proxmox-backup-server_`+regexp.QuoteMeta(release)+`\.iso`)
	case "proxmox-mail-gateway":
		// 7.x was proxmox-mailgateway_<v>.iso; 8.x+ is
		// proxmox-mail-gateway_<v>.iso.
		return indexISO("https://enterprise.proxmox.com/iso/", `proxmox-mail-?gateway_`+regexp.QuoteMeta(release)+`\.iso`)
	case "raspberrypios-desktop":
		d, err := latestDir("https://downloads.raspberrypi.org/rpd_x86/images/", `(rpd_x86-[^"/]+)`)
		if err != nil {
			return "", "", err
		}
		return indexISO(d, `[^"/]*\.iso`)
	case "steamos":
		return "https://archive.org/download/SteamOSDVD/SteamOSDVD.iso", "", nil
	case "tribblix":
		// download page lists iso.tribblix.org links (index itself is bare).
		body, err := fetchString("https://www.tribblix.org/download.html")
		if err != nil {
			return "", "", err
		}
		for _, m := range matchAll(body, `(https://iso\.tribblix\.org/iso/tribblix-[^"]*\.iso)`) {
			if !strings.Contains(m, "minimal") && !strings.Contains(m, "sparc") {
				return m, "", nil
			}
		}
		return "", "", fmt.Errorf("tribblix: no iso link")

	case "xcp-ng":
		// mirrors.xcp-ng.org: isos/<major.minor>/xcp-ng-<ver>[-<spin>].iso
		// + SHA256SUMS; 7.6/8.0 have no -<spin> suffix.
		base := "https://mirrors.xcp-ng.org/isos/" + release
		u, _, err := indexISO(base, `xcp-ng-[0-9.]+-[0-9]+\.iso`)
		if err != nil {
			u, _, err = indexISO(base, `xcp-ng-[0-9.]+\.iso`)
		}
		if err != nil {
			return "", "", err
		}
		iso := u[strings.LastIndex(u, "/")+1:]
		sha := ""
		if b, err := fetchString(base + "/SHA256SUMS"); err == nil {
			for _, l := range strings.Split(b, "\n") {
				if strings.Contains(l, iso) {
					if f := strings.Fields(l); len(f) > 0 {
						sha = f[0]
					}
					break
				}
			}
		}
		return u, sha, nil
	case "ultramarine":
		return indexISO("https://images.fyralabs.com/isos/ultramarine/"+release+"/", `ultramarine-[^"]*x86_64[^"]*\.iso`)
	case "umbrelos":
		// download.umbrel.com/release/latest 307-redirects to the R2 asset.
		return "https://download.umbrel.com/release/latest/umbrelos-amd64-usb-installer.iso.xz", "", nil
	case "yunohost":
		return indexISO("https://repo.yunohost.org/images/", `yunohost-[^"]*amd64[^"]*\.iso`)

	// --- index/mirror (batch 4) ---
	case "trisquel":
		// edition→file prefix: mate=trisquel_, lxde=trisquel-mini_,
		// kde=triskel_, sugar=trisquel-sugar_, netinst=trisquel-netinst_.
		prefix := map[string]string{
			"lxde": "trisquel-mini_", "kde": "triskel_",
			"sugar": "trisquel-sugar_", "netinst": "trisquel-netinst_",
		}[edition]
		if prefix == "" {
			prefix = "trisquel_"
		}
		return indexISOsum("https://mirrors.ocf.berkeley.edu/trisquel-images/",
			prefix+regexp.QuoteMeta(release)+`_amd64\.iso`, ".sha256")
	case "bunsenlabs":
		// <codename>-1[-<date>]-amd64.hybrid.iso; carbon+ add a build date.
		return indexISO("https://ddl.bunsenlabs.org/ddl/", release+`-[0-9-]*amd64\.hybrid\.iso`)
	case "ghostbsd":
		// New: GhostBSD-<rel>.iso / GhostBSD-<rel>-XFCE.iso.
		// 18.x-19.x: GhostBSD<rel>.iso (no dash).
		ed := ""
		if edition == "xfce" {
			ed = "-XFCE"
		}
		return indexISOsum("https://download.ghostbsd.org/releases/amd64/"+release+"/",
			`GhostBSD-?`+regexp.QuoteMeta(release)+ed+`\.iso`, ".sha256")
	case "primtux":
		// SF Distribution dir; naming drifted across major versions
		// (PrimTux7-amd64-2022-10, primtux8-amd64, primtux9-m22.3-64bit-*).
		re := `(?i)primtux` + regexp.QuoteMeta(release) + `[^"]*(amd64|64bit)[^"]*\.iso`
		if edition != "" {
			re = `(?i)primtux` + regexp.QuoteMeta(release) + `[^"]*` + regexp.QuoteMeta(edition) + `[^"]*\.iso`
		}
		return sfISO("primtux", "Distribution", re)
	case "fatdog64":
		// Fatdog64-<rel>.iso; 6xx builds also ship -firefox/-seamonkey.
		re := `Fatdog64-` + regexp.QuoteMeta(release) + `\.iso`
		if edition != "" && edition != "standard" {
			re = `(?i)Fatdog64-` + regexp.QuoteMeta(release) + `-` + regexp.QuoteMeta(edition) + `\.iso`
		}
		return indexISOsum("https://ftp.nluug.nl/pub/os/Linux/distr/fatdog/iso/", re, ".md5sum.txt")
	case "sparkylinux":
		// SF per-edition dirs: cli=minimalcli, base=minimalgui;
		// kde/lxqt/mate/xfce/gameover/multimedia/rescue own dirs.
		dir := map[string]string{
			"minimalcli": "cli", "minimalgui": "base",
		}[edition]
		if dir == "" {
			dir = edition
		}
		return sfISO("sparkylinux", dir,
			`sparkylinux-`+regexp.QuoteMeta(release)+`-x86_64-`+regexp.QuoteMeta(edition)+`\.iso`)
	case "peppermintos":
		return sfISO("peppermintos", "isos/XFCE", `(?i)peppermint[^"/]*amd64\.iso`)
	case "android-x86":
		return sfISO("android-x86", "Release%20"+release, `android-x86_64-`+regexp.QuoteMeta(release)+`[^"]*\.iso`)
	case "koozali":
		// releases/<maj>/iso/x86_64/smeserver-<ver>-x86_64[-netinstall].iso;
		// 10.1 only exists as releases/10/. The mirrors round-robin to
		// individual hosts, so fall back across host+dir combinations.
		suf := ""
		if edition == "netinstall" {
			suf = "-netinstall"
		}
		re := `smeserver-[^"/]*-x86_64` + suf + `\.iso`
		maj := strings.SplitN(release, ".", 2)[0]
		for _, base := range []string{
			"https://mirror.koozali.org/smeserver/releases/" + release + "/iso/x86_64/",
			"https://mirror.koozali.org/smeserver/releases/" + maj + "/iso/x86_64/",
			"https://mirror.contribs.org/smeserver/releases/" + release + "/iso/x86_64/",
			"https://mirror.contribs.org/smeserver/releases/" + maj + "/iso/x86_64/",
		} {
			if u, s, err := indexISOsum(base, re, ".sha1sum"); err == nil {
				return u, s, nil
			}
		}
		return "", "", fmt.Errorf("koozali: no smeserver iso for %s/%s", release, edition)
	case "springdale":
		// Springdale%20Linux-<rel>-x86_64-netinst.iso (name has a space).
		base := "https://mirror.math.princeton.edu/pub/springdale/springdale/" + release + "/x86_64/iso/"
		return indexISO(base, `Springdale%20Linux-`+regexp.QuoteMeta(release)+`-x86_64-netinst\.iso`)
	case "solydxk":
		// nluug mirror: solyd{x,k}_<deb>_64_<yyyymm>.iso; x=XFCE, k=KDE.
		flav := map[string]string{"solydx": "x", "solydk": "k"}[edition]
		if flav == "" {
			flav = "x"
		}
		return indexISO("https://ftp.nluug.nl/pub/os/Linux/distr/solydxk/downloads/",
			`solyd`+flav+`_`+regexp.QuoteMeta(release)+`_64_[0-9]+\.iso`)
	case "armbian":
		// Armbian_<ver>_Uefi-x86_<suite>_<channel>_<kernel>_<flav>.iso +
		// .sha sidecar. minimal=plain minimal; DEs -> <de>_desktop.
		flav := edition
		switch flav {
		case "", "minimal":
			flav = "minimal"
		default:
			flav += "_desktop"
		}
		return indexISOsum("https://dl.armbian.com/uefi-x86/archive/",
			`Armbian_[0-9.]+_Uefi-x86_[a-z]+_[a-z]+_[0-9.]+_`+regexp.QuoteMeta(flav)+`\.iso`, ".sha")
	case "predator-os":
		// Dir not listable: deterministic filename per edition.
		infix := map[string]string{
			"sec-plasma":     "SecurityEdition-plasma-lxqt",
			"sec-mate":       "SecurityEdition-mate-lxde",
			"sec-nodesktop":  "Security-NoDesktopEdition",
			"home-plasma":    "HomeEdition-plasma-lxqt",
			"home-mate":      "HomeEdition-mate-lxde",
			"home-nodesktop": "HomeEdition-NoDesktop",
			"home-windows":   "HomeEdition-windows",
			"home-mac":       "HomeEdition-MAC",
		}[edition]
		if infix == "" {
			infix = "SecurityEdition-plasma-lxqt"
		}
		// Release dir drops the dot: v3.5 -> v35.
		return "https://www.seilany.ir/predator-os/download/" + strings.ReplaceAll(release, ".", "") +
			"/Predator-OS-" + release + "-amd64-" + infix + ".iso", "", nil
	case "zentyal":
		// 7.x lives at files/ root; older releases under "Old versions"
		// with point releases like zentyal-2.0-4-amd64.iso.
		dir := ""
		if !strings.HasPrefix(release, "7") {
			dir = "Old%20versions"
		}
		return sfISOat("https://sourceforge.net/projects/zentyal/files/"+dir,
			`zentyal-`+regexp.QuoteMeta(release)+`-amd64\.iso`)
	case "funtoo":
		return sfISO("funtoo", "old-multilib", `Funtoo-x86_64-[0-9-]+\.iso`)
	case "mofolinux":
		return sfISOat("https://sourceforge.net/projects/mofolinux/files",
			`mofolinux-[0-9.]+\.iso`)
	case "openthos":
		return sfISOat("https://sourceforge.net/projects/openthos/files",
			`openthos-[0-9.]+\.img`)
	case "dragora":
		// cedia mirror v3/iso/<stage>/; alphas share the "alpha" dir.
		dir := release
		if strings.HasPrefix(release, "alpha") {
			dir = "alpha"
		}
		return indexISO("https://mirror.cedia.org.ec/dragora/v3/iso/"+dir+"/",
			`dragora-3\.0-(x86_64|amd64)-`+regexp.QuoteMeta(release)+`(-live|-dvd)?\.iso`)

	// --- JSON APIs ---
	case "fedora-coreos":
		body, err := fetchString("https://builds.coreos.fedoraproject.org/streams/" + release + ".json")
		if err != nil {
			return "", "", err
		}
		var j struct {
			Architectures map[string]struct {
				Artifacts map[string]struct {
					Formats map[string]struct {
						Disk struct {
							Location string `json:"location"`
							Sha256   string `json:"sha256"`
						} `json:"disk"`
					} `json:"formats"`
				} `json:"artifacts"`
			} `json:"architectures"`
		}
		if err := json.Unmarshal([]byte(body), &j); err != nil {
			return "", "", err
		}
		f := j.Architectures["x86_64"].Artifacts["qemu"].Formats["qcow2.xz"]
		if f.Disk.Location == "" {
			return "", "", fmt.Errorf("fedora-coreos: no qemu artifact")
		}
		return f.Disk.Location, f.Disk.Sha256, nil
	}
	return "", "", fmt.Errorf("scrape not implemented for %s", osName)
}
