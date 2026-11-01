// providers_extra_test.go — offline coverage of the mainstream-OS
// providers: autounattend/preseed/desktop-autoinstall rendering,
// provider selection per catalog OS, completion modes, interactive
// step trimming and the serial-marker wait.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// --- inference + metadata ----------------------------------------------

func TestInferProviderMainstream(t *testing.T) {
	cases := map[string]string{
		"ubuntu-server":              "ubuntu-autoinstall",
		"ubuntu":                     "ubuntu-desktop-autoinstall",
		"ubuntu-unity":               "ubuntu-desktop-autoinstall",
		"ubuntucinnamon":             "ubuntu-desktop-autoinstall",
		"ubuntudde":                  "ubuntu-desktop-autoinstall",
		"debian":                     "debian-preseed",
		"kali":                       "debian-preseed",
		"devuan":                     "debian-preseed",
		"debian-edu":                 "debian-preseed",
		"windows":                    "windows-autounattend",
		"windows-server":             "windows-autounattend",
		"macos":                      "visual",
		"alma":                       "anaconda-kickstart",
		"centos":                     "anaconda-kickstart",
		"centos-stream":              "anaconda-kickstart",
		"rockylinux":                 "anaconda-kickstart",
		"oraclelinux":                "anaconda-kickstart",
		"tencentos":                  "anaconda-kickstart",
		"anolis":                     "anaconda-kickstart",
		"eurolinux":                  "anaconda-kickstart",
		"opencloudos":                "anaconda-kickstart",
		"openeuler":                  "anaconda-kickstart",
		"springdale":                 "anaconda-kickstart",
		"opensuse-slowroll":          "opensuse-autoyast",
		"archlinux":                  "arch-pacstrap",
		"nethserver":                 "anaconda-kickstart",
		"ovirt-node":                 "anaconda-kickstart",
		"koozali":                    "anaconda-kickstart",
		"proxmox-ve":                 "proxmox-autoinstall",
		"proxmox-backup-server":      "proxmox-autoinstall",
		"proxmox-mail-gateway":       "proxmox-autoinstall",
		"proxmox-datacenter-manager": "proxmox-autoinstall",
		"nixos":                      "nixos-serial",
		"openbsd":                    "openbsd-autoinstall",
		"freebsd":                    "freebsd-shell",
		"void":                       "void-xbps",
		"gentoo":                     "gentoo-stage3",
		"flatcar":                    "flatcar-ignition",
		"fedora-coreos":              "coreos-ignition",
		"parabola":                   "arch-pacstrap",
		"linuxmint":                  "ubiquity-preseed",
		"zorin":                      "ubiquity-preseed",
		"xubuntu":                    "ubiquity-preseed",
		"ubuntu-mate":                "ubiquity-preseed",
		"edubuntu":                   "ubuntu-desktop-autoinstall",
		"slackware":                  "slackware-live",
		"artixlinux":                 "artix-basestrap",
		"calculate-linux":            "calculate-clinstall",
	}
	for osName, want := range cases {
		var p Profile
		p.System.OS = osName
		if got := inferProvider(&p); got != want {
			t.Fatalf("%s → %q, want %q", osName, got, want)
		}
	}
	// fedora is edition-aware: Server runs anaconda, spins are live.
	var srv, wk Profile
	srv.System.OS, srv.System.Edition = "fedora", "Server"
	wk.System.OS, wk.System.Edition = "fedora", "Workstation"
	if inferProvider(&srv) != "anaconda-kickstart" {
		t.Fatal("fedora Server → kickstart")
	}
	if inferProvider(&wk) != "visual" {
		t.Fatal("fedora Workstation is a live image → visual")
	}
	// debian: only installer editions run d-i; DE editions are live.
	var dNet, dLive Profile
	dNet.System.OS, dNet.System.Edition = "debian", "netinst"
	dLive.System.OS, dLive.System.Edition = "debian", "gnome"
	if inferProvider(&dNet) != "debian-preseed" {
		t.Fatal("debian netinst → preseed")
	}
	if inferProvider(&dLive) != "visual" {
		t.Fatal("debian live → visual")
	}
	// trisquel: only netinst runs d-i.
	var tNet, tLive Profile
	tNet.System.OS, tNet.System.Edition = "trisquel", "netinst"
	tLive.System.OS, tLive.System.Edition = "trisquel", "mate"
	if inferProvider(&tNet) != "debian-preseed" {
		t.Fatal("trisquel netinst → preseed")
	}
	if inferProvider(&tLive) != "visual" {
		t.Fatal("trisquel live → visual")
	}
	// opensuse: tumbleweed/leap autoyast; microos is combustion.
	var leap, micro Profile
	leap.System.OS, leap.System.Release = "opensuse", "16.0"
	micro.System.OS, micro.System.Release = "opensuse", "microos"
	if inferProvider(&leap) != "opensuse-autoyast" {
		t.Fatal("opensuse 16.0 → autoyast")
	}
	if inferProvider(&micro) != "visual" {
		t.Fatal("opensuse microos → visual")
	}
	// alpine: unattended needs mkinitfs ≥3.19 for the ssh_key URL.
	var a19, a18 Profile
	a19.System.OS, a19.System.Release = "alpine", "v3.22"
	a18.System.OS, a18.System.Release = "alpine", "v3.18"
	if inferProvider(&a19) != "alpine-unattended" {
		t.Fatal("alpine v3.22 → unattended")
	}
	if inferProvider(&a18) != "visual" {
		t.Fatal("alpine v3.18 → visual")
	}
	// non-cloud-init disk images must not silently claim nocloud —
	// they boot as self-configured appliances under visual.
	for _, osName := range []string{"dietpi", "mobian", "openwrt"} {
		var p Profile
		p.System.OS = osName
		if inferProvider(&p) != "visual" {
			t.Fatalf("%s → visual, got %q", osName, inferProvider(&p))
		}
	}
}

