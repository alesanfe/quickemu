package main

import "fmt"

// spiceArgs: configure_display SPICE stack parity — emitted when
// display is none/spice/spice-app (Bash always sets up the SPICE
// channels for agent, vdagent, webdav, usbredir and smartcard).
// Endpoint variants (launch.sh):
//
//	spice-app      -> disable-ticketing=on,gl=<gl>
//	spice + local  -> disable-ticketing=on,unix=on,addr=<vmdir>/<vm>.sock
//	spice + remote -> disable-ticketing=on,port=<p>,addr=<ip|"">
//	none           -> disable-ticketing=on,port=<p>,addr=127.0.0.1|<ip|"">
//
// `access` in the conf stands in for the Bash --access CLI option.
func spiceArgs(c Conf, dir, vmname, osName string) []string {
	d := c.get("display", "none")
	var a []string
	// -spice + agent/vdagent/webdav channels only in the spice gate;
	// usbredir/smartcard below are emitted for every display mode.
	if d == "none" || d == "spice" || d == "spice-app" {
		spice := "disable-ticketing=on"
		switch {
		case d == "spice-app":
			spice += ",gl=" + c.get("gl", "on")
		case d == "spice":
			switch access := c.get("access", "local"); access {
			case "", "local":
				// Unix socket for local access — no port consumed.
				spice += fmt.Sprintf(",unix=on,addr=%s%s.sock", dotdir(dir), vmname)
			default:
				addr := access
				if addr == "remote" {
					addr = ""
				}
				spice += ",port=" + firstFreePort(c.get("spice_port", ""), 5930, 5939) + ",addr=" + addr
			}
		default: // none
			addr := "127.0.0.1"
			switch access := c.get("access", ""); access {
			case "", "local":
			case "remote":
				addr = ""
			default:
				addr = access
			}
			spice += ",port=" + firstFreePort(c.get("spice_port", ""), 5930, 5939) + ",addr=" + addr
		}
		a = append(a, "-spice", spice)
		a = append(a, "-device", "virtio-serial-pci")
		a = append(a, "-chardev", fmt.Sprintf("socket,id=agent0,path=%s%s-agent.sock,server=on,wait=off", dotdir(dir), vmname))
		a = append(a, "-device", "virtserialport,chardev=agent0,name=org.qemu.guest_agent.0")
		a = append(a, "-chardev", "spicevmc,id=vdagent0,name=vdagent")
		a = append(a, "-device", "virtserialport,chardev=vdagent0,name=com.redhat.spice.0")
		a = append(a, "-chardev", "spiceport,id=webdav0,name=org.spice-space.webdav.0")
		a = append(a, "-device", "virtserialport,chardev=webdav0,name=org.spice-space.webdav.0")
	}
	// usb redirection (3 channels) + smartcard passthrough
	a = append(a, "-device", "qemu-xhci,id=spicepass")
	for i := 1; i <= 3; i++ {
		a = append(a, "-chardev", fmt.Sprintf("spicevmc,id=usbredirchardev%d,name=usbredir", i))
		a = append(a, "-device", fmt.Sprintf("usb-redir,chardev=usbredirchardev%d,id=usbredirdev%d", i, i))
	}
	a = append(a, "-device", "pci-ohci,id=smartpass")
	a = append(a, "-device", "usb-ccid")
	a = append(a, "-chardev", "spicevmc,id=ccid,name=smartcard")
	a = append(a, "-device", "ccid-card-passthru,chardev=ccid")
	return a
}

// inputArgs: usb input stack + keyboard layout — usb-ehci bus with
// usb-kbd + usb-tablet, and the -k keymap arg.
func inputArgs(c Conf) []string {
	k := c.get("keyboard", "en-us")
	return []string{
		"-device", "usb-ehci,id=input",
		"-device", "usb-kbd,bus=input.0",
		"-k", k,
		"-device", "usb-tablet,bus=input.0",
	}
}
