// extras_test.go: exact-arg assertions for the opt-in hardware
// features (qga channel, nested, iothreads, mq, watchdog, vsock,
// tpm_dev, pxe) plus their validateConf matrix.
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// argSeq returns the args slice element following each occurrence of
// flag — for exact "-device <spec>" assertions.
func argAfter(args []string, flag string) []string {
	var out []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			out = append(out, args[i+1])
		}
	}
	return out
}

func TestQGAChannelEmit(t *testing.T) {
	dir := t.TempDir()
	for _, display := range []string{"none", "spice", "spice-app", "sdl", "gtk", "cocoa"} {
		c := Conf{"display": display, "_vmdir": dir, "_vmname": "vm"}
		a := qgaArgs(c, dir, "vm")
		// The channel is display-independent — always the same 6 args:
		// bus, chardev, virtserialport.
		if len(a) != 6 || a[0] != "-device" || a[1] != "virtio-serial-pci" ||
			a[2] != "-chardev" || a[4] != "-device" ||
			a[5] != "virtserialport,chardev=agent0,name=org.qemu.guest_agent.0" {
			t.Fatalf("display %s qgaArgs %v", display, a)
		}
		if !strings.Contains(a[3], "path="+dir+"/.s/agent") ||
			!strings.Contains(a[3], "server=on,wait=off") || !strings.HasPrefix(a[3], "socket,id=agent0,") {
			t.Fatalf("chardev spec %q", a[3])
		}
	}
	// qga=off disables the whole emission.
	if a := qgaArgs(Conf{"qga": "off"}, dir, "vm"); a != nil {
		t.Fatalf("qga=off must emit nothing: %v", a)
	}
	if a := qgaArgs(Conf{"qga": "false"}, dir, "vm"); a != nil {
		t.Fatalf("qga=false must emit nothing: %v", a)
	}
	// SPICE block drops its own virtio-serial-pci while qga provides
	// it; with qga=off the vdagent/webdav ports need the bus back.
	sp := spiceArgs(Conf{"display": "spice", "_vmdir": dir, "_vmname": "vm"}, dir, "vm", "linux")
	if has(sp, "virtio-serial-pci") {
		t.Fatalf("duplicate bus: %v", sp)
	}
	sp = spiceArgs(Conf{"display": "spice", "qga": "off", "_vmdir": dir, "_vmname": "vm"}, dir, "vm", "linux")
	if !has(sp, "virtio-serial-pci") {
		t.Fatalf("qga=off must leave the SPICE bus: %v", sp)
	}
	if has(sp, "guest_agent") {
		t.Fatalf("agent must never come from spiceArgs: %v", sp)
	}
}

func TestChmodAgentSock(t *testing.T) {
	dir, name := t.TempDir(), "vm"
	sock := filepath.Join(dir, name+"-agent.sock")
	// A real unix socket gets tightened to 0600 on the first poll —
	// non-socket/missing paths exit quietly after the poll window.
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("unix sockets unavailable")
	}
	defer ln.Close()
	os.Chmod(sock, 0o755)
	chmodAgentSock(dir, name)
	if st, _ := os.Stat(sock); st.Mode().Perm() != 0o600 {
		t.Fatalf("agent socket must be 0600: %v", st.Mode())
	}
}

