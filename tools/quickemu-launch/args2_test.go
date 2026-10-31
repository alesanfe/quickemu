// args2_test — storage/conf/ports arg-generation tests (split of args2_test.go).
package main

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
)

func has(args []string, sub string) bool {
	for _, a := range args {
		if strings.Contains(a, sub) {
			return true
		}
	}
	return false
}

func TestUsbAndStorageArgs(t *testing.T) {
	c := Conf{"usb_devices": "046d:c52b 05ac:12a8 bad"}
	a := usbArgs(c, OsTuning{})
	if !has(a, "qemu-xhci") || !has(a, "vendorid=0x046d") || !has(a, "productid=0x12a8") {
		t.Fatalf("usbArgs %v", a)
	}
	a = usbArgs(c, OsTuning{UsbPassthrough: "nec-usb-xhci"})
	if !has(a, "nec-usb-xhci") {
		t.Fatalf("ctl %v", a)
	}
	if usbArgs(Conf{}, OsTuning{}) != nil {
		t.Fatal("empty devs should be nil")
	}

	dir := t.TempDir()
	sc := Conf{"disk_img": "d.qcow2", "iso": "i.iso", "img": "r.img",
		"fixed_iso": "f.iso", "storage": "nvme"}
	a = storageArgs(sc, dir, OsTuning{})
	for _, want := range []string{"virtio-blk-pci", "SystemDisk", "nvme"} {
		if !has(a, want) {
			t.Fatalf("storageArgs missing %q in %v", want, a)
		}
	}
	// iso/fixed_iso emit moved to bootMediaArgs (vm_boot_media order).
	bm := bootMediaArgs(sc, dir)
	for _, want := range []string{"index=0", "index=1", "f.iso", "i.iso"} {
		if !has(bm, want) {
			t.Fatalf("bootMediaArgs missing %q in %v", want, bm)
		}
	}
	a = storageArgs(Conf{"disk_img": "d"}, dir, OsTuning{MacDiskDev: "virtio-blk-device"})
	if !has(a, "virtio-blk-device") {
		t.Fatalf("mac dev %v", a)
	}
}

func TestBootChannelPassthruArgs(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		os   string
		want string
	}{
		{"freedos", "order=dc"},
		{"kolibrios", "index=2"},
		{"reactos", "order=d"},
	}
	for _, tc := range cases {
		a := bootMediaArgs(Conf{"iso": "i.iso", "guest_os": tc.os}, dir)
		if !has(a, tc.want) {
			t.Fatalf("%s: %v", tc.os, a)
		}
	}
	// windows + unattended.iso present → extra drive.
	os.WriteFile(filepath.Join(dir, "unattended.iso"), []byte("x"), 0o600)
	a := bootMediaArgs(Conf{"iso": "i", "guest_os": "windows"}, dir)
	if !has(a, "unattended.iso") {
		t.Fatalf("unattended %v", a)
	}
	a = bootMediaArgs(Conf{"floppy": "f.img", "fixed_iso": "d.iso"}, dir)
	if !has(a, "if=floppy") || !has(a, "index=1") {
		t.Fatalf("floppy/fixed_iso %v", a)
	}
	a = passthruArgs(Conf{"gpu_passthrough": "0000:01:00.0 0000:01:00.1"})
	if len(a) != 4 || !has(a, "host=0000:01:00.1") {
		t.Fatalf("passthru %v", a)
	}
	// The guest-agent channel moved out of the SPICE block (QE-010):
	// qgaArgs emits it for every display mode, and the SPICE block
	// keeps only the spicevmc/webdav ports.
	a = spiceArgs(Conf{"display": "spice"}, dir, "vm", "linux")
	if has(a, "org.qemu.guest_agent.0") || !has(a, "spicevmc") {
		t.Fatalf("spice block should not carry the agent channel %v", a)
	}
}

