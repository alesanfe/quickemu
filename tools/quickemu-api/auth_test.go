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

func withToken(t *testing.T, tok string) {
	t.Helper()
	old, oldFails := tokens, authFails
	tokens, authFails = []string{tok}, map[string]*failRec{}
	t.Cleanup(func() { tokens, authFails = old, oldFails })
}

func req() *httptest.ResponseRecorder { return httptest.NewRecorder() }

func TestAuthAdversarial(t *testing.T) {
	withToken(t, "0123456789abcdef")
	h := auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })

	cases := []struct {
		name, hdr string
		want      int
	}{
		{"correct", "Bearer 0123456789abcdef", 200},
		{"no header", "", 401},
		{"wrong token", "Bearer nope", 401},
		{"empty bearer", "Bearer ", 401},
		{"no scheme", "0123456789abcdef", 401},
		{"basic scheme", "Basic 0123456789abcdef", 401},
		{"token without prefix", "bearer 0123456789abcdef", 401},
		{"extra spaces", "Bearer  0123456789abcdef", 401},
		{"trailing space", "Bearer 0123456789abcdef ", 401},
		{"token prefix match", "Bearer 0123456789abcde", 401},
		{"token with suffix", "Bearer 0123456789abcdefX", 401},
		{"token embedded", "Bearer x0123456789abcdef", 401},
	}
	for i, c := range cases {
		r := httptest.NewRequest("GET", "/v1/vms", nil)
		r.RemoteAddr = fmt.Sprintf("10.0.0.%d:1", i+1) // distinct IPs → no throttle bleed
		if c.hdr != "" {
			r.Header.Set("Authorization", c.hdr)
		}
		rec := req()
		h.ServeHTTP(rec, r)
		if rec.Code != c.want {
			t.Errorf("%s: got %d want %d", c.name, rec.Code, c.want)
		}
		if c.want != 200 && strings.Contains(rec.Body.String(), "0123456789abcdef") {
			t.Errorf("%s: token leaked into error body: %s", c.name, rec.Body.String())
		}
	}
}

func TestAuthThrottle(t *testing.T) {
	withToken(t, "0123456789abcdef")
	h := auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	r := func() *httptest.ResponseRecorder {
		q := httptest.NewRequest("GET", "/v1/vms", nil)
		q.RemoteAddr = "198.51.100.7:9"
		q.Header.Set("Authorization", "Bearer wrong")
		rec := req()
		h.ServeHTTP(rec, q)
		return rec
	}
	for i := 0; i < authFailMax; i++ {
		if rec := r(); rec.Code != 401 {
			t.Fatalf("attempt %d: %d", i, rec.Code)
		}
	}
	if rec := r(); rec.Code != 429 {
		t.Fatalf("throttled attempt: %d, want 429", rec.Code)
	}
}

// Rotation window: old+new tokens both valid until the old one is
// removed — clients migrate without an insecure gap.
func TestTokenRotation(t *testing.T) {
	withToken(t, "old-token-00000000")
	tokens = append(tokens, "new-token-00000000")
	h := auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	for _, tok := range []string{"old-token-00000000", "new-token-00000000"} {
		r := httptest.NewRequest("GET", "/v1/vms", nil)
		r.RemoteAddr = "10.9.9.9:1"
		r.Header.Set("Authorization", "Bearer "+tok)
		rec := req()
		h.ServeHTTP(rec, r)
		if rec.Code != 200 {
			t.Fatalf("%s: %d", tok, rec.Code)
		}
	}
}

func TestIsLocalBind(t *testing.T) {
	for listen, want := range map[string]bool{
		"/run/quickemu/api.sock": true,
		"127.0.0.1:8490":         true,
		"[::1]:8490":             true,
		"localhost:8490":         true,
		"0.0.0.0:8490":           false,
		"[::]:8490":              false,
		":8490":                  false, // binds every interface
		"192.168.1.5:8490":       false,
		"garbage":                false,
	} {
		if got := isLocalBind(listen); got != want {
			t.Errorf("isLocalBind(%q)=%v want %v", listen, got, want)
		}
	}
}

func TestReadTokenFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "tok")
	if err := os.WriteFile(good, []byte("0123456789abcdef\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if tok, err := readTokenFile(good); err != nil || tok != "0123456789abcdef" {
		t.Fatalf("readTokenFile: %q %v", tok, err)
	}
	world := filepath.Join(dir, "world")
	os.WriteFile(world, []byte("x"), 0644)
	if _, err := readTokenFile(world); err == nil {
		t.Fatal("world-readable token file must be rejected")
	}
	if _, err := readTokenFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing token file must error")
	}
}