func TestNestedEmit(t *testing.T) {
	dir := t.TempDir()
	// Host probe fixture: kvm_intel nested=Y.
	old := sysfsNestedFiles
	sysfsNestedFiles = []string{filepath.Join(dir, "nested")}
	defer func() { sysfsNestedFiles = old }()
	os.WriteFile(sysfsNestedFiles[0], []byte("Y\n"), 0o600)
	if nestedHostState() != "enabled" {
		t.Fatal("fixture must read enabled")
	}
	os.WriteFile(sysfsNestedFiles[0], []byte("0\n"), 0o600)
	if nestedHostState() != "disabled" {
		t.Fatal("fixture must read disabled")
	}
	sysfsNestedFiles = []string{filepath.Join(dir, "missing")}
	if nestedHostState() != "unknown" {
		t.Fatal("unreadable params must be unknown")
	}
	// CPU flag: host model under kvm gets the vendor bit.
	flag := nestedCPUFlag("host")
	if hostCPUVendor() == "GenuineIntel" && flag != ",+vmx" ||
		hostCPUVendor() == "AuthenticAMD" && flag != ",+svm" {
		t.Fatalf("nestedCPUFlag %q for vendor %s", flag, hostCPUVendor())
	}
	if nestedCPUFlag("qemu64") != "" {
		t.Fatal("non-host cpu must not get nested flags")
	}
	// cpuArgs applies the flag under kvm, warns under tcg.
	out := captureStdout(t, func() {
		c := Conf{"nested": "on", "cpu_cores": "4", "_accel": "kvm"}
		a := cpuArgs(c, "linux", "x86_64", OsTuning{})
		for i, tok := range a {
			if tok == "-cpu" && i+1 < len(a) {
				if !strings.HasPrefix(a[i+1], "host") {
					t.Fatalf("nested needs -cpu host: %v", a)
				}
			}
		}
	})
	_ = out
	out = captureStdout(t, func() {
		c := Conf{"nested": "on", "cpu_cores": "4", "_accel": "tcg"}
		a := cpuArgs(c, "linux", "x86_64", OsTuning{})
		for i, tok := range a {
			if tok == "-cpu" && i+1 < len(a) && strings.Contains(a[i+1], "vmx") {
				t.Fatalf("TCG must not get +vmx: %v", a)
			}
		}
	})
	if !strings.Contains(out, "nested") {
		t.Fatalf("TCG warning missing: %s", out)
	}
}

func TestIothreadEmit(t *testing.T) {
	c := Conf{"iothreads": "3"}
	a := iothreadArgs(c)
	if len(a) != 6 {
		t.Fatalf("want 3 objects: %v", a)
	}
	devs := argAfter(a, "-object")
	for i, d := range devs {
		if d != fmt.Sprintf("iothread,id=iothread%d", i) {
			t.Fatalf("iothread spec %q", d)
		}
	}
	if iothreadArgs(Conf{}) != nil || iothreadArgs(Conf{"iothreads": "abc"}) != nil {
		t.Fatal("iothreads=0/junk must emit nothing")
	}
	// Storage attach: SystemDisk + extras get iothread=, IDE doesn't.
	sc := Conf{"disk_img": "d.qcow2", "iothreads": "2"}
	a = storageArgs(sc, t.TempDir(), OsTuning{})
	if !has(a, "virtio-blk-pci,drive=SystemDisk,iothread=iothread0") {
		t.Fatalf("SystemDisk iothread: %v", a)
	}
	sc = Conf{"disk_img": "d.qcow2", "guest_os": "windows-server", "iothreads": "1"}
	a = storageArgs(sc, t.TempDir(), OsTuning{})
	if has(a, "iothread=") {
		t.Fatalf("ide-hd cannot take iothread: %v", a)
	}
	// Round-robin across extras.
	sc = Conf{"extra_disks": "a.qcow2 b.qcow2 c.qcow2", "iothreads": "2"}
	dir := t.TempDir()
	for _, f := range []string{"a.qcow2", "b.qcow2", "c.qcow2"} {
		os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600)
	}
	a = storageArgs(sc, dir, OsTuning{})
	for _, want := range []string{"ExtraDisk0,iothread=iothread0",
		"ExtraDisk1,iothread=iothread1", "ExtraDisk2,iothread=iothread0"} {
		if !has(a, want) {
			t.Fatalf("missing %q: %v", want, a)
		}
	}
}

