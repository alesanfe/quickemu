// hw.go: cpu/ram/bios/network arg building — migrates configure_cpu,
// configure_ram, configure_bios, configure_network from launch.sh.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hostSMT reports whether the host has SMT/hyperthreading on —
// Bash reads /sys/devices/system/cpu/smt/control.
func hostSMT() bool {
	b, err := os.ReadFile("/sys/devices/system/cpu/smt/control")
	return err == nil && strings.TrimSpace(string(b)) == "on"
}

func cpuArgs(c Conf, osName, arch string) []string {
	cores := c.get("cpu_cores", "2")
	cpu := "host"
	accel := "tcg"
	if arch == "x86_64" {
		accel = "kvm"
	}
	if accel == "tcg" {
		cpu = "max"
	}
	var a []string
	switch osName {
	case "macos":
		a = append(a, "-cpu", cpu+",-pdpe1gb,+hypervisor,vmware-cpuid-freq=on,vendor=GenuineIntel")
	case "windows":
		a = append(a, "-cpu", cpu+",+hypervisor,+invtsc,l3-cache=on")
	default:
		a = append(a, "-cpu", cpu)
	}
	// Bash configure_cpu: cpu_cores is LOGICAL cpus — split into
	// cores x threads when host SMT is on (cores>=2).
	n := 2
	fmt.Sscanf(cores, "%d", &n)
	threads := 1
	if n >= 2 && hostSMT() {
		threads = 2
	}
	a = append(a, "-smp", fmt.Sprintf("cores=%d,threads=%d,sockets=1", n/threads, threads))
	return a
}

func ramArgs(c Conf) []string {
	ram := c.get("ram", "4G")
	a := []string{"-m", ram}
	if c.get("balloon", "") == "true" {
		a = append(a, "-device", "virtio-balloon")
	}
	return a
}

func biosArgs(c Conf, osName, arch string) []string {
	var a []string
	if arch == "aarch64" {
		a = append(a, "-bios", "/usr/share/AAVMF/AAVMF_CODE.fd")
		return a
	}
	efi := c.get("efi", "") == "true" || osName == "windows" || osName == "macos"
	if !efi {
		return a
	}
	// Bash configure_bios: OVMF uses TWO pflash drives — CODE (unit=0,
	// readonly) + VARS (unit=1, writable per-VM copy) — plus the secure
	// pflash global. The vars file is copied into the vmdir at launch.
	vars := filepath.Join(c["_vmdir"], "OVMF_VARS.fd")
	a = append(a, "-global", "driver=cfi.pflash01,property=secure,value=on")
	if c.get("secureboot", "") == "true" {
		a = append(a, "-drive", "if=pflash,format=raw,unit=0,file=/usr/share/OVMF/OVMF_CODE.secboot.fd,readonly=on")
	} else {
		a = append(a, "-drive", "if=pflash,format=raw,unit=0,file=/usr/share/OVMF/OVMF_CODE_4M.fd,readonly=on")
	}
	a = append(a, "-drive", "if=pflash,format=raw,unit=1,file="+vars)
	if c.get("tpm", "") == "true" {
		a = append(a,
			"-chardev", "socket,id=chrtpm,path="+filepath.Join(c["_vmdir"], "tpm.sock"),
			"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
			"-device", "tpm-tis,tpmdev=tpm0")
	}
	return a
}

func networkArgs(c Conf, t OsTuning) []string {
	net := c.get("network", "user")
	ssh := c.get("ssh_port", "")
	var a []string
	switch net {
	case "user", "":
		fwd := ""
		if ssh != "" {
			fwd = ",hostfwd=tcp::" + ssh + "-:22"
		}
		a = append(a, "-netdev", "user,id=net0"+fwd, "-device", t.NetDevice+",netdev=net0")
	case "bridge":
		br := c.get("bridge", "br0")
		mac := c.get("mac_addr", "")
		nd := "-netdev"
		args := "bridge,id=net0,br=" + br
		dev := t.NetDevice + ",netdev=net0"
		if mac != "" {
			dev += ",mac=" + mac
		}
		a = append(a, nd, args, "-device", dev)
	case "none":
		// no net device
	}
	return a
}

// portArgs: SSH/SPICE port forwarding — configure_ports. Allocates
// ports when not specified (delegates to quickemu-ports).
func portArgs(c Conf, dir string) []string {
	var a []string
	ssh := c.get("ssh_port", "")
	if ssh != "" {
		// already handled in networkArgs hostfwd — nothing extra
		return a
	}
	spice := c.get("spice_port", "")
	if spice != "" {
		return a // spice args built in displayArgs
	}
	return a
}
