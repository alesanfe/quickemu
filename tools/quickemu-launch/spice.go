package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
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
// spicePortsExhausted: the shared all-ports-taken refusal — same two
// lines + exit(1) wherever the 5930-5939 range is scanned.
func spicePortsExhausted() {
	fmt.Println(" - SPICE:    All SPICE ports have been exhausted.")
	fmt.Println("             ERROR! Requested SPICE display, but no SPICE ports are free.")
	os.Exit(1)
}

func spiceArgs(c Conf, dir, vmname, osName string) []string {
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
				sock := qelib.SockBind(dir, qelib.SockSpice)
				spice += ",unix=on,addr=" + qemuOptEsc(qelib.SockBind(dotdir(dir), qelib.SockSpice))
				portsRecord(dir, vmname, "unix,"+sock, c["_run"] == "1")
				spiceRecord(dir, vmname, sock, c["_run"] == "1")
				fmt.Printf(" - SPICE:    On host:  spicy --uri=\"spice+unix://%s\" --title \"%s\"%s%s\n",
					sock, vmname, share, fs)
			default:
				addr := access
				if addr == "remote" {
					addr = ""
				}
				port := firstFreePort(c.get("spice_port", ""), 5930, 5939, vmname)
				if port == "" {
					spicePortsExhausted()
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
			port := firstFreePort(c.get("spice_port", ""), 5930, 5939, vmname)
			if port == "" {
				spicePortsExhausted()
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
		// The virtio-serial bus is emitted by qgaArgs whenever the
		// agent channel is on (it is display-independent); when qga=off
		// the vdagent/webdav ports below still need a bus of their own.
		if !qgaEnabled(c) {
			a = append(a, "-device", "virtio-serial-pci")
		}
		a = append(a, "-chardev", "spicevmc,id=vdagent0,name=vdagent")
		a = append(a, "-device", "virtserialport,chardev=vdagent0,name=com.redhat.spice.0")
		a = append(a, "-chardev", "spiceport,id=webdav0,name=org.spice-space.webdav.0")
		a = append(a, "-device", "virtserialport,chardev=webdav0,name=org.spice-space.webdav.0")
	}
	return a
}

// spiceUSBArgs: the Linux usbredir/smartcard stack (vm_boot, right
// after the rng object — emitted separately from the -spice block).
// vm.sh gates the whole block on OS_KERNEL==Linux; macOS QEMU builds
// lack the SPICE chardev entirely.
func spiceUSBArgs(arch string, t OsTuning) []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	var a []string
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
	// Smartcard passthrough needs QEMU's ccid-card-passthru — probe like
	// Bash (`qemu -device help | grep "passthrough smartcard"`), warn
	// instead of emitting when unavailable.
	if strings.Contains(qemuDeviceHelp(arch), "passthrough smartcard") {
		a = append(a, "-chardev", "spicevmc,id=ccid,name=smartcard")
		a = append(a, "-device", "ccid-card-passthru,chardev=ccid")
	} else {
		fmt.Printf(" - WARNING!  %s or SPICE was not compiled with support for smartcard devices\n",
			"qemu-system-"+arch)
	}
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
		fmt.Printf(" - WARNING!  Unknown usb-controller value: '%s'\n", ctl)
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
		fmt.Printf(" - WARNING!  Unknown keyboard value: '%s'; Fallback to ps2\n", kbd)
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
		fmt.Printf(" - WARNING!  Unknown mouse value: '%s'; Falling back to ps2\n", mouse)
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
