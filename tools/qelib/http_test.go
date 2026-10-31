package qelib

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// Regression: FinalURL ran on DLClient (no Timeout — correct for
// multi-GB payloads) so a hung redirect target stalled quickget
// forever. It must ride a bounded client.
func TestFinalURLBoundedClient(t *testing.T) {
	if RedirClient.Timeout <= 0 {
		t.Fatal("RedirClient must be bounded")
	}
	if RedirClient.CheckRedirect == nil {
		t.Fatal("RedirClient must keep the scheme/hop guard")
	}
	// Swap in a short-timeout client and hit a server that never
	// answers. The handler must still return once the client gives
	// up — a bare select{} would deadlock httptest.Server.Close.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() { close(release); srv.Close() }()
	old := RedirClient
	RedirClient = &http.Client{Timeout: 150 * time.Millisecond,
		Transport: HTTPTransport, CheckRedirect: redirectGuard}
	defer func() { RedirClient = old }()
	done := make(chan error, 1)
	go func() { _, err := FinalURL(srv.URL); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("hung server must error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FinalURL unbounded — hung server blocks forever")
	}
}

// The scheme guard must apply on the metadata path too — a redirect
// to file:// must not resolve.
func TestFinalURLRejectsSchemeRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "file:///etc/passwd")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	if u, err := FinalURL(srv.URL); err == nil {
		t.Fatalf("file:// redirect must be refused, got %q", u)
	}
}

func TestTLSInsecure(t *testing.T) {
	t.Setenv("QUICKGET_INSECURE", "")
	if TLSInsecure() {
		t.Fatal("empty env must not enable insecure")
	}
	t.Setenv("QUICKGET_INSECURE", "1")
	if !TLSInsecure() {
		t.Fatal("QUICKGET_INSECURE=1 must enable insecure")
	}
	t.Setenv("QUICKGET_INSECURE", "true")
	if !TLSInsecure() {
		t.Fatal("QUICKGET_INSECURE=true must enable insecure")
	}
}
