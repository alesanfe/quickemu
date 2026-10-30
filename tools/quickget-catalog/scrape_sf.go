// scrape_sf.go — SourceForge resolvers (scrape batch).
package main

import (
	"fmt"
	"regexp"
	"strings"
)

func scrapeSF(osName, release, edition, arch string) (string, string, error) {
	switch osName {
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
		// Rhino-Linux-<ver>-<arch>[-lomiri].iso + .sha256 sidecars;
		// lomiri only exists on the newest release dir.
		base := "https://sourceforge.net/projects/rhino-linux-builder/files/" + release + "/"
		ver := regexp.QuoteMeta(release)
		if release == "latest" || release == "" {
			d, err := sfLatestDir("rhino-linux-builder", "", `[0-9]{4}\.[0-9]+`)
			if err != nil {
				return "", "", err
			}
			base = d
			ver = `[0-9.]+`
		}
		a := "amd64"
		if arch == "arm64" {
			a = "arm64"
		}
		suf := ""
		if edition == "lomiri" {
			suf = "-lomiri"
		}
		u, _, err := sfISOat(base, `Rhino-Linux-`+ver+`-`+a+suf+`\.iso`)
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
	case "remixos":
		b := "B2016112101"
		if edition == "32bit" {
			b = "B2016112201"
		}
		return "https://sourceforge.net/projects/remix-os/files/Remix_OS_for_PC_Android_M_" + edition + "_" + b + ".zip/download", "", nil
	case "bsdrp":
		return sfLatest("bsdrp")
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
	case "primtux":
		// SF Distribution dir; naming drifted across major versions
		// (PrimTux7-amd64-2022-10, primtux8-amd64, primtux9-m22.3-64bit-*).
		re := `(?i)primtux` + regexp.QuoteMeta(release) + `[^"]*(amd64|64bit)[^"]*\.iso`
		if edition != "" {
			re = `(?i)primtux` + regexp.QuoteMeta(release) + `[^"]*` + regexp.QuoteMeta(edition) + `[^"]*\.iso`
		}
		return sfISO("primtux", "Distribution", re)
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
	case "eznixos":
		// files/EznixOS<rel>/eznixOS<rel>{X,K}-<date>.iso
		ed := "X"
		if edition == "kde" {
			ed = "K"
		}
		return sfISO("eznixos", "EznixOS"+release,
			`eznixOS`+regexp.QuoteMeta(release)+ed+`-[0-9]+\.iso`)
	case "extix":
		// Files live at the project root; each release is one ISO.
		return sfISOat("https://sourceforge.net/projects/extix/files",
			`extix-`+regexp.QuoteMeta(release)+`-[^"]*\.iso`)
	case "justbrowsing":
		// SF: 64bit/<name>.iso standard; browser variants in subdirs.
		dir, suff := "64bit", ""
		switch edition {
		case "chrome":
			dir, suff = "64bit/OnlyChrome64", "_chrome"
		case "firefox":
			dir, suff = "64bit/OnlyFirefox64", "_firefox"
		}
		return sfISO("justbrowsing", dir, `justbrowsing64_[0-9]+`+suff+`\.iso`)
	case "kiro":
		// SF root dir: kiro-v<ver>-x86_64.iso
		ver := `[0-9.]+`
		if release != "latest" && release != "" {
			ver = regexp.QuoteMeta(release)
		}
		return sfISO("kiro", "", `kiro-v`+ver+`-x86_64\.iso`)
	case "jatayuos":
		// SF per-release dir + SHA256SUMS manifest file.
		u, _, err := sfISO("jatayuos", release,
			`JatayuOS-`+regexp.QuoteMeta(release)+`-amd64\.iso`)
		if err != nil {
			return "", "", err
		}
		sha := ""
		if b, err := fetchString("https://sourceforge.net/projects/jatayuos/files/" +
			release + "/SHA256SUMS/download"); err == nil {
			sha = sumForISO(b, "JatayuOS-"+release+"-amd64.iso")
		}
		return u, sha, nil
	case "nemesis":
		// Nemesis-v<ver>-<DE>-x86_64.iso under files/ISO/<ver>/.
		ed := strings.ToUpper(edition)
		base := "https://sourceforge.net/projects/nemesis-linux/files/ISO/" + release + "/"
		ver := regexp.QuoteMeta(release)
		if release == "latest" || release == "" {
			d, err := sfLatestDir("nemesis-linux", "ISO", `[0-9.]+`)
			if err != nil {
				return "", "", err
			}
			base = d
			ver = `[0-9.]+`
		}
		u, _, err := sfISOat(base, `Nemesis-v`+ver+`-`+ed+`-x86_64\.iso`)
		if err != nil {
			return "", "", err
		}
		sha := ""
		if b, err := fetchString("https://sourceforge.net/projects/nemesis-linux/files/ISO/sha256sums.txt/download"); err == nil {
			sha = sumForISO(b, u[strings.LastIndex(strings.TrimSuffix(u, "/download"), "/")+1:])
		}
		return u, sha, nil
	}
	return "", "", errScrapeSkip
}
