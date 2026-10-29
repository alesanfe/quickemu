// conf_test.go: property test (property_spec.sh parity) — the conf
// parser must return a verdict for ANY byte stream: never panic,
// never hang.
package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestConfKVNeverPanics(t *testing.T) {
	dir := t.TempDir()
	vmConf = filepath.Join(dir, "c.conf")
	r := rand.New(rand.NewSource(41))
	for i := 0; i < 200; i++ {
		b := make([]byte, r.Intn(512))
		r.Read(b)
		if err := os.WriteFile(vmConf, b, 0644); err != nil {
			t.Fatal(err)
		}
		_ = confKV() // verdict: parse or clean garbage — never panic
	}
}

func TestConfKVParsesKnownShape(t *testing.T) {
	dir := t.TempDir()
	vmConf = filepath.Join(dir, "c.conf")
	os.WriteFile(vmConf, []byte("# comment\nguest_os=\"linux\"\nram=4G\nbadline\nk=v\n"), 0644)
	m := confKV()
	if m["guest_os"] != "linux" || m["ram"] != "4G" || m["k"] != "v" {
		t.Fatalf("unexpected parse: %v", m)
	}
	if _, ok := m["badline"]; ok {
		t.Error("line without '=' must not produce a key")
	}
}

// TestSetConfOpt: flag→conf-key mapping (options.sh parity). Dashed
// flags land as underscore keys the launcher reads; --usb-passthrough
// accumulates into the usb_devices list.
func TestSetConfOpt(t *testing.T) {
	defer func() { confOpts = map[string]string{} }()
	confOpts = map[string]string{}
	setConfOpt("ssh-port", "2222")
	setConfOpt("monitor-telnet-host", "0.0.0.0")
	setConfOpt("sound-card", "ac97")
	if confOpts["ssh_port"] != "2222" || confOpts["monitor_telnet_host"] != "0.0.0.0" ||
		confOpts["sound_card"] != "ac97" {
		t.Fatalf("dashed flags must produce underscore keys: %v", confOpts)
	}
	for _, k := range []string{"ssh-port", "monitor-telnet-host", "sound-card"} {
		if _, bad := confOpts[k]; bad {
			t.Fatalf("dash key %q must not leak into confOpts", k)
		}
	}
	setConfOpt("usb-passthrough", "046d:c52b")
	setConfOpt("usb-passthrough", "05ac:12a8")
	if confOpts["usb_devices"] != "046d:c52b 05ac:12a8" {
		t.Fatalf("usb-passthrough must accumulate: %v", confOpts)
	}
}
