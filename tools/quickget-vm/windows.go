// windows.go: download_windows_workstation + download_windows_server
// parity (Mido-derived Microsoft API flow), language tables and the
// get_windows orchestration.
package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var winLanguages = []string{
	"Arabic", "Brazilian Portuguese", "Bulgarian", "Chinese (Simplified)",
	"Chinese (Traditional)", "Croatian", "Czech", "Danish", "Dutch",
	"English (United States)", "English International", "Estonian",
	"Finnish", "French", "French Canadian", "German", "Greek", "Hebrew",
	"Hungarian", "Italian", "Japanese", "Korean", "Latvian", "Lithuanian",
	"Norwegian", "Polish", "Portuguese", "Romanian", "Russian",
	"Serbian Latin", "Slovak", "Slovenian", "Spanish", "Spanish (Mexico)",
	"Swedish", "Thai", "Turkish", "Ukrainian",
}

var winServerLanguages = []string{
	"English (United States)", "Chinese (Simplified)", "French", "German",
	"Italian", "Japanese", "Russian", "Spanish",
}

// Server eval-center culture/country map.
var winServerCultures = map[string][2]string{
	"English (Great Britain)": {"en-gb", "GB"},
	"Chinese (Simplified)":    {"zh-cn", "CN"},
	"Chinese (Traditional)":   {"zh-tw", "TW"},
	"French":                  {"fr-fr", "FR"},
	"German":                  {"de-de", "DE"},
	"Italian":                 {"it-it", "IT"},
	"Japanese":                {"ja-jp", "JP"},
	"Korean":                  {"ko-kr", "KR"},
	"Portuguese (Brazil)":     {"pt-br", "BR"},
	"Spanish":                 {"es-es", "ES"},
	"Russian":                 {"ru-ru", "RU"},
}

func validLang(list []string, l string) bool {
	for _, v := range list {
		if v == l {
			return true
		}
	}
	return false
}

const winUA = "Mozilla/5.0 (X11; Linux x86_64; rv:100.0) Gecko/20100101 Firefox/100.0"

// msGet issues a GET with the Firefox UA and empty Accept header like
// the Bash curl calls, returning the body (bounded).
func msGet(url string, max int64, referer string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", winUA)
	req.Header.Set("Accept", "")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := httpProbe.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	// max<=0 means no cap (the GetProductDownloadLinks call relies on
	// it) — LimitReader(0) would silently return an empty body.
	var r io.Reader = resp.Body
	if max > 0 {
		r = io.LimitReader(r, max)
	}
	b, err := io.ReadAll(r)
	return string(b), err
}

// uuid mirrors `uuidgen`/`/proc/sys/kernel/random/uuid`.
func uuid() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var (
	reProductOpt = regexp.MustCompile(`<option value="([0-9]+)">Windows`)
	reMdtW       = regexp.MustCompile(`[?&]w=([A-F0-9]+)`)
	reMdtRticks  = regexp.MustCompile(`rticks="?\+?([0-9]+)`)
	reFwLink     = regexp.MustCompile(`https://go\.microsoft\.com/fwlink/p/\?LinkID=[0-9]+&clcid=0x[0-9a-z]+&culture=[^&"']+&country=[^&"']+`)
)

