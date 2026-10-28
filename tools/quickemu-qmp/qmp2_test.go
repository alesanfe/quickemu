// qmp2_test.go — second sweep: watch EOF paths, socket readiness,
// monitor SendCmd/SendKeys/TypeText, hotplug helpers, screenshot and
// conf persistence — all over real unix sockets or fixture files.
package main

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// monServer accepts unix connections, reads one line and replies —
// the human-monitor contract SendCmd expects.
func monServer(t *testing.T, reply string) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "mon.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				bufio.NewReader(c).ReadString('\n')
				c.Write([]byte(reply))
			}(c)
		}
	}()
	return sock
}

func TestSocketReady(t *testing.T) {
	if SocketReady(filepath.Join(t.TempDir(), "none")) {
		t.Fatal("missing must not be ready")
	}
	reg := filepath.Join(t.TempDir(), "f")
	os.WriteFile(reg, []byte("x"), 0o600)
	if SocketReady(reg) {
		t.Fatal("regular file must not be ready")
	}
	sock := monServer(t, "")
	if !SocketReady(sock) {
		t.Fatal("real unix socket must be ready")
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(sock, link)
	if SocketReady(link) {
		t.Fatal("symlink must not be ready")
	}
}

func TestSendCmdAndKeys(t *testing.T) {
	sock := monServer(t, "OK reply\n")
	out, err := SendCmd(sock, "info status")
	if err != nil || !strings.Contains(out, "OK reply") {
		t.Fatalf("SendCmd %q %v", out, err)
	}
	if _, err := SendCmd(filepath.Join(t.TempDir(), "none"), "x"); err == nil {
		t.Fatal("expected dial error")
	}
	// SendKeys normalizes + to -.
	if err := SendKeys(sock, "ctrl+alt+f1"); err != nil {
		t.Fatalf("SendKeys %v", err)
	}
	if err := SendKeys(sock, ""); err == nil {
		t.Fatal("empty combo must fail")
	}
}

func TestTypeText(t *testing.T) {
	sock := monServer(t, "")
	// Short string keeps the 33ms/char pacing cheap.
	if err := TypeText(sock, "Az"); err != nil {
		t.Fatalf("TypeText %v", err)
	}
	if err := TypeText(filepath.Join(t.TempDir(), "none"), "x"); err == nil {
		t.Fatal("expected error")
	}
	// sendkeyName coverage across classes.
	for _, ch := range " \n\t.,-=/\\'\"[]`!@#$%^&*(){}|;:<>?~_+aZ09" {
		if sendkeyName(ch) == "" && ch != '?' {
			// every printable char above must map
			t.Fatalf("unmapped %q", ch)
		}
	}
}

func TestHotplugHelpers(t *testing.T) {
	t.Setenv("QUICKEMU_QMP_RAW", "1") // device_add et al need the raw bypass
	sock := fakeQMP(t, nil)
	if id, err := HotplugUSB(sock, "0123:abcd"); err != nil || id == "" {
		t.Fatalf("HotplugUSB %q %v", id, err)
	}
	for _, bad := range []string{"nocolon", "gg:00", "0:1", "1:0"} {
		if _, err := HotplugUSB(sock, bad); err == nil {
			t.Fatalf("bad vidpid %q must fail", bad)
		}
	}
	f := filepath.Join(t.TempDir(), "d.qcow2")
	os.WriteFile(f, []byte("x"), 0o600)
	if id, err := HotplugDisk(sock, f); err != nil || id == "" {
		t.Fatalf("HotplugDisk %q %v", id, err)
	}
	if _, err := HotplugDisk(sock, filepath.Join(t.TempDir(), "no")); err == nil {
		t.Fatal("missing disk must fail")
	}
	if err := Hotunplug(sock, "hotusb0123abcd"); err != nil {
		t.Fatalf("Hotunplug %v", err)
	}
	if err := Screenshot(sock, filepath.Join(t.TempDir(), "s.ppm")); err != nil {
		t.Fatalf("Screenshot %v", err)
	}
}

func TestHotplugPersist(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "vm.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o600)
	if err := HotplugPersist(conf, "usb", "01:02"); err != nil {
		t.Fatal(err)
	}
	if err := HotplugPersist(conf, "usb", "03:04"); err != nil { // append
		t.Fatal(err)
	}
	b, _ := os.ReadFile(conf)
	if !strings.Contains(string(b), `"01:02"`) || !strings.Contains(string(b), `"03:04"`) {
		t.Fatalf("conf persist %s", b)
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
		t.Fatalf("toml persist %s", b)
	}
	if err := HotplugPersist(filepath.Join(dir, "none.conf"), "usb", "x"); err == nil {
		t.Fatal("missing conf must fail")
	}
}

// watchEOF: server that sends one event then closes → QMP_EOF rc=1.
func TestWatchEOF(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "w.qmp")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		defer c.Close()
		c.Write([]byte(`{"QMP":{"version":{}}}` + "\n"))
		bufio.NewReader(c).ReadString('\n') // qmp_capabilities
		c.Write([]byte(`{"event":"STOP"}` + "\n"))
	}()
	if rc := runWatch(sock); rc != 1 {
		t.Fatalf("watch eof rc=%d", rc)
	}
	if rc := runWatch(filepath.Join(t.TempDir(), "none")); rc != 1 {
		t.Fatalf("watch unavailable rc=%d", rc)
	}
}
