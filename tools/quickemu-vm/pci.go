package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jaypipes/pcidb"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// pciGPUDevice is one display-class PCI device for the passthrough
// listing (lspci -Dnn parity via sysfs + pci.ids).
type pciGPUDevice struct {
	PCI, Name, Driver string
}

// pciGPUDevices replaces `lspci -Dnn | grep vga|3d|display` with a
// sysfs walk + pcidb name resolution.
func pciGPUDevices() []pciGPUDevice {
	ents, err := os.ReadDir("/sys/bus/pci/devices")
	if err != nil {
		return nil
	}
	db, derr := pcidb.New()
	var out []pciGPUDevice
	for _, e := range ents {
		cls := pciAttr(e.Name(), "class")
		if len(cls) < 4 || cls[:4] != "0x03" {
			continue // display family only (VGA/XGA/3D/other)
		}
		vhex, dhex := pciHex(e.Name(), "vendor"), pciHex(e.Name(), "device")
		vendor, product := vhex, dhex
		className := "Display controller"
		if derr == nil {
			if v := db.Vendors[vhex]; v != nil {
				vendor = v.Name
			}
			if p := db.Products[vhex+dhex]; p != nil {
				product = p.Name
			}
			if c := db.Classes[cls[2:4]]; c != nil {
				for _, sc := range c.Subclasses {
					if sc.ID == cls[4:6] {
						className = sc.Name
						break
					}
				}
			}
		}
		drv := ""
		if t, err := os.Readlink(filepath.Join("/sys/bus/pci/devices", e.Name(), "driver")); err == nil {
			drv = filepath.Base(t)
		}
		name := fmt.Sprintf("%s [%s]: %s %s [%s:%s]",
			className, cls[2:6], vendor, product, vhex, dhex)
		out = append(out, pciGPUDevice{PCI: e.Name(), Name: name, Driver: drv})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PCI < out[j].PCI })
	return out
}

func pciAttr(dev, attr string) string { return qelib.PciAttr(dev, attr) }

func pciHex(dev, attr string) string { return qelib.PciHex(dev, attr) }