func TestPickProviderNew(t *testing.T) {
	for _, name := range []string{
		"ubuntu-desktop-autoinstall", "debian-preseed",
		"windows-autounattend", "ubiquity-preseed", "slackware-live",
		"artix-basestrap", "calculate-clinstall", "visual"} {
		var p Profile
		p.Automation.Provider = name
		prov, err := pickProvider(&p)
		if err != nil || prov.id() != name {
			t.Fatalf("provider %s: %v", name, err)
		}
	}
}

func TestProviderMetaNew(t *testing.T) {
	w := &windowsAutounattend{}
	if !w.needsSecondBoot() || w.interactive() ||
		w.completionMode() != "poweroff" {
		t.Fatal("windows meta")
	}
	if got := w.postInstallFiles(); len(got) != 1 || got[0] != "unattended.iso" {
		t.Fatalf("unattended.iso must be removed post-install: %v", got)
	}
	if got := w.dropConfKeys(); len(got) != 2 || got[0] != "iso" {
		t.Fatalf("windows drop keys %v", got)
	}
	d := &debianPreseed{}
	if !d.needsSecondBoot() || d.completionMode() != "poweroff" ||
		len(d.dropConfKeys()) != 5 {
		t.Fatal("preseed meta")
	}
	u := &ubuntuDesktopAutoinstall{}
	if !u.needsSecondBoot() || !strings.HasPrefix(u.completionMode(), "serial:") {
		t.Fatal("desktop must signal completion via serial marker")
	}
	v := &visualProvider{}
	if !v.interactive() || v.needsSecondBoot() {
		t.Fatal("visual meta")
	}
	a := &anacondaKickstart{}
	if !a.needsSecondBoot() || a.interactive() ||
		a.completionMode() != "poweroff" || len(a.dropConfKeys()) != 5 {
		t.Fatal("kickstart meta")
	}
	s := &opensuseAutoyast{}
	if !s.needsSecondBoot() || s.interactive() ||
		s.completionMode() != "poweroff" || len(s.dropConfKeys()) != 5 {
		t.Fatal("autoyast meta")
	}
	al := &alpineUnattended{}
	if !al.needsSecondBoot() || al.interactive() ||
		al.completionMode() != "poweroff" || len(al.dropConfKeys()) != 5 {
		t.Fatal("alpine meta")
	}
	ar := &archPacstrap{}
	if !ar.needsSecondBoot() || ar.interactive() ||
		ar.completionMode() != "poweroff" || len(ar.dropConfKeys()) != 5 {
		t.Fatal("arch meta")
	}
	var _ serialDriver = ar // must implement the serial-driving hook
	nx := &nixosSerial{}
	if !nx.needsSecondBoot() || nx.interactive() ||
		nx.completionMode() != "poweroff" || len(nx.dropConfKeys()) != 5 {
		t.Fatal("nixos meta")
	}
	var _ serialDriver = nx
	px := &proxmoxAutoInstall{}
	if !px.needsSecondBoot() || px.interactive() ||
		px.completionMode() != "webhook" || len(px.dropConfKeys()) != 2 {
		t.Fatal("proxmox meta")
	}
	if got := px.postInstallFiles(); len(got) != 1 ||
		got[0] != "proxmox-answer.iso" {
		t.Fatalf("answer iso must be removed post-install: %v", got)
	}
	ob := &openbsdAutoinstall{}
	if !ob.needsSecondBoot() || ob.interactive() ||
		!strings.HasPrefix(ob.completionMode(), "serial:") {
		t.Fatal("openbsd meta")
	}
	var _ serialDriver = ob
	fb := &freebsdShell{}
	if !fb.needsSecondBoot() || fb.interactive() ||
		fb.completionMode() != "poweroff" || len(fb.dropConfKeys()) != 2 {
		t.Fatal("freebsd meta")
	}
	var _ serialDriver = fb
	vx := &voidXbps{}
	if !vx.needsSecondBoot() || vx.interactive() ||
		vx.completionMode() != "poweroff" || len(vx.dropConfKeys()) != 5 {
		t.Fatal("void meta")
	}
	var _ serialDriver = vx
	gt := &gentooStage3{}
	if !gt.needsSecondBoot() || gt.interactive() ||
		gt.completionMode() != "poweroff" || len(gt.dropConfKeys()) != 5 {
		t.Fatal("gentoo meta")
	}
	var _ serialDriver = gt
	fc := &flatcarIgnition{}
	if !fc.needsSecondBoot() || fc.interactive() ||
		!strings.HasPrefix(fc.completionMode(), "serial:") {
		t.Fatal("flatcar meta")
	}
	var _ serialDriver = fc
	ci := &coreosIgnition{}
	if ci.needsSecondBoot() || ci.interactive() ||
		len(ci.dropConfKeys()) != 1 || ci.dropConfKeys()[0] != "extra_args" {
		t.Fatal("coreos meta")
	}
	ub := &ubiquityPreseed{}
	if !ub.needsSecondBoot() || ub.interactive() ||
		ub.completionMode() != "poweroff" || len(ub.dropConfKeys()) != 5 {
		t.Fatal("ubiquity meta")
	}
	sk := &slackwareLive{}
	if !sk.needsSecondBoot() || sk.interactive() ||
		sk.completionMode() != "poweroff" || len(sk.dropConfKeys()) != 5 {
		t.Fatal("slackware meta")
	}
	var _ serialDriver = sk
	ax := &artixBasestrap{}
	if !ax.needsSecondBoot() || ax.interactive() ||
		ax.completionMode() != "poweroff" || len(ax.dropConfKeys()) != 5 {
		t.Fatal("artix meta")
	}
	var _ serialDriver = ax
	cl := &calculateClinstall{}
	if !cl.needsSecondBoot() || cl.interactive() ||
		cl.completionMode() != "poweroff" || len(cl.dropConfKeys()) != 5 {
		t.Fatal("calculate meta")
	}
	var _ serialDriver = cl
}

