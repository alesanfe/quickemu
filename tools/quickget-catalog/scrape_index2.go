// scrape_index2.go — Directory-index / mirror resolvers,
// alphabetical band n–z (split of the original scrapeIndex dispatch).

package main

import (
	"fmt"
	"regexp"
	"strings"
)

func scrapeIndexNZ(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "photonos":
		// 5.0 names carry an arch infix; 1.0-4.0 GA isos are
		// photon-<rel>-<hexhash>.iso (hex guard drops .debug.iso).
		re := `photon-[0-9][^"]*x86_64\.iso`
		if release != "5.0" {
			re = `photon-[0-9][0-9.]*-[a-f0-9]+\.iso`
		}
		return indexISO(fmt.Sprintf("https://packages.broadcom.com/photon/%s/GA/iso/", release), re)
	case "openeuler":
		ed := "netinst"
		if edition != "" {
			ed = edition
		}
		return indexISOsum("https://repo.openeuler.org/openEuler-"+release+"/ISO/x86_64/", `openEuler-`+regexp.QuoteMeta(release)+`-`+regexp.QuoteMeta(ed)+`-x86_64-dvd\.iso`, ".sha256sum")
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
		// 2026+ naming: t2-<ver>-x86-64-desktop-glibc-gcc-nocona.iso and
		// t2-<ver>-x86-64-musl-clang-desktop-musl-clang-nocona.iso + .sha.
		if release == "latest" || release == "" || strings.HasPrefix(release, "26.") {
			flavor := "desktop-glibc-gcc-nocona"
			if edition == "musl-clang" {
				flavor = "musl-clang-desktop-musl-clang-nocona"
			}
			ver := `[0-9.]+`
			if release != "latest" && release != "" {
				ver = regexp.QuoteMeta(release)
			}
			return indexISOsum("https://dl.t2sde.org/binary/2026/",
				`t2-`+ver+`-x86-64-`+flavor+`\.iso`, ".sha")
		}
		u, _, err := indexISO("https://dl.t2sde.org/binary/2024/", `t2-`+regexp.QuoteMeta(release)+`-x86-64-base-wayland-glibc-gcc-nocona\.iso`)
		if err != nil {
			return "", "", err
		}
		return u, sumAt(strings.TrimSuffix(u, ".iso") + ".sha256"), nil
	case "sculpt-os":
		re := `sculpt-[0-9-]+\.img`
		if release != "" && release != "latest" {
			re = `sculpt-` + regexp.QuoteMeta(release) + `\.img`
		}
		return indexISO("https://genode.org/files/sculpt/", re)
	case "truenas-core":
		return indexISO("https://download.freenas.org/"+release+"/STABLE/latest/x64/", `TrueNAS-`+regexp.QuoteMeta(release)+`-U[0-9.]+\.iso`)
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
	case "opensuse-slowroll":
		return indexISO("https://download.opensuse.org/slowroll/iso/", `openSUSE-Slowroll-DVD-x86_64-Build[0-9.]+-Media\.iso`)
	case "zenwalk":
		return indexISOsum("https://download.zenwalk.org/x86_64/"+release+"/", `zenwalk-`+regexp.QuoteMeta(release)+`(-[0-9]+)?\.iso`, ".md5")
	case "parabola":
		return indexISO("https://repo.parabola.nu/iso/x86_64-experimental-lxde-2019.06-pre/", `parabola-x86_64-systemd-lxde-[^"/]*complete\.iso`)

	// --- SourceForge ---
	case "vicibox":
		// ViciBox_V<maj>.x86_64-<rel>.iso (+ optional -md minimal variant).
		suf := ""
		if edition == "md" {
			suf = "-md"
		}
		maj := strings.SplitN(release, ".", 2)[0]
		return indexISO("https://download.vicidial.com/iso/vicibox/server/",
			`ViciBox_V`+regexp.QuoteMeta(maj)+`\.x86_64-`+regexp.QuoteMeta(release)+suf+`\.iso`)
	case "neptune":
		return indexISO("https://download.neptuneos.com/download/", `Neptune`+regexp.QuoteMeta(release)+`[^"/]*\.iso`)
	case "nomadbsd":
		// nomadbsd-<rel>.amd64.<ufs|zfs>.img.lzma + .sha256 sidecar.
		ed := edition
		if ed == "" {
			ed = "ufs"
		}
		return indexISOsum("https://nomadbsd.org/download/",
			`nomadbsd-`+regexp.QuoteMeta(release)+`\.amd64\.`+regexp.QuoteMeta(ed)+`\.img\.lzma`, ".sha256")
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
	case "turnkey":
		// One flat index: turnkey-<appliance>-<ver>-<codename>-amd64.iso
		// + signed .hash (sha256 embedded before the PGP armor).
		return indexISOsum("https://mirror.turnkeylinux.org/turnkeylinux/images/iso/",
			`turnkey-`+regexp.QuoteMeta(edition)+`-[0-9.]+-[a-z]+-amd64\.iso`, ".hash")
	case "xebian":
		// download/ is not listable; download/pending/ indexes both
		// unstable (+.sha256) and trixie (no sidecar).
		return indexISOsum("https://xebian.org/download/pending/",
			`xebian-`+regexp.QuoteMeta(release)+`-amd64\.hybrid\.iso`, ".sha256")
	case "rosa":
		// mirror.rosalab.ru/rosa/rosa<N>/iso/ROSA.FRESH.<N>/<de>/ROSA.FRESH.<DE>.<ver>.x86_64.iso
		ver := `[0-9.]+`
		if release != "latest" && release != "" {
			ver = regexp.QuoteMeta(release)
		}
		base := "https://mirror.rosalab.ru/rosa/rosa13/iso/ROSA.FRESH.13/" + edition + "/"
		return indexISOsum(base,
			`ROSA\.FRESH\.`+strings.ToUpper(edition)+`\.`+ver+`\.x86_64\.iso`, ".md5sum")
	case "osgeolive":
		// osgeo-live-<ver>-amd64.iso + .iso.md5 under releases/<ver>/.
		base := "https://download.osgeo.org/livedvd/releases/" + release + "/"
		ver := regexp.QuoteMeta(release)
		if release == "latest" || release == "" {
			d, err := latestDir("https://download.osgeo.org/livedvd/releases/", `[0-9.]+`)
			if err != nil {
				return "", "", err
			}
			d = strings.TrimRight(d, "/")
			ver = regexp.QuoteMeta(d[strings.LastIndex(d, "/")+1:])
			base = d + "/"
		}
		// osgeo-live- <= 11.x, osgeolive- >= 12.x
		return indexISOsum(base, `osgeo-?live-`+ver+`-amd64\.iso`, ".md5")
	}
	return "", "", errScrapeSkip
}
