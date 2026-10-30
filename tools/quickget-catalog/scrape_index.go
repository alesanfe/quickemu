// scrape_index.go — Directory-index / mirror resolvers (scrape batch).
package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func scrapeIndex(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "photonos":
		// 5.0 names carry an arch infix; 1.0-4.0 GA isos are
		// photon-<rel>-<hexhash>.iso (hex guard drops .debug.iso).
		re := `photon-[0-9][^"]*x86_64\.iso`
		if release != "5.0" {
			re = `photon-[0-9][0-9.]*-[a-f0-9]+\.iso`
		}
		return indexISO(fmt.Sprintf("https://packages.broadcom.com/photon/%s/GA/iso/", release), re)
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
	case "openeuler":
		ed := "netinst"
		if edition != "" {
			ed = edition
		}
		return indexISOsum("https://repo.openeuler.org/openEuler-"+release+"/ISO/x86_64/", `openEuler-`+regexp.QuoteMeta(release)+`-`+regexp.QuoteMeta(ed)+`-x86_64-dvd\.iso`, ".sha256sum")
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
		re := `sculpt-[0-9-]+\.img`
		if release != "" && release != "latest" {
			re = `sculpt-` + regexp.QuoteMeta(release) + `\.img`
		}
		return indexISO("https://genode.org/files/sculpt/", re)
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
	case "truenas-core":
		return indexISO("https://download.freenas.org/"+release+"/STABLE/latest/x64/", `TrueNAS-`+regexp.QuoteMeta(release)+`-U[0-9.]+\.iso`)
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
	case "drauger-os":
		re := `Drauger_OS-[^"]*AMD64\.iso`
		if release != "" && release != "latest" {
			re = `Drauger_OS-` + regexp.QuoteMeta(release) + `-AMD64\.iso`
		}
		return indexISO("https://download.draugeros.org/ISOs/", re)
	case "vicibox":
		// ViciBox_V<maj>.x86_64-<rel>.iso (+ optional -md minimal variant).
		suf := ""
		if edition == "md" {
			suf = "-md"
		}
		maj := strings.SplitN(release, ".", 2)[0]
		return indexISO("https://download.vicidial.com/iso/vicibox/server/",
			`ViciBox_V`+regexp.QuoteMeta(maj)+`\.x86_64-`+regexp.QuoteMeta(release)+suf+`\.iso`)
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
	case "mabox":
		// repo.maboxlinux.org/iso/ root holds the current ISO + .md5.
		return indexISOsum("https://repo.maboxlinux.org/iso/",
			`mabox-linux-[0-9.]+-[A-Za-z]+-[0-9]+-linux[0-9]+\.iso`, ".md5")
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

		// --- JSON APIs ---
	}
	return "", "", errScrapeSkip
}