func TestPlanStepsVisualTrimmed(t *testing.T) {
	steps := planSteps(&visualProvider{})
	want := []string{"resolve", "acquire-image", "prepare", "launch"}
	if len(steps) != len(want) {
		t.Fatalf("visual steps %v", steps)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Fatalf("visual steps %v", steps)
		}
	}
	// poweroff providers keep the full tail.
	steps = planSteps(&windowsAutounattend{})
	if steps[len(steps)-1] != "seal" || len(steps) < 10 {
		t.Fatalf("windows steps %v", steps)
	}
}

// --- answer file rendering ----------------------------------------------

func TestRenderAutounattend(t *testing.T) {
	var p Profile
	p.System.OS, p.System.Release = "windows-server", "2022"
	p.Identity.Hostname = "srv&x"
	p.Identity.Username = "dev"
	p.Identity.Password = ""
	p.Locale.Lang = "es_ES.UTF-8"
	xml := renderAutounattend(&p, "ssh-ed25519 AAAA key")
	for _, want := range []string{
		"<ComputerName>srv&amp;x</ComputerName>",
		"<Name>dev</Name>",
		"Quickemu#2026", // default compliant password
		"OpenSSH.Server~~~~0.0.1.0",
		"ssh-ed25519 AAAA key",
		"administrators_authorized_keys",
		"shutdown /s /t 5",
		"E:\\viostor\\w2k22\\amd64",
		"es-ES",
		"WX4NM-KYWYW-QJJR4-XV3QB-6VM33", // Server 2022 GVLK
		"BypassTPMCheck",
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("autounattend missing %q", want)
		}
	}
	// driver dir per release
	var w Profile
	w.System.OS, w.System.Release = "windows", "10"
	if !strings.Contains(renderAutounattend(&w, "k"), `w10\amd64`) {
		t.Fatal("win10 driver dir")
	}
}

