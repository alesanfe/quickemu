// tools/quickget-vm/cache.go: secure download cache — every cached
// artifact keeps its validators (ETag/Last-Modified) so revalidation
// is a conditional GET (304 = reuse); downloads stage into a .part
// tmp file, hash-verify, then rename atomically; a per-URL lockdir
// (ports-registry pattern) serialises concurrent downloads of the
// same URL; total size is capped with LRU eviction; --offline serves
// cache hits only.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// dlCacheDir resolves the download-cache root: $QE_DL_CACHE or
// XDG (~/.cache/quickemu/dl). Var for tests.
var dlCacheDir = func() string {
	if p := os.Getenv("QE_DL_CACHE"); p != "" {
		return p
	}
	return filepath.Join(qelib.XDGCache(), "quickemu", "dl")
}

// dlCacheMax bounds the cache in bytes: $QE_CACHE_MAX (decimal bytes or
// K/M/G suffix) or 10 GiB default. Var for tests.
var dlCacheMax = func() int64 {
	if v := os.Getenv("QE_CACHE_MAX"); v != "" {
		if n, err := parseSize(v); err == nil && n > 0 {
			return n
		}
	}
	return 10 << 30
}

// parseSize accepts "123", "10K", "512M", "2G" byte counts.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "K") || strings.HasSuffix(s, "k"):
		mult, s = 1<<10, s[:len(s)-1]
	case strings.HasSuffix(s, "M") || strings.HasSuffix(s, "m"):
		mult, s = 1<<20, s[:len(s)-1]
	case strings.HasSuffix(s, "G") || strings.HasSuffix(s, "g"):
		mult, s = 1<<30, s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return n * mult, nil
}

// cacheMeta is the sidecar JSON kept beside every cached payload.
type cacheMeta struct {
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	Size         int64  `json:"size"`
	LastUsed     int64  `json:"last_used"`
}

// cacheKey names a cache entry: sha256(url) so any URL maps to a
// filesystem-safe key and collisions are impossible in practice.
func cacheKey(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])
}

func cachePaths(key string) (data, meta string) {
	dir := dlCacheDir()
	return filepath.Join(dir, key), filepath.Join(dir, key+".meta")
}

