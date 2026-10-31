package main

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *tDoc {
	t.Helper()
	d, err := parseTOML("test.toml", src)
	if err != nil {
		t.Fatalf("parseTOML: %v", err)
	}
	return d
}

func TestTOMLSectionsAndKeys(t *testing.T) {
	d := mustParse(t, `
# comment line
[env]
name = "demo"       # trailing comment
version = 1

[[vm]]
name = "a"
cpu = 4
big = 1_000
flag = true
ratio = 1.5

[[vm]]
name = 'literal'
`)
	env := d.singles["env"]
	if env.vals["name"].s != "demo" || env.vals["version"].i != 1 {
		t.Fatalf("env section wrong: %+v", env.vals)
	}
	if len(d.arrays["vm"]) != 2 {
		t.Fatalf("want 2 vms, got %d", len(d.arrays["vm"]))
	}
	v0 := d.arrays["vm"][0]
	if v0.vals["cpu"].i != 4 || !v0.vals["flag"].b || v0.vals["ratio"].f != 1.5 || v0.vals["big"].i != 1000 {
		t.Fatalf("scalars wrong: %+v", v0.vals)
	}
	if d.arrays["vm"][1].vals["name"].s != "literal" {
		t.Fatal("literal string failed")
	}
	if env.pos["name"][0] != 4 {
		t.Fatalf("name key pos want 4, got %v", env.pos["name"])
	}
	if v0.vals["cpu"].line != 9 {
		t.Fatalf("cpu line want 9, got %d", v0.vals["cpu"].line)
	}
}

func TestTOMLArrays(t *testing.T) {
	d := mustParse(t, `[[vm]]
name = "a"
depends_on = ["b", "c"]
nets = [
  "n1",
  "n2",  # trailing comma below
]
empty = []
`)
	v := d.arrays["vm"][0]
	deps, err := v.vals["depends_on"].strs()
	if err != nil || len(deps) != 2 || deps[0] != "b" || deps[1] != "c" {
		t.Fatalf("depends_on: %v %v", deps, err)
	}
	nets, _ := v.vals["nets"].strs()
	if len(nets) != 2 || nets[0] != "n1" || nets[1] != "n2" {
		t.Fatalf("multi-line nets: %v", nets)
	}
	if l, _ := v.vals["empty"].strs(); len(l) != 0 {
		t.Fatal("empty array")
	}
}

func TestTOMLEscapes(t *testing.T) {
	d := mustParse(t, `x = "a\"b\\c\td"`+"\n"+`y = 'no\escape'`)
	v := d.globals.vals["x"]
	if v.s != `a"b\c`+"\td" {
		t.Fatalf("escapes: %q", v.s)
	}
	if d.globals.vals["y"].s != `no\escape` {
		t.Fatalf("literal: %q", d.globals.vals["y"].s)
	}
}

func TestTOMLErrors(t *testing.T) {
	for _, tc := range []struct {
		src     string
		wantSub string
	}{
		{"[env\nx=1", "unterminated"},
		{"[[vm\nx=1", "unterminated"},
		{"[env] junk", "unexpected text"},
		{"x = ", "missing value"},
		{"x = \"unclosed", "unterminated"},
		{"x = [\"a\", [1]]", "nested arrays"},
		{"x = 2020-01-01", "unsupported"},
		{"x = tru", "invalid value"},
		{"x = [\"a\" \"b\"]", "expected ','"},
		{"foo bar = 1", "invalid key"},
		{"= 1", "expected key"},
		{"x = \"a\" tail", "trailing text"},
		{"x = [1,2", "unterminated array"},
	} {
		_, err := parseTOML("t.toml", tc.src)
		if err == nil {
			t.Fatalf("%q: want error", tc.src)
		}
		if pe, ok := err.(*parseErr); ok {
			if !strings.Contains(pe.msg, tc.wantSub) {
				t.Fatalf("%q: error %q lacks %q", tc.src, pe.msg, tc.wantSub)
			}
			if pe.line < 1 {
				t.Fatalf("%q: no line in error", tc.src)
			}
		} else {
			t.Fatalf("%q: not a parseErr: %T", tc.src, err)
		}
	}
}

func TestTOMLDuplicates(t *testing.T) {
	_, err := parseTOML("t.toml", "[env]\na=1\n[env]\nb=2\n")
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatal("dup [env] not caught")
	}
	_, err = parseTOML("t.toml", "[[vm]]\nname=\"a\"\nname=\"b\"\n")
	if err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatal("dup key not caught")
	}
	_, err = parseTOML("t.toml", "[env]\n[[env]]\n")
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatal("[x] after [[x]] not caught")
	}
}

func TestTOMLIndentedPositions(t *testing.T) {
	d := mustParse(t, "[[vm]]\n    name = \"x\"\n")
	v := d.arrays["vm"][0]
	if v.pos["name"] != [2]int{2, 5} {
		t.Fatalf("indented key pos: %v", v.pos["name"])
	}
	if v.vals["name"].col != 11 {
		t.Fatalf("indented value col: %v", v.vals["name"].col)
	}
}
