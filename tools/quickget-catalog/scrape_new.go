// scrape_new.go — additional scrape resolvers (second catalog-expansion batch).
// Shared helpers keep each case a few lines; every resolver mirrors the
// upstream mirror layout it scrapes.
package main

import (
	"encoding/json"
	"fmt"
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
// sha256sum.txt) whose download URL is returned as the checksum field.
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
// matching re (e.g. release folders on a mirror).
func latestDir(base, re string) (string, error) {
	body, err := fetchString(base)
	if err != nil {
		return "", err
	}
	d := latestMatch(body, re)
	if d == "" {
		return "", fmt.Errorf("no dir matching %s in %s", re, base)
	}
	return strings.TrimRight(base, "/") + "/" + strings.Trim(d, "/"), nil
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
	return strings.TrimRight(base, "/") + "/" + best + "/", nil
}

// indexISOsha is indexISO plus the "<file>.sha256" sidecar URL as
// checksum field (Bandshed-style mirror layout).
func indexISOsha(base, re string) (string, string, error) {
	u, _, err := indexISO(base, re)
	if err != nil {
		return "", "", err
	}
	return u, u + ".sha256", nil
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
		// 5.0 names carry an arch infix; 4.0's GA iso has none.
		re := `photon-[0-9][^"]*x86_64\.iso`
		if release == "4.0" {
			re = `photon-[0-9][0-9.]*-[a-z0-9]+\.iso`
		}
		return indexISO(fmt.Sprintf("https://packages.broadcom.com/photon/%s/GA/iso/", release), re)
	case "taloslinux":
		return ghISO("siderolabs/talos", `metal-amd64\.iso`)
	case "toaruos":
		return ghISO("klange/toaruos", `\.iso$`)
	case "zimaos":
		return ghISO("IceWhaleTech/ZimaOS", `zimaos.*\.(img|iso)`)

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
		return sfISO("loc-os", "", `Loc-OS-[^"]*\.iso`)
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
		return indexISO("https://vault.centos.org/"+release+"/isos/x86_64/", `CentOS-[^"]*x86_64-([Bb]in-[Dd][Vv][Dd]1|[Dd][Vv][Dd]1|[Dd][Vv][Dd]-[0-9]+|[Bb]in-1of[0-9]+)\.iso`)
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
		return indexISO("https://enterprise.proxmox.com/iso/", `proxmox-backup-server_[^"]*\.iso`)
	case "proxmox-mail-gateway":
		return indexISO("https://enterprise.proxmox.com/iso/", `proxmox-mailgateway_[^"]*\.iso`)
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
		// mirrors.xcp-ng.org: isos/<major.minor>/xcp-ng-<ver>-<date>.iso + SHA256SUMS.
		base := "https://mirrors.xcp-ng.org/isos/" + release
		u, _, err := indexISO(base, `xcp-ng-[0-9.]+-[0-9]+\.iso`)
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
