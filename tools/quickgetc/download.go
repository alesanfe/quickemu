// download.go: cache populate + web transport + checksum + archive
// extraction — download/{cache,web}.sh parity. Compression is pure
// Go (klauspost zstd, ulikunitz xz, stdlib gz/bz2/zip/tar) so no
// external decompressor binaries are needed.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const (
	cacheFile = "quickget_data.json.zst"
	cacheJSON = "quickget_data.json"
)

// cacheURL is a var so tests can point the daily-catalog download at a
// fixture server (same seam as checkClient elsewhere).
var cacheURL = "https://github.com/lj3954/quickget_configs/releases/download/daily/quickget_data.json.zst"

// HTTP clients come from qelib — same transport policy everywhere:
// probes die in 30s, downloads bound dial+header but stream
// unbounded, redirect scheme-guard + cap (curl parity).
var (
	httpProbe = qelib.ProbeClient
	httpDL    = qelib.DLClient
)

const qgcUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"

// cacheIsValid: cache file must be from the current UTC day.
func cacheIsValid(p string) bool {
	st, err := os.Stat(p)
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	m := st.ModTime().UTC()
	return now.Year() == m.Year() && now.YearDay() == m.YearDay()
}

// populateCache: download the daily dataset if stale, then
// zstd-decompress it into the catalog model.
func populateCache() []OSEntry {
	// QUICKGETC_CATALOG=<path> loads an uncompressed JSON catalog from
	// anywhere (offline catalogs, pinned datasets, hermetic tests) —
	// it wins over the cache and skips the download entirely.
	if override := os.Getenv("QUICKGETC_CATALOG"); override != "" {
		if raw, err := os.ReadFile(override); err == nil {
			var cat []OSEntry
			if err := json.Unmarshal(raw, &cat); err == nil {
				return cat
			}
		}
	}
	cacheDir := qelib.XDGCache()
	// A plain quickget_data.json next to the cache is an authoritative
	// override (offline catalogs, pinned datasets, tests) — it skips
	// both the download and the zstd decode entirely.
	if raw, err := os.ReadFile(filepath.Join(cacheDir, cacheJSON)); err == nil {
		var cat []OSEntry
		if err := json.Unmarshal(raw, &cat); err == nil {
			return cat
		}
	}
	p := filepath.Join(cacheDir, cacheFile)
	if !cacheIsValid(p) {
		fmt.Println("Downloading quickget data from " + cacheURL)
		os.MkdirAll(cacheDir, 0755)
		if webGet(cacheURL, cacheDir, cacheFile, nil) != 0 {
			fmt.Println("ERROR! Failed to download catalog data.")
			os.Exit(1)
		}
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		fmt.Printf("ERROR! Failed to read %s\n", p)
		os.Exit(1)
	}
	zr, err := zstd.NewReader(nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer zr.Close()
	data, err := zr.DecodeAll(raw, nil)
	if err != nil {
		fmt.Printf("ERROR! Failed to decompress %s. Try running quickgetc again.\n", p)
		os.Remove(p)
		os.Exit(1)
	}
	var cat []OSEntry
	if err := json.Unmarshal(data, &cat); err != nil {
		fmt.Printf("ERROR! Failed to parse catalog data: %v\n", err)
		os.Exit(1)
	}
	return cat
}

// webGet: redirect resolution + resume download with the Chrome UA.
func webGet(url, dir, file string, headers [][2]string) int {
	url = webRedirect(url)
	if file == "" {
		file = filepath.Base(url)
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("ERROR! Unable to create directory %s\n", dir)
			return 1
		}
	}
	dest := filepath.Join(dir, file)
	var have int64
	if st, err := os.Stat(dest); err == nil {
		have = st.Size()
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	req.Header.Set("User-Agent", qgcUA)
	for _, h := range headers {
		req.Header.Set(h[0], h[1])
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := httpDL.Do(req)
	if err != nil {
		fmt.Printf("ERROR! Failed to download %s\n", url)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && have > 0 {
		// Resume offset at or beyond the remote size: the local file is
		// already complete — keep it (decode/parse validates it below).
		return 0
	}
	if resp.StatusCode >= 400 {
		fmt.Printf("ERROR! Failed to download %s (HTTP %d)\n", url, resp.StatusCode)
		return 1
	}
	flag := os.O_CREATE | os.O_WRONLY
	if have > 0 && resp.StatusCode == http.StatusPartialContent {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
		have = 0
	}
	f, err := os.OpenFile(dest, flag, 0644)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		fmt.Printf("ERROR! Failed to download %s\n", url)
		os.Remove(dest)
		return 1
	}
	return 0
}

// webRedirect: resolve final URL after redirects (HEAD).
func webRedirect(url string) string {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return url
	}
	resp, err := httpProbe.Do(req)
	if err != nil {
		return url
	}
	resp.Body.Close()
	if resp.Request.URL.String() != url {
		return resp.Request.URL.String()
	}
	return url
}

// webCheck: reachability probe (HEAD, GET-range fallback inside).
func webCheck(url string, headers [][2]string) bool {
	h := map[string]string{}
	for _, kv := range headers {
		h[kv[0]] = kv[1]
	}
	return qelib.WebCheck(url, h)
}

// checkHash: guess the hash algorithm by hex length and verify —
// md5(32)/sha1(40)/sha256(64)/sha512(128).
func checkHash(path, want string) bool {
	var h hash.Hash
	var name string
	switch len(want) {
	case 32:
		h, name = md5.New(), "md5sum"
	case 40:
		h, name = sha1.New(), "sha1sum"
	case 64:
		h, name = sha256.New(), "sha256sum"
	case 128:
		h, name = sha512.New(), "sha512sum"
	default:
		fmt.Printf("WARNING! Can't guess hash algorithm, not checking %s hash.\n", path)
		return true
	}
	fmt.Printf("Checking %s with %s... ", path, name)
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("ERROR!")
		return false
	}
	h.Write(b)
	if fmt.Sprintf("%x", h.Sum(nil)) != strings.ToLower(want) {
		fmt.Println("ERROR!")
		fmt.Printf("%s doesn't match %s. Try running 'quickgetc' again.\n", path, want)
		return false
	}
	fmt.Println("Good!")
	return true
}

