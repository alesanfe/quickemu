// quickemu-launch: generate the qemu-system command line from a .conf.
// Migrates the arg-building core of launch.sh — reads the VM config and
// emits the exact QEMU invocation. Launch itself stays Bash-spawned.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Conf map[string]string

func parseConf(path string) Conf {
	if strings.HasSuffix(path, ".toml") {
		return parseTomlConf(path)
	}
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
	conf := applyPresets(parseConf(os.Args[1]))
	conf["_vmdir"] = filepath.Dir(os.Args[1])
	conf["_vmname"] = strings.TrimSuffix(filepath.Base(os.Args[1]), filepath.Ext(os.Args[1]))
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
	resolvePublic(conf)
	vmname := strings.TrimSuffix(filepath.Base(os.Args[1]), filepath.Ext(os.Args[1]))
	osName := conf.get("guest_os", "linux")
	arch := conf.get("arch", "x86_64")
	machine := "q35"
	if arch == "aarch64" {
		machine = "virt"
	}

	args := []string{"qemu-system-" + arch}

	// machine + accel
	accel := "tcg"
	if arch == "x86_64" && hostHasKVM() {
		accel = "kvm"
	}
	args = append(args, "-accel", accel+",tb-size=256,thread=multi")
	args = append(args, "-machine", machine+",smm=off,vmport=off")

	// vm_boot order: cpu->ram->bios->os_quirks->storage->display->audio
	// ->ports->fileshare->usb->tpm->boot_media->channels->spawn
	args = append(args, cpuArgs(conf, osName, arch)...)
	args = append(args, ramArgs(conf)...)
	args = append(args, biosArgs(conf, osName, arch)...)
	t := osQuirks(conf, osName, arch)
	args = append(args, "-global", "kvm-pit.lost_tick_policy=discard")
	args = append(args, t.ExtraArgs...)
	if t.Balloon != "" {
		args = append(args, strings.Fields(t.Balloon)...)
	}
	if t.Keyboard == "ps2" {
		args = append(args, "-device", "ps2-kbd")
	}
	if t.Mouse == "ps2" {
		args = append(args, "-device", "ps2-mouse")
	} else if t.Mouse == "usb" {
		args = append(args, "-device", "usb-mouse")
	}
	if t.UsbPassthrough != "" {
		args = append(args, "-device", t.UsbPassthrough)
	}
	args = append(args, storageArgs(conf, dir, t)...)
	args = append(args, displayArgs(conf, dir, osName, arch)...)
	args = append(args, audioArgs(conf, osName, t)...)
	args = append(args, networkArgs(conf, t)...)
	args = append(args, portArgs(conf, dir)...)
	args = append(args, spiceArgs(conf, dir, vmname, osName)...)
	args = append(args, fileshareArgs(conf, dir)...)
	args = append(args, inputArgs(conf)...)
	args = append(args, passthruArgs(conf)...)
	args = append(args, bootMediaArgs(conf, dir)...)
	args = append(args, channelArgs(conf, dir)...)
	// monitor + serial sockets
	args = append(args, "-monitor", "unix:"+dotdir(dir)+vmname+"-monitor.socket"+",server,nowait")
	args = append(args, "-qmp", "unix:"+dotdir(dir)+vmname+"-qmp.socket"+",server,nowait")
	args = append(args, "-serial", "unix:"+dotdir(dir)+vmname+"-serial.socket"+",server,nowait")

	// pidfile + daemonize
	args = append(args, "-name", vmname+",process="+vmname+",debug-threads=on")
	args = append(args, "-pidfile", dotdir(dir)+vmname+".pid")
	args = append(args, "-rtc", "base=utc,clock=host")
	args = append(args, "-device", "virtio-balloon")
	args = append(args, "-object", "rng-random,id=rng0,filename=/dev/urandom", "-device", "virtio-rng-pci,rng=rng0")

	if len(os.Args) > 2 && os.Args[2] == "--run" {
		if err := runQEMU(os.Args[1], args); err != nil {
			fmt.Fprintln(os.Stderr, "launch failed:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Println(strings.Join(args, " "))
}

// parseTomlConf loads a .toml VM config via the quickemu-toml binary —
// same parser as Bash toml.sh + loader.sh. It emits a JSON envelope;
// the `data` field is the flattened key=value config.
func parseTomlConf(path string) Conf {
	c := Conf{}
	out, err := exec.Command("quickemu-toml", "parse", "--file", path).Output()
	if err != nil {
		return c
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(out, &env) != nil {
		return c
	}
	for k, v := range env.Data {
		if strings.Contains(k, ".") {
			fmt.Fprintf(os.Stderr, "WARNING! Ignoring nested TOML key %q (flat config model).\n", k)
			continue
		}
		switch x := v.(type) {
		case string:
			c[k] = x
		case bool:
			if x {
				c[k] = "true"
			}
		case float64:
			c[k] = fmt.Sprintf("%v", x)
		}
	}
	return c
}

// dotdir: Bash launches with cwd=vmdir so artifacts render as ./name —
// emit the same prefix when the conf sits in the current directory.
func dotdir(dir string) string {
	if dir == "." {
		return "./"
	}
	return dir + "/"
}
