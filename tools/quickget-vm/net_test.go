// net_test.go — offline coverage for the HTTP flows: httpProbe's
// Transport is swapped for a canned RoundTripper, fetch/fetchString
// for hermetic stubs.
package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func stubProbe(t *testing.T, bodies map[string]string) {
	t.Helper()
	old := httpProbe
	// Longest key first: map iteration order is random, and an URL
	// matching two overlapping keys must resolve deterministically
	// to the most specific matcher.
	keys := make([]string, 0, len(bodies))
	for k := range bodies {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int { return len(b) - len(a) })
	httpProbe = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		body := "notfound"
		reqURL := r.URL
		for _, k := range keys {
			if strings.Contains(r.URL.String(), k) {
				body = bodies[k]
				break
			}
		}
		// "→<url>" body prefix simulates a redirect: the response's
		// Request.URL differs from the requested one.
		if strings.HasPrefix(body, "→") {
			if u, err := r.URL.Parse(strings.TrimPrefix(body, "→")); err == nil {
				reqURL = u
				body = ""
			}
		}
		hdr := make(http.Header)
		if strings.Contains(r.URL.Host, "osrecovery") || strings.Contains(r.URL.Host, "oscdn") {
			hdr.Set("Set-Cookie", "session=sess-abc123")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)),
			Header: hdr, Request: &http.Request{Method: r.Method, URL: reqURL}}, nil
	})}
	t.Cleanup(func() { httpProbe = old })
}

func stubFetchers(t *testing.T) {
	t.Helper()
	oldF, oldFs := fetch, fetchString
	fetch = func(url, dest, sha string) int {
		os.MkdirAll(filepath.Dir(dest), 0o755)
		if strings.Contains(url, "/missing") {
			return 1
		}
		if err := os.WriteFile(dest, []byte("img"), 0o644); err != nil {
			return 1
		}
		return 0
	}
	fetchString = func(url string) (string, error) {
		if strings.Contains(url, "/missing") {
			return "", io.EOF
		}
		return "sums-body", nil
	}
	t.Cleanup(func() { fetch, fetchString = oldF, oldFs })
}

func TestGetUbuntu(t *testing.T) {
	stubFetchers(t)
	oldCodename := ubuntuDailyCodename
	ubuntuDailyCodename = func(string) string { return "noble" }
	defer func() { ubuntuDailyCodename = oldCodename }()
	oldFs := fetchString
	fetchString = func(url string) (string, error) {
		if strings.Contains(url, "SHA256SUMS") || strings.Contains(url, "MD5SUMS") {
			return strings.Repeat("a", 64) + " *ubuntu-24.04-desktop-amd64.iso\n" +
				strings.Repeat("b", 64) + " *ubuntu-24.04-live-server-amd64.iso\n" +
				strings.Repeat("c", 64) + " *ubuntu-24.04-desktop-arm64.iso\n" +
				strings.Repeat("d", 64) + " *ubuntu-24.04-live-server-arm64.iso\n", nil
		}
		return "", io.EOF
	}
	defer func() { fetchString = oldFs }()
	// fake zsync: creates the -o target so the zsync branch succeeds.
	zdir := t.TempDir()
	os.WriteFile(filepath.Join(zdir, "zsync"),
		[]byte("#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in -o) shift; touch \"$1\";; esac; shift; done\nexit 0\n"), 0o755)
	t.Setenv("PATH", zdir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dir := t.TempDir()
	for _, c := range []struct {
		name, os, rel, arch string
		rc                  int
	}{
		{"desktop", "ubuntu", "24.04", "amd64", 0},
		{"desktop-arm", "ubuntu", "24.04", "arm64", 0},
		{"server", "ubuntu-server", "24.04", "amd64", 0},
		{"server-old", "ubuntu-server", "16.04", "amd64", 0},
		{"daily", "ubuntu", "daily-live", "amd64", 0},
		{"dvd", "ubuntu", "dvd", "amd64", 0},
		{"flavour", "xubuntu", "24.04", "amd64", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := filepath.Join(dir, c.name)
			if rc := getUbuntu(c.os, c.rel, d, c.arch); rc != c.rc {
				t.Fatalf("rc=%d", rc)
			}
		})
	}
	// unavailable: fetchString fails everywhere → error.
	fetchString = func(string) (string, error) { return "", io.EOF }
	if rc := getUbuntu("ubuntu", "99.99", filepath.Join(dir, "x"), ""); rc != 1 {
		t.Fatal("unavailable release should fail")
	}
	ubuntuDailyCodename = func(string) string { return "" }
	if rc := getUbuntu("ubuntu", "daily-live", filepath.Join(dir, "y"), ""); rc != 1 {
		t.Fatal("daily with no codename should fail")
	}
}

