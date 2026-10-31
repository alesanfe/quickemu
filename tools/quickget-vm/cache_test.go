// cache_test.go — QE-043 secure download-cache coverage, all offline
// via httptest: ETag/Last-Modified persistence + conditional
// revalidation, Range resume on .part, tmp+verify+rename, per-URL
// lock serialisation, LRU eviction and --offline.
package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
)

func freshCache(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "dlcache")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QE_DL_CACHE", d)
	t.Setenv("QE_CACHE_MAX", "0")
	return d
}

func TestCacheStoreAnd304Revalidate(t *testing.T) {
	freshCache(t)
	body := []byte("the payload body")
	var hits int32
	var sawCond atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		sawCond.Store(r.Header.Get("If-None-Match"))
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		w.Write(body)
	}))
	defer srv.Close()
	u := srv.URL + "/f"

	p, err := cachedFetch(u, "", false)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if got, _ := os.ReadFile(p); string(got) != string(body) {
		t.Fatalf("body %q", got)
	}
	// Meta persisted with the validators for the next lookup.
	_, mp := cachePaths(cacheKey(u))
	m := cacheRead(mp)
	if m == nil || m.ETag != `"v1"` || m.LastModified == "" || m.Size != int64(len(body)) {
		t.Fatalf("meta %+v", m)
	}
	// Revalidate → 304 → cached entry served again.
	p2, err := cachedFetch(u, "", false)
	if err != nil || p2 != p {
		t.Fatalf("revalidated fetch %v %q", err, p2)
	}
	// 3 hits: the GET download, the HEAD probe that captured the
	// validators, and the conditional revalidation GET.
	if hits != 3 {
		t.Fatalf("expected 3 server hits, got %d", hits)
	}
	if sawCond.Load() != `"v1"` {
		t.Fatalf("If-None-Match not sent: %v", sawCond.Load())
	}
}

func TestCacheResume(t *testing.T) {
	dir := freshCache(t)
	full := []byte("0123456789ABCDEF")
	var sawRange atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rg := r.Header.Get("Range"); rg != "" {
			sawRange.Store(rg)
			w.Header().Set("Content-Range", "bytes 8-15/16")
			w.Header().Set("ETag", `"r"`)
			w.WriteHeader(http.StatusPartialContent)
			w.Write(full[8:])
			return
		}
		w.Header().Set("ETag", `"r"`)
		w.Write(full)
	}))
	defer srv.Close()
	u := srv.URL + "/blob"
	// Pre-seed a partial .part as if a previous attempt died mid-way.
	part := filepath.Join(dir, cacheKey(u)+".part")
	os.WriteFile(part, full[:8], 0o644)
	p, err := cachedFetch(u, "", false)
	if err != nil {
		t.Fatalf("resumed fetch: %v", err)
	}
	if got, _ := os.ReadFile(p); string(got) != string(full) {
		t.Fatalf("resumed body %q", got)
	}
	if rg, _ := sawRange.Load().(string); !strings.HasPrefix(rg, "bytes=") {
		t.Fatalf("no Range resume sent: %v", sawRange.Load())
	}
	if _, err := os.Stat(part); err == nil {
		t.Fatal(".part must be renamed away on success")
	}
}

func TestCacheConcurrentSameURL(t *testing.T) {
	freshCache(t)
	var downloads int32
	var reqs sync.Map
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Store(fmt.Sprintf("%s inm=%q ims=%q", r.Method,
			r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since")), true)
		if r.Header.Get("If-None-Match") == `"c"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"c"`)
		if r.Method == http.MethodGet {
			atomic.AddInt32(&downloads, 1)
			w.Write([]byte("shared-body"))
		}
	}))
	defer srv.Close()
	u := srv.URL + "/same"
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := cachedFetch(u, "", false)
			if err != nil {
				errs <- err
				return
			}
			if b, _ := os.ReadFile(p); string(b) != "shared-body" {
				errs <- fmt.Errorf("bad body %q", b)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	// The per-URL lock serialises: exactly ONE real download happened;
	// the losers revalidated (304) or reused — never a duplicate fetch,
	// never an "already in progress" error.
	if downloads != 1 {
		reqs.Range(func(k, _ any) bool { t.Logf("saw %v", k); return true })
		t.Fatalf("body downloads %d, want 1", downloads)
	}
}

func TestCacheOffline(t *testing.T) {
	dir := freshCache(t)
	u := "https://example.invalid/x.qcow2"
	// Offline miss is a hard error — no network attempted.
	if _, err := cachedFetch(u, "", true); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("offline miss err=%v", err)
	}
	// Populate the cache, then an offline hit serves the bytes.
	data, mp := cachePaths(cacheKey(u))
	os.MkdirAll(dir, 0o755)
	os.WriteFile(data, []byte("cached"), 0o644)
	cacheWrite(mp, &cacheMeta{URL: u, ETag: `"e"`, Size: 6})
	p, err := cachedFetch(u, "", true)
	if err != nil {
		t.Fatalf("offline hit: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "cached" {
		t.Fatalf("offline body %q", b)
	}
	// A meta/data size mismatch is treated as a miss (tampered cache).
	cacheWrite(mp, &cacheMeta{URL: u, ETag: `"e"`, Size: 999})
	if _, err := cachedFetch(u, "", true); err == nil {
		t.Fatal("size-mismatched offline entry must be refused")
	}
}