// extractArchive: decompress a downloaded source by archive_format
// (tar*/xz/gz/bz2/zip). Returns the final file path.
func extractArchive(format, file, vmPath, imageType string) string {
	open := func() (io.ReadCloser, error) {
		return os.Open(file)
	}
	switch {
	case strings.HasPrefix(format, "tar"):
		rc, err := open()
		if err != nil {
			return file
		}
		defer rc.Close()
		tr := tar.NewReader(rc)
		for {
			hdr, err := tr.Next()
			if err != nil {
				break
			}
			dest := filepath.Join(vmPath, hdr.Name)
			if hdr.FileInfo().IsDir() {
				os.MkdirAll(dest, 0755)
				continue
			}
			os.MkdirAll(filepath.Dir(dest), 0755)
			w, err := os.Create(dest)
			if err != nil {
				continue
			}
			io.Copy(w, tr)
			w.Close()
		}
		if m, _ := filepath.Glob(filepath.Join(vmPath, "*."+imageType)); len(m) > 0 {
			return m[0]
		}
		return file
	case format == "xz":
		rc, err := open()
		if err != nil {
			return file
		}
		defer rc.Close()
		xr, err := xz.NewReader(rc)
		if err != nil {
			return file
		}
		out := strings.TrimSuffix(file, ".xz")
		w, err := os.Create(out)
		if err != nil {
			return file
		}
		io.Copy(w, xr)
		w.Close()
		return out
	case format == "gz":
		rc, err := open()
		if err != nil {
			return file
		}
		defer rc.Close()
		gr, err := gzip.NewReader(rc)
		if err != nil {
			return file
		}
		out := strings.TrimSuffix(file, ".gz")
		w, err := os.Create(out)
		if err != nil {
			return file
		}
		io.Copy(w, gr)
		w.Close()
		return out
	case format == "bz2":
		rc, err := open()
		if err != nil {
			return file
		}
		defer rc.Close()
		out := strings.TrimSuffix(file, ".bz2")
		w, err := os.Create(out)
		if err != nil {
			return file
		}
		io.Copy(w, bzip2.NewReader(rc))
		w.Close()
		return out
	case format == "zip":
		zr, err := zip.OpenReader(file)
		if err != nil {
			return file
		}
		defer zr.Close()
		for _, zf := range zr.File {
			if zf.FileInfo().IsDir() {
				os.MkdirAll(filepath.Join(vmPath, zf.Name), 0755)
				continue
			}
			rc, err := zf.Open()
			if err != nil {
				continue
			}
			dest := filepath.Join(vmPath, zf.Name)
			os.MkdirAll(filepath.Dir(dest), 0755)
			w, err := os.Create(dest)
			if err == nil {
				io.Copy(w, rc)
				w.Close()
			}
			rc.Close()
		}
		if m, _ := filepath.Glob(filepath.Join(vmPath, "*."+imageType)); len(m) > 0 {
			return m[0]
		}
		return file
	}
	return file
}

// copyFile: EXDEV fallback for os.Rename when the input image lives on
// a different filesystem than the VM dir.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
