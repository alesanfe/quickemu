package main

// profileDefaults: vm_profile_defaults parity — preset conf values
// applied BEFORE the conf's own keys (conf wins on conflicts).
var profileDefaults = map[string]map[string]string{
	"windows-gaming": {"guest_os": "windows", "ram": "8G", "cpu_cores": "6", "video": "virtio-vga-gl"},
	"linux-desktop":  {"guest_os": "linux", "ram": "4G", "cpu_cores": "4"},
	"server":         {"guest_os": "linux", "ram": "4G", "display": "none", "network": "user"},
	"minimal":        {"guest_os": "linux", "ram": "512M", "cpu_cores": "1"},
	"ci":             {"guest_os": "linux", "ram": "2G", "display": "none", "network": "none"},
}

// networkPreset: vm_network_preset parity — nat/bridge/hostonly/
// isolated mapped to the network mode the launch path understands.
var networkPreset = map[string]string{
	"nat": "user", "user": "user",
	"bridge":   "bridge",
	"hostonly": "host",
	"isolated": "none", "none": "none",
}

// applyPresets merges profile + network-preset defaults into conf,
// only for keys the conf does not already set (conf overrides profile,
// matching Bash loader.sh ordering).
func applyPresets(c Conf) Conf {
	if p, ok := profileDefaults[c.get("profile", "")]; ok {
		for k, v := range p {
			if c[k] == "" {
				c[k] = v
			}
		}
	} else if c.get("profile", "") != "" {
		// unknown profile — same warning as loader.sh
		println("WARNING! Unknown profile '" + c["profile"] + "'.")
	}
	if n, ok := networkPreset[c.get("network_preset", "")]; ok {
		if c["network"] == "" {
			c["network"] = n
		}
	}
	return c
}
