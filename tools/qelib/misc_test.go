// misc_test.go — sendkey keymap, HotplugPersist conf/toml,
// UbuntuDailyCodename via injected fetch/probe, Tool, DataFile and
// HostFreeMemMB.
package qelib

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendkeyNameMap(t *testing.T) {
	cases := map[rune]string{
		' ': "spc", '\n': "ret", '\t': "tab", '.': "dot", ',': "comma",
		'-': "minus", '=': "equal", '/': "slash", ';': "semicolon",
		'\'': "apostrophe", '[': "bracket_left", ']': "bracket_right",
		'\\': "backslash", '`': "grave_accent", 'a': "a", 'z': "z",
		'0': "0", '9': "9", 'A': "shift-a", 'Z': "shift-z",
		'!': "shift-1", '@': "shift-2", '#': "shift-3", '$': "shift-4",
		'%': "shift-5", '^': "shift-6", '&': "shift-7", '*': "shift-8",
		'(': "shift-9", ')': "shift-0", ':': "shift-semicolon",
		'"': "shift-apostrophe", '<': "shift-comma", '>': "shift-dot",
		'?': "shift-slash", '_': "shift-minus", '+': "shift-equal",
		'{': "shift-bracket_left", '}': "shift-bracket_right",
		'|': "shift-backslash", '~': "shift-grave_accent",
	}
	for ch, want := range cases {
		if got := SendkeyName(ch); got != want {
			t.Errorf("SendkeyName(%q)=%q want %q", ch, got, want)
		}
	}
	if SendkeyName(0x01) != "" {
		t.Fatal("control char must be unmapped")
	}
}

func TestHotplugPersist(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "vm.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o600)
	if err := HotplugPersist(conf, "usb", "01:02"); err != nil {
		t.Fatal(err)
	}
	if err := HotplugPersist(conf, "usb", "03:04"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(conf)
	if !strings.Contains(string(b), `"01:02"`) || !strings.Contains(string(b), `"03:04"`) {
		t.Fatalf("conf %s", b)
	}
	toml := filepath.Join(dir, "vm.toml")
	os.WriteFile(toml, []byte("guest_os = \"linux\"\n"), 0o600)
	if err := HotplugPersist(toml, "disk", "/d.qcow2"); err != nil {
		t.Fatal(err)
	}
	if err := HotplugPersist(toml, "disk", "/e.qcow2"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(toml)
	if !strings.Contains(string(b), `"/d.qcow2"`) || !strings.Contains(string(b), `"/e.qcow2"`) {
		t.Fatalf("toml %s", b)
	}
	if err := HotplugPersist(filepath.Join(dir, "none.conf"), "usb", "x"); err == nil {
		t.Fatal("missing conf must fail")
	}
}

func TestUbuntuDailyCodename(t *testing.T) {
	oldF, oldP := httpFetch, httpProbeURL
	defer func() { httpFetch, httpProbeURL = oldF, oldP }()
	probed := []string{}
	httpProbeURL = func(u string, h map[string]string) bool {
		probed = append(probed, u)
		return strings.Contains(u, "warty")
	}
	httpFetch = func(u string, max int64) (string, error) {
		return `{"entries":[{"name":"xenial"},{"name":"warty"}]}`, nil
	}
	got, err := UbuntuDailyCodename("kubuntu")
	if err != nil || got != "warty" {
		t.Fatalf("codename %q %v", got, err)
	}
	// First entry skipped (probe failed) → probed twice.
	if len(probed) != 2 {
		t.Fatalf("probes %v", probed)
	}
	// Fetch error propagates.
	httpFetch = func(u string, max int64) (string, error) {
		return "", errors.New("net down")
	}
	if _, err = UbuntuDailyCodename("kubuntu"); err == nil {
		t.Fatal("expected fetch error")
	}
	// Bad JSON → parse error.
	httpFetch = func(u string, max int64) (string, error) { return "not json", nil }
	if _, err = UbuntuDailyCodename("kubuntu"); err == nil {
		t.Fatal("expected parse error")
	}
	// No matching series → empty, no error.
	httpFetch = func(u string, max int64) (string, error) {
		return `{"entries":[{"name":"none"}]}`, nil
	}
	httpProbeURL = func(u string, h map[string]string) bool { return false }
	if got, err = UbuntuDailyCodename("kubuntu"); got != "" || err != nil {
		t.Fatalf("empty %q %v", got, err)
	}
}

func TestToolAndDataFile(t *testing.T) {
	// Tool runs a resolved sibling binary — stub one on PATH.
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "qe-stub"),
		[]byte("#!/bin/sh\necho stub-out-$MARKER\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := Tool("qe-stub", []string{"MARKER=42"})
	if err != nil || !strings.Contains(out, "stub-out-42") {
		t.Fatalf("Tool %q %v", out, err)
	}
	if _, err = Tool("qe-definitely-missing", nil); err == nil {
		t.Fatal("missing tool must fail")
	}
	// DataFile: QUICKEMU_DATA_DIR override resolves inside it.
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "catalog", "data"), 0o755)
	os.WriteFile(filepath.Join(d, "catalog", "data", "x.json"), []byte("{}"), 0o600)
	t.Setenv("QUICKEMU_DATA_DIR", d)
	if p := DataFile("catalog", "data", "x.json"); !strings.HasSuffix(p, "x.json") {
		t.Fatalf("DataFile %q", p)
	}
	if HostFreeMemMB() < 0 {
		t.Fatal("HostFreeMemMB")
	}
}