// cacheLock takes the per-URL lockdir (quickemu-ports registry
// pattern): mkdir is atomic, a stale lock whose pid is dead is
// reclaimed. Returns the unlock func.
func cacheLock(key string) (func(), error) {
	lk := filepath.Join(dlCacheDir(), key+".lock")
	for i := 0; i < 100; i++ {
		if err := os.Mkdir(lk, 0o700); err == nil {
			if err := os.WriteFile(filepath.Join(lk, "pid"),
				[]byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
				os.RemoveAll(lk)
				continue
			}
			return func() { os.RemoveAll(lk) }, nil
		}
		// Reclaim ONLY a provably-dead holder: the pid file must parse
		// cleanly AND the pid must be gone. An empty/partially-written
		// pid file (the window between the winner's mkdir and its
		// pid write) is treated as a live lock — reclaiming it would
		// let two downloads of the same URL run concurrently.
		if b, err := os.ReadFile(filepath.Join(lk, "pid")); err == nil && len(b) > 0 {
			var pid int
			if _, err := fmt.Sscanf(string(b), "%d", &pid); err == nil &&
				pid > 0 && !qelib.Alive(pid) {
				os.RemoveAll(lk)
				continue
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("download for this URL is already in progress")
}

// cacheRead loads a meta sidecar; nil when absent/corrupt.
func cacheRead(meta string) *cacheMeta {
	b, err := os.ReadFile(meta)
	if err != nil {
		return nil
	}
	var m cacheMeta
	if json.Unmarshal(b, &m) != nil || m.URL == "" {
		return nil
	}
	return &m
}

func cacheWrite(meta string, m *cacheMeta) {
	b, err := json.Marshal(m)
	if err == nil {
		os.WriteFile(meta, b, 0o644)
	}
}

// cachedFetch downloads url into the cache and returns the cached path.
// Revalidation uses the stored validators: If-None-Match /
// If-Modified-Since; a 304 refreshes LastUsed and reuses the body.
// offline=true never issues a request — cache hit or error.
func cachedFetch(url, wantSHA string, offline bool) (string, error) {
	key := cacheKey(url)
	data, meta := cachePaths(key)
	if err := os.MkdirAll(dlCacheDir(), 0o755); err != nil {
		return "", err
	}
	unlock, err := cacheLock(key)
	if err != nil {
		return "", err
	}
	defer unlock()
	if m := cacheRead(meta); m != nil {
		if st, err := os.Stat(data); err == nil && st.Size() == m.Size {
			if offline {
				m.LastUsed = time.Now().Unix()
				cacheWrite(meta, m)
				return data, nil
			}
			return cacheRevalidate(url, data, meta, m, wantSHA)
		}
	}
	if offline {
		return "", fmt.Errorf("offline: %s not in cache", url)
	}
	return cacheDownload(url, data, meta, key, nil, wantSHA)
}

// cacheRevalidate issues a conditional GET: 304 keeps the entry (touch
// LastUsed), 200 replaces it via the staging path, other errors keep
// the cached copy (a flaky mirror must not evict a good artifact).
func cacheRevalidate(url, data, meta string, m *cacheMeta, wantSHA string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	if m.ETag != "" {
		req.Header.Set("If-None-Match", m.ETag)
	}
	if m.LastModified != "" {
		req.Header.Set("If-Modified-Since", m.LastModified)
	}
	resp, err := httpDL.Do(req)
	if err != nil {
		// Network trouble: prefer the known-good cached copy.
		return data, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		m.LastUsed = time.Now().Unix()
		cacheWrite(meta, m)
		return data, nil
	}
	if resp.StatusCode != http.StatusOK {
		return data, nil
	}
	return cacheStore(url, data, meta, m, resp, wantSHA)
}

// cacheStore drains resp into <key>.part, verifies wantSHA (when the
// catalog gave one) or records the computed digest, renames into place
// and refreshes the validators from the response.
func cacheStore(url, data, meta string, m *cacheMeta, resp *http.Response, wantSHA string) (string, error) {
	part := data + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(f, io.TeeReader(resp.Body, h))
	if cerr := f.Close(); err == nil && cerr != nil {
		err = cerr
	}
	if err != nil {
		os.Remove(part)
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if wantSHA != "" {
		if rc := fetchVerify(part, wantSHA); rc != 0 {
			return "", fmt.Errorf("%s failed checksum verification", url)
		}
		sum = wantSHA
	}
	if err := os.Rename(part, data); err != nil {
		return "", err
	}
	if m == nil {
		m = &cacheMeta{}
	}
	m.URL = url
	m.SHA256 = sum
	m.Size = n
	m.LastUsed = time.Now().Unix()
	m.ETag = resp.Header.Get("ETag")
	m.LastModified = resp.Header.Get("Last-Modified")
	cacheWrite(meta, m)
	evictLRU()
	return data, nil
}

// cacheDownload performs the first fetch of a URL: same staging
// (Range resume on the .part via fetchOnce, which handles 206/416),
// verify, rename, meta. Reuses fetchOnce so resume/stall-watchdog
// semantics match the plain fetch path.
func cacheDownload(url, data, meta, key string, m *cacheMeta, wantSHA string) (string, error) {
	part := data + ".part"
	if fetchOnce(url, part, wantSHA) != 0 {
		return "", fmt.Errorf("download failed: %s", url)
	}
	if err := os.Rename(part, data); err != nil {
		return "", err
	}
	st, err := os.Stat(data)
	if err != nil {
		return "", err
	}
	if m == nil {
		m = &cacheMeta{}
	}
	m.URL = url
	m.Size = st.Size()
	m.LastUsed = time.Now().Unix()
	// fetchOnce does not surface the headers — a HEAD probe fills the
	// validators so the next lookup can still go conditional.
	if req, err := http.NewRequest("HEAD", url, nil); err == nil {
		if resp, err := httpProbe.Do(req); err == nil {
			m.ETag = resp.Header.Get("ETag")
			m.LastModified = resp.Header.Get("Last-Modified")
			resp.Body.Close()
		}
	}
	if wantSHA != "" {
		m.SHA256 = wantSHA
	}
	cacheWrite(meta, m)
	evictLRU()
	return data, nil
}

// evictLRU drops least-recently-used entries until the cache fits
// dlCacheMax(). Lock dirs and in-flight .part files are skipped —
// an entry being downloaded by another process is never evicted.
func evictLRU() {
	dir := dlCacheDir()
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type item struct {
		path string
		size int64
		used int64
	}
	var items []item
	var total int64
	for _, e := range ents {
		n := e.Name()
		if strings.HasSuffix(n, ".meta") || strings.HasSuffix(n, ".lock") ||
			strings.HasSuffix(n, ".part") {
			continue
		}
		p := filepath.Join(dir, n)
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		used := st.ModTime().Unix()
		if m := cacheRead(p + ".meta"); m != nil && m.LastUsed > 0 {
			used = m.LastUsed
		}
		items = append(items, item{p, st.Size(), used})
		total += st.Size()
	}
	sort.Slice(items, func(i, j int) bool { return items[i].used < items[j].used })
	for _, it := range items {
		if total <= dlCacheMax() {
			break
		}
		key := filepath.Base(it.path)
		if _, err := os.Stat(filepath.Join(dir, key+".lock")); err == nil {
			continue // in-flight download — never evict under the lock
		}
		if os.Remove(it.path) == nil {
			os.Remove(it.path + ".meta")
			total -= it.size
		}
	}
}

// cmdCache implements `quickget-vm cache <list|prune|clear>` —
// operator-facing cache management.
func cmdCache(args []string) int {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	dir := dlCacheDir()
	switch sub {
	case "list", "":
		ents, _ := os.ReadDir(dir)
		var total int64
		for _, e := range ents {
			n := e.Name()
			if strings.HasSuffix(n, ".meta") || strings.HasSuffix(n, ".lock") ||
				strings.HasSuffix(n, ".part") {
				continue
			}
			p := filepath.Join(dir, n)
			st, err := os.Stat(p)
			if err != nil || !st.Mode().IsRegular() {
				continue
			}
			total += st.Size()
			url := n
			if m := cacheRead(p + ".meta"); m != nil {
				url = m.URL
			}
			fmt.Printf("%10d  %s\n", st.Size(), url)
		}
		fmt.Printf("%10d  total (limit %d)\n", total, dlCacheMax())
		return 0
	case "prune":
		evictLRU()
		return cmdCache([]string{"list"})
	case "clear":
		os.RemoveAll(dir)
		return 0
	}
	fmt.Println("usage: quickget-vm cache <list|prune|clear>")
	return 2
}
