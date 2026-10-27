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
	if len(os.Args) > 2 && os.Args[2] == "--validate" {
		if errs := validateConf(conf, os.Args[1]); len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, "invalid:", e)
			}
			os.Exit(1)
		}
		fmt.Println("valid")
		return
	}
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

	// vm_boot order: cpu->ram->bios->os_quirks->storage->display->audio
	// ->ports->fileshare->usb->tpm->boot_media->channels->spawn
	args = append(args, cpuArgs(conf, osName, arch)...)
	args = append(args, ramArgs(conf)...)
	args = append(args, biosArgs(conf, osName, arch)...)
	args = append(args, osQuirks(conf, osName, arch)...)
	args = append(args, storageArgs(conf, dir)...)
	args = append(args, displayArgs(conf, dir, osName, arch)...)
	args = append(args, audioArgs(conf, osName)...)
	args = append(args, portArgs(conf, dir)...)
	args = append(args, fileshareArgs(conf, dir)...)
	args = append(args, passthruArgs(conf)...)
	args = append(args, bootMediaArgs(conf, dir)...)
	args = append(args, channelArgs(conf, dir)...)
	// monitor + serial sockets
	args = append(args, "-monitor", "unix:"+filepath.Join(dir, "monitor.sock")+",server,nowait")
	args = append(args, "-qmp", "unix:"+filepath.Join(dir, "qmp.sock")+",server,nowait")
	args = append(args, "-serial", "unix:"+filepath.Join(dir, "serial.sock")+",server,nowait")

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