func TestMQEmit(t *testing.T) {
	// virtio-net-pci tuning → mq applied with vectors=2N+2.
	c := Conf{"mq": "4", "cpu_cores": "8", "guest_os": "linux", "_fwd": "", "_vmname": "v"}
	a := networkArgs(c, OsTuning{NetDevice: "virtio-net-pci"})
	if !has(a, "virtio-net-pci,netdev=nic,mq=on,vectors=10") {
		t.Fatalf("mq args %v", a)
	}
	// auto clamps to vCPUs (cap 16).
	c["mq"] = "auto"
	c["cpu_cores"] = "6"
	a = networkArgs(c, OsTuning{NetDevice: "virtio-net-pci"})
	if !has(a, "vectors=14") {
		t.Fatalf("auto mq %v", a)
	}
	// N > cores clamps down.
	c["mq"] = "32"
	a = networkArgs(c, OsTuning{NetDevice: "virtio-net-pci"})
	if !has(a, "vectors=14") {
		t.Fatalf("clamped mq %v", a)
	}
	// Non-virtio device → warning, no mq params.
	c["mq"] = "4"
	a = networkArgs(c, OsTuning{NetDevice: "e1000"})
	if has(a, "mq=on") {
		t.Fatalf("e1000 must not get mq: %v", a)
	}
	// mq=off → nothing.
	c["mq"] = "off"
	a = networkArgs(c, OsTuning{NetDevice: "virtio-net-pci"})
	if has(a, "mq=on") {
		t.Fatalf("mq=off: %v", a)
	}
}

func TestWatchdogEmit(t *testing.T) {
	a := watchdogArgs(Conf{"watchdog": "ib700"})
	if len(a) != 2 || a[0] != "-device" || a[1] != "ib700" {
		t.Fatalf("ib700 %v", a)
	}
	a = watchdogArgs(Conf{"watchdog": "virtio", "watchdog_action": "poweroff"})
	if len(a) != 4 || a[1] != "i6300esb" || a[2] != "-watchdog-action" || a[3] != "poweroff" {
		t.Fatalf("virtio watchdog %v", a)
	}
	if watchdogArgs(Conf{"watchdog": "bogus"}) != nil {
		t.Fatal("bogus watchdog must emit nothing")
	}
	if watchdogArgs(Conf{}) != nil {
		t.Fatal("unset watchdog must emit nothing")
	}
}

func TestVsockEmitAndRegistry(t *testing.T) {
	regEnv(t) // isolates QE_PORT_REGISTRY; cidRegistry sits beside it
	reg := cidRegistry()
	if !strings.HasSuffix(reg, "vsock-cids") {
		t.Fatalf("registry must be the ports sibling: %s", reg)
	}
	c := Conf{"vsock": "auto", "_vmname": "vm"}
	a := vsockArgs(c)
	if len(a) != 2 || !strings.HasPrefix(a[1], "vhost-vsock-pci,guest-cid=") {
		t.Fatalf("vsock emit %v", a)
	}
	if c["_vsock_cid"] == "" {
		t.Fatal("auto must resolve a CID")
	}
	// The allocated CID is reserved under our pid.
	var n int
	fmt.Sscanf(c["_vsock_cid"], "%d", &n)
	if n < 3 || !registryTaken(reg, n) {
		t.Fatalf("cid %d must be reserved", n)
	}
	// Next auto-allocation must skip the taken CID.
	c2 := Conf{"vsock": "auto", "_vmname": "vm2"}
	_ = vsockArgs(c2)
	var n2 int
	fmt.Sscanf(c2["_vsock_cid"], "%d", &n2)
	if n2 == n {
		t.Fatalf("double allocation: %d", n2)
	}
	// Explicit CID emits verbatim.
	c3 := Conf{"vsock": "42", "_vmname": "vm3"}
	a = vsockArgs(c3)
	if !has(a, "guest-cid=42") {
		t.Fatalf("explicit cid %v", a)
	}
	if !registryTaken(reg, 42) {
		t.Fatal("explicit cid must be reserved")
	}
	registryRelease(reg, os.Getpid())
	// vsock=off → nothing.
	if vsockArgs(Conf{"vsock": "off"}) != nil {
		t.Fatal("vsock=off must emit nothing")
	}
}

