// quickemu-launch: generate the qemu-system command line from a .conf.
// Migrates the arg-building core of launch.sh — reads the VM config and
// emits the exact QEMU invocation. Launch itself stays Bash-spawned.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Conf map[string]string

func parseConf(path string) Conf {
	c := Conf{}
	b, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		v := strings.Trim(strings.TrimSpace(kv[1]), `"'`)
		c[strings.TrimSpace(kv[0])] = v
	}
	return c
}

func (c Conf) get(k, def string) string {
	if v, ok := c[k]; ok && v != "" {
		return v
	}
	return def
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-launch <conf> [--dry-run]")
		os.Exit(2)
	}
	conf := parseConf(os.Args[1])
	conf["_vmdir"] = filepath.Dir(os.Args[1])
	dir := filepath.Dir(os.Args[1])
	osName := conf.get("guest_os", "linux")
	arch := conf.get("arch", "x86_64")
	machine := "q35"
	if arch == "aarch64" {
		machine = "virt"
	}

	args := []string{"qemu-system-" + arch}

	// machine + accel
	accel := "tcg"
	if arch == "x86_64" {
		if _, err := os.Stat("/dev/kvm"); err == nil {
			accel = "kvm"
		}
	}
	args = append(args, "-machine", machine+",accel="+accel)

	// cpu
	cores := conf.get("cpu_cores", "2")
	cpu := "host"
	if accel == "tcg" {
		cpu = "max"
	}
	args = append(args, "-cpu", cpu, "-smp", "cores="+cores+",threads=1,sockets=1")

	// ram
	ram := conf.get("ram", "4G")
	args = append(args, "-m", ram)

	// display
	display := conf.get("display", "none")
	if display != "none" {
		args = append(args, "-display", display)
	}
	spice := conf.get("spice_port", "")
	if spice != "" {
		args = append(args, "-spice", "port="+spice+",disable-ticketing=on")
	}

	// firmware
	if conf.get("efi", "") == "true" || osName == "windows" || osName == "macos" {
		if conf.get("secureboot", "") == "true" {
			args = append(args, "-drive", "if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE.secboot.fd")
		} else {
			args = append(args, "-drive", "if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE.fd")
		}
		if conf.get("tpm", "") == "true" {
			args = append(args, "-chardev", "socket,id=chrtpm,path="+filepath.Join(dir, "tpm.sock"), "-tpmdev", "emulator,id=tpm0,chardev=chrtpm", "-device", "tpm-tis,tpmdev=tpm0")
		}
	}

	// disk
	disk := conf.get("disk_img", "")
	if disk != "" {
		args = append(args, "-drive", "file="+filepath.Join(dir, disk)+",format=qcow2,if=virtio,cache=writeback,discard=unmap")
	}
	iso := conf.get("iso", "")
	if iso != "" {
		args = append(args, "-drive", "file="+filepath.Join(dir, iso)+",media=cdrom,if=none,id=cdrom", "-device", "virtio-cdrom,drive=cdrom")
	}
	img := conf.get("img", "")
	if img != "" {
		args = append(args, "-drive", "file="+filepath.Join(dir, img)+",format=raw,if=virtio")
	}
	fixed := conf.get("fixed_iso", "")
	if fixed != "" {
		args = append(args, "-drive", "file="+filepath.Join(dir, fixed)+",media=cdrom")
	}

	// network
	net := conf.get("network", "user")
	switch net {
	case "user", "":
		ssh := conf.get("ssh_port", "")
		if ssh != "" {
			args = append(args, "-netdev", "user,id=net0,hostfwd=tcp::"+ssh+"-:22", "-device", "virtio-net,netdev=net0")
		} else {
			args = append(args, "-netdev", "user,id=net0", "-device", "virtio-net,netdev=net0")
		}
	case "bridge":
		br := conf.get("bridge", "br0")
		mac := conf.get("mac_addr", "")
		args = append(args, "-netdev", "bridge,id=net0,br="+br, "-device", "virtio-net,netdev=net0,mac="+mac)
	}

	// monitor + serial sockets
	args = append(args, "-monitor", "unix:"+filepath.Join(dir, "monitor.sock")+",server,nowait")
	args = append(args, "-qmp", "unix:"+filepath.Join(dir, "qmp.sock")+",server,nowait")
	args = append(args, "-serial", "unix:"+filepath.Join(dir, "serial.sock")+",server,nowait")

	args = append(args, displayArgs(conf, dir, osName, arch)...)
	args = append(args, audioArgs(conf, osName)...)
	args = append(args, osQuirks(conf, osName, arch)...)
	// usb passthrough
	if devs := conf.get("usb_devices", ""); devs != "" {
		for _, d := range strings.Fields(devs) {
			args = append(args, "-device", "usb-host,vendorid=0x"+strings.Split(d, ":")[0]+",productid=0x"+strings.Split(d, ":")[1])
		}
	}

	// pidfile + daemonize
	args = append(args, "-pidfile", filepath.Join(dir, "vm.pid"))
	args = append(args, "-name", filepath.Base(dir))

	if len(os.Args) > 2 && os.Args[2] == "--run" {
		if err := runQEMU(os.Args[1], args); err != nil {
			fmt.Fprintln(os.Stderr, "launch failed:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Println(strings.Join(args, " "))
}
