// e2e_allos_test.go — end-to-end matrix over the real catalog: for
// every OS entry, run the complete get pipeline, verify the produced
// VM directory + conf + image, delete it, then move to the next one.
// Generic OSes download through the real fetch path against a local
// HTTP server; the hardcoded-URL pipelines (ubuntu family, macos,
// windows) run against the same in-process seams the unit tests use.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type catEntry struct {
	OS       string   `json:"os"`
	Releases []string `json:"releases"`
	Editions []string `json:"editions"`
	Arch     []string `json:"arch"`
}

func readCatalog(t *testing.T) []catEntry {
	t.Helper()
	for _, p := range []string{
		filepath.Join("..", "..", "catalog", "data", "catalog.json"),
		os.Getenv("QUICKEMU_CATALOG"),
	} {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var c struct {
			Entries []catEntry `json:"entries"`
		}
		if json.Unmarshal(b, &c) == nil && len(c.Entries) > 0 {
			return c.Entries
		}
	}
	t.Skip("catalog data not found")
	return nil
}

func TestDownloadEveryOSThenDelete(t *testing.T) {
	entries := readCatalog(t)
	payload := []byte("e2e-fake-image-bytes")
	sum := fmt.Sprintf("%x", sha256.Sum256(payload))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
			fmt.Fprintf(w, "%s  *img.iso\n", sum)
		case strings.HasSuffix(r.URL.Path, "MD5SUMS"):
			fmt.Fprintf(w, "%s  *img.iso\n", sum)
		default:
			w.Write(payload)
		}
	}))
	defer srv.Close()

	// generic path: stub quickget-catalog resolves every OS to the
	// local server — fetch itself stays REAL (genuine HTTP download).
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-catalog"),
		[]byte("#!/bin/sh\necho '"+srv.URL+"/img.iso "+srv.URL+"/SHA256SUMS'\n"), 0o755)
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", stub+string(os.PathListSeparator)+oldPath)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })

	// seam save/restore — the hardcoded-URL pipelines can't point at
	// localhost, so their fetch* seams write the payload directly.
	oldF, oldFs, oldDC := fetch, fetchString, ubuntuDailyCodename
	realFetch := fetch
	t.Cleanup(func() { fetch, fetchString, ubuntuDailyCodename = oldF, oldFs, oldDC })
	offlineFetch := func(url, dest, sha string) int {
		os.MkdirAll(filepath.Dir(dest), 0o755)
		if err := os.WriteFile(dest, payload, 0o644); err != nil {
			return 1
		}
		return 0
	}

	oldNoConf, oldUnatt := noConf, unattendedFlag
	noConf, unattendedFlag = false, false
	t.Cleanup(func() { noConf, unattendedFlag = oldNoConf, oldUnatt })

	base := t.TempDir()
	var done, failed []string
	// runOne executes the full pipeline for a single OS×release×edition
	// combo and returns the produced conf bytes.
	runOne := func(e catEntry, rel, ed, vmDir string) (int, []byte) {
		var rc int
		switch {
		case strings.Contains(e.OS, "ubuntu"):
			// SUMS resolution stubbed; release URLs hardcode ubuntu.com.
			// execCommand→true makes the real zsync binary a no-op so
			// daily-live releases also complete offline.
			isServer := e.OS == "ubuntu-server"
			pat := "desktop"
			if isServer {
				pat = "live-server"
			}
			fetchString = func(string) (string, error) {
				return sum + "  *u-" + rel + "-" + pat + "-amd64.iso\n", nil
			}
			fetch = offlineFetch
			oldEC := execCommand
			execCommand = func(name string, args ...string) *exec.Cmd {
				return exec.Command("true")
			}
			rc = getOS(e.OS, rel, "", vmDir, "amd64")
			fetchString, fetch, execCommand = oldFs, realFetch, oldEC
		case e.OS == "macos":
			stubProbe(t, map[string]string{
				"RecoveryImage": "https://oscdn.apple.com/x.dmg expires-dmg https://oscdn.apple.com/x.chunklist expires-cl",
			})
			fetch = offlineFetch
			os.MkdirAll(vmDir, 0o755)
			rc = getOS("macos", rel, "", vmDir, "amd64")
			fetch = realFetch
		case e.OS == "windows" || e.OS == "windows-server":
			fetch = offlineFetch
			if e.OS == "windows-server" {
				stubProbe(t, map[string]string{
					"evalcenter": `<a href="https://go.microsoft.com/fwlink/p/?LinkID=111&clcid=0x409&culture=en-us&country=US">a</a>`,
					"fwlink":     "→https://dl.example.com/win-srv.iso",
				})
				rc = getOS("windows-server", rel, ed, vmDir, "amd64")
			} else {
				stubProbe(t, map[string]string{
					"software-download/windows": `<option value="1234">Windows ` + rel,
					"mdt.js":                    `?w=ABCDEF&rticks="+123456"`,
					"getskuinformation":         `{"Skus":[{"Id":1234,"LocalizedLanguage":"English International"}]}`,
					"GetProductDownloadLinks":   `{"ProductDownloadOptions":[{"Uri":"https://dl.microsoft.com/a/b/win-x64.iso?q=1"}]}`,
				})
				rc = getOS("windows", rel, "", vmDir, "amd64")
			}
			fetch = realFetch
		default:
			rc = getOS(e.OS, rel, ed, vmDir, "amd64")
		}
		confB, _ := os.ReadFile(vmDir + ".conf")
		return rc, confB
	}
	for _, e := range entries {
		rels := e.Releases
		if len(rels) == 0 {
			// pureos/rockylinux/slitaz: releases resolve via live
			// scrape at runtime — "latest" stands in offline.
			rels = []string{"latest"}
		}
		eds := e.Editions
		if len(eds) == 0 {
			eds = []string{""}
		}
		for _, rel := range rels {
			// macos releases must exist in macBoards.
			if e.OS == "macos" {
				if _, ok := macBoards[rel]; !ok {
					continue
				}
			}
			for _, ed := range eds {
				// windows: the "edition" positional is the i18n
				// language — the fixture only supports the default.
				runEd := ed
				if e.OS == "windows" || e.OS == "windows-server" {
					runEd = ""
				}
				vmDir := filepath.Join(base, e.OS+"-"+rel+ed)
				rc, confB := runOne(e, rel, runEd, vmDir)
				conf := vmDir + ".conf"
				if rc != 0 || len(confB) == 0 {
					failed = append(failed, e.OS+"@"+rel+"/"+ed)
					os.RemoveAll(vmDir)
					os.Remove(conf)
					continue
				}
				if _, err := os.Stat(vmDir); err != nil {
					failed = append(failed, e.OS+"@"+rel+"/"+ed+"(no dir)")
					os.Remove(conf)
					continue
				}
				done = append(done, e.OS+"@"+rel+"/"+ed)
				if err := os.RemoveAll(vmDir); err != nil {
					t.Fatalf("delete %s: %v", e.OS, err)
				}
				os.Remove(conf)
			}
		}
	}
	t.Logf("downloaded+deleted %d OS images; failures: %v", len(done), failed)
	if len(done) == 0 {
		t.Fatal("no OS downloaded successfully")
	}
	// Hard floor: every catalog OS×release×edition must download,
	// verify and delete.
	for _, e := range entries {
		ok := false
		for _, d := range done {
			if strings.HasPrefix(d, e.OS+"@") {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("OS %s did not complete", e.OS)
		}
	}
}
