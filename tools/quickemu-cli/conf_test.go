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
