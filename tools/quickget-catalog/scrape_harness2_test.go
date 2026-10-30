// scrape_harness2_test.go — bodyFor tail band (split of scrape_harness_test.go)
package main

import "strings"

// bodyForTail: second half of the URL-keyed fixture switch — bodyFor
// delegates here when its first band does not match.
func bodyForTail(url string) (string, error) {
	switch {
	case strings.Contains(url, "caelinux"):
		return `"name":"CAELinux2020" "name":"caelinux2020lite.iso"`, nil
	case strings.Contains(url, "windows-linux"):
		return `"name":"11.26-noble-lts" "name":"winux-11.26.11-noble-lts.iso"`, nil
	case strings.Contains(url, "supergrub2"):
		return `"name":"supergrub2-classic-2.06s4-i386_pc-CD.iso" "name":"supergrub2-classic-2.06s4-i386_efi-CD.iso"`, nil
	case strings.Contains(url, "refracta"):
		return `"name":"refracta12_xfce_amd64-20240101.iso"`, nil
	// --- sixth-expansion fixtures ---
	case strings.Contains(url, "trisquel-images"):
		return `<a href="trisquel_12.0_amd64.iso">a</a><a href="triskel_12.0_amd64.iso">b</a><a href="trisquel-mini_12.0_amd64.iso">c</a><a href="trisquel-netinst_12.0_amd64.iso">d</a><a href="trisquel-sugar_12.0_amd64.iso">e</a><a href="trisquel_11.0.1_amd64.iso">f</a>`, nil
	case strings.Contains(url, "bunsenlabs"):
		return `<a href="beryllium-1-amd64.hybrid.iso">a</a><a href="boron-1-240123-amd64.hybrid.iso">b</a><a href="carbon-1-260211-amd64.hybrid.iso">c</a>`, nil
	case strings.Contains(url, "ghostbsd.org/releases/amd64/19"):
		return `<a href="GhostBSD19.10.iso">x</a><a href="GhostBSD19.10-XFCE.iso">y</a>`, nil
	case strings.Contains(url, "ghostbsd"):
		return `<a href="GhostBSD-24.10.1.iso">x</a><a href="GhostBSD-24.10.1-XFCE.iso">y</a>`, nil
	case strings.Contains(url, "primtux"):
		return `"name":"PrimTux7-amd64-2022-10.iso" "name":"primtux8-amd64.iso" "name":"primtux9-m22.3-64bit-2026-02-27.iso"`, nil
	case strings.Contains(url, "peppermintos"):
		return `"name":"peppermint_devuan-amd64.iso"`, nil
	case strings.Contains(url, "android-x86"):
		return `"name":"android-x86_64-9.0-r2.iso" "name":"android-x86_64-8.1-r6.iso"`, nil
	// --- seventh-expansion fixtures ---
	case strings.Contains(url, "armbian"):
		return `<a href="Armbian_26.8.3_Uefi-x86_trixie_edge_7.1.9_minimal.iso">a</a><a href="Armbian_26.8.3_Uefi-x86_resolute_edge_7.1.9_gnome_desktop.iso">b</a><a href="Armbian_26.8.3_Uefi-x86_resolute_edge_7.1.9_kde-plasma_desktop.iso">c</a>`, nil
	case strings.Contains(url, "zentyal") && strings.Contains(url, "Old"):
		return `"name":"zentyal-4.0-amd64.iso" "name":"zentyal-3.5-amd64.iso"`, nil
	case strings.Contains(url, "zentyal"):
		return `"name":"zentyal-7.0-development-amd64.iso"`, nil
	case strings.Contains(url, "funtoo"):
		return `"name":"Funtoo-x86_64-29-01-2019.iso"`, nil
	case strings.Contains(url, "mofolinux"):
		return `"name":"mofolinux-9.7.0.iso"`, nil
	case strings.Contains(url, "openthos"):
		return `"name":"openthos-8.1.0.191102.img"`, nil
	case strings.Contains(url, "dragora"):
		return `<a href="dragora-3.0-amd64-beta2-dvd.iso">a</a><a href="dragora-3.0-x86_64-beta1-live.iso">b</a><a href="dragora-3.0-x86_64-beta1-packages.iso">c</a><a href="dragora-3.0-x86_64-alpha2.iso">d</a><a href="dragora-3.0-x86_64-alpha1.iso">e</a>`, nil
	case strings.Contains(url, "eznixos"):
		return `"name":"eznixOS13X-260919.iso" "name":"eznixOS13K-260919.iso"`, nil
	case strings.Contains(url, "extix"):
		return `"name":"extix-26.4-64bit-gnome-waydroid-3290mb-260410.iso" "name":"extix-26.7-deepin-25.2-refractasnapshot-5540mb-260710.iso" "name":"extix-26.8-64bit-lxqt-refractasnapshot-2800mb-260811.iso"`, nil
	case strings.Contains(url, "xebian.org") && strings.HasSuffix(url, ".sha256"):
		return shaX + "  xebian-unstable-amd64.hybrid.iso\n", nil
	case strings.Contains(url, "xebian.org"):
		return `<a href="xebian-unstable-amd64.hybrid.iso">x</a><a href="xebian-trixie-amd64.hybrid.iso">y</a>`, nil
	case strings.Contains(url, "justbrowsing"):
		return `"name":"justbrowsing64_20140808.iso" "name":"justbrowsing64_20140808_chrome.iso" "name":"justbrowsing64_20140808_firefox.iso"`, nil
	case strings.Contains(url, "maboxlinux.org/iso/") && strings.HasSuffix(url, ".md5"):
		return "5d41402abc4b2a76b9719d911017c592  mabox-linux-26.09-Istredd-260911-linux618.iso", nil
	case strings.Contains(url, "maboxlinux.org/iso/"):
		return `<a href="mabox-linux-26.09-Istredd-260911-linux618.iso">x</a>`, nil
	case strings.Contains(url, "almalinux") && strings.HasSuffix(url, "CHECKSUM"):
		return `SHA256 (AlmaLinux-10-latest-x86_64-dvd.iso) = 90244ac532f67c978831a381b420da8bda363729e1f4cf8fb8991daca70ee287
SHA256 (AlmaLinux-10.2-x86_64-boot.iso) = b3f865468075bcada8f208d830289302c67529789d668041d24e8d6fc697ba6a`, nil
	case strings.Contains(url, "almalinux"):
		return `<a href="AlmaLinux-10-latest-x86_64-dvd.iso">a</a><a href="AlmaLinux-10.2-x86_64-boot.iso">b</a><a href="AlmaLinux-10-latest-aarch64-dvd.iso">c</a>`, nil
	// --- fourth-expansion fixtures ---
	case strings.Contains(url, "rhino-linux-builder") && strings.Contains(url, ".sha256"):
		return shaX + "  Rhino-Linux-2026.1-amd64.iso\n", nil
	case strings.Contains(url, "rhino-linux-builder") && strings.Contains(url, "2026.1"):
		return `"name":"Rhino-Linux-2026.1-amd64.iso" "name":"Rhino-Linux-2026.1-arm64.iso" "name":"Rhino-Linux-2026.1-amd64-lomiri.iso"`, nil
	case strings.Contains(url, "rhino-linux-builder") && strings.Contains(url, "2025.4"):
		return `"name":"Rhino-Linux-2025.4-amd64.iso" "name":"Rhino-Linux-2025.4-arm64.iso"`, nil
	case strings.Contains(url, "rhino-linux-builder") && strings.Contains(url, "2023.4"):
		return `"name":"Rhino-Linux-2023.4-amd64.iso" "name":"Rhino-Linux-2023.4-arm64.iso"`, nil
	case strings.Contains(url, "rhino-linux-builder"):
		return `"name":"2026.1" "name":"2025.4" "name":"2023.4"`, nil
	case strings.Contains(url, "ubuntudde") && strings.Contains(url, "23.04"):
		return `"name":"ubuntudde-23.04-amd64-desktop.iso"`, nil
	case strings.Contains(url, "ubuntudde"):
		return `"name":"23.04"`, nil
	case strings.Contains(url, "regataos") && strings.Contains(url, "regataos-26"):
		return `"name":"Regata_OS_26_en-US.x86_64-26.0.7.iso" "name":"Regata_OS_26_pt-BR.x86_64-26.0.7.iso" "name":"Regata_OS_26_NV_en-US.x86_64-26.0.7.iso"`, nil
	case strings.Contains(url, "regataos"):
		return `"name":"regataos-26.0"`, nil
	case strings.Contains(url, "archman") && strings.Contains(url, "xfce"):
		return `"name":"Archman_Xfce_20261001.iso"`, nil
	case strings.Contains(url, "archman"):
		return `"name":"2026-10" "name":"2025-09"`, nil
	case strings.Contains(url, "kamarada") && strings.Contains(url, "manjaro"):
		return `"name":"kamarada-gnome-2025.05-x86_64.iso"`, nil
	case strings.Contains(url, "kamarada"):
		return `"name":"Linux_Kamarada-15.6-x86_64-Build1.0.iso"`, nil
	case strings.Contains(url, "slackel"):
		return `"name":"slackellive64-openbox-7.5.iso" "name":"slackellive64-mate-7.5.iso"`, nil
	case strings.Contains(url, "easynas"):
		return `"name":"EasyNAS.x86_64-1.1.2-0.install.iso"`, nil
	case strings.Contains(url, "elastix"):
		return `"name":"debian-amd64-netinst-elastix.iso"`, nil
	case strings.Contains(url, "m0n0wall") && strings.Contains(url, "1.33"):
		return `"name":"cdrom-1.33.iso"`, nil
	case strings.Contains(url, "m0n0wall"):
		return `"name":"1.33" "name":"1.236"`, nil
	case strings.Contains(url, "dban"):
		return `"name":"dban-2.3.0_i586.iso"`, nil
	case strings.Contains(url, "drbl") && strings.Contains(url, "2.5.1-16"):
		return `"name":"drbl-live-xfce-2.5.1-16-amd64.iso"`, nil
	case strings.Contains(url, "drbl"):
		return `"name":"2.5.1-16"`, nil
	case strings.Contains(url, "septor"):
		return `"name":"Septor-2024-amd64.iso"`, nil
	case strings.Contains(url, "hefftorlinux") && strings.Contains(url, "v4"):
		return `"name":"hefftorlinux-plasma-v4-x86_64.iso" "name":"hefftorlinux-kodi-v4-x86_64.iso" "name":"hefftorlinux-xfce-v4-x86_64.iso"`, nil
	case strings.Contains(url, "hefftorlinux"):
		return `"name":"v4-0" "name":"v3-1"`, nil
	case strings.Contains(url, "ctlos") && strings.Contains(url, "v2.5"):
		return `"name":"ctlos_v2.5.0_20260419.iso"`, nil
	case strings.Contains(url, "ctlos"):
		return `"name":"v2.5.0"`, nil
	case strings.Contains(url, "adelielinux"):
		return `<a href="adelie-inst-x86_64-1.0-rc1.iso">x</a><a href="adelie-live-kde-x86_64-1.0-rc1.iso">y</a><a href="adelie-live-lxqt-x86_64-1.0-rc1.iso">z</a><a href="adelie-live-mate-x86_64-1.0-rc1.iso">m</a><a href="adelie-live-xfce-x86_64-1.0-rc1.iso">f</a><a href="SHA512SUMS">s</a>`, nil
	case strings.Contains(url, "openanolis") && strings.Contains(url, "7.9"):
		return `<a href="AnolisOS-7.9-GA-x86_64-dvd.iso">x</a><a href="AnolisOS-7.9-Minimal-x86_64-dvd.iso">y</a>`, nil
	case strings.Contains(url, "openanolis"):
		return `<a href="AnolisOS-8.8-x86_64-boot.iso">x</a><a href="AnolisOS-8.8-x86_64-dvd.iso">y</a><a href="AnolisOS-8.8-x86_64-minimal.iso">z</a><a href="AnolisOS-23.5-x86_64-boot.iso">p</a><a href="AnolisOS-23.5-x86_64-dvd.iso">q</a>`, nil
	case strings.Contains(url, "openeuler"):
		return `<a href="openEuler-24.03-LTS-everything-x86_64-dvd.iso">x</a><a href="openEuler-24.03-LTS-netinst-x86_64-dvd.iso">y</a><a href="openEuler-22.03-LTS-SP4-netinst-x86_64-dvd.iso">z</a><a href="openEuler-24.03-LTS-SP1-netinst-x86_64-dvd.iso">w</a>`, nil
	case strings.Contains(url, "cdimage.openkylin.top"):
		return `<a href="openKylin-Desktop-V3.0-20260918.1-x86_64.iso">x</a><a href="openKylin-Server-V3.0-20260916-x86_64.iso">y</a><a href="openKylin-Desktop-V3.0-Beta-20260807-x86_64.iso">b</a><a href="openKylin-Desktop-V2.0-SP2-20260813-x86_64.iso">z</a><a href="openKylin-Desktop-V2.0-SP2-20260407-x86_64.iso">w</a><a href="openKylin-Desktop-V2.0-SP1-x86_64.iso">s</a>`, nil
	case strings.Contains(url, "astralinux"):
		return `<a href="alce-current.iso">x</a>`, nil
	case strings.Contains(url, "helenos"):
		return `<a href="HelenOS-0.14.1-amd64.iso">x</a><a href="HelenOS-0.9.1-amd64.iso">y</a><a href="HelenOS-0.2.0.1-amd64.iso">z</a>`, nil
	case strings.Contains(url, "galliumos") && strings.Contains(url, "3."):
		return `<a href="galliumos-3.1-baytrail.iso">a</a><a href="galliumos-3.1-sandyivy.iso">b</a><a href="galliumos-3.0-skylake.iso">c</a>`, nil
	case strings.Contains(url, "galliumos"):
		return `<a href="galliumos-baytrail-2.1.iso">a</a><a href="galliumos-braswell-2.1.iso">b</a><a href="galliumos-broadwell-2.1.iso">c</a><a href="galliumos-haswell-2.1.iso">d</a><a href="galliumos-samus-2.1.iso">e</a><a href="galliumos-sandy-2.1.iso">f</a><a href="galliumos-skylake-2.1.iso">g</a>`, nil
	case strings.Contains(url, "sortix.org") && strings.Contains(url, "builds"):
		return `<a href="sortix-1.0-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "sortix"):
		return `<a href="1.0/">d</a><a href="0.9/">d</a>`, nil
	case strings.Contains(url, "postmarketos") && strings.Contains(url, "20261023-1747/"):
		return `<a href="20261023-1747-postmarketOS-edge-console-generic-x86_64-lts.img.xz">a</a><a href="20261023-1747-postmarketOS-edge-gnome-generic-x86_64-lts.img.xz">b</a><a href="20261023-1747-postmarketOS-edge-phosh-generic-x86_64-lts.img.xz">c</a><a href="20261023-1747-postmarketOS-edge-plasma-desktop-generic-x86_64-lts.img.xz">d</a><a href="20261023-1747-postmarketOS-edge-sway-generic-x86_64-lts.img.xz">e</a>`, nil
	case strings.Contains(url, "postmarketos"):
		return `<a href="20261023-1747/">d</a><a href="20261020-1837/">d</a>`, nil
	case strings.Contains(url, "ovirt"):
		return `<a href="ovirt-node-ng-installer-latest-el8.iso">x</a><a href="ovirt-node-ng-installer-latest-el9.iso">y</a>`, nil
	case strings.Contains(url, "plop") && strings.Contains(url, "live"):
		return `<a href="ploplinux-24.2-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "plop"):
		return `<a href="24.2/">d</a><a href="24.1/">d</a>`, nil
	case strings.Contains(url, "koozali") && strings.Contains(url, "releases/10.1"):
		return `<a href="readme.txt">x</a>`, nil
	case strings.Contains(url, "koozali") && strings.Contains(url, "releases/11"):
		return `<a href="smeserver-11.0-x86_64.iso">x</a><a href="smeserver-11.0-x86_64-netinstall.iso">y</a>`, nil
	case strings.Contains(url, "koozali"):
		return `<a href="smeserver-10.1-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "contribs"):
		return `<a href="smeserver-10.1-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "t2sde") && strings.HasSuffix(url, ".sha"):
		return shaX + "  t2-26.6-x86-64-desktop-glibc-gcc-nocona.iso\n", nil
	case strings.Contains(url, "t2sde"):
		return `<a href="t2-24.10-x86-64-base-wayland-glibc-gcc-nocona.iso">x</a><a href="t2-24.11-x86-64-base-wayland-glibc-gcc-nocona.iso">y</a><a href="t2-24.12-x86-64-base-wayland-glibc-gcc-nocona.iso">z</a>
			<a href="t2-26.6-x86-64-desktop-glibc-gcc-nocona.iso">n</a>
			<a href="t2-26.6-x86-64-musl-clang-desktop-musl-clang-nocona.iso">m</a>`, nil
	case strings.Contains(url, "fiwix"):
		return `<a href="FiwixOS-3.1-i386.iso">x</a>`, nil
	case strings.Contains(url, "genode"):
		return `<a href="sculpt-26-04.img">x</a><a href="sculpt-25-04.img">y</a>`, nil
	case strings.Contains(url, "mikrotik"):
		return `7.19.4 1700221125`, nil
	// --- fifth-expansion fixtures ---
	case strings.Contains(url, "issabelpbx") && strings.Contains(url, "Issabel%205"):
		return `"name":"issabel5-USB-DVD-x86_64-20261021.iso" "name":"issabel5-USB-DVD-x86_64-20261001.iso"`, nil
	case strings.Contains(url, "issabelpbx"):
		return `"name":"Issabel 5" "name":"Issabel 4"`, nil
	case strings.Contains(url, "syslinuxos") && strings.Contains(url, "SysLinuxOS-13"):
		return `"name":"SysLinuxOS-13.3-GNOME.iso" "name":"SysLinuxOS-13.3-MATE.iso"`, nil
	case strings.Contains(url, "syslinuxos"):
		return `"name":"SysLinuxOS-13"`, nil
	case strings.Contains(url, "cyborghawk"):
		return `"name":"Cyborg-Hawk-Linux.iso"`, nil
	case strings.Contains(url, "cuerdos"):
		return `"name":"CuerdOS-2.1.1-Xfce.iso" "name":"CuerdOS-2.1.1-Sway.iso" "name":"CuerdOS-2.1.1-Shell.iso" "name":"CuerdOS-2.1.1-Plasma.iso"`, nil
	case strings.Contains(url, "openfiler"):
		return `"name":"openfileresa-2.99.2-x86_64-disc1.iso" "name":"dd.iso.gz"`, nil
	case strings.Contains(url, "redobackup") && strings.Contains(url, "SHA256SUMS"):
		return shaX + "  redorescue-4.0.0.iso\n", nil
	case strings.Contains(url, "redobackup"):
		return `"name":"redorescue-4.0.0.iso" "name":"redorescue-3.0.2.iso"`, nil
	case strings.Contains(url, "fuduntu"):
		return `"name":"Fuduntu-2013.2-x86_64-LiveDVD.iso" "name":"Fuduntu-2013.2-x86_64-LiteDVD.iso"`, nil
	case strings.Contains(url, "apodio"):
		return `"name":"Apodio13-1.2.7.iso"`, nil
	case strings.Contains(url, "openartist"):
		return `"name":"openArtist.iso" "name":"openArtist_base_12.04_alpha1.iso"`, nil
	case strings.Contains(url, "pinguy-os"):
		return `"name":"Pinguy_OS_18.04.2_Full-LTS_x86-64.iso" "name":"Pinguy_OS_18.04.2_Mini-LTS_x86-64.iso"`, nil
	case strings.Contains(url, "blackbuntu"):
		return `"name":"bb03-amd64-final.iso"`, nil
	case strings.Contains(url, "nodezero"):
		return `"name":"NodeZero.iso"`, nil
	case strings.Contains(url, "matriux"):
		return `"name":"Matriux_Leandros20130923.iso"`, nil
	case strings.Contains(url, "antergos"):
		return `"name":"antergos-minimal-19.4-x86_64.iso" "name":"antergos-19.4-x86_64.iso"`, nil
	case strings.Contains(url, "academix"):
		return `"name":"AcademiX_2.7-stable_64bit.iso"`, nil
	case strings.Contains(url, "arch-linux-gui"):
		return `"name":"alg-plasma-2025.10-x86_64.iso" "name":"alg-gnome-2025.10-x86_64.iso" "name":"alg-xfce-2025.10-x86_64.iso"`, nil
	case strings.Contains(url, "freepbxdistro") && strings.Contains(url, ".sha256sum"):
		return shaX + "  SNGDEB-PBX17-amd64-12-13-0-2603-3.iso\n", nil
	case strings.Contains(url, "freepbxdistro"):
		return `<a href="SNGDEB-PBX17-amd64-12-13-0-2603-3.iso">a</a><a href="SNG7-PBX16-64bit-2306-1.iso">b</a><a href="SNG7-FPBX-64bit-2008-1.iso">c</a>`, nil
	case strings.Contains(url, "ubuntusway") && strings.Contains(url, "sha256.txt"):
		return shaX + "  ubuntusway-25.04-desktop-amd64.iso\n", nil
	case strings.Contains(url, "ubuntusway") && strings.Contains(url, "25.04/"):
		return `<a href="ubuntusway-25.04-desktop-amd64.iso">x</a>`, nil
	case strings.Contains(url, "ubuntusway"):
		return `<a href="25.04/">a</a><a href="24.10/">b</a>`, nil
	case strings.Contains(url, "mythbuntu"):
		return `<a href="mythbuntu-16.04.6-desktop-amd64.iso">x</a>`, nil
	case strings.Contains(url, "finnix") && strings.Contains(url, "125/"):
		return `<a href="finnix-125.iso">x</a>`, nil
	case strings.Contains(url, "finnix"):
		return `<a href="125/">a</a><a href="124/">b</a>`, nil
	case strings.Contains(url, "mobian") && strings.Contains(url, ".sha256sums"):
		return shaX + "  mobian-amd64-phosh-13.0.img.xz\n" + shaX + "  mobian-amd64-plasma-mobile-13.0.img.xz\n", nil
	case strings.Contains(url, "mobian"):
		return `<a href="mobian-amd64-phosh-13.0.img.xz">x</a>
			<a href="mobian-amd64-plasma-mobile-13.0.img.xz">y</a>
			<a href="mobian-amd64-phosh-13.0.sha256sums">s</a>`, nil
	case strings.Contains(url, "rosalab") && strings.Contains(url, ".md5sum"):
		return shaX[:32] + "  ROSA.FRESH.PLASMA6.13.3.x86_64.iso\n", nil
	case strings.Contains(url, "rosalab") && strings.Contains(url, "lxqt"):
		return `<a href="ROSA.FRESH.LXQT.13.3.x86_64.iso">x</a>
			<a href="ROSA.FRESH.LXQT.13.2.x86_64.iso">y</a>`, nil
	case strings.Contains(url, "rosalab") && strings.Contains(url, "plasma6"):
		return `<a href="ROSA.FRESH.PLASMA6.13.3.x86_64.iso">x</a>
			<a href="ROSA.FRESH.PLASMA6.13.0.x86_64.iso">y</a>`, nil
	case strings.Contains(url, "rosalab"):
		return `<a href="ROSA.FRESH.GNOME.13.3.x86_64.iso">x</a>
			<a href="ROSA.FRESH.GNOME.13.0.x86_64.iso">y</a>`, nil
	case strings.Contains(url, "nemesis") && strings.Contains(url, "sha256sums"):
		return shaX + "  Nemesis-v26.09-XFCE-x86_64.iso\n", nil
	case strings.Contains(url, "nemesis") && strings.Contains(url, "ISO/26.09"):
		return `"name":"Nemesis-v26.09-XFCE-x86_64.iso" "name":"Nemesis-v26.09-LXQT-x86_64.iso" "name":"Nemesis-v26.09-CINNAMON-x86_64.iso"`, nil
	case strings.Contains(url, "nemesis"):
		return `"name":"26.09" "name":"26.06"`, nil
	case strings.Contains(url, "osgeo") && strings.Contains(url, ".md5"):
		return shaX[:32] + "  osgeolive-17.0-amd64.iso\n", nil
	case strings.Contains(url, "osgeo") && strings.Contains(url, "17.0"):
		return `<a href="osgeolive-17.0-amd64.iso">x</a>`, nil
	case strings.Contains(url, "osgeo") && strings.Contains(url, "12.0"):
		return `<a href="osgeolive-12.0-amd64.iso">x</a>`, nil
	case strings.Contains(url, "osgeo") && strings.Contains(url, "11.0"):
		return `<a href="osgeo-live-11.0-amd64.iso">x</a>`, nil
	case strings.Contains(url, "osgeo") && strings.Contains(url, "9.5"):
		return `<a href="osgeo-live-9.5-amd64.iso">x</a>`, nil
	case strings.Contains(url, "osgeo"):
		return `<a href="17.0/">a</a><a href="12.0/">b</a><a href="11.0/">c</a><a href="9.5/">d</a>`, nil
	case strings.Contains(url, "9front.org"):
		return `<a href="9front-11952.amd64.iso.gz">9</a>
			<a href="9front-11952.amd64.qcow2.gz">9</a>
			<a href="9front-11952.arm64.qcow2.gz">9</a>`, nil
	case strings.Contains(url, "jatayuos") && strings.Contains(url, "SHA256SUMS"):
		return shaX + "  JatayuOS-1.0-Udaan-amd64.iso\n", nil
	case strings.Contains(url, "jatayuos"):
		return `"name":"JatayuOS-1.0-Udaan-amd64.iso" "name":"SHA256SUMS"`, nil
	case strings.Contains(url, "kiro/files"):
		return `"name":"kiro-v26.11.01-x86_64.iso" "name":"kiro-v26.10.01-x86_64.iso" "name":"kiro-v26.09.01-x86_64.iso"`, nil
	case strings.Contains(url, "graphiteos") && strings.Contains(url, "SHA256SUMS"):
		return shaX + "  graphite-2026.10.24b-x86_64.iso\n", nil
	case strings.Contains(url, "graphiteos"):
		return `<a href="https://download.graphiteos.org/graphite-2026.10.24b-x86_64.iso">dl</a>`, nil
	case strings.Contains(url, "SHA256SUMS-v"):
		return shaX + "  abora-gnome-2026.10.03-x86_64-v4.1.iso\n" + shaX + "  abora-kde-2026.10.03-x86_64-v4.1.iso\n" + shaX + "  abora-kde-2026.08.28-x86_64-v4.0.iso\n", nil
	case strings.Contains(url, "gh/SHA256SUMS"):
		return shaX + "  vinix-amd64.iso\n" + shaX + "  vinix-arm64.iso\n", nil
	case strings.Contains(url, "bastillebsd") && strings.Contains(url, "CHECKSUM"):
		return "SHA256 (BastilleBSD-15.1-RELEASE-amd64-memstick.iso) = " + shaX + "\n", nil
	case strings.Contains(url, "bastillebsd") && strings.Contains(url, "15.1"):
		return `<a href="BastilleBSD-15.1-RELEASE-amd64-memstick.iso.xz">x</a>
			<a href="BastilleBSD-15.1-RELEASE-amd64-memstick.img.xz">y</a>`, nil
	case strings.Contains(url, "bastillebsd"):
		return `<a href="15.1/">a</a><a href="15.0/">b</a>`, nil
	default:
		// generic index page: iso links + version dirs + current.iso
		return `<a href="backbox-8-desktop-amd64.iso">b</a>
			<a href="batocera-x86_64-40.img.gz">bt</a>
			<a href="EndeavourOS-mercury.iso">e</a>
			<a href="9front-10000.amd64.iso">9</a>
			<a href="9front-9000.amd64.iso">9</a>
			<a href="pentoo-full-2024x.iso">p</a>
			<a href="tsurugi_linux_2.0.iso">t</a>
			<a href="vitalinux-3.5-2024-desktop-amd64.iso">v</a>
			<a href="TUXEDO-OS-2-20240101-current.iso">tx</a>
			<a href="Zorin-OS-17.1-Core-64-bit.iso">z</a>
			<a href="MAX-13.0.iso">mx</a>
			<a href="cachyos-x86_64.iso">c</a>
			&ISO/desktop;x;https://m/cachyos-desktop.iso;y
			`, nil
	}
}