func TestWinHelpers(t *testing.T) {
	if winDriverDir("windows", "11") != "w11" ||
		winDriverDir("windows-server", "2019") != "w2k19" {
		t.Fatal("driver dir")
	}
	if debianSuite("12.15.0") != "bookworm" || debianSuite("13.7.0") != "trixie" ||
		debianSuite("x") != "stable" {
		t.Fatal("debian suite")
	}
	if c, _ := winCulture("fr_FR.UTF-8"); c != "fr-FR" {
		t.Fatal("culture")
	}
	if c, _ := winCulture("xx"); c != "en-US" {
		t.Fatal("culture fallback")
	}
	if xmlEscape(`a<b"&`) != `a&lt;b&quot;&amp;` {
		t.Fatal("xmlEscape")
	}
}

func TestRenderPreseed(t *testing.T) {
	var p Profile
	p.System.OS, p.System.Release = "debian", "12.15.0"
	p.Identity.Hostname = "deb1"
	p.Identity.Username = "dev"
	p.Locale.Timezone = "Europe/Madrid"
	cfg := renderPreseed(&p, "$6$salt$hash", []string{"ssh-ed25519 AAAA"})
	for _, want := range []string{
		"deb.debian.org", "/debian", "bookworm",
		"debian-installer/exit/poweroff boolean true",
		"dev ALL=(ALL) NOPASSWD:ALL",
		"ssh-ed25519 AAAA",
		"openssh-server sudo curl qemu-guest-agent",
		"Europe/Madrid",
		"passwd/user-password-crypted password $6$salt$hash",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("preseed missing %q", want)
		}
	}
	// kali variant: rolling mirror keys
	p.System.OS = "kali"
	cfg = renderPreseed(&p, "$6$x", nil)
	if !strings.Contains(cfg, "http.kali.org") ||
		!strings.Contains(cfg, "kali-rolling") {
		t.Fatalf("kali mirror keys:\n%s", cfg)
	}
	// devuan: releases are suite names; the merged mirror carries them.
	p.System.OS, p.System.Release = "devuan", "daedalus"
	cfg = renderPreseed(&p, "$6$x", nil)
	if !strings.Contains(cfg, "deb.devuan.org") ||
		!strings.Contains(cfg, "/merged") ||
		!strings.Contains(cfg, "daedalus") {
		t.Fatalf("devuan mirror keys:\n%s", cfg)
	}
	// trisquel: numeric release → codename + trisquel mirror.
	p.System.OS, p.System.Release = "trisquel", "12.0"
	cfg = renderPreseed(&p, "$6$x", nil)
	if !strings.Contains(cfg, "archive.trisquel.info") ||
		!strings.Contains(cfg, "ecne") {
		t.Fatalf("trisquel mirror keys:\n%s", cfg)
	}
	// debian-edu: default answers for the edu metapackage question.
	p.System.OS, p.System.Release = "debian-edu", "latest"
	if cfg = renderPreseed(&p, "$6$x", nil); !strings.Contains(cfg,
		"debian-edu-install/profile") {
		t.Fatalf("debian-edu must preseed the profile question:\n%s", cfg)
	}
}

