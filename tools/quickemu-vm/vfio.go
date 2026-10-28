package main

import "path/filepath"

// vfioScan: read-only IOMMU/GPU inventory (vfio_scan).
func vfioScan(gpuPassthrough string) int {
	iommu := "absent"
	groups := 0
	if g, err := filepath.Glob("/sys/kernel/iommu_groups/*/devices"); err == nil && len(g) > 0 {
		groups = len(g)
		iommu = "present"
	}
	devs := pciGPUDevices()
	if outputJSON {
		var dj []map[string]any
		for _, d := range devs {
			ready := "false"
			if d.Driver == "vfio-pci" {
				ready = "true"
			}
			dj = append(dj, map[string]any{"pci": d.PCI, "name": d.Name,
				"driver": d.Driver, "vfioReady": ready == "true"})
		}
		okMap(map[string]any{"iommu": iommu, "iommuGroups": groups,
			"configuredPassthrough": gpuPassthrough, "devices": dj})
		return 0
	}
	info("VFIO/IOMMU inventory:")
	infof(" - IOMMU groups: %d (%s)\n", groups, iommu)
	if iommu == "absent" {
		info("   -> Enable intel_iommu=on or amd_iommu=on in the kernel command line.")
	}
	for _, d := range devs {
		suffix := ""
		if d.Driver != "" {
			suffix = " [driver: " + d.Driver + "]"
		}
		infof(" - %s %s%s\n", d.PCI, d.Name, suffix)
		if d.Driver == "vfio-pci" {
			info("     -> bound to vfio-pci; passable via gpu_passthrough=\"" + d.PCI + "\"")
		}
	}
	if gpuPassthrough != "" {
		info(" - Configured gpu_passthrough: " + gpuPassthrough)
	}
	return 0
}
