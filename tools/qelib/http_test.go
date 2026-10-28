package qelib

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// HEAD-blocked servers (presigned URLs, some CDNs) must fall back to
// GET Range:bytes=0-0 — regression for ubuntu.go/quickgetc false
// negatives when they gained the shared client.
func TestWebCheckHeadFallbackGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("Range") != "bytes=0-0" {
			t.Error("fallback must be a Range request")
		}
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer srv.Close()
	if !WebCheck(srv.URL, nil) {
		t.Fatal("HEAD 403 + GET 206 must pass")
	}
}

func TestWebCheckTotalFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if WebCheck(srv.URL, nil) {
		t.Fatal("500 on both must fail")
	}
	if WebCheck("http://127.0.0.1:1/", nil) {
		t.Fatal("unreachable must fail")
	}
}

// FetchString must bound the body — an unbounded read was how the
// Bash curl path could OOM on a hostile/huge response.
func TestFetchStringBound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strings.Repeat("x", 1<<20)))
	}))
	defer srv.Close()
	s, err := FetchString(srv.URL, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 10 {
		t.Fatalf("bound ignored: got %d bytes", len(s))
	}
}

func TestFetchStringNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := FetchString(srv.URL, 100); err == nil {
		t.Fatal("404 must error")
	}
}

// DLClient must refuse to follow redirects out of http(s) —
// redirect->file:// would turn a downloader into a local-file read.
func TestDLClientRejectsSchemeRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "file:///etc/passwd")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	resp, err := DLClient.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("file:// redirect must be refused")
	}
}

// Redirects inside http(s) work, and the hop cap stops loops.
func TestDLClientRedirectCap(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, httptest.NewRequest("GET", srv.URL+"/loop", nil), "/loop", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := DLClient.Get(srv.URL); err == nil ||
		!strings.Contains(err.Error(), "redirect") {
		t.Fatalf("redirect loop must hit the cap, got %v", err)
	}
}

// A body cut short of its declared Content-Length must surface an
// error — silently accepting a truncated SUMS/catalog response would
// corrupt downstream parsing (LimitReader only caps the top).
func TestFetchStringTruncatedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.Write([]byte("short"))
	}))
	defer srv.Close()
	if _, err := FetchString(srv.URL, 1<<20); err == nil {
		t.Fatal("truncated declared-length body must error")
	}
}

func TestFinalURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/end" {
			w.Write([]byte("ok"))
			return
		}
		http.Redirect(w, r, "/end", http.StatusFound)
	}))
	defer srv.Close()
	u, err := FinalURL(srv.URL + "/start")
	if err != nil || u != srv.URL+"/end" {
		t.Fatalf("FinalURL=%q %v", u, err)
	}
}
