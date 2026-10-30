// macos.go — shared Apple osrecovery surface: the board table and the
// session handshake (session cookie scrape + InstallationPayload POST).
// Both quickget-catalog (url/check side) and quickget-vm (download
// pipeline) consume it; the *http.Client is injected so each caller
// keeps its own seam (ProbeClient / httpProbe).
package qelib

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MacBoards: Board-ID / MLB / OS_TYPE per macOS release (get_macos
// case table contract).
var MacBoards = map[string][3]string{
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

// MacGenID: random uppercase-hex string of n chars (generate_id).
func MacGenID(n int) string {
	var b [64]byte
	rand.Read(b[:])
	var s strings.Builder
	for i := 0; i < n; i++ {
		s.WriteString("0123456789ABCDEF"[b[i]&0x0f : b[i]&0x0f+1])
	}
	return s.String()
}

// AppleSession replicates the curl -v cookie scrape: GET the
// osrecovery root and read the `session` cookie value.
func AppleSession(cli *http.Client) (string, error) {
	req, _ := http.NewRequest("GET", "https://osrecovery.apple.com/", nil)
	req.Header.Set("Host", "osrecovery.apple.com")
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "InternetRecovery/1.0")
	resp, err := cli.Do(req)
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

// AppleInfo POSTs the InstallationPayload request and splits the
// space-separated response into fields.
func AppleInfo(cli *http.Client, session, mlb, boardID, osType string) ([]string, error) {
	body := "cid=" + MacGenID(16) + "\nsn=" + mlb + "\nbid=" + boardID +
		"\nk=" + MacGenID(64) + "\nfg=" + MacGenID(64) + "\nos=" + osType
	req, _ := http.NewRequest("POST",
		"https://osrecovery.apple.com/InstallationPayload/RecoveryImage",
		strings.NewReader(body))
	req.Header.Set("Host", "osrecovery.apple.com")
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "InternetRecovery/1.0")
	req.Header.Set("Cookie", "session=\""+session+"\"")
	req.Header.Set("Content-Type", "text/plain")
	resp, err := cli.Do(req)
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
