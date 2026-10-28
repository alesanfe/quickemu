package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/jaypipes/pcidb"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// pciGPUs enumerates display-class PCI devices via sysfs + the pci.ids
// database (pcidb), replacing the `lspci -Dnn` subprocess. Returned
// lines match lspci -Dnn output shape:
//
//	0000:01:00.0 VGA compatible controller [0300]: Vendor Product [10de:1b81] (rev a1)
func pciGPUs() []string {
	ents, err := os.ReadDir("/sys/bus/pci/devices")
	if err != nil {
		return nil
	}
	db, derr := pcidb.New()
	var out []string
	for _, e := range ents {
		cls := pciAttr(e.Name(), "class")
		if len(cls) < 4 || cls[:4] != "0x03" {
			continue // display family only (VGA/XGA/3D/other)
		}
		vhex := pciHex(e.Name(), "vendor")
		dhex := pciHex(e.Name(), "device")
		vendor, product := vhex, dhex
		if derr == nil {
			if v := db.Vendors[vhex]; v != nil {
				vendor = v.Name
			}
			if p := db.Products[vhex+dhex]; p != nil {
				product = p.Name
			}
		}
		className := "Display controller"
		if derr == nil && len(cls) >= 6 {
			if c := db.Classes[cls[2:4]]; c != nil {
				for _, sc := range c.Subclasses {
					if sc.ID == cls[4:6] {
						className = sc.Name
						break
					}
				}
			}
		}
		line := fmt.Sprintf("%s %s [%s]: %s %s [%s:%s]",
			e.Name(), className, cls[2:6], vendor, product, vhex, dhex)
		if rev := pciAttr(e.Name(), "revision"); rev != "" {
			line += " (rev " + strings.TrimPrefix(rev, "0x") + ")"
		}
		out = append(out, line)
	}
	slices.Sort(out)
	return out
}

func pciAttr(dev, attr string) string { return qelib.PciAttr(dev, attr) }

func pciHex(dev, attr string) string { return qelib.PciHex(dev, attr) }