func TestTPMPassthroughEmit(t *testing.T) {
	a := tpmArgs(Conf{"tpm_dev": "passthrough:/dev/tpm0"}, "x86_64")
	if len(a) != 4 || a[0] != "-tpmdev" ||
		a[1] != "passthrough,id=tpm0,path=/dev/tpm0" ||
		a[2] != "-device" || a[3] != "tpm-tis,tpmdev=tpm0" {
		t.Fatalf("passthrough emit %v", a)
	}
	a = tpmArgs(Conf{"tpm_dev": "passthrough:/dev/tpmrm0"}, "aarch64")
	if !has(a, "tpm-tis-device,tpmdev=tpm0") {
		t.Fatalf("aarch64 tpm device %v", a)
	}
	// tpm=on alone still emits the swtpm stack.
	a = tpmArgs(Conf{"tpm": "on", "_vmdir": "/v", "_vmname": "x"}, "x86_64")
	if !has(a, "emulator,id=tpm0") {
		t.Fatalf("swtpm emit %v", a)
	}
	// Both set: passthrough wins (warning), no swtpm chardev.
	a = tpmArgs(Conf{"tpm": "on", "tpm_dev": "passthrough:/dev/tpm0",
		"_vmdir": "/v", "_vmname": "x"}, "x86_64")
	if has(a, "emulator") || !has(a, "passthrough,id=tpm0") {
		t.Fatalf("conflict resolution %v", a)
	}
}

func TestPXEEmit(t *testing.T) {
	a := pxeArgs(Conf{"pxe": "on"})
	if len(a) != 2 || a[0] != "-boot" || a[1] != "order=nc" {
		t.Fatalf("pxe emit %v", a)
	}
	if pxeArgs(Conf{}) != nil || pxeArgs(Conf{"pxe": "off"}) != nil {
		t.Fatal("pxe unset/off must emit nothing")
	}
}

func TestValidateNewKeys(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "v.conf")
	os.WriteFile(conf, nil, 0o644)
	bad := []Conf{
		{"qga": "maybe"}, {"nested": "x"}, {"pxe": "y"},
		{"iothreads": "0"}, {"iothreads": "65"}, {"iothreads": "2x"},
		{"mq": "3x"}, {"mq": "0"}, {"mq": "8", "cpu_cores": "4"},
		{"watchdog": "cat"}, {"watchdog_action": "explode"},
		{"vsock": "2"}, {"vsock": "abc"}, {"vsock": "5000000000"},
		{"tpm_dev": "/dev/tpm0"}, {"tpm_dev": "passthrough:"},
		{"tpm_dev": "passthrough:rel/path"}, {"tpm_dev": "passthrough:/dev/tpm0,x"},
	}
	for i, c := range bad {
		if errs := validateConf(c, conf); len(errs) == 0 {
			t.Fatalf("case %d %+v must be rejected", i, c)
		}
	}
	good := []Conf{
		{"qga": "off"}, {"pxe": "on"},
		{"iothreads": "4"}, {"mq": "auto"}, {"mq": "4", "cpu_cores": "8"},
		{"watchdog": "ib700"}, {"watchdog": "virtio", "watchdog_action": "reset"},
		{"vsock": "auto"}, {"vsock": "3"}, {"vsock": "off"},
		{"tpm_dev": "passthrough:/dev/tpm0"},
	}
	for i, c := range good {
		if errs := validateConf(c, conf); len(errs) != 0 {
			t.Fatalf("case %d %+v must validate: %v", i, c, errs)
		}
	}
	// nested: sysfs-disabled host is a hard error; unknown/enabled ok.
	old := sysfsNestedFiles
	defer func() { sysfsNestedFiles = old }()
	sysfsNestedFiles = []string{filepath.Join(dir, "n")}
	os.WriteFile(sysfsNestedFiles[0], []byte("0"), 0o600)
	if errs := validateConf(Conf{"nested": "on"}, conf); len(errs) == 0 {
		t.Fatal("nested=on on disabled host must error")
	}
	os.WriteFile(sysfsNestedFiles[0], []byte("Y"), 0o600)
	if errs := validateConf(Conf{"nested": "on"}, conf); len(errs) != 0 {
		t.Fatalf("nested=on on enabled host: %v", errs)
	}
	sysfsNestedFiles = []string{filepath.Join(dir, "missing")}
	if errs := validateConf(Conf{"nested": "on"}, conf); len(errs) != 0 {
		t.Fatalf("unverifiable host must stay permissive: %v", errs)
	}
}