func TestRenderDesktopAutoinstall(t *testing.T) {
	var p Profile
	p.Identity.Hostname = "desk1"
	p.Identity.Username = "dev"
	y := renderDesktopAutoinstall(&p, "$6$salt$hash", "      - ssh-ed25519 K\n")
	for _, want := range []string{
		"autoinstall:",
		"interactive-sections: []",
		"hostname: desk1",
		"username: dev",
		"install-server: true",
		"ssh-ed25519 K",
		"name: direct",
	} {
		if !strings.Contains(y, want) {
			t.Fatalf("desktop autoinstall missing %q:\n%s", want, y)
		}
	}
	// no server-only keys the desktop schema would reject
	if strings.Contains(y, "late-commands") || strings.Contains(y, "shutdown:") {
		t.Fatal("desktop schema must not carry server-only keys")
	}
}

func TestRenderKickstart(t *testing.T) {
	var p Profile
	p.Identity.Hostname = "rocky1"
	p.Identity.Username = "dev"
	p.Locale.Timezone = "Europe/Madrid"
	ks := renderKickstart(&p, "$6$salt$hash",
		[]string{"ssh-ed25519 AAAA", "ssh-ed25519 BBBB"})
	for _, want := range []string{
		"poweroff",
		"user --name=dev --password=$6$salt$hash --iscrypted --groups=wheel",
		"sshkey --username=dev",
		"ssh-ed25519 AAAA",
		"ssh-ed25519 BBBB",
		"services --enabled=\"sshd qemu-guest-agent\"",
		"openssh-server",
		"Europe/Madrid --utc",
		"NOPASSWD:ALL",
		"cdrom",
	} {
		if !strings.Contains(ks, want) {
			t.Fatalf("kickstart missing %q:\n%s", want, ks)
		}
	}
	// Netinst editions carry no packages — the source command must be
	// omitted so anaconda uses the product's default mirror instead.
	p.System.Edition = "boot"
	if strings.Contains(renderKickstart(&p, "$6$salt$hash", nil), "cdrom") {
		t.Fatal("boot/netinst kickstart must not source packages from cdrom")
	}
}

