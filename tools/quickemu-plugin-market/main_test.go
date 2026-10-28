package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const fixture = `{"templates":[
	{"name":"win11-secboot","conf":"guest_os=windows\nsecureboot=true\n"},
	{"name":"macos-sonoma","conf":"guest_os=macos\nmacos_release=sonoma\n"}
]}`

func TestLoadFromFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "market.json")
	if err := os.WriteFile(f, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MARKET_CATALOG", f)
	c := load()
	if len(c.Templates) != 2 || c.Templates[0].Name != "win11-secboot" {
		t.Fatalf("load: %+v", c.Templates)
	}
}

func TestLoadFromHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(fixture))
	}))
	defer srv.Close()
	t.Setenv("MARKET_CATALOG", srv.URL+"/market.json")
	c := load()
	if len(c.Templates) != 2 {
		t.Fatalf("load over http: %+v", c.Templates)
	}
}

func TestFind(t *testing.T) {
	var c catalog
	c.Templates = append(c.Templates, template{Name: "a"}, template{Name: "b"})
	if find(c, "b") == nil || find(c, "b").Name != "b" {
		t.Fatal("find hit")
	}
	if find(c, "zzz") != nil {
		t.Fatal("find miss must be nil")
	}
}

// install writes <name>.conf with exactly one trailing newline.
func TestWriteConf(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x.conf")
	if err := writeConf(f, "a=1\n\n\n"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	if string(b) != "a=1\n" {
		t.Fatalf("conf not normalized: %q", b)
	}
}
