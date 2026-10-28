package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// actHotplugUSB: vm_hotplug_usb parity — usb-host device_add.
func actHotplugUSB() int {
	vp := extra["hotplug-usb"]
	if ok, _ := filepath.Match("[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]:[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]", vp); !ok {
		fmt.Fprintln(os.Stderr, "ERROR! --hotplug-usb needs vendor:product hex (e.g. 05ac:12a8).")
		return 1
	}
	var vid, pid uint64
	fmt.Sscanf(vp, "%x:%x", &vid, &pid)
	id := "hotusb" + strings.ReplaceAll(vp, ":", "")
	payload := qmpCmd("device_add", map[string]any{"driver": "usb-host", "id": id, "vendorid": vid, "productid": pid})
	out, err := qmpRaw(payload)
	fmt.Print(out)
	if persistFlag {
		// Desired state vs observed QMP state (vm_hotplug_usb parity):
		// the conf records the device for next boot even when the live
		// attach failed.
		hotplugPersist("usb", vp)
	}
	if err == nil {
		qeEvent("vm.hotplug", jsonStr(map[string]any{"device": "usb", "id": id}))
		fmt.Println(" - Hot-plugged USB " + vp + " as " + id + ".")
	}
	return rc(err)
}

// actHotplugDisk: vm_hotplug_disk parity — virtio-blk + blockdev-add.
func actHotplugDisk() int {
	f := extra["hotplug-disk"]
	abs, err := filepath.Abs(f)
	if err != nil || !fileExists(abs) {
		fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", f)
		return 1
	}
	base := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, filepath.Base(f))
	id := "hotdisk" + base
	node := id + "node"
	out, err := qmpRaw(qmpCmd("blockdev-add", map[string]any{"node-name": node,
		"driver": "qcow2", "file": map[string]any{"driver": "file", "filename": abs}}))
	fmt.Print(out)
	if err == nil {
		out, err = qmpRaw(qmpCmd("device_add",
			map[string]any{"driver": "virtio-blk-pci", "id": id, "drive": node}))
		fmt.Print(out)
	}
	if persistFlag {
		hotplugPersist("disk", f)
	}
	if err == nil {
		qeEvent("vm.hotplug", jsonStr(map[string]any{"device": "disk", "id": id}))
		fmt.Println(" - Hot-plugged disk " + f + " as " + id + ".")
	}
	return rc(err)
}

// actHotunplug: vm_hotunplug parity — device_del by id.
func actHotunplug() int {
	id := extra["hotunplug"]
	payload := qmpCmd("device_del", map[string]any{"id": id})
	out, err := qmpRaw(payload)
	fmt.Print(out)
	if err == nil {
		qeEvent("vm.hotunplug", jsonStr(map[string]any{"id": id}))
		fmt.Println(" - Hot-unplugged " + id + ".")
	}
	return rc(err)
}

// hotplugPersist: append the device to the conf so it reattaches on
// next boot (--persist parity with monitor.sh). Shared with
// quickemu-qmp via qelib — a single implementation for the splice.
func hotplugPersist(kind, val string) {
	if err := qelib.HotplugPersist(vmConf, kind, val); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not persist %s %s: %v\n", kind, val, err)
		return
	}
	confKVCache.Lock()
	confKVCache.path = "" // conf changed on disk — next confKV reparses
	confKVCache.Unlock()
	fmt.Println(" - Persisted " + kind + " " + val + " into " + vmConf + " (applies next boot).")
}