func TestRenderAutoyast(t *testing.T) {
	var p Profile
	p.Identity.Hostname = "suse1"
	p.Identity.Username = "dev"
	p.Locale.Lang = "es_ES.UTF-8"
	p.Locale.Timezone = "Europe/Madrid"
	xml := renderAutoyast(&p, "$6$salt$hash",
		[]string{"ssh-ed25519 AAAA", "ssh-ed25519 BBBB"})
	for _, want := range []string{
		`<final_halt config:type="boolean">true</final_halt>`,
		"<username>dev</username>",
		"$6$salt$hash",
		"ssh-ed25519 AAAA",
		"ssh-ed25519 BBBB",
		"<service>sshd</service>",
		"<package>openssh</package>",
		"<language>es_ES</language>", // .UTF-8 stripped for YaST
		"Europe/Madrid",
		"NOPASSWD:ALL",
		"/dev/vda",
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("autoyast missing %q:\n%s", want, xml)
		}
	}
}

func TestRenderArchInstall(t *testing.T) {
	var p Profile
	p.Identity.Hostname = "arch1"
	p.Identity.Username = "dev"
	p.Locale.Timezone = "Europe/Madrid"
	sh := renderArchInstall(&p, "$6$salt$hash",
		[]string{"ssh-ed25519 AAAA", "ssh-ed25519 BBBB"})
	for _, want := range []string{
		"pacstrap -K /mnt base linux linux-firmware",
		"mkfs.ext4 -F /dev/vda1",
		"/sys/firmware/efi", // runtime firmware detection
		"grub-install --target=i386-pc",
		"useradd -m -G wheel",
		"usermod -p '$6$salt$hash' dev",
		"ssh-ed25519 AAAA",
		"ssh-ed25519 BBBB",
		"enable sshd dhcpcd",
		"Europe/Madrid",
		"NOPASSWD:ALL",
		"poweroff",
	} {
		if !strings.Contains(sh, want) {
			t.Fatalf("install.sh missing %q:\n%s", want, sh)
		}
	}
}

func TestRenderProxmoxFiles(t *testing.T) {
	var p Profile
	p.Identity.Hostname = "pve1"
	p.Identity.Username = "root"
	p.Locale.Keyboard = "es"
	p.Locale.Lang = "es_ES.UTF-8"
	p.Locale.Timezone = "Europe/Madrid"
	keys := []string{"ssh-ed25519 AAAA", "ssh-ed25519 BBBB"}
	ans := renderProxmoxAnswer(&p, "$6$salt$hash", keys, 8080)
	for _, want := range []string{
		`root-password-hashed = "$6$salt$hash"`,
		`ssh-ed25519 AAAA`,
		`ssh-ed25519 BBBB`,
		`reboot-mode = "poweroff"`,
		`keyboard = "es"`,
		`country = "es"`, // derived from locale
		`Europe/Madrid`,
		`source = "from-dhcp"`,
		`filesystem = "ext4"`,
		`disk-list = ["vda"]`,
		`http://10.0.2.2:8080/firstboot.sh`,
		`http://10.0.2.2:8080/done`,
	} {
		if !strings.Contains(ans, want) {
			t.Fatalf("answer.toml missing %q:\n%s", want, ans)
		}
	}
	fb := renderProxmoxFirstboot(&p, keys)
	for _, want := range []string{
		"/root/.ssh/authorized_keys",
		"ssh-ed25519 AAAA",
		"pve1",
	} {
		if !strings.Contains(fb, want) {
			t.Fatalf("firstboot.sh missing %q:\n%s", want, fb)
		}
	}
}