// guestCores: the resolution cpuArgs shares with the mq clamp.
func TestGuestCores(t *testing.T) {
	if n := guestCores(Conf{"cpu_cores": "6"}, "linux"); n != 6 {
		t.Fatalf("cores %d", n)
	}
	if n := guestCores(Conf{"cpu_cores": "7", "_vmname": "windows-11-24H2"}, "windows"); n != 7 {
		t.Fatalf("windows cores %d", n)
	}
	if n := guestCores(Conf{"cpu_cores": "1", "_vmname": "windows-11-x"}, "windows"); n != 2 {
		t.Fatalf("win11 floor %d", n)
	}
	if n := guestCores(Conf{"cpu_cores": "6"}, "macos"); n != 4 {
		t.Fatalf("macos pow2 %d", n)
	}
	if n := guestCores(Conf{"cpu_cores": "abc"}, "linux"); n != 2 {
		t.Fatalf("junk cores %d", n)
	}
}

// sockPreflight: a VM dir deep enough that even the fixed .s/<kind>
// basenames overflow sun_path must fail BEFORE QEMU spawns — the
// prov-e2e debian-cloud-13-genericcloud launch died on exactly this,
// and QEMU's own error only surfaces deep in <vm>.log.
func TestSockPreflightTooDeep(t *testing.T) {
	if os.Getenv("QE_SOCKFATAL_CHILD") == "1" {
		deep := filepath.Join(t.TempDir(), strings.Repeat("d", 120))
		p := &launchPlan{vmdir: deep, conf: Conf{}}
		p.sockPreflight()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSockPreflightTooDeep$")
	cmd.Env = append(os.Environ(), "QE_SOCKFATAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected exit 1, got 0: %s", out)
	}
	if !strings.Contains(string(out), "over the 107-byte unix limit") {
		t.Fatalf("missing socket-limit error: %s", out)
	}
}

// Regression for the debian-cloud-13-genericcloud prov-e2e failure:
// a long VM name nested under a deep workdir pushed the old
// <vm>-agent.sock past sun_path; .s/<kind> keeps every emitted path
// short regardless.
func TestSockPathsStayShort(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "prov-e2e-20261101",
		"debian-cloud-13-genericcloud")
	vm := "debian-cloud-13-genericcloud"
	for _, a := range qgaArgs(Conf{}, dir, vm) {
		if i := strings.Index(a, "path="); i >= 0 {
			path := a[i+5:]
			if j := strings.IndexByte(path, ','); j >= 0 {
				path = path[:j]
			}
			if len(path) > 107 {
				t.Fatalf("qga path over sun_path: %q", a)
			}
		}
	}
	p := &launchPlan{conf: Conf{}, vmdir: dir, vmname: vm}
	var args []string
	sockPathLen := func(tok string) int {
		if i := strings.Index(tok, "unix:"); i >= 0 {
			return len(tok[i+5 : i+5+strings.IndexByte(tok[i+5:], ',')])
		}
		return -1
	}
	for _, tok := range p.monitorChan(append(args, "x")) {
		if n := sockPathLen(tok); n > 107 {
			t.Fatalf("monitor path over sun_path: %q", tok)
		}
	}
	for _, tok := range p.serialChan(append(args, "x")) {
		if n := sockPathLen(tok); n > 107 {
			t.Fatalf("serial path over sun_path: %q", tok)
		}
	}
	if !has(qgaArgs(Conf{}, dir, vm), "socket,id=agent0,path="+dir+"/.s/agent,server=on,wait=off") {
		t.Fatal("agent channel must bind .s/agent")
	}
}