func TestValidateConfMatrix(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "v.conf")
	os.WriteFile(conf, nil, 0o644)
	errs := validateConf(Conf{}, conf)
	if len(errs) != 0 {
		t.Fatalf("empty conf errs %v", errs)
	}
	errs = validateConf(Conf{
		"guest_os":      "BAD OS",
		"cpu_cores":     "999",
		"efi":           "maybe",
		"disk_img":      "missing.qcow2",
		"iso":           "missing.iso",
		"port_forwards": "abc",
		"ssh_port":      "99999",
		"spice_port":    "0",
	}, conf)
	if len(errs) < 6 {
		t.Fatalf("expected many errors, got %v", errs)
	}
	os.WriteFile(filepath.Join(dir, "ok.qcow2"), nil, 0o600)
	errs = validateConf(Conf{"guest_os": "linux", "cpu_cores": "4",
		"efi": "true", "disk_img": "ok.qcow2", "ssh_port": "2222",
		"port_forwards": "8080:80"}, conf)
	if len(errs) != 0 {
		t.Fatalf("valid conf errs %v", errs)
	}
}

// Bash ports/cpu checks are strict [0-9]+ — fmt.Sscanf("%d") reads a
// numeric PREFIX, so "22x" validates and then reaches QEMU verbatim.
func TestValidateConfStrictDigits(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "v.conf")
	os.WriteFile(conf, nil, 0o644)
	for k, v := range map[string]string{
		"ssh_port": "22x", "spice_port": "5930bad", "cpu_cores": "4x",
	} {
		if errs := validateConf(Conf{k: v}, conf); len(errs) == 0 {
			t.Fatalf("%s=%q accepted — junk suffix must fail", k, v)
		}
	}
}

func TestResolvePublicAndXDG(t *testing.T) {
	// public_dir=none → early return.
	c := Conf{"public_dir": "none"}
	resolvePublic(c)
	if c["_public"] != "" {
		t.Fatal("none should stay empty")
	}
	// missing dir → warning, unset.
	c = Conf{"public_dir": filepath.Join(t.TempDir(), "nope")}
	resolvePublic(c)
	if c["_public"] != "" {
		t.Fatal("missing dir should warn+skip")
	}
	// existing dir → resolved.
	pub := t.TempDir()
	c = Conf{"public_dir": pub}
	resolvePublic(c)
	if c["_public"] != pub {
		t.Fatalf("public %q", c["_public"])
	}
	// xdgUserDir via XDG_CONFIG_HOME user-dirs.dirs.
	xc := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xc)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.WriteFile(filepath.Join(xc, "user-dirs.dirs"),
		[]byte(`XDG_PUBLICSHARE_DIR="$HOME/Public"`+"\n"), 0o600)
	if got := xdgUserDir("PUBLICSHARE"); got != filepath.Join(home, "Public") {
		t.Fatalf("xdgUserDir %q", got)
	}
	// disabled → "".
	os.WriteFile(filepath.Join(xc, "user-dirs.conf"), []byte("enabled=False\n"), 0o600)
	if xdgUserDir("PUBLICSHARE") != "" {
		t.Fatal("disabled dirs should be empty")
	}
	// fallback: ~/Public exists → _public resolves.
	os.WriteFile(filepath.Join(xc, "user-dirs.conf"), []byte(""), 0o600)
	os.Remove(filepath.Join(xc, "user-dirs.dirs"))
	os.MkdirAll(filepath.Join(home, "Public"), 0o755)
	c = Conf{}
	resolvePublic(c)
	if c["_public"] != filepath.Join(home, "Public") {
		t.Fatalf("home fallback %q", c["_public"])
	}
}

