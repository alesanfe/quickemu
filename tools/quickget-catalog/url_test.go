// url_test.go — pure URL/template resolution + httptest-backed hash fetch.
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFindLine(t *testing.T) {
	body := "junk\nabc123 sha256\nmore"
	if got := findLine(body, `([a-f0-9]{6})`, 1); got != "abc123" {
		t.Fatalf("field %q", got)
	}
	if got := findLine(body, `abc`, 0); got != "abc" {
		t.Fatalf("whole %q", got)
	}
	if got := findLine(body, `zzz`, 0); got != "" {
		t.Fatalf("miss %q", got)
	}
}

func TestResolveURLTemplate(t *testing.T) {
	e := &Entry{URL: "https://cdn/%RELEASE%/%EDITION%/%ARCH%",
		ISO: "img-%RELEASE%.iso"}
	full, hash := resolveURL(e, "3.20", "virt", "amd64")
	if full != "https://cdn/3.20/virt/amd64/img-3.20.iso" || hash != "" {
		t.Fatalf("template %q %q", full, hash)
	}
	// Trailing-slash base joins without extra separator.
	e2 := &Entry{URL: "https://cdn/base/", ISO: "a.iso"}
	if full, _ = resolveURL(e2, "", "", "amd64"); full != "https://cdn/base/a.iso" {
		t.Fatalf("slash join %q", full)
	}
	// QArch swaps tokens.
	e3 := &Entry{URL: "https://cdn/%ARCH%/", ISO: "a.iso"}
	if full, _ = resolveURL(e3, "", "", "amd64"); !strings.Contains(full, "amd64") {
		t.Fatalf("plain arch %q", full)
	}
	e3.QArch = true
	if full, _ = resolveURL(e3, "", "", "amd64"); !strings.Contains(full, "x86_64") {
		t.Fatalf("qarch amd64 %q", full)
	}
	if full, _ = resolveURL(e3, "", "", "arm64"); !strings.Contains(full, "aarch64") {
		t.Fatalf("qarch arm64 %q", full)
	}
	if full, _ = resolveURL(&Entry{}, "", "", ""); full != "" {
		t.Fatalf("empty %q", full)
	}
}

func TestResolveURLHash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("img-1.iso  deadbeef" + strings.Repeat("0", 56) + "\n"))
	}))
	defer srv.Close()
	e := &Entry{URL: "https://cdn/", ISO: "img-%RELEASE%.iso",
		HashURL: srv.URL + "/SUMS"}
	full, hash := resolveURL(e, "1", "", "amd64")
	if !strings.HasSuffix(full, "img-1.iso") || len(hash) != 64 {
		t.Fatalf("hash resolve %q %q", full, hash)
	}
	// Explicit HashRE wins over the iso-derived default.
	e.HashRE = `([0-9a-f]{6})`
	_, hash = resolveURL(e, "1", "", "amd64")
	if hash != "deadbe" {
		t.Fatalf("hashre %q", hash)
	}
	// Broken hash source → empty hash, still resolves URL.
	e.HashURL = srv.URL + "/missing"
	srv.Close()
	_, hash = resolveURL(e, "1", "", "amd64")
	if hash != "" {
		t.Fatalf("bad hash %q", hash)
	}
}

func TestExpandVars(t *testing.T) {
	cases := map[string]string{
		"${URL}/x":        "BASE/x",
		"${BASE}":         "BASE",
		"${ISO}":          "ISO",
		"${ISONAME}":      "ISO",
		"${edition}":      "ed",
		"${ED_UPPER}":     "ED",
		"${README}":       "README",
		"${VER_MAJ}":      "3",
		"${VER_MIN}":      "20",
		"${RELEASE#3.}":   "20",
		"${RELEASE:0:3}":  "3.2",
		"${RELEASE:2}":    "20",
		"${RELEASE//./-}": "3-20",
		"${ISO//IS/x}":    "xO",
		"plain":           "plain",
		"${RELEASE:9:2}":  "",
		"${UNKNOWN}":      "${UNKNOWN}",
	}
	for in, want := range cases {
		if got := expandVars(in, "BASE", "ISO", "3.20", "ed"); got != want {
			t.Errorf("expandVars(%q)=%q want %q", in, got, want)
		}
	}
}
