package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

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
func spiceArgs(c Conf, dir, vmname, osName string, t OsTuning) []string {
	d := c.get("display", "none")
	var a []string
	// -spice + agent/vdagent/webdav channels only in the spice gate;
	// usbredir/smartcard below are emitted for every display mode.
	if d == "none" || d == "spice" || d == "spice-app" {
		spice := "disable-ticketing=on"
		// spicy hint tail: --spice-shared-dir (non-macOS, PUBLIC set)
		// plus the --fullscreen flag's --full-screen suffix.
		share, fs := "", ""
		if pub := c["_public"]; pub != "" && osName != "macos" {
			share = " --spice-shared-dir " + pub
		}
		if confOn(c, "fullscreen") {
			fs = " --full-screen"
		}
		switch {
		case d == "spice-app":
			spice += ",gl=" + qemuOptEsc(c.get("gl", "on"))
			fmt.Printf(" - SPICE:    Enabled\n")
		case d == "spice":
			switch access := c.get("access", "local"); access {
			case "", "local":
				// Unix socket for local access — no port consumed.
				sock := filepath.Join(dir, vmname+".sock")
				spice += ",unix=on,addr=" + qemuOptEsc(dotdir(dir)+vmname+".sock")
				portsRecord(dir, vmname, "unix,"+sock, c["_run"] == "1")
				spiceRecord(dir, vmname, sock, c["_run"] == "1")
				fmt.Printf(" - SPICE:    On host:  spicy --uri=\"spice+unix://%s\" --title \"%s\"%s%s\n",
					sock, vmname, share, fs)
			default:
				addr := access
				if addr == "remote" {
					addr = ""
				}
				port := firstFreePort(c.get("spice_port", ""), 5930, 5939)
				if port == "" {
					fmt.Println(" - SPICE:    All SPICE ports have been exhausted.")
					fmt.Println("             ERROR! Requested SPICE display, but no SPICE ports are free.")
					os.Exit(1)
				}
				spice += ",port=" + qemuOptEsc(port) + ",addr=" + qemuOptEsc(addr)
				portsRecord(dir, vmname, "spice,"+port, c["_run"] == "1")
				spiceRecord(dir, vmname, port, c["_run"] == "1")
				fmt.Printf(" - SPICE:    On host:  spicy --title \"%s\" --port %s%s%s\n",
					vmname, port, share, fs)
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
			port := firstFreePort(c.get("spice_port", ""), 5930, 5939)
			if port == "" {
				fmt.Println(" - SPICE:    All SPICE ports have been exhausted.")
				fmt.Println("             ERROR! Requested SPICE display, but no SPICE ports are free.")
				os.Exit(1)
			}
			spice += ",port=" + qemuOptEsc(port) + ",addr=" + qemuOptEsc(addr)
			portsRecord(dir, vmname, "spice,"+port, c["_run"] == "1")
			spiceRecord(dir, vmname, port, c["_run"] == "1")
			host := ""
			if addr != "" {
				host = " --host " + addr
			}
			fmt.Printf(" - SPICE:    On host:  spicy --title \"%s\"%s --port %s%s%s\n",
				vmname, host, port, share, fs)
		}
		a = append(a, "-spice", spice)
		a = append(a, "-device", "virtio-serial-pci")
		a = append(a, "-chardev", fmt.Sprintf("socket,id=agent0,path=%s,server=on,wait=off", qemuOptEsc(dotdir(dir)+vmname+"-agent.sock")))
		a = append(a, "-device", "virtserialport,chardev=agent0,name=org.qemu.guest_agent.0")
		a = append(a, "-chardev", "spicevmc,id=vdagent0,name=vdagent")
		a = append(a, "-device", "virtserialport,chardev=vdagent0,name=com.redhat.spice.0")
		a = append(a, "-chardev", "spiceport,id=webdav0,name=org.spice-space.webdav.0")
		a = append(a, "-device", "virtserialport,chardev=webdav0,name=org.spice-space.webdav.0")
	}
	// usb redirection (3 channels) + smartcard passthrough — the
	// spicepass controller follows USB_HOST_PASSTHROUGH_CONTROLLER.
	spctl := t.UsbPassthrough
	if spctl == "" {
		spctl = "qemu-xhci"
	}
	a = append(a, "-device", spctl+",id=spicepass")
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

// inputArgs: usb-controller + keyboard + mouse stack (vm.sh). Quirk
// values (t.*) beat conf keys, conf keys beat the ehci/usb/tablet
// defaults. ps2/none emit nothing; -k only when keyboard_layout set.
func inputArgs(c Conf, t OsTuning) []string {
	var a []string
	ctl := t.UsbController
	if ctl == "" {
		ctl = c.get("usb_controller", "ehci")
	}
	switch ctl {
	case "ehci":
		a = append(a, "-device", "usb-ehci,id=input")
	case "xhci":
		a = append(a, "-device", "qemu-xhci,id=input")
	case "none":
	default:
		fmt.Fprintf(os.Stderr, " - WARNING!  Unknown usb-controller value: '%s'\n", ctl)
	}
	kbd := t.Keyboard
	if kbd == "" {
		kbd = c.get("keyboard", "usb")
	}
	switch kbd {
	case "usb":
		a = append(a, "-device", "usb-kbd,bus=input.0")
	case "virtio":
		a = append(a, "-device", "virtio-keyboard")
	case "ps2":
	default:
		fmt.Fprintf(os.Stderr, " - WARNING!  Unknown keyboard value: '%s'; Fallback to ps2\n", kbd)
	}
	if l := c.get("keyboard_layout", ""); l != "" {
		a = append(a, "-k", l)
	}
	mouse := t.Mouse
	if mouse == "" {
		mouse = c.get("mouse", "tablet")
	}
	switch mouse {
	case "usb":
		a = append(a, "-device", "usb-mouse,bus=input.0")
	case "tablet":
		a = append(a, "-device", "usb-tablet,bus=input.0")
	case "virtio":
		a = append(a, "-device", "virtio-mouse")
	case "ps2":
	default:
		fmt.Fprintf(os.Stderr, " - WARNING!  Unknown mouse value: '%s'; Falling back to ps2\n", mouse)
	}
	return a
}

// usbArgs: usb_devices passthrough (vm_boot_passthru) — the hostpass
// controller + one usb-host device per declared vid:pid. The lsusb
// reachability/permission gate is checked at --run time.
func usbArgs(c Conf, t OsTuning) []string {
	devs := confList(c, "usb_devices")
	if len(devs) == 0 {
		return nil
	}
	ctl := t.UsbPassthrough
	if ctl == "" {
		ctl = "qemu-xhci"
	}
	a := []string{"-device", ctl + ",id=hostpass"}
	for _, d := range devs {
		vp := strings.SplitN(d, ":", 2)
		if len(vp) != 2 {
			continue
		}
		a = append(a, "-device", "usb-host,bus=hostpass.0,vendorid=0x"+qemuOptEsc(vp[0])+",productid=0x"+qemuOptEsc(vp[1]))
	}
	return a
}

// checkUSBPassthrough: configure_usb_passthrough — probe each declared
// vid:pid via lsusb. Devices that do not resolve are dropped from the
// passthrough list (Bash `continue`s before appending them); a present
// but unwritable /dev/bus/usb node aborts the launch so udev rules can
// be fixed first.
func checkUSBPassthrough(c Conf) {
	devs := confList(c, "usb_devices")
	if len(devs) == 0 || runtime.GOOS != "linux" {
		return
	}
	var kept []string
	notReady := false
	for _, d := range devs {
		if strings.Count(d, ":") != 1 {
			continue
		}
		var out []byte
		if bin, err := exec.LookPath("lsusb"); err == nil {
			out, _ = execCommand(bin, "-d", d).Output()
		}
		// Bus 001 Device 002: ID 046d:c52b Logitech, Inc. Unifying Receiver
		f := strings.Fields(string(out))
		name, bus, dev := "", "", ""
		if len(f) >= 6 {
			bus = f[1]
			dev = strings.TrimSuffix(f[3], ":")
			name = strings.Join(f[6:], " ")
		}
		if name == "" {
			fmt.Printf("             ! USB device %s not found. Check your configuration\n", d)
			continue
		}
		devPath := "/dev/bus/usb/" + bus + "/" + dev
		if fh, err := os.OpenFile(devPath, os.O_WRONLY, 0); err == nil {
			fh.Close()
			fmt.Printf("             o %s on bus %s device %s is accessible.\n", name, bus, dev)
		} else {
			fmt.Printf("             x %s on bus %s device %s needs permission changes:\n", name, bus, dev)
			fmt.Printf("               sudo chown -v %s:root %s\n", os.Getenv("USER"), devPath)
			notReady = true
		}
		kept = append(kept, d)
	}
	if notReady {
		fmt.Printf("               ERROR! USB permission changes are required 👆\n")
		os.Exit(1)
	}
	c["usb_devices"] = strings.Join(kept, " ")
}
