package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jaypipes/pcidb"
)

// pciGPUNames returns "Vendor Product" display-class device names via
// sysfs + the pci.ids database (pcidb), replacing the `lspci` subprocess.
func pciGPUNames() []string {
	ents, err := os.ReadDir("/sys/bus/pci/devices")
	if err != nil {
		return nil
	}
	db, derr := pcidb.New()
	var out []string
	for _, e := range ents {
		cls := pciAttr(e.Name(), "class")
		if len(cls) < 4 || cls[:4] != "0x03" {
			continue // display family only
		}
		vhex, dhex := pciHex(e.Name(), "vendor"), pciHex(e.Name(), "device")
		vendor, product := vhex, dhex
		if derr == nil {
			if v := db.Vendors[vhex]; v != nil {
				vendor = v.Name
			}
			if p := db.Products[vhex+dhex]; p != nil {
				product = p.Name
			}
		}
		out = append(out, strings.TrimSpace(vendor+" "+product))
	}
	slices.Sort(out)
	return out
}

func pciAttr(dev, attr string) string {
	b, err := os.ReadFile(filepath.Join("/sys/bus/pci/devices", dev, attr))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func pciHex(dev, attr string) string {
	return strings.TrimPrefix(pciAttr(dev, attr), "0x")
}