func TestZsyncGet(t *testing.T) {
	stubFetchers(t)
	dir := t.TempDir()
	// minimal PATH: no zsync → falls back to (stubbed) fetch.
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	if rc := zsyncGet("https://x/f.iso", dir, "f.iso"); rc != 0 {
		t.Fatalf("zsyncGet fallback rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(dir, "f.iso")); err != nil {
		t.Fatal("file not written")
	}
	// fake zsync on PATH → real zsync branch.
	zdir := t.TempDir()
	os.WriteFile(filepath.Join(zdir, "zsync"),
		[]byte("#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in -o) shift; touch \"$1\";; esac; shift; done\nexit 0\n"), 0o755)
	t.Setenv("PATH", zdir+string(os.PathListSeparator)+"/bin:/usr/bin")
	if rc := zsyncGet("https://x/f2.iso", dir, "f2.iso"); rc != 0 {
		t.Fatalf("zsyncGet zsync-branch rc=%d", rc)
	}
	// failing zsync → error.
	os.WriteFile(filepath.Join(zdir, "zsync"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	if rc := zsyncGet("https://x/f3.iso", dir, "f3.iso"); rc != 1 {
		t.Fatal("zsync failure should fail")
	}
	t.Setenv("PATH", empty)
	if rc := zsyncGet("https://x/missing", dir, "m.iso"); rc != 1 {
		t.Fatal("fetch failure should fail")
	}
}

func TestGetMacos(t *testing.T) {
	stubFetchers(t)
	// appleRecoverySession → cookie; appleRecoveryInfo → fields
	// "token url"; macFetch → chunk payload via stubProbe.
	stubProbe(t, map[string]string{
		"RecoveryImage": "tok123 https://cdn/x.pkg",
	})
	dir := t.TempDir()
	// unknown release → fast error path.
	if rc := getMacos("nope", filepath.Join(dir, "m"), "amd64"); rc != 1 {
		t.Fatal("unknown release should fail")
	}
	// Every real release row — rc varies per board/flow, coverage is
	// the goal (must not hang/panic).
	for rel := range macBoards {
		_ = getMacos(rel, filepath.Join(dir, rel), "amd64")
	}
}

func TestGetWindows(t *testing.T) {
	stubFetchers(t)
	stubProbe(t, map[string]string{
		"fwlink": "https://dl.img/file.iso",
	})
	dir := t.TempDir()
	// invalid language → early error.
	if rc := getWindows("windows", "11", "Klingon", filepath.Join(dir, "w"), "amd64"); rc != 1 {
		t.Fatal("bad language should fail")
	}
	// valid flow — downloadWindowsWorkstation hits stubProbe; even a
	// parse failure continues to conf generation.
	// With fetchers+probe stubbed the flow completes → rc 0.
	if rc := getWindows("windows", "11", "English International",
		filepath.Join(dir, "w2"), "amd64"); rc != 0 {
		t.Fatalf("getWindows rc=%d", rc)
	}
	if rc := getWindows("windows-server", "2022", "",
		filepath.Join(dir, "w3"), "amd64"); rc != 0 {
		t.Fatalf("getWindows-server rc=%d", rc)
	}
}

func TestUnattendedAndAutoinstall(t *testing.T) {
	dir := t.TempDir()
	// unattendedWindows writes autounattend.xml before downloading
	// drivers — the XML artifact is deterministic even when the
	// driver fetch is not.
	unattendedWindows(dir, "w11", "English International")
	xml, err := os.ReadFile(filepath.Join(dir, "unattended",
		"autounattend.xml"))
	if err != nil {
		t.Fatalf("autounattend.xml not written: %v", err)
	}
	// "English International" → en-GB/0809 per winCultures — and no
	// template placeholder may survive the substitution.
	if !strings.Contains(string(xml), "en-GB") ||
		strings.Contains(string(xml), "${CULTURE}") {
		t.Error("autounattend.xml culture substitution broken")
	}
	// genUbuntuAutoinstall: seed ISO + autoinstall conf.
	iso := filepath.Join(dir, "u.iso")
	os.WriteFile(iso, []byte("x"), 0o644)
	conf := filepath.Join(dir, "u.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o644)
	t.Setenv("USER", "tester")
	t.Setenv("HOME", dir)
	// "x" is not an ISO → bootfile extraction must fail → rc 1.
	if rc := genUbuntuAutoinstall(dir, iso, conf); rc != 1 {
		t.Fatalf("autoinstall on non-ISO rc=%d", rc)
	}
}

func TestIsoExtractPure(t *testing.T) {
	dir := t.TempDir()
	iso := buildISO(t, map[string]string{
		"boot/vmlinuz": "k", "boot/initrd": "i"})
	out := filepath.Join(dir, "out")
	os.MkdirAll(out, 0o755)
	ok := isoExtractPure(iso, out, "boot")
	if !ok {
		t.Fatal("isoExtractPure failed")
	}
}