// downloadWindowsWorkstation: consumer Windows 10/11 ISO via the
// gated Microsoft download API. Returns the saved file name.
func downloadWindowsWorkstation(release, i18n, vmPath string) (string, error) {
	fmt.Printf("Downloading Windows %s (%s)\n", release, i18n)
	url := "https://www.microsoft.com/en-us/software-download/windows" + release
	if release == "10" {
		url += "ISO"
	}
	sessionID := uuid()
	fmt.Println(" - Parsing download page: " + url)
	page, err := msGet(url, 1<<20, "")
	if err != nil {
		return "", err
	}
	fmt.Print(" - Getting Product edition ID: ")
	m := reProductOpt.FindStringSubmatch(page)
	if m == nil {
		return "", fmt.Errorf("no product edition found")
	}
	productEditionID := m[1]
	if len(productEditionID) > 16 {
		productEditionID = productEditionID[:16]
	}
	fmt.Println(productEditionID)
	fmt.Println(" - Permit Session ID: " + sessionID)
	if _, err := msGet("https://vlscppe.microsoft.com/tags?org_id=y6jn8c31&session_id="+sessionID, 100<<10, ""); err != nil {
		return "", err
	}
	fmt.Println(" - Request Microsoft ov-df mdt.js")
	instanceID := "560dc9f3-1aa5-4a2f-b63c-9e18f8d0e175"
	mdt, err := msGet("https://ov-df.microsoft.com/mdt.js?instanceId={"+instanceID+"}&PageId=si&session_id="+sessionID, 100<<10, "")
	if err != nil {
		return "", err
	}
	w := ""
	if m := reMdtW.FindStringSubmatch(mdt); m != nil {
		w = m[1]
	}
	rticks := ""
	if m := reMdtRticks.FindStringSubmatch(mdt); m != nil {
		rticks = m[1]
	}
	mdtTS := fmt.Sprintf("%d", time.Now().UnixMilli())
	if _, err := msGet("https://ov-df.microsoft.com/?session_id={"+sessionID+"}&CustomerId={"+instanceID+"}&PageId=si&w={"+w+"}&mdt={"+mdtTS+"}&rticks={"+rticks+"}", 100<<10, ""); err != nil {
		return "", err
	}
	profile := "606624d44113"
	fmt.Print(" - Getting language SKU ID: ")
	skuJSON, err := msGet("https://www.microsoft.com/software-download-connector/api/getskuinformationbyproductedition?profile="+profile+"&ProductEditionId="+productEditionID+"&SKU=undefined&friendlyFileName=undefined&Locale=en-US&sessionID="+sessionID, 100<<10, "")
	if err != nil {
		return "", err
	}
	var skus struct {
		Skus []struct {
			ID                json.Number `json:"Id"`
			LocalizedLanguage string      `json:"LocalizedLanguage"`
			Language          string      `json:"Language"`
		} `json:"Skus"`
	}
	if err := json.Unmarshal([]byte(skuJSON), &skus); err != nil {
		return "", err
	}
	skuID := ""
	for _, s := range skus.Skus {
		if s.LocalizedLanguage == i18n || s.Language == i18n {
			skuID = s.ID.String()
			break
		}
	}
	fmt.Println(skuID)
	if skuID == "" {
		return "", fmt.Errorf("no SKU for language %q", i18n)
	}
	fmt.Println(" - Getting ISO download link...")
	linkJSON, err := msGet("https://www.microsoft.com/software-download-connector/api/GetProductDownloadLinksBySku?profile="+profile+"&productEditionId=undefined&SKU="+skuID+"&friendlyFileName=undefined&Locale=en-US&sessionID="+sessionID, 0, url)
	failed := false
	if err != nil || linkJSON == "" {
		fmt.Println(" - Microsoft servers gave us an empty response to our request for an automated download.")
		failed = true
	} else if strings.Contains(linkJSON, "Sentinel marked this request as rejected.") {
		fmt.Println(" - WARNING! Microsoft blocked the automated download request based on your IP address.")
		failed = true
	}
	if failed {
		fmt.Printf("   Manually download the Windows %s ISO using a web browser from: %s\n", release, url)
		if abs, err := filepath.Abs(vmPath); err == nil {
			fmt.Println("   Save the downloaded ISO to: " + abs)
		}
		fmt.Println("   Update the config file to reference the downloaded ISO: ./" + vmPath + ".conf")
		fmt.Println("   Continuing with the VM creation process...")
		return "", fmt.Errorf("microsoft blocked the download")
	}
	var links struct {
		ProductDownloadOptions []struct {
			URI string `json:"Uri"`
		} `json:"ProductDownloadOptions"`
	}
	if err := json.Unmarshal([]byte(linkJSON), &links); err != nil {
		return "", err
	}
	isoLink := ""
	for _, o := range links.ProductDownloadOptions {
		if strings.Contains(o.URI, "x64") {
			isoLink = o.URI
			break
		}
	}
	if isoLink == "" {
		return "", fmt.Errorf("no x64 download link; please manually download this ISO in a web browser: %s", url)
	}
	fmt.Println(" - URL: " + strings.SplitN(isoLink, "?", 2)[0])
	// FILE_NAME = 5th path segment of the link.
	fileName := ""
	parts := strings.Split(strings.SplitN(isoLink, "?", 2)[0], "/")
	if len(parts) > 4 {
		fileName = parts[4]
	}
	if fileName == "" {
		fileName = "windows-" + release + ".iso"
	}
	if fetch(isoLink, filepath.Join(vmPath, fileName), "") != 0 {
		return "", fmt.Errorf("download failed")
	}
	return fileName, nil
}

