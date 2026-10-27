// hw.go: cpu/ram/bios/network arg building — migrates configure_cpu,
// configure_ram, configure_bios, configure_network from launch.sh.
package main

import (
	"fmt"
	"os"
	"os/exec"
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
	// vm.sh network emit parity:
	//   network=""      -> user: -device D,netdev=nic[M] -netdev user,hostname=VM[fwd],id=nic
	//   network=none    -> -nic none
	//   network=restrict-> -device D,netdev=nic[M] -netdev user,hostname=VM,restrict=y,id=nic
	//   network=passt   -> -device D,netdev=nic[M] -netdev passt,id=nic
	//   network=<X>     -> bridge: -nic bridge,br=X,helper=<h>,model=D[M]
	// 'bridge' conf key overrides network (vm.sh line ~961).
	net := c.get("network", "")
	if b := c.get("bridge", ""); b != "" {
		net = b
	}
	vm := c["_vmname"]
	mac := ""
	if m := c.get("macaddr", c.get("mac_addr", "")); m != "" && net != "none" {
		mac = ",mac=" + m
	}
	var a []string
	switch net {
	case "none":
		return []string{"-nic", "none"}
	case "restrict":
		return []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "user,hostname=" + vm + ",restrict=y,id=nic"}
	case "passt":
		return []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "passt,id=nic"}
	case "":
		// user mode: hostfwd appended to the user netdev
		fwd := ""
		if p := c.get("ssh_port", ""); p != "" {
			fwd = ",hostfwd=tcp:127.0.0.1:" + p + "-:22"
		}
		for _, pf := range strings.Fields(c.get("port_forwards", "")) {
			kv := strings.SplitN(pf, ":", 2)
			if len(kv) == 2 {
				fwd += ",hostfwd=tcp::" + kv[0] + "-:" + kv[1] + ",hostfwd=udp::" + kv[0] + "-:" + kv[1]
			}
		}
		return []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "user,hostname=" + vm + fwd + ",id=nic"}
	default:
		// bridge mode: network value IS the interface name
		helper, _ := exec.LookPath("qemu-bridge-helper")
		return []string{"-nic", "bridge,br=" + net + ",helper=" + helper + ",model=" + t.NetDevice + mac}
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
