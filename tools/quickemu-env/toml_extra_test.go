package main

import (
	"strings"
	"testing"
)

func TestKindName(t *testing.T) {
	for k, want := range map[tKind]string{
		tString:  "string",
		tInt:     "integer",
		tFloat:   "float",
		tBool:    "boolean",
		tArray:   "list",
		tKind(9): "value",
	} {
		if got := (tValue{kind: k}).kindName(); got != want {
			t.Fatalf("kindName(%d) = %q, want %q", k, got, want)
		}
	}
}

func TestFirstTok(t *testing.T) {
	if got := firstTok("hello"); got != "hello" {
		t.Fatalf("firstTok = %q", got)
	}
	if got := firstTok("ab cd"); got != "ab" {
		t.Fatalf("firstTok space = %q", got)
	}
	long := strings.Repeat("x", 40)
	if got := firstTok(long); got != strings.Repeat("x", 24)+"..." {
		t.Fatalf("firstTok long = %q", got)
	}
}

func TestKeyEq(t *testing.T) {
	// '=' inside quotes is skipped.
	if i := keyEq(`"a=b" = 1`); i != 6 {
		t.Fatalf("keyEq = %d", i)
	}
	if i := keyEq("k=1"); i != 1 {
		t.Fatalf("keyEq = %d", i)
	}
	if i := keyEq("k = 1"); i != 2 {
		t.Fatalf("keyEq = %d", i)
	}
	if i := keyEq("no-equals"); i != -1 {
		t.Fatalf("keyEq none = %d", i)
	}
	// Escaped quote inside a basic string does not close it, and the
	// '=' inside the quoted span is skipped too.
	if i := keyEq(`"a\"=b" = 1`); i != 8 {
		t.Fatalf("keyEq escaped = %d", i)
	}
}

func TestScanStringEscapes(t *testing.T) {
	s, n, err := scanString(`"a\nb\tc\rd\be\ff\"g\\h"`, 0)
	if err != nil || s != "a\nb\tc\rd\be\ff\"g\\h" || n != len(`"a\nb\tc\rd\be\ff\"g\\h"`) {
		t.Fatalf("escapes: %q n=%d err=%v", s, n, err)
	}
	s, _, err = scanString(`"A"`, 0)
	if err != nil || s != "A" {
		t.Fatalf("unicode: %q %v", s, err)
	}
	s, _, err = scanString(`"é"`, 0)
	if err != nil || s != "é" {
		t.Fatalf("unicode8: %q %v", s, err)
	}
	s, _, err = scanString(`'lit\nral'`, 0)
	if err != nil || s != `lit\nral` {
		t.Fatalf("literal: %q %v", s, err)
	}
}

func TestScanStringErrors(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{`"abc`, "unterminated string"},
		{`"ab\`, "unterminated escape"},
		{`"a\qb"`, "unknown escape"},
		{`"a\u12"`, "short \\u escape"},
		{`"a\uXXXX"`, "bad \\u escape"},
		{`"a\U0000"`, "short \\U escape"},
		{"\"a\nb\"", "newline in string"},
		{"'a\nb'", "newline in string"},
		{"'unterminated", "unterminated string"},
	} {
		if _, _, err := scanString(tc.in, 0); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%q: err = %v, want %q", tc.in, err, tc.want)
		}
	}
}

func TestBracketDepth(t *testing.T) {
	if d := bracketDepth(`["]", "[")]`); d != 0 {
		t.Fatalf("depth with quoted brackets = %d", d)
	}
	if d := bracketDepth("[1, [2]"); d != 1 {
		t.Fatalf("open depth = %d", d)
	}
	if d := bracketDepth("abc"); d != 0 {
		t.Fatalf("depth = %d", d)
	}
}

func TestParseValueScalars(t *testing.T) {
	v, err := parseValue("true")
	if err != nil || v.kind != tBool || !v.b {
		t.Fatalf("true: %+v %v", v, err)
	}
	v, err = parseValue("false")
	if err != nil || v.kind != tBool || v.b {
		t.Fatalf("false: %+v %v", v, err)
	}
	v, err = parseValue("1_000")
	if err != nil || v.kind != tInt || v.i != 1000 {
		t.Fatalf("int: %+v %v", v, err)
	}
	v, err = parseValue("-5")
	if err != nil || v.kind != tInt || v.i != -5 {
		t.Fatalf("neg: %+v %v", v, err)
	}
	v, err = parseValue("1.5")
	if err != nil || v.kind != tFloat || v.f != 1.5 {
		t.Fatalf("float: %+v %v", v, err)
	}
}

func TestParseValueErrors(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"", "missing value"},
		{"   ", "missing value"},
		{"tru", "invalid value"},
		{"falsey", "invalid value"},
		{"2020-01-01", "unsupported value"},
		{`"a" tail`, "trailing text after string"},
		{`"abc`, "unterminated"},
	} {
		if _, err := parseValue(tc.in); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%q: err = %v, want %q", tc.in, err, tc.want)
		}
	}
}

