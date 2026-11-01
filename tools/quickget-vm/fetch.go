// tools/quickget-vm/fetch.go: image download + stall-watchdog + hash verify pipeline
package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
	"golang.org/x/crypto/blake2b"
)

// fetch: web_get/check_hash parity — download url -> dest via a .part
// staging file (atomic rename), resume a partial download with the
// Range header, bounded retries (curl --retry 3), http(s)-only scheme
// restriction, and sha256 verify when provided. Uses net/http so no
// curl/zsync dependency is required.
var fetch = func(url, dest, wantSHA string) int {
	if u, err := neturl.Parse(url); err != nil ||
		(u.Scheme != "http" && u.Scheme != "https") {
		fmt.Printf("ERROR! unsupported URL scheme: %s\n", url)
		return 1
	}
	part := dest + ".part"
	for attempt := 1; attempt <= 3; attempt++ {
		if fetchOnce(url, part, wantSHA) == 0 {
			break
		}
		if attempt == 3 {
			return 1
		}
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	if err := os.Rename(part, dest); err != nil {
		fmt.Println(err)
		return 1
	}
	fmt.Println(" - Downloaded " + dest)
	return 0
}

func fetchOnce(url, part, wantSHA string) int {
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := httpDL.Do(req)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	defer resp.Body.Close()
	// 416 with a staged file means the local .part is already at/past
	// the remote size — treat as complete and let the SHA verify below
	// decide correctness (curl parity: a finished .part resumes to done).
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && have > 0 {
		return fetchVerify(part, wantSHA)
	}
	if resp.StatusCode >= 400 {
		fmt.Printf("download failed: HTTP %d\n", resp.StatusCode)
		return 1
	}
	// Server ignored Range -> restart the staging file.
	if have > 0 && resp.StatusCode != http.StatusPartialContent {
		have = 0
	}
	flag := os.O_CREATE | os.O_WRONLY
	if have > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(part, flag, 0644)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	// curl --speed-limit 1024 --speed-time 60 parity: if the body
	// stalls for 60s the connection is dropped — io.Copy returns an
	// error and the outer retry resumes from the .part.
	var lastRead atomic.Int64
	lastRead.Store(time.Now().UnixNano())
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(5 * time.Second):
				if time.Since(time.Unix(0, lastRead.Load())) > 60*time.Second {
					resp.Body.Close()
					return
				}
			}
		}
	}()
	prog := &progressPrinter{
		total: resp.ContentLength + have, have: have,
	}
	_, err = io.Copy(f, &stallWatchReader{r: resp.Body, last: &lastRead, prog: prog})
	prog.finish()
	close(done)
	if err != nil {
		f.Close()
		fmt.Println(err)
		return 1
	}
	// A failing Close (ENOSPC on delayed-alloc filesystems, NFS) can
	// leave the staged .part short — verify only a fully-flushed file.
	if err := f.Close(); err != nil {
		fmt.Println(err)
		return 1
	}
	return fetchVerify(part, wantSHA)
}

// stallWatchReader pings lastRead on every chunk so the watchdog in
// fetchOnce can drop a stalled connection.
type stallWatchReader struct {
	r    io.Reader
	last *atomic.Int64
	prog *progressPrinter
}

func (s *stallWatchReader) Read(b []byte) (int, error) {
	n, err := s.r.Read(b)
	if n > 0 {
		s.last.Store(time.Now().UnixNano())
		if s.prog != nil {
			s.prog.add(int64(n))
		}
	}
	return n, err
}

// progressPrinter emits curl-style " N.N%" lines on stderr — the
// meter QuickGUI's downloader parses (RegExp "( [0-9.]+%)") for its
// progress bar. Rate-limited to ~1 line/sec like curl's TTY meter;
// a no-op when Content-Length is unknown (GUI shows indeterminate).
type progressPrinter struct {
	total int64
	have  int64 // resume offset — % counts the whole file
	last  time.Time
}

func (p *progressPrinter) add(n int64) {
	p.have += n
	if p.total <= 0 || time.Since(p.last) < time.Second {
		return
	}
	p.last = time.Now()
	fmt.Fprintf(os.Stderr, " %.1f%%\n", float64(p.have)*100/float64(p.total))
}

// finish lands the terminal 100.0% tick so the GUI's bar always
// completes even when the last increment fell inside the 1s window.
func (p *progressPrinter) finish() {
	if p.total > 0 && p.have >= p.total {
		fmt.Fprintln(os.Stderr, " 100.0%")
	}
}

// fetchVerify: check_hash parity — verify the staged .part (streaming;
// multi-GB images never enter memory) and drop it on mismatch. Called
// after a completed body and on the 416 already-complete path. The
// expected digest carries an optional "algo:" prefix (b2sum/blake2
// included); bare digests pick the algorithm by hex length.
func fetchVerify(part, wantSHA string) int {
	if wantSHA == "" {
		return 0
	}
	want, h, algo := hashFor(wantSHA)
	if h == nil {
		if strings.Contains(wantSHA, ":") {
			fmt.Printf("WARNING! Unknown hash algorithm '%s', not checking %s hash.\n",
				strings.ToLower(wantSHA[:strings.Index(wantSHA, ":")]), part)
		} else {
			fmt.Printf("WARNING! Can't guess hash algorithm, not checking %s hash.\n", part)
		}
		return 0
	}
	disp := strings.TrimSuffix(part, ".part")
	fmt.Printf("Checking %s with %ssum... ", disp, algo)
	f, err := os.Open(part)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		fmt.Println(err)
		return 1
	}
	sum := fmt.Sprintf("%x", h.Sum(nil))
	if !strings.EqualFold(sum, want) {
		os.Remove(part)
		fmt.Printf("ERROR!\n%s doesn't match %s. Try running 'quickget' again.\n", disp, want)
		return 1
	}
	fmt.Println("Good!")
	return 0
}

// hashFor resolves the digest string to (bare-hex, hasher): explicit
// "algo:hex" prefixes win over the length guess — 32 md5, 40 sha1
// (trisquel), 64 sha256, 128 sha512.
func hashFor(digest string) (string, hash.Hash, string) {
	if i := strings.Index(digest, ":"); i >= 0 {
		algo := strings.ToLower(digest[:i])
		digest = digest[i+1:]
		switch algo {
		case "md5":
			return digest, md5.New(), "md5"
		case "sha1":
			return digest, sha1.New(), "sha1"
		case "sha256":
			return digest, sha256.New(), "sha256"
		case "sha512":
			return digest, sha512.New(), "sha512"
		case "b2sum", "blake2", "blake2b":
			b, err := blake2b.New512(nil)
			if err != nil {
				return "", nil, ""
			}
			return digest, b, "b2"
		}
		return "", nil, "" // unknown prefix: warn+skip like Bash
	}
	switch len(digest) {
	case 32:
		return digest, md5.New(), "md5"
	case 40:
		return digest, sha1.New(), "sha1"
	case 64:
		return digest, sha256.New(), "sha256"
	case 128:
		return digest, sha512.New(), "sha512"
	}
	return "", nil, ""
}

// prettyNameFor: PRETTY_OS_NAME parity — the catalog info Name field
// (pretty_name), falling back to the bare OS name on any miss.
func prettyNameFor(osName string) string {
	if cat, err := qelib.FindTool("quickget-catalog"); err == nil {
		if out, err := execCommand(cat, "info", osName).Output(); err == nil {
			if f := strings.SplitN(strings.TrimSpace(string(out)), "|", 2); f[0] != "" {
				return f[0]
			}
		}
	}
	return osName
}
