// inject_test.go — values derived from conf keys or the vm name land
// inside QEMU comma-separated option strings. An unescaped ',' splits
// into extra options: file=d,ro=on.qcow2 opens "d" and injects a bogus
// option; hostname=x,hostfwd=... silently adds a port forward. QEMU's
// own escaping is ',,' — every interpolated value must carry it.
package main

import (
	"strings"
	"testing"
)

func TestOptionInjectionViaConfValues(t *testing.T) {
	dir := "/vmdir"
	tun := OsTuning{NetDevice: "virtio-net-pci"}

	// disk path containing a comma: unescaped it becomes file=/vmdir/a
	// plus a junk option.
	a := storageArgs(Conf{"disk_img": "a,b.qcow2", "guest_os": "linux"}, dir, tun)
	if j := strings.Join(a, " "); !strings.Contains(j, "file=/vmdir/a,,b.qcow2") {
		t.Fatalf("comma in disk_img must escape as ,,: %v", a)
	}

	// macaddr injecting a -device option.
	a = networkArgs(Conf{"macaddr": "52:54:00:1,x=y", "network": "none", "_vmname": "v"}, tun)
	if j := strings.Join(a, " "); strings.Contains(j, "mac=52:54:00:1,x=y") {
		t.Fatalf("macaddr split into an option boundary: %v", a)
	}

	// hostname=<vmname> — injection via the conf filename itself.
	a = networkArgs(Conf{"network": "restrict", "_vmname": "evil,hostfwd=tcp::1337-:22"}, tun)
	if j := strings.Join(a, " "); !strings.Contains(j, "hostname=evil,,hostfwd") {
		t.Fatalf("comma in vm name must escape inside hostname=: %v", a)
	}

	// port_forwards operands ride through ':'-split — commas inject.
	a = networkArgs(Conf{"port_forwards": "80:8080,ro=on", "_vmname": "v"}, tun)
	if j := strings.Join(a, " "); strings.Contains(j, "-:8080,ro=on") {
		t.Fatalf("port_forwards injected an option boundary: %v", a)
	}

	// explicit ssh_port rides into hostfwd verbatim.
	a = networkArgs(Conf{"ssh_port": "2222,ro=on", "_vmname": "v"}, tun)
	if j := strings.Join(a, " "); strings.Contains(j, ":2222,ro=on-") {
		t.Fatalf("ssh_port injected an option boundary: %v", a)
	}

	// iso path.
	a = bootMediaArgs(Conf{"iso": "i,so.iso", "guest_os": "kolibrios"}, dir)
	if j := strings.Join(a, " "); !strings.Contains(j, "file=/vmdir/i,,so.iso") {
		t.Fatalf("comma in iso path must escape: %v", a)
	}

	// vfio device host= atom.
	a = passthruArgs(Conf{"vfio": "on", "vfio_devices": "0000:01:00.1,ro=on"})
	if j := strings.Join(a, " "); strings.Contains(j, "host=0000:01:00.1,ro=on") {
		t.Fatalf("vfio device injected an option boundary: %v", a)
	}

	// SPICE unix socket path derived from vmdir+vmname.
	a = spiceArgs(Conf{"display": "spice", "access": "local", "_vmdir": dir, "_vmname": "v,x"}, dir, "v,x", "linux", tun)
	if j := strings.Join(a, " "); strings.Contains(j, "addr=/vmdir/v,x.sock") {
		t.Fatalf("comma in socket path split the -spice options: %v", a)
	}

	// samba share path on the user netdev.
	a = networkArgs(Conf{"_vmname": "v", "_public": "/pub,lic"}, tun)
	if j := strings.Join(a, " "); strings.Contains(j, "smb=/pub,lic") {
		t.Fatalf("comma in share path split the netdev options: %v", a)
	}
}
