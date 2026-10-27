// macos.go: get_macos parity — Apple osrecovery session + recovery
// image download, chunklist verify, dmg→raw convert, OpenCore/OVMF
// fetch, then the conf.
package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Board-ID / MLB / OS_TYPE per macOS release (get_macos case table).
var macBoards = map[string][3]string{
	"lion":         {"Mac-2E6FAB96566FE58C", "00000000000F25Y00", "default"},
	"10.7":         {"Mac-2E6FAB96566FE58C", "00000000000F25Y00", "default"},
	"mountainlion": {"Mac-7DF2A3B5E5D671ED", "00000000000F65100", "default"},
	"10.8":         {"Mac-7DF2A3B5E5D671ED", "00000000000F65100", "default"},
	"mavericks":    {"Mac-F60DEB81FF30ACF6", "00000000000FNN100", "default"},
	"10.9":         {"Mac-F60DEB81FF30ACF6", "00000000000FNN100", "default"},
	"yosemite":     {"Mac-E43C1C25D4880AD6", "00000000000GDVW00", "default"},
	"10.10":        {"Mac-E43C1C25D4880AD6", "00000000000GDVW00", "default"},
	"elcapitan":    {"Mac-FFE5EF870D7BA81A", "00000000000GQRX00", "default"},
	"10.11":        {"Mac-FFE5EF870D7BA81A", "00000000000GQRX00", "default"},
	"sierra":       {"Mac-77F17D7DA9285301", "00000000000J0DX00", "default"},
	"10.12":        {"Mac-77F17D7DA9285301", "00000000000J0DX00", "default"},
	"high-sierra":  {"Mac-BE088AF8C5EB4FA2", "00000000000J80300", "default"},
	"10.13":        {"Mac-BE088AF8C5EB4FA2", "00000000000J80300", "default"},
	"mojave":       {"Mac-7BA5B2DFE22DDD8C", "00000000000KXPG00", "default"},
	"10.14":        {"Mac-7BA5B2DFE22DDD8C", "00000000000KXPG00", "default"},
	"catalina":     {"Mac-00BE6ED71E35EB86", "00000000000000000", "default"},
	"10.15":        {"Mac-00BE6ED71E35EB86", "00000000000000000", "default"},
	"big-sur":      {"Mac-2BD1B31983FE1663", "00000000000000000", "default"},
	"11":           {"Mac-2BD1B31983FE1663", "00000000000000000", "default"},
	"monterey":     {"Mac-B809C3757DA9BB8D", "00000000000000000", "latest"},
	"12":           {"Mac-B809C3757DA9BB8D", "00000000000000000", "latest"},
	"ventura":      {"Mac-4B682C642B45593E", "00000000000000000", "latest"},
	"13":           {"Mac-4B682C642B45593E", "00000000000000000", "latest"},
	"sonoma":       {"Mac-827FAC58A8FDFA22", "00000000000000000", "default"},
	"14":           {"Mac-827FAC58A8FDFA22", "00000000000000000", "default"},
	"sequoia":      {"Mac-7BA5B2D9E42DDD94", "00000000000000000", "default"},
	"15":           {"Mac-7BA5B2D9E42DDD94", "00000000000000000", "default"},
	"tahoe":        {"Mac-CFF7D910A743CAAF", "00000000000000000", "latest"},
	"26":           {"Mac-CFF7D910A743CAAF", "00000000000000000", "latest"},
}

// generate_id: random uppercase-hex string of n chars.
func generateID(n int) string {
	var b [64]byte
	rand.Read(b[:])
	var s strings.Builder
	for i := 0; i < n; i++ {
		s.WriteString("0123456789ABCDEF"[b[i]&0x0f : b[i]&0x0f+1])
	}
	return s.String()
}

// appleRecoverySession: replicate the curl -v cookie scrape — GET the
// osrecovery root and read the `session` cookie value.
func appleRecoverySession() (string, error) {
	req, _ := http.NewRequest("GET", "https://osrecovery.apple.com/", nil)
	req.Header.Set("Host", "osrecovery.apple.com")
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "InternetRecovery/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			return c.Value, nil
		}
	}
	// Fallback: scan raw Set-Cookie headers for session=...
	for _, h := range resp.Header["Set-Cookie"] {
		if strings.HasPrefix(h, "session=") {
			v := strings.TrimPrefix(h, "session=")
			if i := strings.IndexAny(v, "; "); i >= 0 {
				v = v[:i]
			}
			return v, nil
		}
	}
	return "", fmt.Errorf("no apple session cookie")
}

