// scrape_harness_test.go — offline scrape() harness: canned pages + seam stubs (split of scrape2_test.go)
package main

import (
	"fmt"
	"strings"
	"testing"
)

const shaX = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const shaY = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

// bodyFor returns a canned page keyed on URL shape, so every scraper
// branch exercises its real parsing path.
func bodyFor(url string) (string, error) {
	switch {
	case strings.Contains(url, "latest-releases.yaml"):
		return fmt.Sprintf("- flavor: alpine-virt\n  arch: x86_64\n  iso: alpine-virt-3.19.0-x86_64.iso\n  sha256: %s\n- flavor: alpine-virt\n  arch: aarch64\n  iso: alpine-virt-3.19.0-aarch64.iso\n  sha256: %s\n- flavor: alpine-standard\n  iso: other.iso\n", shaX, shaX), nil
	case strings.Contains(url, "releng/releases/json"):
		return fmt.Sprintf(`{"releases":[{"iso_url":"archlinux-2024.01.01-x86_64.iso","sha256_sum":"%s"}]}`, shaX), nil
	case strings.Contains(url, "file-info.json"):
		return `{"official":{"kde":{"image":"https://m/manjaro-kde.iso"},"kde.minimal":{"image":"https://m/m.iso"}},"community":{"cinnamon":{"image":"https://m/c.iso"}}}`, nil
	case strings.Contains(url, "manjaro-sway"):
		return `[{"name":"manjaro-sway-1.0.iso","url":"https://m/sway.iso"}]`, nil
	case strings.Contains(url, "rss"):
		return `<rss><link>https://x/Release/ISO/nitrux-2.0.iso</link>
			<link>https://x/files/release/ROME/omv-plasma6x11.x86_64.iso</link></rss>`, nil
	case strings.Contains(url, "Athena-OS"):
		return `[{"browser_download_url":"https://gh/athenaos-live-x86_64.iso"},{"browser_download_url":"https://gh/athenaos-live-x86_64.iso.sha256"}]`, nil
	case strings.Contains(url, "install-media"):
		return `[{"browser_download_url":"https://gh/chimeraos-2025.04.21-x86_64.iso"},{"browser_download_url":"https://gh/sha256sum.txt"}]`, nil
	case strings.Contains(url, "harvester"):
		return `{"tag_name":"v1.9.0"}`, nil
	case strings.Contains(url, "api.github.com"):
		return `[{"browser_download_url":"https://gh/VanillaOS-2.0-oracular.iso"},
			{"browser_download_url":"https://gh/k3os-amd64.iso"},
			{"browser_download_url":"https://gh/haos_ova-14.0.ova"},
			{"browser_download_url":"https://gh/hello-0.9.0-amd64.iso"},
			{"browser_download_url":"https://gh/kairos-3.0-amd64.iso"},
			{"browser_download_url":"https://gh/metal-amd64.iso"},
			{"browser_download_url":"https://gh/image.iso"},
			{"browser_download_url":"https://gh/dahliaOS-24.04.img"},
			{"browser_download_url":"https://gh/zimaos-1.5.0.img"},
			{"browser_download_url":"https://gh/crystal-1.0.iso"},
			{"browser_download_url":"https://gh/instantos-1.0-x86_64.iso"},
			{"browser_download_url":"https://gh/instantos-1.0-x86_64.iso.sha256"},
			{"browser_download_url":"https://gh/minios-4.0-standard-amd64-2026.iso"},
			{"browser_download_url":"https://gh/minios-4.0-standard-amd64-2026.iso.sha256"},
			{"browser_download_url":"https://gh/rescuezilla-2.4-64bit.resolute.iso"},
			{"browser_download_url":"https://gh/SHA256SUMS"},
			{"browser_download_url":"https://gh/rancheros.iso"},
			{"browser_download_url":"https://gh/shredos-2024.11.2_23_x86-64_v0.38_20250110.iso"},
			{"browser_download_url":"https://gh/ventoy-1.1.18-livecd.iso"},
			{"browser_download_url":"https://gh/ventoy-sha256.txt"},
			{"browser_download_url":"https://gh/minikube-v1.32.0-amd64.iso"},
			{"browser_download_url":"https://gh/minikube-v1.32.0-amd64.iso.sha256"},
			{"browser_download_url":"https://gh/abora-gnome-2026.10.03-x86_64-v4.1.iso"},
			{"browser_download_url":"https://gh/abora-cosmic-2026.10.03-x86_64-v4.1.iso"},
			{"browser_download_url":"https://gh/abora-hyprland-2026.10.03-x86_64-v4.1.iso"},
			{"browser_download_url":"https://gh/abora-kde-2026.10.03-x86_64-v4.1.iso"},
			{"browser_download_url":"https://gh/abora-other-2026.10.03-x86_64-v4.1.iso"},
			{"browser_download_url":"https://gh/abora-kde-2026.08.28-x86_64-v4.0.iso"},
			{"browser_download_url":"https://gh/SHA256SUMS-v4.1.txt"},
			{"browser_download_url":"https://gh/asterinas-nixos-0.19.0-x86_64.iso"},
			{"browser_download_url":"https://gh/vinix-amd64.iso"},
			{"browser_download_url":"https://gh/vinix-arm64.iso"},
			{"browser_download_url":"https://gh/CervusOS-v0.0.1.iso"},
			{"browser_download_url":"https://gh/r2.iso"},
			{"browser_download_url":"https://gh/lateralus-os.iso"},
			{"browser_download_url":"https://gh/moros-v0.12.0.bin"},
			{"browser_download_url":"https://gh/moros-v0.11.0.bin"},
			{"browser_download_url":"https://gh/hello-0.9.0_0I67-FreeBSD-14.0-amd64.iso"},
			{"browser_download_url":"https://gh/hello-0.9.0_0I67-FreeBSD-14.0-amd64.iso.sha256"},
			{"browser_download_url":"https://gh/harvey-cdrom.iso.bz2"}]`, nil
	case strings.Contains(url, "broadcom.com"):
		return `<a href="photon-5.0-dde71ec57.x86_64.iso">x</a><a href="photon-minimal-5.0-dde71ec57.x86_64.iso">y</a><a href="photon-5.0-dde71ec57.x86_64.debug.iso">z</a><a href="photon-3.0-26156e2.iso">p</a><a href="photon-3.0-26156e2.debug.iso">q</a>`, nil
	case strings.Contains(url, "bandshed"):
		return `<a href="AVL_MXe-25.2_x64.iso">x</a><a href="AVL_MXe-25.2_x64.iso.sha256">s</a>`, nil
	case strings.Contains(url, "draugeros"):
		return `<a href="Drauger_OS-7.7-AMD64.iso">a</a><a href="Drauger_OS-7.8-AMD64.iso">b</a>`, nil
	case strings.Contains(url, "primeos"):
		return `"name":"PrimeOS-2.1.3-64-bit-20220719-BETA.iso" "name":"PrimeOS-2.1.2-64-bit-20220531-BETA.iso"`, nil
	case strings.Contains(url, "gparted-live-stable/1"):
		return `"name":"gparted-live-1.8.1-6-amd64.iso"`, nil
	case strings.Contains(url, "gparted"):
		return `"name":"1.8.1-6" "name":"1.8.1-3" "name":"gparted-live-stable"`, nil
	case strings.Contains(url, "mocaccino/files/v"):
		return `"name":"MocaccinoOS-MATE-0.20261019.iso" "name":"MocaccinoOS-KDE-0.20261019.iso" "name":"MocaccinoOS-MATE-0.20261019.iso.sha256"`, nil
	case strings.Contains(url, "mocaccino"):
		return `"name":"v26.10" "name":"v26.09"`, nil
	case strings.Contains(url, "loc-os") && strings.Contains(url, "%2023"):
		return `"name":"Loc-OS-23-KDE-x86_64.iso" "name":"Loc-OS-23-AHS.iso"`, nil
	case strings.Contains(url, "loc-os"):
		return `"name":"Loc-OS-24-current_amd64.iso" "name":"Loc-OS-24-current_i686.iso"`, nil
	case strings.Contains(url, "release/2"):
		return `<a href="cldx-20261022-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "calculate-linux"):
		return `<a href="20261022/">d</a><a href="20261019/">d</a>`, nil
	case strings.Contains(url, "centos.org/8"):
		return `<a href="CentOS-8.5.2111-x86_64-dvd1.iso">x</a>`, nil
	case strings.Contains(url, "centos.org/5"):
		return `<a href="CentOS-5.11-x86_64-bin-1of9.iso">x</a>`, nil
	case strings.Contains(url, "centos.org"):
		return `<a href="CentOS-6.10-x86_64-bin-DVD1.iso">x</a>`, nil
	case strings.Contains(url, "core204"):
		return `<a href="ipfire-2.29-core204-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "freedif.org/crux") && strings.HasSuffix(url, ".sha256"):
		return shaX + "  crux-3.8.iso\n", nil
	case strings.Contains(url, "freedif.org/crux") && strings.Contains(url, "iso"):
		return `<a href="crux-3.8.iso">x</a>`, nil
	case strings.Contains(url, "freedif.org/crux"):
		return `<a href="crux-3.8/">d</a><a href="crux-3.7/">d</a>`, nil
	case strings.Contains(url, "freedif"):
		return `<a href="2.29-core204/">d</a><a href="2.29-core203/">d</a>`, nil
	case strings.Contains(url, "fatdog"):
		return `<a href="Fatdog64-903.iso">x</a><a href="Fatdog64-500.iso">y</a>`, nil
	case strings.Contains(url, "solydxk"):
		return `<a href="solydx_13_64_202512.iso">x</a><a href="solydk_13_64_202512.iso">y</a>`, nil
	case strings.Contains(url, "nluug"):
		return `<a href="KNOPPIX_V9.1CD-2021-01-25-EN.iso">x</a>
			<a href="KNOPPIX_V7.2.0CD-2013-06-16-EN.iso">x</a>
			<a href="KNOPPIX_V7.0.5CD-2012-12-21-EN.iso">x</a>`, nil
	case strings.Contains(url, "minix3"):
		return `<a href="minix_R3.3.0-588a35b.iso.bz2">x</a>`, nil
	case strings.Contains(url, "neptuneos"):
		return `<a href="Neptune9-20260314.iso">x</a>`, nil
	case strings.Contains(url, "nomadbsd") && strings.Contains(url, ".sha256"):
		return shaX + "  nomadbsd-141R-20240711.amd64.ufs.img.lzma\n", nil
	case strings.Contains(url, "stamus-networks"):
		return `<a href="https://my.stamus-networks.com/sn-dl/ClearNDRCommunity/0c6cf7ac31bb211ea3b21c05c291d5145f624ad0164c456b9bf07db9f080d47f/ClearNDR-nodesktop-1.0.0.iso">x</a>
			<a href="https://my.stamus-networks.com/sn-dl/ClearNDRCommunity/6d46518e85f546bddcc82d9d1143fbe0ce2749abc862646fda470bad3335f1aa/ClearNDR-desktop-1.0.0.iso">y</a>`, nil
	case strings.Contains(url, "nomadbsd"):
		return `<a href="nomadbsd-141R-20240711.amd64.ufs.img.lzma">x</a>
			<a href="nomadbsd-141R-20240711.amd64.zfs.img.lzma">x</a>
			<a href="nomadbsd-140R-20240126.amd64.ufs.img.lzma">x</a>`, nil
	case strings.Contains(url, "alpix.eu"):
		return `<a href="EndeavourOS_Titan-Nova-2026.08.15.iso">x</a>
			<a href="EndeavourOS_Ganymede-2025.11.24.iso">x</a>
			<a href="EndeavourOS_Mercury-Neo-2025.03.19.iso">x</a>
			<a href="EndeavourOS_Apollo_22_1.iso">x</a>`, nil
	case strings.Contains(url, "openmamba"):
		return `<a href="https://cdn.openmamba.org/pub/openmamba/media/rolling/livecd/en/openmamba-livecd-en-snapshot-20261021.x86_64.iso">x</a>`, nil
	case strings.Contains(url, "yunohost"):
		return `<a href="yunohost-bookworm-12.0-amd64-stable.iso">x</a>`, nil
	case strings.Contains(url, "amazonlinux"):
		return `<a href="al2023-kvm-2023.12.20261026.1-kernel-6.1-x86_64.xfs.gpt.qcow2">x</a>`, nil
	case strings.Contains(url, "dietpi"):
		return `<a href="DietPi_NativePC-UEFI-x86_64-Forky.img.xz">x</a>
			<a href="DietPi_NativePC-UEFI-x86_64-Trixie.img.xz">x</a>
			<a href="DietPi_NativePC-BIOS-x86_64-Trixie.img.xz">x</a>
			<a href="DietPi_NativePC-UEFI-x86_64-Bookworm.img.xz">x</a>`, nil
	case strings.Contains(url, "euro-linux"):
		return `<a href="EL-9-x86_64-latest-appstream.iso">x</a>
			<a href="EL-9-x86_64-latest-minimal.iso">x</a>
			<a href="EL-8-x86_64-latest-appstream.iso">x</a>
			<a href="EL-8-x86_64-latest-minimal.iso">x</a>`, nil
	case strings.Contains(url, "hurd-amd64/2"):
		return `<a href="debian-sid-hurd-amd64-NETINST-1.iso">x</a>`, nil
	case strings.Contains(url, "hurd-amd64"):
		return `<a href="20260314/">d</a>`, nil
	case strings.Contains(url, "leaseweb") && strings.Contains(url, "SHA256SUMS"):
		return shaX + "  grml-full-2026.09-amd64.iso\n" +
			shaY + "  grml-small-2026.09-amd64.iso\n" +
			shaY + "  grml-full-2026.09-arm64.iso\n", nil
	case strings.Contains(url, "leaseweb"):
		return `<a href="grml-full-2026.09-amd64.iso">x</a>
			<a href="grml-small-2026.09-amd64.iso">x</a>
			<a href="grml-full-2026.09-arm64.iso">x</a>
			<a href="grml-full-2026.04-amd64.iso">x</a>`, nil
	case strings.Contains(url, "omnios"):
		return `<a href="omnios-r151058.iso">x</a>`, nil
	case strings.Contains(url, "enterprise.proxmox") && strings.Contains(url, "SHA256SUMS"):
		return shaX + "  proxmox-datacenter-manager_1.1-1.iso\n", nil
	case strings.Contains(url, "enterprise.proxmox"):
		return `<a href="proxmox-backup-server_4.2-1.iso">a</a><a href="proxmox-backup-server_4.1-1.iso">b</a><a href="proxmox-mailgateway_7.3-1.iso">c</a><a href="proxmox-mail-gateway_9.1-1.iso">d</a><a href="proxmox-mail-gateway_9.0-1.iso">e</a><a href="proxmox-mail-gateway_8.2-1.iso">f</a><a href="proxmox-datacenter-manager_1.1-1.iso">g</a><a href="proxmox-ve_9.2-1.iso">h</a>`, nil
	case strings.Contains(url, "pisilinux/files/lxqt"):
		return `<a href="Pisi-Linux-lxqt-2.4.1-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "pisilinux"):
		return `<a href="Pisi-Linux-2.4.3-KDE6-x86_64.iso">x</a><a href="Pisi-Linux-2.4.3-Minimal-x86_64.iso">y</a>`, nil
	case strings.Contains(url, "vicidial"):
		return `<a href="ViciBox_V12.x86_64-12.0.2.iso">x</a><a href="ViciBox_V12.x86_64-12.0.2-md.iso">y</a>`, nil
	case strings.Contains(url, "images/rpd"):
		return `<a href="2024-11-19-rpd-x86.iso">x</a>`, nil
	case strings.Contains(url, "rpd_x86"):
		return `<a href="rpd_x86-2024-11-19/">d</a>`, nil
	case strings.Contains(url, "tribblix"):
		return `<a href="https://iso.tribblix.org/iso/tribblix-0m41.1.iso">x</a>`, nil
	case strings.Contains(url, "xcp-ng.org/isos/7"), strings.Contains(url, "xcp-ng.org/isos/8.0"):
		return `<a href="xcp-ng-7.6.0.iso">x</a><a href="xcp-ng-8.0.2.iso">y</a>`, nil
	case strings.Contains(url, "xcp-ng"):
		return `<a href="xcp-ng-8.3-20250606.iso">x</a>`, nil
	case strings.Contains(url, "ultramarine/40"):
		return `<a href="ultramarine-40-x86_64-flagship.iso">x</a><a href="ultramarine-40-x86_64-kde.iso">y</a>`, nil
	case strings.Contains(url, "ultramarine/39"):
		return `<a href="ultramarine-39-x86_64-flagship.iso">x</a>`, nil
	case strings.Contains(url, "fyralabs"):
		return `<a href="ultramarine-42-x86_64-flagship.iso">x</a>`, nil
	case strings.Contains(url, "coreos.fedoraproject"):
		return `{"architectures":{"x86_64":{"artifacts":{"qemu":{"formats":{"qcow2.xz":{"disk":{"location":"https://fcos/qemu.qcow2.xz","sha256":"fcos"}}}}}}}}`, nil
	case strings.Contains(url, "chimera-linux"):
		return shaX + "  chimera-linux-x86_64-LIVE-20251220-base.iso\n" + shaX + "  chimera-linux-x86_64-LIVE-20251220-gnome.iso\n" + shaX + "  chimera-linux-x86_64-LIVE-20251220-plasma.iso\n", nil
	case strings.Contains(url, "xigmanas"):
		return `"name":"XigmaNAS-x64-13.0.0.5.iso"`, nil
	case strings.Contains(url, "salix"):
		return `"name":"salixlive64-xfce-15.0.iso"`, nil
	case strings.Contains(url, "SHA"), strings.Contains(url, "sha256"),
		strings.Contains(url, "md5"), strings.Contains(url, "MD5"),
		strings.Contains(url, "SHASUMS"), strings.Contains(url, "sha512"),
		strings.Contains(url, "checksums"), strings.Contains(url, "sha256sum"),
		strings.Contains(url, "sha256sums"):
		h := shaX + "  "
		return h + "artix-xfce-2024-x86_64.iso\n" + h + "void-live-x86_64-1-base.iso a b " + shaX +
			"\na-b-c-d-20240101-x x86_64-LIVE\n" + h + "chimera linux x86_64-LIVE gnome\n" + h +
			"siduction-x.iso.md5\nx y z " + shaX + "\n" + h + "debian-live-12.5.0-amd64-xfce.iso\n" +
			h + "Zorin-OS-17.1-Core-64-bit.iso\n" + shaX + "\n", nil
	case strings.Contains(url, "README"):
		return shaX + "  antiX-23-runit_x64-full.iso\n", nil
	case strings.Contains(url, "debian-cd") && strings.HasSuffix(url, "/"):
		return `<a href="12.5.0/">x</a>`, nil
	case strings.Contains(url, "rocky"):
		return `<a class="link" href="9.3/">9.3/</a><a class="link" href="8.9/">8.9/</a><a class="link" href="full/">full/</a>`, nil
	case strings.Contains(url, "slitaz"):
		return `<a class='iso' href='slitaz-rolling-core64.iso'>slitaz-rolling-core64.iso</a><a class='iso'>slitaz-rolling-core.iso</a>`, nil
	case strings.Contains(url, "pureos"):
		return `"https://downloads.puri.sm/pureos-10.3-librem-2024-x/"`, nil
	case strings.Contains(url, "springdale") && strings.Contains(url, "SHA256SUMS"):
		return shaX + "  Springdale Linux-9.2-x86_64-netinst.iso\n", nil
	case strings.Contains(url, "springdale"):
		return `<a href="Springdale%20Linux-9.2-x86_64-netinst.iso">x</a>`, nil
	case strings.Contains(url, "princeton"), strings.Contains(url, "dotsrc"):
		return `<a class="folder" href="siduction-24.01/">x</a><a href="x-1.0-y-z-20240101.iso.md5">m</a>`, nil
	case strings.Contains(url, "kirkstone"):
		return `<a href="2025/">x</a><a href="2024/">y</a>`, nil
	case strings.Contains(url, "truenas"), strings.Contains(url, "sys.truenas"):
		return `<a href="24.04.1/">v</a><a href="TrueNAS-SCALE-24.04.1.iso">i</a>`, nil
	case strings.Contains(url, "kicksecure"):
		return `<a href="17.9.9/">v</a><a href="Kicksecure-CLI-17.9.9.Intel_AMD64.qcow2.libvirt.xz">i</a>`, nil
	// --- third-expansion fixtures ---
	case strings.Contains(url, "opencloudos") && strings.Contains(url, "8.6") && strings.Contains(url, "CHECKSUM"):
		return shaX + "  opencloudos-8.6-x86_64-minimal.iso\n", nil
	case strings.Contains(url, "opencloudos") && strings.Contains(url, "8.6"):
		return `<a href="opencloudos-8.6-x86_64-boot.iso">a</a><a href="opencloudos-8.6-x86_64-dvd1.iso">b</a><a href="opencloudos-8.6-x86_64-minimal.iso">c</a>`, nil
	case strings.Contains(url, "opencloudos") && strings.Contains(url, "8.8") && strings.Contains(url, "CHECKSUM"):
		return shaX + "  OpenCloudOS-8.8-x86_64-dvd1.iso\n", nil
	case strings.Contains(url, "opencloudos") && strings.Contains(url, "8.8"):
		return `<a href="OpenCloudOS-8.8-x86_64-dvd1.iso">x</a>`, nil
	case strings.Contains(url, "opencloudos") && strings.Contains(url, "8.10") && strings.Contains(url, "CHECKSUM"):
		return shaX + "  OpenCloudOS-8.10-20241210.0-x86_64-minimal.iso\n", nil
	case strings.Contains(url, "opencloudos") && strings.Contains(url, "8.10"):
		return `<a href="OpenCloudOS-8.10-20241210.0-x86_64-boot.iso">a</a><a href="OpenCloudOS-8.10-20241210.0-x86_64-minimal.iso">b</a><a href="OpenCloudOS-8.10-20241210.0-x86_64-dvd1.iso">c</a>`, nil
	case strings.Contains(url, "opencloudos") && strings.Contains(url, "CHECKSUM"):
		return shaX + "  OpenCloudOS-9.4-20260313.0-x86_64-minimal.iso\n", nil
	case strings.Contains(url, "opencloudos") && strings.Contains(url, "20260313.0"):
		return `<a href="OpenCloudOS-9.4-20260313.0-x86_64-minimal.iso">x</a>`, nil
	case strings.Contains(url, "opencloudos") && strings.Contains(url, "9.0"):
		return `<a href="OpenCloudOS-9.0-everything-x86_64.iso">x</a><a href="OpenCloudOS-9.0-minimal-x86_64.iso">y</a><a href="OpenCloudOS-9.0-netinst-x86_64.iso">z</a>`, nil
	case strings.Contains(url, "opencloudos"):
		return `<a href="20260313.0/">d</a>`, nil
	case strings.Contains(url, "tencentos") && strings.Contains(url, "4.0") && strings.Contains(url, "CHECKSUM"):
		return shaX + "  TencentOS-Server-4.0-netinst-x86_64.iso\n", nil
	case strings.Contains(url, "tencentos") && strings.Contains(url, "4.0"):
		return `<a href="TencentOS-Server-4.0-netinst-x86_64.iso">x</a>`, nil
	case strings.Contains(url, "tencentos") && strings.Contains(url, "CHECKSUM"):
		return shaX + "  TencentOS-Server-4.2-20250227.0-x86_64-boot.iso\n", nil
	case strings.Contains(url, "tencentos") && strings.Contains(url, "20250227.0"):
		return `<a href="TencentOS-Server-4.2-20250227.0-x86_64-boot.iso">x</a>`, nil
	case strings.Contains(url, "tencentos"):
		return `<a href="20250227.0/">d</a>`, nil
	case strings.Contains(url, "netgate"):
		return `<a href="pfSense-CE-2.7.2-RELEASE-amd64.iso.gz">x</a>`, nil
	case strings.Contains(url, "ubuntu-core/26"):
		return `<a href="ubuntu-core-26-amd64.img.xz">x</a>`, nil
	case strings.Contains(url, "ubuntu-core"):
		return `<a href="ubuntu-core-24-amd64.img.xz">x</a>`, nil
	case strings.Contains(url, "grommunio"):
		return shaX + "\n", nil
	case strings.Contains(url, "turnkeylinux") && strings.Contains(url, ".hash"):
		return shaX + "  turnkey-core-19.0-trixie-amd64.iso\n", nil
	case strings.Contains(url, "turnkeylinux"):
		return `<a href="turnkey-core-18.1-bookworm-amd64.iso">x</a><a href="turnkey-core-19.0-trixie-amd64.iso">w</a><a href="turnkey-lamp-19.0-trixie-amd64.iso">y</a><a href="turnkey-nextcloud-19.0-trixie-amd64.iso">z</a><a href="turnkey-wordpress-19.0-trixie-amd64.iso">v</a>`, nil
	case strings.Contains(url, "freenas.org") && strings.Contains(url, "12.0"):
		return `<a href="TrueNAS-12.0-U8.1.iso?download=1">x</a>`, nil
	case strings.Contains(url, "freenas.org"):
		return `<a href="TrueNAS-13.0-U6.8.iso?download=1">x</a>`, nil
	case strings.Contains(url, "files.devuan.org") && strings.Contains(url, "SHA256SUMS"):
		return shaX + "  devuan_daedalus_5.0.0_amd64_desktop-live.iso\n", nil
	case strings.Contains(url, "files.devuan.org") && strings.Contains(url, "desktop-live"):
		return `<a href="devuan_daedalus_5.0.0_amd64_desktop-live.iso">x</a><a href="devuan_excalibur_6.1.1_amd64_desktop-live.iso">y</a>`, nil
	case strings.Contains(url, "netrunner"):
		return `<a href="netrunner-25-64-bit.iso">x</a>`, nil
	case strings.Contains(url, "daphile"):
		return `<a href="daphile-24.02-x86_64.iso">x</a><a href="daphile-24.02-x86_64-rt.iso">y</a>`, nil
	case strings.Contains(url, "cdimage/release"):
		return `<a href="debian-edu-12.5.0-amd64-netinst.iso">x</a>`, nil
	case strings.Contains(url, "slowroll"):
		return `<a href="openSUSE-Slowroll-DVD-x86_64-Build2024.09.15-Media.iso">x</a>`, nil
	case strings.Contains(url, "hardenedbsd") && strings.Contains(url, "MANIFEST"):
		return "SHA256 (disc1.iso) = " + shaX + "\n", nil
	case strings.Contains(url, "hardenedbsd"):
		return `<a href="disc1.iso">x</a>`, nil
	case strings.Contains(url, "zenwalk.org/x86_64/15"):
		return `<a href="zenwalk-15.0-220204.iso">x</a>`, nil
	case strings.Contains(url, "zenwalk.org/x86_64/8.0"):
		return `<a href="zenwalk-8.0.iso">x</a><a href="zenwalk_core-8.0.iso">y</a>`, nil
	case strings.Contains(url, "zenwalk"):
		return `<a href="zenwalk-current-250715.iso">x</a>`, nil
	case strings.Contains(url, "aerynos.dev") && strings.HasSuffix(url, ".sha256sum"):
		return shaX + "  AerynOS-2026.08-GNOME-live-x86_64.iso\n", nil
	case strings.Contains(url, "aerynos.dev"):
		return `<a href="./AerynOS-2026.08-GNOME-live-x86_64.iso">x</a><a href="./AerynOS-2026.05.2-GNOME-live-x86_64.iso">y</a>`, nil
	case strings.Contains(url, "freedombox"):
		return `<a href="freedombox-trixie_all-amd64.img.xz">x</a>`, nil
	case strings.Contains(url, "parabola"):
		return `<a href="parabola-x86_64-systemd-lxde-2019.06-pre-complete.iso">x</a>`, nil
	case strings.Contains(url, "geckolinux") && strings.HasSuffix(url, "Rolling/"):
		return `"name":"999.220820"`, nil
	case strings.Contains(url, "geckolinux") && strings.HasSuffix(url, "Static/"):
		return `"name":"154.220822"`, nil
	case strings.Contains(url, "geckolinux"):
		return `"name":"GeckoLinux_ROLLING_Cinnamon.x86_64-999.220820.0.iso" "name":"GeckoLinux_STATIC_BareBones.x86_64-154.220822.0.iso"`, nil
	case strings.Contains(url, "robolinux"):
		return `"name":"robolinux-cinnamon-12.10.iso"`, nil
	case strings.Contains(url, "voyagerlive"):
		return `"name":"Voyager-24.04-debian-amd64.iso" "name":"Voyager-11-LTS-amd64.iso"`, nil
	case strings.Contains(url, "ubuntuce"):
		return `"name":"ubuntuce-24.04-desktop-amd64.iso"`, nil
	case strings.Contains(url, "smoothwall"):
		return `"name":"Express-3.1-SP6-x86_64.iso"`, nil
	case strings.Contains(url, "efw"):
		return `"name":"community-3.3.2-devel-x86-64.iso"`, nil
	case strings.Contains(url, "t1n1wall"):
		return `"name":"generic-pc-2.11b123.iso"`, nil
	case strings.Contains(url, "jnode"):
		return `"name":"0.2.8" "name":"jnode-x86-0.2.8.iso.gz"`, nil
	case strings.Contains(url, "ultimateedition"):
		return `"name":"ultimate-arch-server-2026.10.20-x86_64-kde.iso"`, nil
	case strings.Contains(url, "nethserver"):
		return `"name":"nethserver-7.9.2009-x86_64.iso"`, nil
	case strings.Contains(url, "archbang"):
		return `"name":"archbang-010126-x86_64.iso"`, nil
	case strings.Contains(url, "risios"):
		return `"name":"40-20260315" "name":"risiOS-Live-40-20260315.iso"`, nil
	case strings.Contains(url, "makulu"):
		return `"name":"MakuluLinux-AI-OS-Debian-2026-09-22.iso"`, nil
	case strings.Contains(url, "ezarch"):
		return `"name":"ezarcher-kde-2026.iso"`, nil
	case strings.Contains(url, "boot-repair-cd"):
		return `"name":"boot-repair-disk-64bit.iso"`, nil
	case strings.Contains(url, "rescatux"):
		return `"name":"rescatux-0.74" "name":"rescatux-0.74.iso"`, nil
	case strings.Contains(url, "lxle"):
		return `"name":"LXLE-Focal-64.iso"`, nil
	case strings.Contains(url, "furybsd"):
		return `"name":"FuryBSD-12.1-KDE" "name":"FuryBSD-12.1-XFCE" "name":"FuryBSD-12.1-XFCE.iso"`, nil
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

func stubNet(t *testing.T) {
	t.Helper()
	oldF, oldU, oldW := fetchString, finalURL, webCheck
	fetchString = bodyFor
	finalURL = func(u string) (string, error) { return u + "?download=1", nil }
	webCheck = func(string) bool { return true }
	t.Cleanup(func() { fetchString, finalURL, webCheck = oldF, oldU, oldW })
}
