package main

import "github.com/alesanfe/quickemu/tools/qelib"

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
	if p, ok := qelib.ProfileDefaults(c.get("profile", "")); ok {
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