func TestParseArrayKindsAndErrors(t *testing.T) {
	v, err := parseArray(`["s", 42, true, 1.5]`)
	if err != nil || len(v.arr) != 4 {
		t.Fatalf("mixed: %+v %v", v, err)
	}
	if v.arr[0].kind != tString || v.arr[1].kind != tInt ||
		v.arr[2].kind != tBool || v.arr[3].kind != tFloat {
		t.Fatalf("kinds: %+v", v.arr)
	}
	// Comma inside a quoted string does not split.
	v, err = parseArray(`["a,b", "c"]`)
	if err != nil || len(v.arr) != 2 || v.arr[0].s != "a,b" {
		t.Fatalf("quoted comma: %+v %v", v, err)
	}
	for _, tc := range []struct {
		in   string
		want string
	}{
		{`[1,2`, "unterminated array"},
		{`["a`, "unterminated"},
		{`[[1]]`, "nested arrays are not supported"},
		{`[1] x`, "trailing text after array"},
		{`["a" "b"]`, "expected ',' or ']' in array"},
		{`[,1]`, "empty array element"},
		{`[tru]`, "invalid value"},
		{`[`, "unterminated array"},
	} {
		if _, err := parseArray(tc.in); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%q: err = %v, want %q", tc.in, err, tc.want)
		}
	}
}

func TestStrsErrors(t *testing.T) {
	v := tValue{kind: tInt, i: 1}
	if _, err := v.strs(); err == nil || !strings.Contains(err.Error(), "expected a list") {
		t.Fatalf("non-array strs: %v", err)
	}
	v = tValue{kind: tArray, arr: []tValue{{kind: tInt, i: 1}}}
	if _, err := v.strs(); err == nil || !strings.Contains(err.Error(), "list of strings") {
		t.Fatalf("int elem strs: %v", err)
	}
}

func TestParseTOMLQuotedKeys(t *testing.T) {
	d := mustParse(t, `"my key" = 1`+"\n"+`'other' = 2`+"\n"+`"a.b" = 3`)
	if d.globals.vals["my key"].i != 1 || d.globals.vals["other"].i != 2 ||
		d.globals.vals["a.b"].i != 3 {
		t.Fatalf("quoted keys: %v", d.globals.vals)
	}
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`"" = 1`, "invalid quoted key"},
		{`"a" junk = 1`, "invalid quoted key"},
		{`x.y = 1`, "invalid key"},
		{`[[bad.name]]`, "invalid table name"},
		{`[bad name]`, "invalid table name"},
		{`[  ]`, "invalid table name"},
	} {
		_, err := parseTOML("t.toml", tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%q: err = %v, want %q", tc.src, err, tc.want)
		}
	}
	// Spaces inside [[ ]] are trimmed — valid.
	if _, err := parseTOML("t.toml", `[[ vm ]]`); err != nil {
		t.Fatalf("spaced header should parse: %v", err)
	}
}

func TestParseTOMLMultiLineArrayWithComments(t *testing.T) {
	d := mustParse(t, `[[vm]]
depends_on = [
  # a comment line inside the array
  "a",
  "b", # trailing comment
]
`)
	l, err := d.arrays["vm"][0].vals["depends_on"].strs()
	if err != nil || len(l) != 2 || l[0] != "a" || l[1] != "b" {
		t.Fatalf("multi-line: %v %v", l, err)
	}
}

func TestParseTOMLErrPositions(t *testing.T) {
	// Error positions inside a value re-anchor to the key's line.
	_, err := parseTOML("t.toml", "x = 1\ny = tru\n")
	if err == nil {
		t.Fatal("want error")
	}
	pe, ok := err.(*parseErr)
	if !ok || pe.line != 2 {
		t.Fatalf("err = %+v", err)
	}
}

func TestStripComment(t *testing.T) {
	for in, want := range map[string]string{
		`x = "a#b" # c`:  `x = "a#b" `,
		`x = 'a#b' # c`:  `x = 'a#b' `,
		`x = 1 # c`:      `x = 1 `,
		`x = "a\"#" # c`: `x = "a\"#" `,
		`no comment`:     `no comment`,
	} {
		if got := stripComment(in); got != want {
			t.Fatalf("stripComment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeyOK(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"", false},
		{"a", true},
		{"a-b_c9", true},
		{"9z", true},
		{"a.b", false},
		{"a b", false},
		{"a/b", false},
	} {
		if got := keyOK(tc.in); got != tc.want {
			t.Fatalf("keyOK(%q) = %v", tc.in, got)
		}
	}
}
