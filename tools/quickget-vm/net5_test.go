// net5_test.go — last-mile: chunklist errors, macos cached-img path,
// getOS unattended branches, fetchOnce range handling.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckChunklistErrors(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ dmg, cl string }{
		{"", ""},
		{filepath.Join(dir, "x.dmg"), filepath.Join(dir, "x.cl")},
	} {
		if tc.dmg != "" {
			os.WriteFile(tc.dmg, []byte("dmg"), 0o644)
			os.WriteFile(tc.cl, []byte("not-chunklist"), 0o644)
		}
		if err := checkChunklist(tc.dmg, tc.cl); err == nil {
			t.Fatal("invalid chunklist should error")
		}
	}
	// CNKL magic but truncated structure → deeper error path.
	cl := filepath.Join(dir, "b.cl")
	os.WriteFile(cl, append([]byte("CNKL"), make([]byte, 64)...), 0o644)
	dmg := filepath.Join(dir, "b.dmg")
	os.WriteFile(dmg, []byte("data"), 0o644)
	_ = checkChunklist(dmg, cl)
}

func TestGetMacosCachedImg(t *testing.T) {
	stubFetchers(t)
	stubProbe(t, map[string]string{
		"RecoveryImage": "https://oscdn.apple.com/x.dmg expires-dmg https://oscdn.apple.com/x.chunklist expires-cl",
	})
	dir := filepath.Join(t.TempDir(), "mac")
	os.MkdirAll(dir, 0o755)
	// Pre-existing RecoveryImage.img → skips the whole download.
	os.WriteFile(filepath.Join(dir, "RecoveryImage.img"), []byte("img"), 0o644)
	var rel string
	for r := range macBoards {
		rel = r
		break
	}
	if rc := getMacos(rel, dir, "amd64"); rc != 0 {
		t.Fatalf("cached-img getMacos rc=%d", rc)
	}
}

func TestGetOSUnattendedUbuntuServer(t *testing.T) {
	stubFetchers(t)
	oldFs := fetchString
	fetchString = func(url string) (string, error) {
		if strings.Contains(url, "SUMS") {
			return strings.Repeat("b", 64) + " *ubuntu-24.04-live-server-amd64.iso\n", nil
		}
		return "", fmt.Errorf("nf")
	}
	defer func() { fetchString = oldFs }()
	old := unattendedFlag
	unattendedFlag = true
	defer func() { unattendedFlag = old }()
	dir := t.TempDir()
	// ubuntu-server + --unattended → genUbuntuAutoinstall attempt on a
	// non-ISO payload → returns error (branch covered either way).
	_ = getOS("ubuntu-server", "24.04", "", filepath.Join(dir, "us"), "amd64")
}

func TestFetchOnceRangeEdge(t *testing.T) {
	payload := []byte("0123456789abcdef")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.WriteHeader(416) // unsatisfiable → error path
			return
		}
		w.Write(payload)
	}))
	defer srv.Close()
	dir := t.TempDir()
	// pre-staged .part + server 416 → fetchOnce fails, retries exhaust.
	os.WriteFile(filepath.Join(dir, "f.part"), payload[:4], 0o644)
	if rc := fetchOnce(srv.URL+"/f", filepath.Join(dir, "f.part"), ""); rc != 1 {
		t.Fatal("416 should fail")
	}
	// server ignores Range → restart from scratch.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload) // 200 even with Range header
	}))
	defer srv2.Close()
	os.WriteFile(filepath.Join(dir, "g.part"), payload[:4], 0o644)
	if rc := fetchOnce(srv2.URL+"/g", filepath.Join(dir, "g.part"), ""); rc != 0 {
		t.Fatal("ignored-range should restart and succeed")
	}
}
