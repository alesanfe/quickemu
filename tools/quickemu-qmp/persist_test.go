package main

import (
	"os"
	"strings"
	"testing"
)

func TestHotplugPersistConf(t *testing.T) {
	f := t.TempDir() + "/vm.conf"
	os.WriteFile(f, []byte("guest_os=\"linux\"\nextra_disks=(\"a.qcow2\")\n"), 0o644)
	if err := HotplugPersist(f, "disk", "b.qcow2"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	if !strings.Contains(string(b), `extra_disks=("a.qcow2" "b.qcow2")`) {
		t.Fatalf("conf not updated: %s", b)
	}
}

func TestHotplugPersistToml(t *testing.T) {
	f := t.TempDir() + "/vm.toml"
	os.WriteFile(f, []byte("guest_os=\"linux\"\nusb_devices = [\"1:2\"]\n"), 0o644)
	if err := HotplugPersist(f, "usb", "3:4"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	if !strings.Contains(string(b), `usb_devices = ["1:2", "3:4"]`) {
		t.Fatalf("toml not updated: %s", b)
	}
}

func TestHotplugPersistNewKey(t *testing.T) {
	f := t.TempDir() + "/vm.conf"
	os.WriteFile(f, []byte("guest_os=\"linux\"\n"), 0o644)
	if err := HotplugPersist(f, "usb", "5:6"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	if !strings.Contains(string(b), `usb_devices=("5:6")`) {
		t.Fatalf("new key not appended: %s", b)
	}
}
