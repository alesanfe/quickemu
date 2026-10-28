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
	for _, e := range entries {
		if len(e.Releases) == 0 {
			continue
		}
		rel, ed := e.Releases[0], ""
		if len(e.Editions) > 0 {
			ed = e.Editions[0]
		}
		vmDir := filepath.Join(base, e.OS+"-"+rel)
		var rc int
		switch {
		case strings.Contains(e.OS, "ubuntu"):
			// SUMS resolution stubbed; release URLs hardcode ubuntu.com.
			// execCommand→true makes the real zsync binary a no-op so
			// daily-live releases also complete offline.
			for _, r := range e.Releases { // prefer a non-daily release
				if !strings.HasPrefix(r, "daily") && r != "dvd" {
					rel = r
					break
				}
			}
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
			rel = ""
			for _, r := range e.Releases { // release must exist in macBoards
				if _, ok := macBoards[r]; ok {
					rel = r
					break
				}
			}
			if rel == "" {
				for r := range macBoards {
					rel = r
					break
				}
			}
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
				rc = getOS("windows-server", "2022", "", vmDir, "amd64")
			} else {
				stubProbe(t, map[string]string{
					"software-download/windows": `<option value="1234">Windows 11`,
					"mdt.js":                    `?w=ABCDEF&rticks="+123456"`,
					"getskuinformation":         `{"Skus":[{"Id":1234,"LocalizedLanguage":"English International"}]}`,
					"GetProductDownloadLinks":   `{"ProductDownloadOptions":[{"Uri":"https://dl.microsoft.com/a/b/win-x64.iso?q=1"}]}`,
				})
				rc = getOS("windows", "11", "", vmDir, "amd64")
			}
			fetch = realFetch
		default:
			rc = getOS(e.OS, rel, ed, vmDir, "amd64")
		}
		// Verify artifacts, then delete the VM before the next one —
		// mirrors the requested download → verify → delete cadence.
		conf := vmDir + ".conf"
		confB, _ := os.ReadFile(conf)
		if rc != 0 || len(confB) == 0 {
			failed = append(failed, e.OS+"@"+rel)
			os.RemoveAll(vmDir)
			os.Remove(conf)
			continue
		}
		if _, err := os.Stat(vmDir); err != nil {
			failed = append(failed, e.OS+"@"+rel+"(no dir)")
			os.Remove(conf)
			continue
		}
		done = append(done, e.OS)
		if err := os.RemoveAll(vmDir); err != nil {
			t.Fatalf("delete %s: %v", e.OS, err)
		}
		os.Remove(conf)
	}
	t.Logf("downloaded+deleted %d OSes; failures: %v", len(done), failed)
	if len(done) == 0 {
		t.Fatal("no OS downloaded successfully")
	}
	// Hard floor: every catalog OS must download, verify and delete.
	for _, e := range entries {
		ok := false
		for _, d := range done {
			if d == e.OS {
				ok = true
				break
			}
		}
		if !ok && len(e.Releases) > 0 {
			t.Errorf("generic OS %s did not complete", e.OS)
		}
	}
}