func TestPresetsPortsAndMisc(t *testing.T) {
	regEnv(t) // firstFreePort reservations stay off the real registry
	c := applyPresets(Conf{"profile": "windows-gaming"})
	if c["guest_os"] != "windows" || c["ram"] == "" {
		t.Fatalf("preset %+v", c)
	}
	c = applyPresets(Conf{"network_preset": "bridge"})
	if c["network"] != "bridge" {
		t.Fatalf("net preset %+v", c)
	}
	c = applyPresets(Conf{"profile": "nope"})
	if c["guest_os"] != "" {
		t.Fatal("unknown profile should not set keys")
	}
	// explicit conf key wins over preset.
	c = applyPresets(Conf{"profile": "minimal", "ram": "16G"})
	if c["ram"] != "16G" {
		t.Fatalf("override ram=%s", c["ram"])
	}
	// getPort: explicit, then free-range.
	if p := firstFreePort("22220", 22220, 22229, "vm"); p != "22220" {
		t.Fatalf("firstFreePort %s", p)
	}
	if p := getPort(22900, 9, os.Getpid(), "vm"); p <= 0 {
		t.Fatalf("getPort %d", p)
	}
	portRegistryRelease(os.Getpid())
	// firstFreePort busy path: hold a bound ephemeral port — the scan
	// must skip it. A fixed port would silently skip the assertion
	// whenever a host already occupies it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	hi := busy + 9
	if hi > 65535 {
		hi = 65535
	}
	if p := firstFreePort("", busy, hi, "vm"); p == strconv.Itoa(busy) || p == "" {
		t.Fatalf("busy firstFreePort %q", p)
	}
	// osQuirks direct sweep over more guest branches.
	for _, g := range []string{"macos", "windows", "windows-server", "freebsd",
		"openbsd", "netbsd", "dragonflybsd", "solaris", "haiku", "kolibrios",
		"reactos", "freedos", "ubuntu", "nixos", "batocera", "linux"} {
		_ = osQuirks(Conf{"guest_os": g}, g, "x86_64")
		_ = osQuirks(Conf{"guest_os": g}, g, "aarch64")
	}
	// hostMacosTSCUnstable: linux guest → false quickly.
	if b, _ := hostMacosTSCUnstable(Conf{"guest_os": "linux"}); b {
		t.Fatal("linux should not need tsc workaround")
	}
	// joinConf relative + absolute.
	if joinConf("/d", "rel.qcow2") != "/d/rel.qcow2" {
		t.Fatal("joinConf rel")
	}
	if joinConf("/d", "/abs.qcow2") != "/abs.qcow2" {
		t.Fatal("joinConf abs")
	}
}

func TestStorageArgsMacOS(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "OpenCore.qcow2"), []byte("qc"), 0o644)
	c := Conf{"guest_os": "macos", "disk_img": "disk.qcow2", "img": "RecoveryImage.img", "_vmdir": dir}
	a := storageArgs(c, dir, OsTuning{MacDiskDev: "virtio-blk-pci"})
	joined := strings.Join(a, " ")
	for _, want := range []string{"ahci", "BootLoader", "bootindex=0", "RecoveryImage", "SystemDisk"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("macOS storage missing %q: %v", want, a)
		}
	}
	// img must be the ide RecoveryImage, never the virtio raw disk.
	if strings.Contains(joined, "if=virtio") {
		t.Fatalf("macOS img must not use virtio: %v", a)
	}
}

func TestSystemDiskFormat(t *testing.T) {
	a := storageArgs(Conf{"disk_img": "d.raw", "disk_format": "raw"}, t.TempDir(), OsTuning{})
	if !has(a, "format=raw,file=") {
		t.Fatalf("disk_format=raw must reach SystemDisk: %v", a)
	}
}

// vm_boot_fileshare: probe qemu -device help for "vhost-user-fs-pci";
// a QEMU that lacks it falls back to 9p (daemon stopped, flag cleared).
func TestFileshareVhostFSProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub script needs a POSIX shell")
	}
	dir, pub, stub := t.TempDir(), t.TempDir(), t.TempDir()
	mkq := func(line string) {
		os.WriteFile(filepath.Join(stub, "qemu-system-x86_64"),
			[]byte("#!/bin/sh\necho '"+line+"'\n"), 0o755)
	}
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("USER", "qe")
	mkq(`name "e1000", bus PCI`)
	c := Conf{"_public": pub, "_virtiofsd": "1", "_vmdir": dir,
		"_vmname": "vm", "guest_os": "linux"}
	a := fileshareArgs(c, dir)
	if !has(a, "virtio-9p-pci") || has(a, "vhost-user-fs-pci") {
		t.Fatalf("unsupported QEMU must fall back to 9p: %v", a)
	}
	if c["_virtiofsd"] != "" {
		t.Fatal("fallback must clear _virtiofsd")
	}
	mkq(`name "vhost-user-fs-pci", bus PCI`)
	c["_virtiofsd"] = "1"
	if a := fileshareArgs(c, dir); !has(a, "vhost-user-fs-pci") {
		t.Fatalf("supported QEMU must emit virtiofs: %v", a)
	}
}

