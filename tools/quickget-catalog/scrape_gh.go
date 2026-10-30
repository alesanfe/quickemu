// scrape_gh.go — GitHub releases API resolvers (scrape batch).
package main

import (
	"fmt"
	"regexp"
	"strings"
)

func scrapeGH(osName, release, edition, arch string) (string, string, error) {
	switch osName {
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
		// hello-<ver>_<build>-FreeBSD-<ver>-amd64.iso + .iso.sha256 sidecar.
		return ghISOsha("helloSystem/ISO",
			`hello-[^"/]*-amd64\.iso$`, `hello-[^"/]*-amd64\.iso\.sha256$`)
	case "k3os":
		return ghISO("rancher/k3os", `k3os-amd64\.iso`)
	case "kairos":
		return ghISO("kairos-io/kairos", `kairos-.*amd64.*\.iso`)
	case "taloslinux":
		return ghISO("siderolabs/talos", `metal-amd64\.iso`)
	case "toaruos":
		return ghISO("klange/toaruos", `\.iso$`)
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
	case "ventoy":
		return ghISOsha("ventoy/Ventoy", `ventoy-[0-9.]+-livecd\.iso`, `sha256\.txt`)
	case "minikube":
		return ghISOsha("kubernetes/minikube", `minikube-v[0-9.]+-amd64\.iso`, `minikube-v[0-9.]+-amd64\.iso\.sha256`)
	case "abora":
		// abora-<ed>-<date>-x86_64-v<ver>.iso + SHA256SUMS-v<ver>.txt manifest.
		ed := edition
		if ed == "" {
			ed = "gnome"
		}
		ver := `[0-9.]+`
		if release != "latest" && release != "" {
			ver = regexp.QuoteMeta(strings.TrimPrefix(release, "v"))
		}
		return ghISOsums("AnimatedGTVR/Abora-OS",
			`abora-`+ed+`-[0-9.]+-x86_64-v`+ver+`\.iso`, `SHA256SUMS-[^/"]*\.txt`)
	case "asterinas":
		// asterinas-nixos-<ver>-x86_64.iso (Rust framekernel; Linux ABI).
		return ghISO("asterinas/asterinas", `asterinas-nixos-[0-9.]+-x86_64\.iso`)
	case "vinix":
		// vlang/vinix: vinix-{amd64,arm64}.iso + SHA256SUMS manifest.
		a := "amd64"
		if arch == "arm64" {
			a = "arm64"
		}
		return ghISOsums("vlang/vinix", `vinix-`+a+`\.iso`, `/SHA256SUMS$`)
	case "cervus":
		// CervusOS-v<ver>.iso — self-hosted x86_64 OS written from scratch.
		return ghISO("VeoQeoOrg/Cervus", `CervusOS-[^"/]*\.iso`)
	case "rou2exos":
		// Hobby protected-mode OS; r2.iso is the bootable asset.
		return ghISO("krustowski/rou2exOS", `/r2\.iso$`)
	case "lateralus":
		// lateralus-os.iso research OS (only the v3.2.0 release ships it).
		return ghISO("bad-antics/lateralus-lang", `lateralus-os\.iso`)
	case "moros":
		// moros-v<ver>.bin raw disk image (rust hobby OS).
		ver := `[0-9.]+`
		if release != "latest" && release != "" {
			ver = regexp.QuoteMeta(strings.TrimPrefix(release, "v"))
		}
		return ghISO("vinc/moros", `moros-v`+ver+`\.bin`)
	case "harvey":
		// harvey-cdrom.iso.bz2 under the "latest" tag (Plan 9 fork).
		return ghISO("Harvey-OS/harvey", `harvey-cdrom\.iso\.bz2`)
	}
	return "", "", errScrapeSkip
}