func TestRenderNixosFiles(t *testing.T) {
	var p Profile
	p.Identity.Hostname = "nix1"
	p.Identity.Username = "dev"
	p.Locale.Timezone = "Europe/Madrid"
	keys := []string{"ssh-ed25519 AAAA"}
	cfg := renderNixosConfig(&p, "$6$salt$hash", keys)
	for _, want := range []string{
		`networking.hostName = "nix1"`,
		`users.users.dev`,
		`"$6$salt$hash"`,
		`"ssh-ed25519 AAAA"`,
		"services.openssh.enable = true",
		"services.qemuGuest.enable = true",
		"wheelNeedsPassword = false",
		`"/dev/vda"`,
		"Europe/Madrid",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("configuration.nix missing %q:\n%s", want, cfg)
		}
	}
	sh := renderNixosInstall(&p, 4242)
	for _, want := range []string{
		"mkpart primary ext4",
		"nixos-generate-config --root /mnt",
		"http://10.0.2.2:4242/configuration.nix",
		"nixos-install",
		"poweroff",
	} {
		if !strings.Contains(sh, want) {
			t.Fatalf("install.sh missing %q:\n%s", want, sh)
		}
	}
}

func TestRenderOpenBSDConf(t *testing.T) {
	var p Profile
	p.Identity.Hostname = "obsd-dev"
	p.Identity.Username = "dev"
	p.Locale.Timezone = "Europe/Madrid"
	got := renderOpenBSDConf(&p, []string{"ssh-ed25519 AAAA run"})
	for _, want := range []string{
		"System hostname = obsd-dev",
		"Setup a user = dev",
		"Public ssh key for user dev = ssh-ed25519 AAAA run",
		"What timezone = Europe/Madrid",
		"Use (W)hole disk or (E)dit the MBR = whole",
		"Location of sets = cd0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install.conf missing %q:\n%s", want, got)
		}
	}
}

func TestRenderIgnitions(t *testing.T) {
	var p Profile
	p.Identity.Hostname = "fcos-dev"
	p.Identity.Username = "dev"
	p.Locale.Timezone = "Europe/Madrid"
	for name, got := range map[string]string{
		"flatcar": renderFlatcarIgnition(&p, "$6$h", []string{"ssh-ed25519 AAAA"}),
		"coreos":  renderCoreOSIgnition(&p, "$6$h", []string{"ssh-ed25519 AAAA"}),
	} {
		var v map[string]any
		if err := json.Unmarshal([]byte(got), &v); err != nil {
			t.Fatalf("%s ignition is not valid JSON: %v\n%s", name, err, got)
		}
		for _, want := range []string{
			`"name":"dev"`, `ssh-ed25519 AAAA`,
			`data:,fcos-dev`, "qemu-guest-agent",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s ignition missing %q", name, want)
			}
		}
	}
}

func TestRenderSerialInstallScripts(t *testing.T) {
	var p Profile
	p.Identity.Hostname = "vm-dev"
	p.Identity.Username = "dev"
	p.Locale.Timezone = "Europe/Madrid"
	p.Locale.Lang = "es_ES.UTF-8"
	keys := []string{"ssh-ed25519 AAAA"}
	hash := "$6$rounds/h"
	for name, got := range map[string]string{
		"freebsd": renderFreeBSDInstall(&p, hash, keys),
		"void":    renderVoidInstall(&p, hash, keys, false),
		"gentoo":  renderGentooInstall(&p, hash, keys),
		"flatcar": renderFlatcarInstall(8443),
	} {
		for _, want := range []string{"#!/bin/sh", "dev"} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s script missing %q", name, want)
			}
		}
	}
	if !strings.Contains(renderVoidInstall(&p, hash, keys, true), "/musl") {
		t.Fatal("musl repo path missing")
	}
	if !strings.Contains(renderFlatcarInstall(1234), "10.0.2.2:1234/ignition.json") {
		t.Fatal("flatcar script missing ignition URL")
	}
}

