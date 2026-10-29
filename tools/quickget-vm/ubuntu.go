// ubuntu.go: get_ubuntu + get_ubuntu-server + zsync_get parity —
// SHA256SUMS/MD5SUMS resolution, daily-live codename discovery and
// zsync-delta downloads with curl fallback.
package main

import (
	"fmt"
	"github.com/alesanfe/quickemu/tools/qelib"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var fetchString = func(url string) (string, error) { return qelib.FetchString(url, 16<<20) }

// ubuntuDailyCodename: first launchpad series name (newest-first)
// whose daily-live SHA256SUMS exists on cdimage.
var ubuntuDailyCodename = func(osName string) string {
	name, _ := qelib.UbuntuDailyCodename(osName)
	return name
}

// sumsPick mirrors the grep/cut pipeline: filter lines matching any of
// namePats + arch + "iso" (excluding +mac), take the last, split into
// hash+filename. md5Field selects the filename column (MD5SUMS uses
// `hash *file` on f2 for desktop, f3 for server).
func sumsPick(body string, namePats []string, arch string, md5Field int) (iso, hash string) {
	var line string
	for _, l := range strings.Split(body, "\n") {
		if !strings.Contains(l, "iso") || strings.Contains(l, "+mac") {
			continue
		}
		ok := len(namePats) == 0
		for _, p := range namePats {
			if strings.Contains(l, p) {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		if arch != "" && !strings.Contains(l, arch) {
			continue
		}
		line = l
	}
	if line == "" {
		return "", ""
	}
	f := strings.Fields(line)
	if len(f) < 2 {
		return "", ""
	}
	file := strings.TrimPrefix(f[len(f)-1], "*")
	if md5Field == 3 && len(f) >= 3 {
		file = strings.TrimPrefix(f[2], "*")
	}
	return file, f[0]
}

// zsyncURL maps a release URL to its .zsync sidecar on forced-http —
// the scheme is downgraded only; a path segment containing "https"
// must not be rewritten.
func zsyncURL(url string) string {
	u := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	return "http://" + u + ".zsync"
}

// zsyncGet: zsync_get parity — prefer the zsync delta downloader on
// the forced-http .zsync URL, fall back to plain fetch when absent.
func zsyncGet(url, dir, out string) int {
	if out == "" {
		out = filepath.Base(url)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("ERROR! Unable to create directory %s\n", dir)
		return 1
	}
	if z, err := exec.LookPath("zsync"); err == nil {
		fmt.Printf("Downloading %s from %s\n", out, url)
		zu := zsyncURL(url)
		cmd := execCommand(z, zu, "-i", filepath.Join(dir, out), "-o", filepath.Join(dir, out))
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if cmd.Run() != nil {
			fmt.Printf("ERROR! Failed to download %s\n", zu)
			return 1
		}
		os.Remove(filepath.Join(dir, out) + ".zs-old")
		return 0
	}
	fmt.Println("INFO: zsync not found, falling back to direct download")
	return fetch(url, filepath.Join(dir, out), "")
}

// getUbuntu covers get_ubuntu + get_ubuntu-server: resolution of the
// release URL (daily-live, arm64 cdimage, releases mirror), SUMS
// parsing, zsync for daily/dvd, then the conf.
func getUbuntu(osName, release, dir, arch string) int {
	if arch == "" {
		arch = "amd64"
	}
	isServer := osName == "ubuntu-server"
	isDaily := strings.HasPrefix(release, "daily") || release == "dvd"
	var url string
	switch {
	case isDaily:
		codename := ubuntuDailyCodename(osName)
		if codename == "" {
			fmt.Printf("%s %s is currently unavailable. Please select other OS/Release combination\n", osName, release)
			return 1
		}
		url = "https://cdimage.ubuntu.com/" + osName + "/" + codename + "/daily-live/current"
		// keep the caller's dir — the dir=="" fallback below already
		// derives osName-release when none was given.
	case osName == "ubuntu" && arch == "arm64":
		url = "https://cdimage.ubuntu.com/releases/" + release + "/release"
	case osName == "ubuntu":
		url = "https://releases.ubuntu.com/" + release
	default:
		url = "https://cdimage.ubuntu.com/" + osName + "/releases/" + release + "/release"
	}
	if dir == "" {
		dir = osName + "-" + release
	}
	if isServer {
		if arch == "arm64" {
			url = "https://cdimage.ubuntu.com/releases/" + release + "/release"
		} else if !isDaily {
			url = "https://releases.ubuntu.com/" + release
		}
	}
	// SUMS parsing — desktop picks desktop|dvd|install lines, server
	// picks live-server (or server for 14*/16*).
	namePats := []string{"desktop", "dvd", "install"}
	md5Field := 2
	if isServer {
		namePats = []string{"live-server"}
		md5Field = 3
		if strings.HasPrefix(release, "14") || strings.HasPrefix(release, "16") {
			namePats = []string{"server"}
		}
	}
	var iso, hash string
	if body, err := fetchString(url + "/SHA256SUMS"); err == nil {
		iso, hash = sumsPick(body, namePats, arch, 0)
	} else if body, err := fetchString(url + "/MD5SUMS"); err == nil {
		iso, hash = sumsPick(body, namePats, arch, md5Field)
	}
	if iso == "" || hash == "" {
		fmt.Printf("%s %s is currently unavailable. Please select other OS/Release combination\n", osName, release)
		return 1
	}
	// The ISO name comes from a remote sums file — it must be a plain
	// filename, never a path: "../x" would write outside the VM dir
	// and "../../x" would also corrupt the download URL.
	if !filepath.IsLocal(iso) || strings.ContainsAny(iso, `/\`) {
		fmt.Printf("ERROR! Hostile ISO name in sums file: %q\n", iso)
		return 1
	}
	if isDaily {
		if zsyncGet(url+"/"+iso, dir, osName+"-devel.iso") != 0 {
			return 1
		}
		return mkconf(osName, release, "", osName+"-devel.iso", "", dir, arch, cloudInitFlag)
	}
	if fetch(url+"/"+iso, filepath.Join(dir, iso), hash) != 0 {
		return 1
	}
	return mkconf(osName, release, "", iso, "", dir, arch, cloudInitFlag)
}
