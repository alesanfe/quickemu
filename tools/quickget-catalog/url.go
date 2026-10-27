// URL resolution for quickget-catalog: implements the get_<os> contract
// generically. Template-based OSes resolve purely in Go (URL + ISO + live
// checksum fetch); scraper OSes resolve live via scrape().
package main

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// fetchString GETs a URL and returns the body (bounded). Mirrors web_pipe.
func fetchString(u string) (string, error) {
	c := &http.Client{Timeout: 30 * time.Second}
	r, err := c.Get(u)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return "", fmt.Errorf("http %d", r.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	return string(b), err
}

// finalURL resolves the last redirect target (web_redirect parity).
func finalURL(u string) (string, error) {
	c := &http.Client{Timeout: 30 * time.Second}
	r, err := c.Get(u)
	if err != nil {
		return "", err
	}
	r.Body.Close()
	return r.Request.URL.String(), nil
}

// findLine greps the body for lines matching re and returns the first,
// extracting group `field` (1-indexed) or the whole match when field==0.
func findLine(body, re string, field int) string {
	r := regexp.MustCompile(re)
	for _, line := range strings.Split(body, "\n") {
		if m := r.FindStringSubmatch(line); m != nil {
			if field > 0 && len(m) > field {
				return m[field]
			}
			return strings.TrimSpace(m[0])
		}
	}
	return ""
}

// resolveURL produces "URL HASH" the way get_<os> does. Templates use
// %RELEASE%/%EDITION%/%ARCH% placeholders captured at dump time; hash_src
// is the checksum-file URL pattern.
func resolveURL(e *Entry, release, edition, arch string) (string, string) {
	if e.URL == "" {
		return "", ""
	}
	sub := func(s string) string {
		s = strings.ReplaceAll(s, "%RELEASE%", release)
		s = strings.ReplaceAll(s, "%EDITION%", edition)
		s = strings.ReplaceAll(s, "%ARCH%", arch)
		return s
	}
	base := sub(e.URL)
	iso := ""
	if e.ISO != "" {
		iso = sub(e.ISO)
	}
	hash := ""
	if e.HashURL != "" {
		src := sub(e.HashURL)
		if body, err := fetchString(src); err == nil {
			re := e.HashRE
			if re == "" {
				re = `(?i)` + regexp.QuoteMeta(iso) + `.*?([a-f0-9]{64})`
			}
			hash = findLine(body, re, 1)
		}
	}
	full := base
	if iso != "" && !strings.HasSuffix(base, "/") {
		full = base + "/" + iso
	} else if iso != "" {
		full = base + iso
	}
	return full, hash
}