func TestCacheLRUEviction(t *testing.T) {
	dir := freshCache(t)
	t.Setenv("QE_CACHE_MAX", "15")
	// Entry 1 (oldest) + entry 2 + the new 9-byte download exceed the
	// cap → only the least-recently-used entry is evicted.
	e1 := filepath.Join(dir, "e1")
	e2 := filepath.Join(dir, "e2")
	os.WriteFile(e1, []byte("12345"), 0o644)
	os.WriteFile(e2, []byte("67890"), 0o644)
	// evictLRU reads <entry>.meta sidecars directly off the dir scan.
	cacheWrite(e1+".meta", &cacheMeta{URL: "http://a/1", Size: 5,
		LastUsed: time.Now().Add(-time.Hour).Unix()})
	cacheWrite(e2+".meta", &cacheMeta{URL: "http://a/2", Size: 5,
		LastUsed: time.Now().Unix()})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new-entry"))
	}))
	defer srv.Close()
	if _, err := cachedFetch(srv.URL+"/n", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e1); err == nil {
		t.Fatal("oldest entry must be evicted")
	}
	if _, err := os.Stat(e2); err != nil {
		t.Fatal("newer entry must survive eviction")
	}
}

func TestCacheHashVerify(t *testing.T) {
	freshCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("body"))
	}))
	defer srv.Close()
	want := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("body")))
	// Wrong SHA → download fails, nothing committed.
	if _, err := cachedFetch(srv.URL+"/x", "sha256:"+strings.Repeat("0", 64), false); err == nil {
		t.Fatal("bad hash must fail")
	}
	if n := countEntries(t); n != 0 {
		t.Fatalf("failed download left %d entries", n)
	}
	// Correct SHA → committed + verified.
	p, err := cachedFetch(srv.URL+"/x", want, false)
	if err != nil {
		t.Fatalf("good hash fetch: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "body" {
		t.Fatalf("body %q", b)
	}
}

func TestCacheSubcommand(t *testing.T) {
	dir := freshCache(t)
	u := "http://h/f"
	data, mp := cachePaths(cacheKey(u))
	os.MkdirAll(dir, 0o755)
	os.WriteFile(data, []byte("xx"), 0o644)
	cacheWrite(mp, &cacheMeta{URL: u, Size: 2})
	rc := 0
	out := qetest.CaptureStdout(t, func() { rc = cmdCache([]string{"list"}) })
	if rc != 0 || !strings.Contains(out, u) {
		t.Fatalf("cache list rc=%d out=%q", rc, out)
	}
	if rc := cmdCache([]string{"prune"}); rc != 0 {
		t.Fatalf("prune rc=%d", rc)
	}
	if rc := cmdCache([]string{"clear"}); rc != 0 {
		t.Fatalf("clear rc=%d", rc)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("clear must remove the cache dir")
	}
	if rc := cmdCache([]string{"bogus"}); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
}

func countEntries(t *testing.T) int {
	t.Helper()
	dir := os.Getenv("QE_DL_CACHE")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		nm := e.Name()
		if !strings.HasSuffix(nm, ".meta") && !strings.HasSuffix(nm, ".part") &&
			!strings.HasSuffix(nm, ".lock") {
			n++
		}
	}
	return n
}

func TestGetOSCacheAndOffline(t *testing.T) {
	freshCache(t)
	payload := []byte("qcow payload")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"g1"`)
		w.Write(payload)
	}))
	defer srv.Close()
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-catalog"),
		[]byte("#!/bin/sh\necho '"+srv.URL+"/img.qcow2'\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))

	// --cache: first get downloads + caches.
	old := cacheFlag
	cacheFlag = true
	t.Cleanup(func() { cacheFlag = old })
	vm := filepath.Join(t.TempDir(), "cvm")
	if rc := getOS("cacheos", "1.0", "", vm, "amd64"); rc != 0 {
		t.Fatalf("getOS --cache rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(vm, "img.qcow2")); err != nil {
		t.Fatal("cached get must still deliver the image")
	}
	// --offline: second VM from the cache without the server.
	cacheFlag = false
	old = offlineFlag
	offlineFlag = true
	t.Cleanup(func() { offlineFlag = old })
	vm2 := filepath.Join(t.TempDir(), "cvm2")
	if rc := getOS("cacheos", "1.0", "", vm2, "amd64"); rc != 0 {
		t.Fatalf("getOS --offline rc=%d", rc)
	}
	b, _ := os.ReadFile(filepath.Join(vm2, "img.qcow2"))
	if string(b) != string(payload) {
		t.Fatalf("offline payload %q", b)
	}
	// --offline on a URL never cached fails cleanly.
	stub2 := t.TempDir()
	os.WriteFile(filepath.Join(stub2, "quickget-catalog"),
		[]byte("#!/bin/sh\necho 'https://unseen.invalid/x.qcow2'\n"), 0o755)
	t.Setenv("PATH", stub2+string(os.PathListSeparator)+os.Getenv("PATH"))
	vm3 := filepath.Join(t.TempDir(), "cvm3")
	if rc := getOS("uncached", "1.0", "", vm3, "amd64"); rc == 0 {
		t.Fatal("offline get of uncached URL must fail")
	}
}