func TestSeedServerDoneEndpoint(t *testing.T) {
	dir := t.TempDir()
	port, srv, err := startSeedHTTPServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	// GET on /done is rejected — only the webhook POST may mark.
	if resp, err := http.Get(fmt.Sprintf(
		"http://127.0.0.1:%d/done", port)); err != nil || resp.StatusCode == 204 {
		t.Fatalf("GET /done must not mark: %v %v", resp.StatusCode, err)
	} else {
		resp.Body.Close()
	}
	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/done", port),
		"application/json", strings.NewReader(`{"status":"ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /done rc=%d", resp.StatusCode)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".install-done"))
	if err != nil || !strings.Contains(string(b), "ok") {
		t.Fatalf("marker file: %v %s", err, b)
	}
}

func TestSerialTapWrite(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("unix sockets unsupported")
	}
	defer l.Close()
	got := make(chan string, 1)
	go func() {
		c, _ := l.Accept()
		b := make([]byte, 64)
		n, _ := c.Read(b)
		got <- string(b[:n])
	}()
	tap := &serialTap{}
	if err := tap.write("x"); err == nil {
		t.Fatal("write with no conn must fail")
	}
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	tap.set(c)
	if err := tap.write("hello\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-got:
		if s != "hello\n" {
			t.Fatalf("got %q", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("write never received")
	}
	tap.set(nil)
	if err := tap.write("x"); err == nil {
		t.Fatal("write after disconnect must fail")
	}
}

// --- preseed prepare: ISO layout + http server ---------------------------

// TestExtractBootfilesDebianLayout builds a fake netinst ISO (the
// d-i kernel lives under install.amd/) and extracts from it — the
// path the preseed provider's prepare takes.
func TestExtractBootfilesDebianLayout(t *testing.T) {
	dir := t.TempDir()
	kdir := filepath.Join(dir, "src", "install.amd")
	os.MkdirAll(kdir, 0o755)
	for _, n := range []string{"vmlinuz", "initrd.gz"} {
		os.WriteFile(filepath.Join(kdir, n), []byte(n+"-data"), 0o644)
	}
	iso := filepath.Join(dir, "netinst.iso")
	if err := writeFilesISO(iso, "d-i", map[string]string{
		"/install.amd/vmlinuz":   filepath.Join(kdir, "vmlinuz"),
		"/install.amd/initrd.gz": filepath.Join(kdir, "initrd.gz"),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := extractBootfiles(iso, "install.amd", dir,
		[]string{"vmlinuz", "initrd.gz"})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"vmlinuz", "initrd.gz"} {
		b, err := os.ReadFile(got[n])
		if err != nil || string(b) != n+"-data" {
			t.Fatalf("%s: %v", n, err)
		}
	}
}

// TestSeedHTTPServer: the loopback file server a d-i guest reaches
// via slirp's 10.0.2.2 gateway.
func TestSeedHTTPServer(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "preseed.cfg"), []byte("d-i x"), 0o644)
	port, srv, err := startSeedHTTPServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if port <= 0 {
		t.Fatal("port")
	}
	// Serve check via the HTTP client against loopback.
	r, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/preseed.cfg")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil || string(b) != "d-i x" {
		t.Fatalf("fetch: %q %v", b, err)
	}
}

// --- serial-marker completion -------------------------------------------

func TestAwaitSerialMarker(t *testing.T) {
	r, dir := testRunCtx(t)
	r.artDir = dir
	os.WriteFile(filepath.Join(dir, "serial.log"),
		[]byte("boot noise\ncurtin: Installation finished.\n"), 0o644)
	if r.e == nil {
		r.e = newEmitter("t", "vm", dir, dir, true)
	}
	// Marker present → immediate match; no pidfile → waitPIDGone is nil.
	if err := r.awaitSerialMarker(context.Background(),
		`Installation (finished|complete)`, 30*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestAwaitSerialMarkerTimeout(t *testing.T) {
	r, dir := testRunCtx(t)
	r.artDir = dir
	if r.e == nil {
		r.e = newEmitter("t", "vm", dir, dir, true)
	}
	os.WriteFile(filepath.Join(dir, "serial.log"), []byte("booting\n"), 0o644)
	err := r.awaitSerialMarker(context.Background(),
		"never-matches", 1500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "not seen") {
		t.Fatalf("expected marker timeout, got %v", err)
	}
}