// configure_usb_passthrough: a vid:pid lsusb cannot resolve is dropped
// from the passthrough list (the accessible/permission paths need a
// real /dev/bus/usb node, so they stay untestable here).
func TestCheckUSBPassthrough(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lsusb gate is linux-only")
	}
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "lsusb"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := Conf{"usb_devices": "046d:c52b dead:beef"}
	checkUSBPassthrough(c)
	if c["usb_devices"] != "" {
		t.Fatalf("unresolved devices must all be dropped: %q", c["usb_devices"])
	}
}

// configure_storage: existing disk > DISK_MIN_SIZE counts as used →
// iso/img detach (installed guests boot from disk); a small disk stays
// unused and keeps the installer media; preallocation is whitelisted.
func TestPrepareStorage(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.qcow2")
	f, _ := os.Create(big)
	f.WriteAt([]byte("d"), 2<<20)
	f.Close()
	c := Conf{"disk_img": "big.qcow2", "iso": "i.iso", "guest_os": "linux"}
	if err := prepareStorage(c, dir); err != nil {
		t.Fatal(err)
	}
	if c["iso"] != "" {
		t.Fatalf("used disk must detach iso: %v", c)
	}
	// kolibrios keeps the media attached.
	c = Conf{"disk_img": "big.qcow2", "iso": "i.iso", "guest_os": "kolibrios"}
	prepareStorage(c, dir)
	if c.get("iso", "") == "" {
		t.Fatal("kolibrios must keep installer media")
	}
	// Tiny disk + no iso/img → unused → hard error (Bash exits).
	small := filepath.Join(dir, "small.qcow2")
	os.WriteFile(small, []byte("d"), 0o600)
	c = Conf{"disk_img": "small.qcow2"}
	if err := prepareStorage(c, dir); err == nil {
		t.Fatal("unused disk without media must error")
	}
	// Tiny disk + iso → ok, media stays.
	c = Conf{"disk_img": "small.qcow2", "iso": "i.iso"}
	if err := prepareStorage(c, dir); err != nil || c.get("iso", "") == "" {
		t.Fatalf("unused disk must keep iso: %v %v", err, c)
	}
	// Bad preallocation fails only in run mode.
	c = Conf{"disk_img": "missing.qcow2", "preallocation": "bogus", "_run": "1"}
	if err := prepareStorage(c, dir); err == nil {
		t.Fatal("bad preallocation must error")
	}
	c = Conf{"disk_img": "missing.qcow2", "preallocation": "bogus"}
	if err := prepareStorage(c, dir); err != nil {
		t.Fatalf("plan mode must not validate preallocation: %v", err)
	}
}

// vm_boot_media: kolibrios/reactos emit the ISO at index=2 instead of
// the generic index=0 (phantom-iso workaround); aarch64 hangs CD-ROMs
// off virtio-scsi with bootindex.
func TestBootMediaSpecialCases(t *testing.T) {
	dir := t.TempDir()
	for _, osn := range []string{"kolibrios", "reactos"} {
		bm := bootMediaArgs(Conf{"guest_os": osn, "iso": "i.iso"}, dir)
		if has(bm, "index=0") {
			t.Fatalf("%s must not emit generic index=0 iso: %v", osn, bm)
		}
		if !has(bm, "index=2") {
			t.Fatalf("%s must emit index=2 iso: %v", osn, bm)
		}
	}
	// aarch64: scsi-cd + bootindex, plus the virtio-scsi controller.
	a := bootMediaArgs(Conf{"guest_os": "linux", "iso": "i.iso",
		"fixed_iso": "f.iso", "arch": "aarch64"}, dir)
	for _, want := range []string{"virtio-scsi-pci", "scsi-cd", "bootindex=1", "bootindex=3"} {
		if !has(a, want) {
			t.Fatalf("aarch64 media missing %q: %v", want, a)
		}
	}
	// floppy carries format=raw.
	if b := bootMediaArgs(Conf{"floppy": "f.img"}, dir); !has(b, "format=raw") {
		t.Fatalf("floppy: %v", b)
	}
}

