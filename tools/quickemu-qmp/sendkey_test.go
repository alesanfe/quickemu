package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression: the Bash keymap covered ~30 punctuation chars the first
// Go port silently dropped — every printable ASCII char must map.
func TestSendkeyNameCoversPrintableASCII(t *testing.T) {
	want := map[rune]string{
		' ': "spc", '\n': "ret", '\t': "tab",
		'.': "dot", ',': "comma", '-': "minus", '=': "equal",
		'/': "slash", ':': "shift-semicolon", ';': "semicolon",
		'\'': "apostrophe", '"': "shift-apostrophe",
		'[': "bracket_left", ']': "bracket_right",
		'\\': "backslash", '`': "grave_accent",
		'!': "shift-1", '@': "shift-2", '#': "shift-3", '$': "shift-4",
		'%': "shift-5", '^': "shift-6", '&': "shift-7", '*': "shift-8",
		'(': "shift-9", ')': "shift-0",
		'_': "shift-minus", '+': "shift-equal",
		'{': "shift-bracket_left", '}': "shift-bracket_right",
		'|': "shift-backslash", '~': "shift-grave_accent",
		'<': "shift-comma", '>': "shift-dot", '?': "shift-slash",
		'A': "shift-a", 'Z': "shift-z", 'a': "a", 'z': "z",
		'0': "0", '9': "9",
	}
	for ch, w := range want {
		if got := sendkeyName(ch); got != w {
			t.Errorf("sendkeyName(%q)=%q want %q", ch, got, w)
		}
	}
	// Every printable ASCII char must produce a name — nothing dropped.
	for ch := rune(0x20); ch < 0x7F; ch++ {
		if sendkeyName(ch) == "" {
			t.Errorf("printable %q (%#x) has no sendkey name", ch, ch)
		}
	}
	if sendkeyName(0x00) != "" || sendkeyName('ñ') != "" {
		t.Error("non-mappable chars must return empty")
	}
}

// Regression: values containing $ were expanded as regex group
// references by ReplaceAllString — a path like disk$1.img corrupted
// the conf. Splice must be literal.
func TestHotplugPersistDollarValue(t *testing.T) {
	for _, ext := range []string{".conf", ".toml"} {
		f := filepath.Join(t.TempDir(), "vm"+ext)
		os.WriteFile(f, []byte("guest_os=linux\n"), 0o600)
		val := `usb$1&\x`
		if err := HotplugPersist(f, "usb", val); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(f)
		if !strings.Contains(string(b), val) {
			t.Fatalf("%s: $-value not persisted literally:\n%s", ext, b)
		}
	}
}

// Regression: multi-line arrays were missed by single-line regexes —
// appending then produced a duplicate (invalid) key line.
func TestHotplugPersistMultilineArray(t *testing.T) {
	f := filepath.Join(t.TempDir(), "vm.toml")
	os.WriteFile(f, []byte("usb_devices = [\n  \"1:1\",\n]\n"), 0o600)
	if err := HotplugPersist(f, "usb", "2:2"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	s := string(b)
	if strings.Count(s, "usb_devices") != 1 {
		t.Fatalf("key duplicated:\n%s", s)
	}
	if !strings.Contains(s, "2:2") || !strings.Contains(s, "1:1") {
		t.Fatalf("elements lost:\n%s", s)
	}

	c := filepath.Join(t.TempDir(), "vm.conf")
	os.WriteFile(c, []byte("extra_disks=(\n  \"a.qcow2\"\n)\n"), 0o600)
	if err := HotplugPersist(c, "disk", "b.qcow2"); err != nil {
		t.Fatal(err)
	}
	cb, _ := os.ReadFile(c)
	cs := string(cb)
	if strings.Count(cs, "extra_disks") != 1 ||
		!strings.Contains(cs, "a.qcow2") || !strings.Contains(cs, "b.qcow2") {
		t.Fatalf("conf array broken:\n%s", cs)
	}
}
