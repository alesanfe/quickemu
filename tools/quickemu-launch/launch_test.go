package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfListBashArray(t *testing.T) {
	c := Conf{"extra_nics": `("user" "restrict" "bridge:br1")`}
	got := confList(c, "extra_nics")
	want := []string{"user", "restrict", "bridge:br1"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestExtraNicsArgs(t *testing.T) {
	c := Conf{
		"network":    "",
		"extra_nics": `("restrict" "user")`,
		"extra_macs": `("52:54:00:aa:bb:cc" "")`,
		"_vmname":    "vm",
	}
	a := networkArgs(c, OsTuning{NetDevice: "virtio-net"})
	joined := strings.Join(a, " ")
	if !strings.Contains(joined, "netdev=nic0,mac=52:54:00:aa:bb:cc") {
		t.Fatalf("extra mac missing: %s", joined)
	}
	if !strings.Contains(joined, "restrict,id=nic0") || !strings.Contains(joined, "user,id=nic1") {
		t.Fatalf("extra netdevs missing: %s", joined)
	}
}

func TestSpiceVariants(t *testing.T) {
	// firstFreePort probes real TCP ports — the host may legitimately
	// have the whole SPICE range bound (other VMs, forwarded ports).
	// Stub the probe: any scanned port reads as free.
	oldListen := netListen
	netListen = func(_, _ string) (net.Listener, error) {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	defer func() { netListen = oldListen }()
	base := Conf{"_vmdir": "/vms/x", "_vmname": "x"}
	mk := func(display, access string) string {
		c := Conf{}
		for k, v := range base {
			c[k] = v
		}
		c["display"] = display
		if access != "" {
			c["access"] = access
		}
		a := spiceArgs(c, "/vms/x", "x", "linux", OsTuning{})
		for i, tok := range a {
			if tok == "-spice" && i+1 < len(a) {
				return a[i+1]
			}
		}
		return ""
	}
	if s := mk("spice", "local"); !strings.Contains(s, "unix=on") || !strings.Contains(s, "x.sock") {
		t.Fatalf("spice local should use unix socket: %s", s)
	}
	if s := mk("spice", "remote"); !strings.Contains(s, "port=") || strings.Contains(s, "unix") {
		t.Fatalf("spice remote should use tcp: %s", s)
	}
	if s := mk("none", ""); !strings.Contains(s, "port=") || !strings.Contains(s, "addr=127.0.0.1") {
		t.Fatalf("headless should bind 127.0.0.1: %s", s)
	}
	if s := mk("spice-app", ""); !strings.HasSuffix(s, ",gl=on") {
		t.Fatalf("spice-app should carry gl: %s", s)
	}
}

func TestFileshareVirtiofs(t *testing.T) {
	c := Conf{
		"guest_os": "linux", "_public": "/home/u/Public",
		"_vmdir": "/vms/x", "_vmname": "x", "_virtiofsd": "1", "ram": "8G",
	}
	a := fileshareArgs(c, "/vms/x")
	joined := strings.Join(a, " ")
	if !strings.Contains(joined, "memory-backend-file") || !strings.Contains(joined, "vhost-user-fs-pci") {
		t.Fatalf("virtiofs stack missing: %s", joined)
	}
	if !strings.Contains(joined, "size=8G") {
		t.Fatalf("memfd size should mirror RAM: %s", joined)
	}
	// 9p fallback when virtiofsd flag is not set
	c["_virtiofsd"] = ""
	a = fileshareArgs(c, "/vms/x")
	joined = strings.Join(a, " ")
	if !strings.Contains(joined, "virtio-9p-pci") {
		t.Fatalf("9p fallback missing: %s", joined)
	}
}

func TestParseTomlConfArrays(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "vm.toml")
	doc := `guest_os = "linux"
cpu_cores = 4
extra_nics = ["user", "restrict"]
`
	if err := os.WriteFile(p, []byte(doc), 0644); err != nil {
		t.Fatal(err)
	}
	c := parseTomlConf(p)
	if c.get("cpu_cores", "") != "4" {
		t.Fatalf("cpu_cores=%q", c["cpu_cores"])
	}
	got := confList(c, "extra_nics")
	if len(got) != 2 || got[0] != "user" || got[1] != "restrict" {
		t.Fatalf("toml array -> confList: %v", got)
	}
}