// SystemDisk emit matrix (vm.sh): windows-server ide-hd, reactos/
// templeos plain if=ide, aarch64 bootindex=2, batocera BootDisk,
// extra_disks with :format + missing-file skip.
func TestSystemDiskMatrix(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "d.qcow2"), []byte("d"), 0o600)
	cases := []struct {
		os     string
		arch   string
		want   string
		absent string
	}{
		{"windows-server", "x86_64", "ide-hd,drive=SystemDisk", "virtio-blk-pci,drive=SystemDisk"},
		{"reactos", "x86_64", "if=ide,index=0,media=disk", "drive=SystemDisk"},
		{"templeos", "x86_64", "if=ide,index=0,media=disk", "drive=SystemDisk"},
		{"linux", "aarch64", "bootindex=2", ""},
		{"batocera", "x86_64", "BootDisk", ""},
	}
	for _, tc := range cases {
		c := Conf{"guest_os": tc.os, "arch": tc.arch, "disk_img": "d.qcow2",
			"img": "b.img"}
		a := storageArgs(c, dir, OsTuning{})
		if !has(a, tc.want) {
			t.Fatalf("%s/%s missing %q: %v", tc.os, tc.arch, tc.want, a)
		}
		if tc.absent != "" && has(a, tc.absent) {
			t.Fatalf("%s/%s must not emit %q: %v", tc.os, tc.arch, tc.absent, a)
		}
	}
	// batocera img must not double-emit a generic virtio drive too.
	a := storageArgs(Conf{"guest_os": "batocera", "disk_img": "d.qcow2",
		"img": "b.img"}, dir, OsTuning{})
	if has(a, "if=virtio") {
		t.Fatalf("batocera img must be BootDisk only: %v", a)
	}
	// extra_disks: :fmt suffix + missing skip.
	os.WriteFile(filepath.Join(dir, "x.raw"), []byte("x"), 0o600)
	a = storageArgs(Conf{"extra_disks": "x.raw:raw missing.qcow2"}, dir, OsTuning{})
	if !has(a, "ExtraDisk0") || !has(a, "format=raw") || has(a, "missing.qcow2") {
		t.Fatalf("extra_disks: %v", a)
	}
}

// captureStdout: capture fmt.Print output produced by fn.
func captureStdout(t *testing.T, fn func()) string {
	return qetest.CaptureStdout(t, fn)
}

// STATUS_QUO (vm.sh): -snapshot is appended right after each writable
// disk drive — SystemDisk, ExtraDisk, if=ide — never to cdrom/pflash.
func TestStatusQuoPerDrive(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"d.qcow2", "x.qcow2", "i.iso"} {
		os.WriteFile(filepath.Join(dir, f), make([]byte, 2<<20), 0o644)
	}
	c := Conf{"guest_os": "linux", "disk_img": "d.qcow2",
		"extra_disks": "x.qcow2", "iso": "i.iso", "snapshot": "on"}
	a := storageArgs(c, dir, OsTuning{})
	j := strings.Join(a, " ")
	if n := strings.Count(j, "-snapshot"); n != 2 {
		t.Fatalf("snapshot count=%d want 2 (SystemDisk+ExtraDisk): %v", n, a)
	}
	// Each -snapshot must immediately follow its two-token
	// "-drive <spec>" element (single-token "-drive id=…" elements are
	// rejected by exec-style spawning — no shell word-splitting).
	for i, s := range a {
		if s == "-snapshot" && !(i > 1 && a[i-2] == "-drive") {
			t.Fatalf("-snapshot does not follow a -drive: %v", a)
		}
	}
	for _, s := range a {
		if strings.HasPrefix(s, "-drive ") {
			t.Fatalf("single-token -drive element would break exec spawn: %v", a)
		}
	}
	// reactos if=ide drive also carries it.
	r := storageArgs(Conf{"guest_os": "reactos", "disk_img": "d.qcow2",
		"snapshot": "on"}, dir, OsTuning{})
	if !has(r, "-snapshot") {
		t.Fatalf("reactos if=ide missing -snapshot: %v", r)
	}
	// snapshot off: no args at all.
	o := storageArgs(Conf{"guest_os": "linux", "disk_img": "d.qcow2"}, dir, OsTuning{})
	if has(o, "-snapshot") {
		t.Fatalf("snapshot=off emitted -snapshot: %v", o)
	}
}
