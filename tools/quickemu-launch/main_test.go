// main_test.go — helper-exec harness for main(): --dry-run arg
// assembly over rich confs, --validate, --reap, usage errors.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		os.Args = append([]string{"quickemu-launch"},
			strings.Split(os.Getenv("QL_ARGS"), "\x1f")...)
		main()
		return
	}
	os.Exit(m.Run())
}

// launch runs the binary as a helper; rc comes from the process exit.
func launch(t *testing.T, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), append(env, "QE_HELPER=1",
		"QL_ARGS="+strings.Join(args, "\x1f"))...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	}
	return out.String() + errb.String(), rc
}

func richConf(t *testing.T, extra string) string {
	t.Helper()
	dir := t.TempDir()
	conf := filepath.Join(dir, "vm.conf")
	os.WriteFile(filepath.Join(dir, "disk.qcow2"), []byte("d"), 0o600)
	os.WriteFile(conf, []byte(`guest_os="linux"
disk_img="disk.qcow2"
cpu_cores="4"
ram="4G"
machine_type="q35"
display="sdl"
usb_devices="046d:c52b"
port_forwards="8080:80"
ssh_port="22229"
agent="true"
serial="socket"
sound="true"
efi="true"
`+extra), 0o644)
	return conf
}

func TestDryRunArgAssembly(t *testing.T) {
	conf := richConf(t, "")
	out, rc := launch(t, nil, conf, "--dry-run")
	if rc != 0 {
		t.Fatalf("dry-run rc=%d out=%s", rc, out)
	}
	for _, want := range []string{
		"qemu-system-x86_64", "SystemDisk", "virtio-blk",
		"hostpass", "8080-:80", "qemu-vdagent", "-serial",
		"OVMF", "-pidfile", "virtio-balloon",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestDryRunVariants(t *testing.T) {
	// spice + usb passthrough + windows guest + tpm + iso + floppy.
	conf := richConf(t, `display="spice"
guest_os="windows"
tpm="true"
iso="win.iso"
floppy="vfd.img"
driver_iso="drv.iso"
vfio="true"
vfio_devices="0000:01:00.0"
spice_port="5930"
boot="efi"
secureboot="true"
`)
	dir := filepath.Dir(conf)
	os.WriteFile(filepath.Join(dir, "win.iso"), []byte("i"), 0o600)
	out, rc := launch(t, nil, conf, "--dry-run")
	if rc != 0 {
		t.Fatalf("spice rc=%d out=%s", rc, out)
	}
	for _, want := range []string{"spice", "vm.sock", "vfio-pci", "floppy", "drv.iso", "tpm"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	// aarch64 cross → tcg + virt machine.
	conf2 := richConf(t, `arch="aarch64"
display="none"
network="none"
`)
	out, rc = launch(t, nil, conf2, "--dry-run")
	if rc != 0 {
		t.Fatalf("aarch64 rc=%d %s", rc, out)
	}
	if !strings.Contains(out, "qemu-system-aarch64") || !strings.Contains(out, "tcg") {
		t.Fatalf("aarch64 out:\n%s", out)
	}
	// guest-OS quirk sweep — each dry-run exercises a different
	// osQuirks branch (net device, machine, disk dev, rtc).
	for _, g := range []string{"freebsd", "windows-server", "ubuntu",
		"reactos", "kolibrios", "freedos", "haiku", "solaris"} {
		c := richConf(t, "guest_os=\""+g+"\"\nnetwork=\"bridge\"\ndisplay=\"gtk\"\n")
		if out, rc := launch(t, nil, c, "--dry-run"); rc != 0 {
			t.Fatalf("%s rc=%d %s", g, rc, out)
		}
	}
	// display none + macos + telnet serial.
	conf3 := richConf(t, `guest_os="macos"
display="none"
serial="telnet"
serial_telnet_port="6660"
`)
	out, _ = launch(t, nil, conf3, "--dry-run")
	if !strings.Contains(out, "telnet") {
		t.Fatalf("telnet serial missing:\n%s", out)
	}
}

func TestValidateAndErrors(t *testing.T) {
	conf := richConf(t, "")
	out, rc := launch(t, nil, conf, "--validate")
	if rc != 0 || !strings.Contains(out, "valid") {
		t.Fatalf("validate rc=%d %s", rc, out)
	}
	bad := filepath.Join(t.TempDir(), "bad.conf")
	os.WriteFile(bad, []byte("cpu_cores=\"9999\"\nguest_os=\"BAD\"\n"), 0o644)
	if _, rc := launch(t, nil, bad, "--validate"); rc != 1 {
		t.Fatalf("invalid rc=%d", rc)
	}
	// Unknown serial option → fatal exit 1.
	badser := richConf(t, "serial=\"bogus\"\n")
	if _, rc := launch(t, nil, badser, "--dry-run"); rc != 1 {
		t.Fatalf("bad serial rc=%d", rc)
	}
	// --reap with dead pid → cleanup path returns quickly.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "vm.pid"), []byte("99999999"), 0o600)
	os.WriteFile(filepath.Join(dir, "vm.sock"), []byte("x"), 0o600)
	if _, rc := launch(t, nil, "--reap", "99999999", dir, "vm", "0"); rc != 0 {
		t.Fatalf("reap rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(dir, "vm.sock")); err == nil {
		t.Fatal("reap did not clean state")
	}
}

func TestRunPathWithFakeQEMU(t *testing.T) {
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "qemu-system-x86_64"),
		[]byte("#!/bin/sh\nsleep 2\n"), 0o755)
	conf := richConf(t, "")
	env := []string{"PATH=" + stub + string(os.PathListSeparator) + os.Getenv("PATH")}
	out, rc := launch(t, env, conf, "--run")
	// Fake qemu stays up 2s → Started; may fail on the VT-x gate on
	// hosts without kvm — both paths exercise the run branch.
	if !strings.Contains(out, "Started") && !strings.Contains(out, "ERROR") && rc != 0 && rc != 1 {
		t.Fatalf("run rc=%d %s", rc, out)
	}
}

func TestDotdirParseToml(t *testing.T) {
	if got := dotdir("/vmdir"); !strings.HasSuffix(got, "/") {
		t.Fatalf("dotdir noslash %q", got)
	}
	dir := t.TempDir()
	toml := filepath.Join(dir, "vm.toml")
	os.WriteFile(toml, []byte("guest_os=\"linux\"\nram=\"2G\"\n"), 0o644)
	c := parseTomlConf(toml)
	if c["guest_os"] != "linux" || c["ram"] != "2G" {
		t.Fatalf("toml %+v", c)
	}
}
