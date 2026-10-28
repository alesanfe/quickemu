// Shared HTTP clients and probes — curl --connect-timeout/--max-time
// parity: probes die in 30s, downloads bound dial+header but stream
// unbounded.
package qelib

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// HTTPTransport is the shared transport policy: 15s dial/TLS,
// 30s response-header bounds.
var HTTPTransport = &http.Transport{
	DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
	TLSHandshakeTimeout:   15 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
}

// ProbeClient for bounded reachability/metadata probes.
var ProbeClient = &http.Client{Timeout: 30 * time.Second, Transport: HTTPTransport}

// DLClient for payload downloads. proto-redir =http,https parity:
// redirects may never leave the http(s) schemes (blocks redirect ->
// file:// etc.) and are bounded like curl --max-redirs.
var DLClient = &http.Client{Transport: HTTPTransport,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("redirect to disallowed scheme %q", req.URL.Scheme)
		}
		return nil
	}}

// WebCheck: HEAD reachability probe with a GET Range:bytes=0-0
// fallback — web_check parity (presigned/CDN URLs that 403 HEAD).
func WebCheck(url string, headers map[string]string) bool {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return false
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if resp, err := ProbeClient.Do(req); err == nil {
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return true
		}
	}
	req, err = http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Range", "bytes=0-0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ProbeClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// FetchString GETs a URL and returns the body, bounded to limit
// bytes. Mirrors web_pipe.
func FetchString(u string, limit int64) (string, error) {
	r, err := ProbeClient.Get(u)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return "", fmt.Errorf("http %d", r.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, limit))
	return string(b), err
}

// FinalURL resolves the last redirect target (web_redirect parity).
func FinalURL(u string) (string, error) {
	r, err := DLClient.Get(u)
	if err != nil {
		return "", err
	}
	r.Body.Close()
	return r.Request.URL.String(), nil
}

// UbuntuDailyCodename returns the first (newest-first) launchpad
// series name whose daily-live SHA256SUMS exists on cdimage.
func UbuntuDailyCodename(osName string) (string, error) {
	body, err := FetchString("https://api.launchpad.net/devel/ubuntu/series", 4<<20)
	if err != nil {
		return "", err
	}
	var j struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	if json.Unmarshal([]byte(body), &j) != nil {
		return "", fmt.Errorf("launchpad parse")
	}
	for _, e := range j.Entries {
		if WebCheck(fmt.Sprintf("https://cdimage.ubuntu.com/%s/%s/daily-live/current/SHA256SUMS", osName, e.Name), nil) {
			return e.Name, nil
		}
	}
	return "", nil
}
