// scrape_index.go — Directory-index / mirror resolvers,
// alphabetical band a–m (split of the original scrapeIndex dispatch).

package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

func scrapeIndexAM(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "aerynos":
		// cdn.aerynos.dev/isos/: AerynOS-<ver>-GNOME-live-x86_64.iso + .sha256sum.
		return indexISOsum("https://cdn.aerynos.dev/isos/",
			`AerynOS-`+regexp.QuoteMeta(release)+`-GNOME-live-x86_64\.iso`, ".sha256sum")
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
	case "astralinux":
		return indexISO("https://download.astralinux.ru/astra/stable/orel/iso/", `alce-current\.iso`)
	case "helenos":
		re := `HelenOS-[0-9.]+-amd64\.iso`
		if release != "" && release != "latest" {
			re = `HelenOS-` + regexp.QuoteMeta(release) + `-amd64\.iso`
		}
		return indexISO("https://www.helenos.org/releases/", re)
	case "9legacy":
		return "https://9legacy.org/download/9legacy.iso.bz2", "", nil
	case "galliumos":
		// 2.x is galliumos-<ed>-<rel>.iso; 3.x flips the order.
		re := `galliumos-(?:` + regexp.QuoteMeta(edition) + `-` + regexp.QuoteMeta(release) +
			`|` + regexp.QuoteMeta(release) + `-` + regexp.QuoteMeta(edition) + `)\.iso`
		return indexISO("https://galliumos.org/releases/"+release+"/", re)
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
	case "freepbx":
		pat := `SNGDEB-PBX` + release + `-amd64-[0-9.-]+\.iso`
		if release == "15" {
			pat = `SNG7-FPBX-64bit-[0-9-]+\.iso`
		} else if release == "16" {
			pat = `SNG7-PBX16-64bit-[0-9-]+\.iso`
		}
		return indexISOsum("https://downloads.freepbxdistro.org/ISO/", pat, ".sha256sum")
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
	case "grommunio":
		file := "grommunio.x86_64-latest"
		if edition == "offline" {
			file = "grommunio-offline-latest"
		}
		u := "https://download.grommunio.com/appliance/" + file + ".install.iso"
		return u, sumAt(u + ".sha256"), nil
	case "daphile":
		suffix := ""
		if edition == "rt" {
			suffix = "-rt"
		}
		return indexISO("https://www.daphile.com/firmware/stable/", `daphile-[0-9.]+-x86_64`+suffix+`\.iso`)
	case "debian-edu":
		return indexISO("https://get.debian.org/cdimage/release/current/amd64/iso-cd/", `debian-edu-[0-9.]+-amd64-netinst\.iso`)
	case "hardenedbsd":
		return indexISOsums("https://installers.hardenedbsd.org/pub/"+release+"/amd64/amd64/installer/LATEST/", `disc1\.iso`, "MANIFEST")
	case "freedombox":
		return indexISO("https://ftp.freedombox.org/pub/freedombox/hardware/amd64/"+release+"/", `freedombox-[^"/]*_all-amd64\.img\.xz`)
	case "avlinux":
		return indexISOsha("https://downloads.bandshed.net/AVL_MXe_25/", `AVL_MXe-[^"]*x64\.iso`)
	case "drauger-os":
		re := `Drauger_OS-[^"]*AMD64\.iso`
		if release != "" && release != "latest" {
			re = `Drauger_OS-` + regexp.QuoteMeta(release) + `-AMD64\.iso`
		}
		return indexISO("https://download.draugeros.org/ISOs/", re)
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
		// releases are Debian codenames; editions map to UEFI/BIOS images.
		ed := "UEFI"
		if edition != "" {
			ed = strings.ToUpper(edition)
		}
		rel := `[A-Za-z]+`
		if release != "" && release != "latest" {
			// Filenames carry a capitalised codename (-Trixie).
			rel = strings.ToUpper(release[:1]) + release[1:]
			rel = regexp.QuoteMeta(rel)
		}
		return indexISO("https://dietpi.com/downloads/images/",
			`DietPi_NativePC-`+ed+`-x86_64-`+rel+`\.img\.xz`)
	case "eurolinux":
		// vault.cdn flat listing: EL-<major>-x86_64-latest-<edition>.iso.
		ed := edition
		if ed == "" {
			ed = "appstream"
		}
		return indexISO("https://vault.cdn.euro-linux.com/isos/",
			`EL-`+regexp.QuoteMeta(release)+`-x86_64-latest-`+regexp.QuoteMeta(ed)+`\.iso`)
	case "gnu-hurd":
		d, err := latestDir("https://cdimage.debian.org/cdimage/ports/latest/hurd-amd64/", `([0-9]+)`)
		if err != nil {
			return "", "", err
		}
		return indexISO(d, `debian-sid-hurd-amd64-NETINST-1\.iso`)
	case "grml":
		// grml-<edition>-<release>-<arch>.iso + SHA256SUMS-<release>.
		ed := edition
		if ed == "" {
			ed = "full"
		}
		a := arch
		if a == "" {
			a = "amd64"
		}
		rel := `[0-9.]+`
		if release != "" && release != "latest" {
			rel = regexp.QuoteMeta(release)
		}
		u, _, err := indexISO("https://mirror.us.leaseweb.net/grml/",
			`grml-`+regexp.QuoteMeta(ed)+`-`+rel+`-`+a+`\.iso`)
		if err != nil {
			return "", "", err
		}
		sha := ""
		if m := regexp.MustCompile(`grml-[a-z]+-([0-9.]+)-`).FindStringSubmatch(u); m != nil {
			if b, err := fetchString("https://mirror.us.leaseweb.net/grml/SHA256SUMS-" + m[1]); err == nil {
				sha = sumForISO(b, u[strings.LastIndex(u, "/")+1:])
			}
		}
		return u, sha, nil
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
	case "clearndr":
		// SELKS successor: the /selks page links signed CDN paths.
		ed := edition
		if ed == "" {
			ed = "desktop"
		}
		rel := `[0-9.]+`
		if release != "" && release != "latest" {
			rel = regexp.QuoteMeta(release)
		}
		body, err := fetchString("https://www.stamus-networks.com/selks")
		if err != nil {
			return "", "", err
		}
		u := latestMatch(body, `https://my\.stamus-networks\.com/sn-dl/ClearNDRCommunity/[0-9a-f]+/ClearNDR-`+regexp.QuoteMeta(ed)+`-`+rel+`\.iso`)
		if u == "" {
			return "", "", fmt.Errorf("clearndr: no iso for %s %s", release, ed)
		}
		return u, "", nil
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
	case "fatdog64":
		// Fatdog64-<rel>.iso; 6xx builds also ship -firefox/-seamonkey.
		re := `Fatdog64-` + regexp.QuoteMeta(release) + `\.iso`
		if edition != "" && edition != "standard" {
			re = `(?i)Fatdog64-` + regexp.QuoteMeta(release) + `-` + regexp.QuoteMeta(edition) + `\.iso`
		}
		return indexISOsum("https://ftp.nluug.nl/pub/os/Linux/distr/fatdog/iso/", re, ".md5sum.txt")
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
	case "mabox":
		// repo.maboxlinux.org/iso/ root holds the current ISO + .md5.
		return indexISOsum("https://repo.maboxlinux.org/iso/",
			`mabox-linux-[0-9.]+-[A-Za-z]+-[0-9]+-linux[0-9]+\.iso`, ".md5")
	case "dynebolic":
		// files.dyne.org listing is JS; names are deterministic.
		return "https://files.dyne.org/dynebolic/dynebolic-" + release + ".iso", "", nil
	case "alma":
		// Majors hold "X-latest" plus the current pinned "X.Y" files.
		a := "x86_64"
		if arch == "arm64" {
			a = "aarch64"
		}
		dir := strings.SplitN(release, ".", 2)[0]
		ver := release
		if !strings.Contains(release, ".") {
			ver += "-latest"
		}
		ed := edition
		if ed == "" {
			ed = "dvd"
		}
		return indexISOsums("https://repo.almalinux.org/almalinux/"+dir+"/isos/"+a+"/",
			`AlmaLinux-`+regexp.QuoteMeta(ver)+`-`+a+`-`+regexp.QuoteMeta(ed)+`\.iso`,
			"CHECKSUM")
	case "dragora":
		// cedia mirror v3/iso/<stage>/; alphas share the "alpha" dir.
		dir := release
		if strings.HasPrefix(release, "alpha") {
			dir = "alpha"
		}
		return indexISO("https://mirror.cedia.org.ec/dragora/v3/iso/"+dir+"/",
			`dragora-3\.0-(x86_64|amd64)-`+regexp.QuoteMeta(release)+`(-live|-dvd)?\.iso`)
	case "graphite":
		// Download page links download.graphiteos.org/graphite-<ver>-x86_64.iso;
		// a flat SHA256SUMS manifest sits next to it.
		body, err := fetchString("https://graphiteos.org/")
		if err != nil {
			return "", "", err
		}
		u := latestMatch(body, `https://download\.graphiteos\.org/graphite-[^"'<> ]*\.iso`)
		if u == "" {
			return "", "", fmt.Errorf("graphite: no iso link on download page")
		}
		sha := ""
		if b, err := fetchString("https://download.graphiteos.org/SHA256SUMS"); err == nil {
			sha = sumForISO(b, u[strings.LastIndex(u, "/")+1:])
		}
		return u, sha, nil
	case "mobian":
		// mobian-amd64-<ed>-<ver>.img.xz + per-edition .sha256sums manifest.
		ed := edition
		if ed == "" {
			ed = "phosh"
		}
		ver := `[0-9.]+`
		if release != "latest" && release != "" {
			ver = regexp.QuoteMeta(release)
		}
		base := "https://images.mobian.org/amd64/"
		u, _, err := indexISO(base, `mobian-amd64-`+ed+`-`+ver+`\.img\.xz`)
		if err != nil {
			return "", "", err
		}
		name := u[strings.LastIndex(u, "/")+1:]
		sha := ""
		if b, err := fetchString(base + strings.TrimSuffix(name, ".img.xz") + ".sha256sums"); err == nil {
			sha = sumForISO(b, name)
		}
		return u, sha, nil
	case "bastillebsd":
		// BastilleBSD-<rel>-RELEASE-amd64-memstick.iso.xz; BSD-style
		// CHECKSUM covers the uncompressed .iso name.
		dir := release
		if dir == "latest" || dir == "" {
			d, err := latestDir("https://download.bastillebsd.org/releases/ISO-IMAGES/", `[0-9.]+`)
			if err != nil {
				return "", "", err
			}
			dir = strings.TrimRight(d, "/")
			dir = dir[strings.LastIndex(dir, "/")+1:]
		}
		base := "https://download.bastillebsd.org/releases/ISO-IMAGES/" + dir + "/"
		u, _, err := indexISO(base,
			`BastilleBSD-`+regexp.QuoteMeta(dir)+`-RELEASE-amd64-memstick\.iso\.xz`)
		if err != nil {
			return "", "", err
		}
		sha := ""
		if b, err := fetchString(base + "CHECKSUM.SHA256-BastilleBSD-" + dir + "-RELEASE-amd64"); err == nil {
			name := u[strings.LastIndex(u, "/")+1:]
			sha = sumForISO(b, strings.TrimSuffix(name, ".xz"))
		}
		return u, sha, nil
		// --- JSON APIs ---
	}
	return "", "", errScrapeSkip
}

// scrapeIndex fans out to the two alphabetical bands; kept as the
// single entry in the scrapeRescued resolver chain.
func scrapeIndex(osName, release, edition, arch string) (string, string, error) {
	for _, resolve := range []func(string, string, string, string) (string, string, error){
		scrapeIndexAM, scrapeIndexNZ,
	} {
		u, h, err := resolve(osName, release, edition, arch)
		if !errors.Is(err, errScrapeSkip) {
			return u, h, err
		}
	}
	return "", "", errScrapeSkip
}
