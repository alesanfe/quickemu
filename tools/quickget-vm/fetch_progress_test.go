// fetch_progress_test.go — QuickGUI's downloader parses curl-style
// " N.N%" lines from quickget's stderr (RegExp "( [0-9.]+%)"). These
// tests pin that contract: the meter must emit parseable ticks on
// stderr and land on 100.0% at completion.
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// captureStderr swaps os.Stderr for a pipe, runs fn, and returns the
// captured bytes.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestProgressPrinterTicks(t *testing.T) {
	out := captureStderr(t, func() {
		p := &progressPrinter{total: 1000}
		// Tick cadence is ~1s — force the window by aging `last`.
		p.add(250)
		p.last = time.Now().Add(-2 * time.Second)
		p.add(250)
		p.last = time.Now().Add(-2 * time.Second)
		p.add(500)
		p.finish()
	})
	// QuickGUI's regex requires " <digits>[.<digits>]%" — a leading
	// space, a number, a percent sign.
	re := regexp.MustCompile(` [0-9.]+%`)
	matches := re.FindAllString(out, -1)
	if len(matches) < 2 {
		t.Fatalf("expected multiple %%-ticks, got %q", out)
	}
	if matches[len(matches)-1] != " 100.0%" {
		t.Fatalf("final tick must be 100.0%%, got %q", matches)
	}
}

func TestProgressPrinterNoTotal(t *testing.T) {
	// Unknown Content-Length → no % lines (GUI shows indeterminate).
	out := captureStderr(t, func() {
		p := &progressPrinter{total: 0}
		p.add(500)
		p.finish()
	})
	if out != "" {
		t.Fatalf("unknown length must stay silent, got %q", out)
	}
}

func TestFetchEmitsCurlProgress(t *testing.T) {
	payload := make([]byte, 4<<20) // 4 MiB — Content-Length known
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		w.Write(payload)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "img.iso")
	var rc int
	out := captureStderr(t, func() {
		rc = fetch(srv.URL+"/img.iso", dest, "")
	})
	if rc != 0 {
		t.Fatalf("fetch rc=%d", rc)
	}
	if st, err := os.Stat(dest); err != nil || st.Size() != int64(len(payload)) {
		t.Fatalf("dest %v size=%v", err, st)
	}
	re := regexp.MustCompile(` [0-9.]+%`)
	if !re.MatchString(out) {
		t.Fatalf("no curl-style progress on stderr: %q", out)
	}
	m := re.FindAllString(out, -1)
	if m[len(m)-1] != " 100.0%" {
		t.Fatalf("last tick must be 100.0%%, got %v", m)
	}
}
