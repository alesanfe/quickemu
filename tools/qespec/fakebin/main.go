// fakebin is a multicall test double replacing the spec/fake-bin bash
// stubs: argv[0] selects qemu-img / qemu-system-x86_64 / socat /
// xdg-open / quickemu. Behaviours mirror the original stubs byte for
// byte where tests depend on them.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	switch filepath.Base(os.Args[0]) {
	case "qemu-img":
		qemuImg(os.Args[1:])
	case "qemu-system-x86_64":
		qemuSystem(os.Args[1:])
	case "socat":
		socat()
	case "xdg-open":
		os.Exit(0)
	case "quickemu":
		fmt.Println("9.9.9")
	default:
		os.Exit(0)
	}
}

// qemu-img stub: create/convert write a 1-byte file (honouring
// STUB_QEMUIMG_FAIL), resize/info canned, everything else exits 0.
func qemuImg(args []string) {
	for _, a := range args {
		if a == "--version" || a == "-version" || a == "version" {
			fmt.Println("qemu-img version 8.2.2 (stub)")
			os.Exit(0)
		}
	}
	if len(args) == 0 {
		os.Exit(1)
	}
	cmd := args[0]
	args = args[1:]
	last := func() string { return args[len(args)-1] }
	switch cmd {
	case "create", "convert":
		os.WriteFile(last(), []byte{0}, 0o644)
		if os.Getenv("STUB_QEMUIMG_FAIL") == "1" {
			os.Exit(1)
		}
	case "resize":
		f, err := os.OpenFile(args[len(args)-2], os.O_WRONLY, 0)
		if err == nil {
			f.WriteAt([]byte{0}, 1)
			f.Close()
		}
	case "info":
		p := last()
		if _, err := os.Stat(p); err != nil {
			os.Exit(1)
		}
		fmt.Printf("image: %s\nfile format: qcow2\nvirtual size: 16 GiB\n", p)
	}
}

// qemu-system stub: answers the probe invocations then sleeps as a
// fake VM (STUB_QEMU_SLEEP seconds, default 1), recording STUB_PID_FILE.
func qemuSystem(args []string) {
	for _, a := range args {
		if a == "--version" || a == "-version" {
			fmt.Println("QEMU emulator version 8.2.2 (stub)")
			return
		}
	}
	prev := ""
	for _, a := range args {
		if prev == "-display" && a == "help" {
			fmt.Print("gtk\nsdl\ncocoa\nnone\nspice\nspice-app\negll-headless\n")
			return
		}
		if prev == "-device" && a == "help" {
			fmt.Print("\"virtio-vga\"\n\"virtio-vga-gl\"\n\"virtio-gpu-gl-pci\"\n\"virtio-gpu-pci\"\n\"qemu-xhci\"\n\"usb-host\"\n\"vhost-user-fs-pci\"\n")
			return
		}
		if a == "-audio-help" {
			fmt.Print("pa\npipewire\nalsa\nnone\n")
			return
		}
		prev = a
	}
	secs, _ := strconv.Atoi(os.Getenv("STUB_QEMU_SLEEP"))
	if secs <= 0 {
		secs = 1
	}
	pf := os.Getenv("STUB_PID_FILE")
	if pf == "" {
		pf = "/tmp/qemu-stub.pid"
	}
	os.WriteFile(pf, []byte(fmt.Sprint(os.Getpid())), 0o644)
	time.Sleep(time.Duration(secs) * time.Second)
}

// socat stub: append stdin lines to STUB_SOCAT_LOG.
func socat() {
	log := os.Getenv("STUB_SOCAT_LOG")
	if log == "" {
		log = "/tmp/socat-stub.log"
	}
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		os.Exit(1)
	}
	defer f.Close()
	io.Copy(f, os.Stdin)
	_ = strings.TrimSpace
}