// appleRecoveryInfo: POST the InstallationPayload request and split
// the space-separated response into fields.
func appleRecoveryInfo(session, mlb, boardID, osType string) ([]string, error) {
	body := "cid=" + generateID(16) + "\nsn=" + mlb + "\nbid=" + boardID +
		"\nk=" + generateID(64) + "\nfg=" + generateID(64) + "\nos=" + osType
	req, _ := http.NewRequest("POST",
		"https://osrecovery.apple.com/InstallationPayload/RecoveryImage",
		strings.NewReader(body))
	req.Header.Set("Host", "osrecovery.apple.com")
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "InternetRecovery/1.0")
	req.Header.Set("Cookie", "session=\""+session+"\"")
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(b)), nil
}

// macFetch: web_get with the oscdn headers + AssetToken cookie.
func macFetch(url, dest, token string) int {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	req.Header.Set("Host", "oscdn.apple.com")
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "InternetRecovery/1.0")
	req.Header.Set("Cookie", "AssetToken="+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "download failed: HTTP %d\n", resp.StatusCode)
		return 1
	}
	f, err := os.Create(dest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer f.Close()
	io.Copy(f, resp.Body)
	return 0
}

// getMacos: the full get_macos pipeline.
func getMacos(release, dir, arch string) int {
	row, ok := macBoards[release]
	if !ok {
		fmt.Fprintf(os.Stderr, "ERROR! Unknown release: %s\n", release)
		return 1
	}
	boardID, mlb, osType := row[0], row[1], row[2]
	if dir == "" {
		dir = "macos-" + release
	}
	os.MkdirAll(dir, 0755)
	session, err := appleRecoverySession()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! Could not get Apple recovery session:", err)
		return 1
	}
	fields, err := appleRecoveryInfo(session, mlb, boardID, osType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! RecoveryImage request failed:", err)
		return 1
	}
	var dmgLink, dmgSession, clLink, clSession string
	for _, f := range fields {
		switch {
		case strings.Contains(f, "oscdn") && strings.Contains(f, "dmg"):
			dmgLink = f
		case strings.Contains(f, "expires") && strings.Contains(f, "dmg"):
			dmgSession = f
		case strings.Contains(f, "oscdn") && strings.Contains(f, "chunklist"):
			clLink = f
		case strings.Contains(f, "expires") && strings.Contains(f, "chunklist"):
			clSession = f
		}
	}
	if dmgLink == "" || clLink == "" {
		fmt.Fprintln(os.Stderr, "ERROR! Apple did not return a recovery image URL.")
		return 1
	}
	dmg := filepath.Join(dir, "RecoveryImage.dmg")
	cl := filepath.Join(dir, "RecoveryImage.chunklist")
	img := filepath.Join(dir, "RecoveryImage.img")
	if _, err := os.Stat(img); err != nil {
		fmt.Printf("Downloading macOS (%s) RecoveryImage\n", release)
		fmt.Println(" - URL: " + dmgLink)
		if macFetch(dmgLink, dmg, dmgSession) != 0 {
			return 1
		}
		if macFetch(clLink, cl, clSession) != 0 {
			return 1
		}
		// chunkcheck verification via the bundled python helper.
		if cc, err := exec.LookPath("chunkcheck"); err == nil {
			if exec.Command(cc, dir).Run() == nil {
				fmt.Println(" - Verification passed")
			} else {
				fmt.Println(" - WARNING! Verification failed, continuing anyway")
			}
		}
		if qimg, err := exec.LookPath("qemu-img"); err == nil {
			fmt.Println(" - Converting RecoveryImage.dmg")
			exec.Command(qimg, "convert", dmg, "-O", "raw", img).Run()
		}
		os.Remove(dmg)
		os.Remove(cl)
		if _, err := os.Stat(img); err == nil {
			fmt.Println(" - RecoveryImage.img is ready.")
		}
	}
	fmt.Println("Downloading OpenCore & UEFI firmware")
	// Pin to last known-good commit; OVMF_CODE.fd was removed from
	// master on 26 Jan 2026 (same pin as the Bash get_macos).
	const commit = "da4b23b5e92c5b939568700034367e8b7649fe90"
	base := "https://github.com/kholia/OSX-KVM/raw/" + commit + "/"
	fetch(base+"OpenCore/OpenCore.qcow2", filepath.Join(dir, "OpenCore.qcow2"), "")
	fetch(base+"OVMF_CODE.fd", filepath.Join(dir, "OVMF_CODE.fd"), "")
	if _, err := os.Stat(filepath.Join(dir, "OVMF_VARS-1920x1080.fd")); err != nil {
		fetch(base+"OVMF_VARS-1920x1080.fd", filepath.Join(dir, "OVMF_VARS-1920x1080.fd"), "")
	}
	if noConf {
		return 0
	}
	return mkconf("macos", release, "", "RecoveryImage.img", "", dir, arch, cloudInitFlag)
}