// downloadWindowsServer: eval-center enterprise ISO via fwlink page
// scraping. Returns the saved file name.
func downloadWindowsServer(release, i18n, vmPath, entType string) (string, error) {
	fmt.Printf("Downloading Windows Server %s (%s)\n", release, i18n)
	url := "https://www.microsoft.com/en-us/evalcenter/download-windows-server-" + release
	fmt.Println(" - Parsing download page: " + url)
	page, err := msGet(url, 1<<20, "")
	if err != nil {
		return "", err
	}
	if page == "" {
		return "", fmt.Errorf("windows server download page gave us an empty response")
	}
	cc := [2]string{"en-us", "US"}
	if c, ok := winServerCultures[i18n]; ok {
		cc = c
	}
	fmt.Println(" - Getting download link..")
	var links []string
	for _, l := range reFwLink.FindAllString(page, -1) {
		if strings.Contains(l, "culture="+cc[0]+"&country="+cc[1]) {
			links = append(links, l)
		}
	}
	if len(links) == 0 {
		return "", fmt.Errorf("windows server download page gave us no download link")
	}
	var link string
	switch entType {
	case "enterprise":
		if len(links) >= 2 {
			link = links[1]
		} else {
			link = links[0]
		}
	case "ltsc":
		if len(links) >= 4 {
			link = links[3]
		} else {
			link = links[len(links)-1]
		}
	default:
		link = links[0]
	}
	if len(link) > 1024 {
		link = link[:1024]
	}
	// Follow redirect for a useful log line (HEAD effective URL).
	req, err := http.NewRequest("HEAD", link, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpProbe.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	final := resp.Request.URL.String()
	if len(final) > 1024 {
		final = final[:1024]
	}
	fmt.Println(" - URL: " + final)
	fileName := final[strings.LastIndex(final, "/")+1:]
	if i := strings.Index(fileName, "?"); i >= 0 {
		fileName = fileName[:i]
	}
	if fetch(final, filepath.Join(vmPath, fileName), "") != 0 {
		return "", fmt.Errorf("download failed")
	}
	return fileName, nil
}

// getWindows: get_windows orchestration — download the ISO (workstation
// or server), fetch virtio-win, build unattended media for 10/11, and
// write the conf with fixed_iso.
func getWindows(osName, release, i18n, dir, arch string) int {
	// Bash only validates/suffixes the language when given explicitly;
	// the "English International" default is not in the server list.
	explicitI18N := i18n != ""
	if explicitI18N {
		list := winLanguages
		if osName == "windows-server" {
			list = winServerLanguages
		}
		if !validLang(list, i18n) {
			fmt.Printf("ERROR! %s is not a supported language\n", i18n)
			return 1
		}
	} else {
		i18n = "English International"
	}
	if dir == "" {
		dir = osName + "-" + release
		if explicitI18N {
			suffix := strings.NewReplacer(" ", "-", "(", "", ")", "").Replace(i18n)
			dir += "-" + suffix
		}
	}
	var fileName string
	var err error
	if osName == "windows-server" {
		fileName, err = downloadWindowsServer(release, i18n, dir, "")
	} else {
		fileName, err = downloadWindowsWorkstation(release, i18n, dir)
	}
	if err != nil {
		fmt.Println(err)
		// Microsoft-blocked flows still continue to conf generation
		// when the user was told to download manually — Bash returns
		// 1 but proceeds to make_vm_config for the placeholder file.
	}
	if noConf {
		return 0
	}
	fmt.Println("Downloading VirtIO drivers...")
	virtio := filepath.Join(dir, "virtio-win.iso")
	if _, serr := os.Stat(virtio); serr != nil {
		fetch("https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso", virtio, "")
	}
	os.Remove(filepath.Join(dir, "unattended.iso"))
	if release == "10" || release == "11" {
		unattendedWindows(dir, "w"+release, i18n)
	}
	if fileName == "" {
		fileName = "windows-" + release + ".iso"
	}
	return mkconf(osName, release, "", fileName, "virtio-win.iso", dir, arch, cloudInitFlag)
}
