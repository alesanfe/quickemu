// profile.go: MachineProfile — the declarative input a run resolves
// against the catalog. TOML, matching the .conf → TOML migration; the
// cloud-init user-data it produces is YAML but that is output, not
// input.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Profile is the apiVersion-1 machine automation profile. Field names
// mirror the MachineProfile kind in the design doc; only the subset
// the engine actually consumes is modelled — unknown keys are ignored
// so profiles can grow ahead of the engine.
type Profile struct {
	sourcePath string // file the profile was loaded from (resume needs it)
	Profile    struct {
		Name        string `toml:"name"`
		Description string `toml:"description"`
	} `toml:"profile"`
	System struct {
		OS      string `toml:"os"`
		Release string `toml:"release"`
		Edition string `toml:"edition"`
		Arch    string `toml:"arch"`
	} `toml:"system"`
	Machine struct {
		CPUs     int    `toml:"cpus"`
		RAM      string `toml:"ram"`
		DiskSize string `toml:"disk_size"`
		SSHPort  int    `toml:"ssh_port"`
		Boot     string `toml:"boot"` // efi|legacy|secure — empty keeps the conf default
	} `toml:"machine"`
	Identity struct {
		Hostname          string   `toml:"hostname"`
		Username          string   `toml:"username"`
		Password          string   `toml:"password"`
		SSHKeyFile        string   `toml:"ssh_key_file"`
		SSHAuthorizedKeys []string `toml:"ssh_authorized_keys"`
		AllowPwAuth       bool     `toml:"allow_pw_auth"`
	} `toml:"identity"`
	Locale struct {
		Lang     string `toml:"lang"`
		Keyboard string `toml:"keyboard"`
		Timezone string `toml:"timezone"`
	} `toml:"locale"`
	Automation struct {
		Provider           string `toml:"provider"`
		InstallTimeoutMin  int    `toml:"install_timeout_min"`
		SSHTimeoutMin      int    `toml:"ssh_timeout_min"`
		AllowVisualInstall bool   `toml:"allow_visual_install"`
	} `toml:"automation"`
	Provision struct {
		Packages []string `toml:"packages"`
		Services []string `toml:"services"`
		Files    []struct {
			Source      string `toml:"source"`
			Destination string `toml:"destination"`
			Mode        string `toml:"mode"`
			Owner       string `toml:"owner"`
		} `toml:"files"`
		Commands []struct {
			Name string `toml:"name"`
			Run  string `toml:"run"`
			As   string `toml:"as"`
		} `toml:"commands"`
	} `toml:"provision"`
	Verify []struct {
		Name   string `toml:"name"`
		Run    string `toml:"run"`
		Expect string `toml:"expect"`
	} `toml:"verify"`
	Finalize struct {
		Shutdown bool   `toml:"shutdown"`
		Snapshot string `toml:"snapshot"`
	} `toml:"finalize"`
}

// loadProfile reads + validates a TOML profile, applying defaults for
// anything omitted. Validation only rejects what would make a run
// nonsensical — capability checks (does this OS support the provider)
// happen at plan time when the catalog is consulted.
func loadProfile(path string) (*Profile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := toml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if abs, err := filepath.Abs(path); err == nil {
		p.sourcePath = abs
	}
	if p.System.OS == "" {
		return nil, fmt.Errorf("system.os is required")
	}
	if p.System.Release == "" {
		return nil, fmt.Errorf("system.release is required")
	}
	if p.System.Arch == "" {
		p.System.Arch = "amd64"
	}
	if p.Profile.Name == "" {
		p.Profile.Name = p.System.OS + "-" + p.System.Release
	}
	if p.Identity.Hostname == "" {
		p.Identity.Hostname = safeHostname(p.Profile.Name)
	}
	if p.Identity.Username == "" {
		p.Identity.Username = "quickemu"
	}
	if p.Machine.CPUs <= 0 {
		p.Machine.CPUs = 2
	}
	if p.Machine.RAM == "" {
		p.Machine.RAM = "4G"
	}
	if p.Automation.Provider == "" || p.Automation.Provider == "auto" {
		p.Automation.Provider = inferProvider(p.System.OS)
	}
	if p.Automation.InstallTimeoutMin <= 0 {
		p.Automation.InstallTimeoutMin = 45
	}
	if p.Automation.SSHTimeoutMin <= 0 {
		p.Automation.SSHTimeoutMin = 15
	}
	return &p, nil
}

// inferProvider picks the install strategy from the catalog entry's
// image_type when the profile does not pin one. raw/img disk images
// are bootable as-is → nocloud-image; ubuntu-server is the one ISO
// with a first-class answer-file path today.
func inferProvider(osName string) string {
	if osName == "ubuntu-server" {
		return "ubuntu-autoinstall"
	}
	if t := catalogImageType(osName); t == "raw" || t == "img" || t == "qcow2" {
		return "nocloud-image"
	}
	return "manual"
}

// catalogImageType resolves the entry's declared image_type from the
// shipped catalog — DataFile walks dev-tree and packaged layouts.
func catalogImageType(osName string) string {
	type entry struct {
		OS        string `json:"os"`
		ImageType string `json:"image_type"`
	}
	var cat struct {
		Entries []entry `json:"entries"`
	}
	b, err := os.ReadFile(dataFile("catalog", "data", "catalog.json"))
	if err != nil {
		return ""
	}
	if jsonUnmarshal(b, &cat) != nil {
		return ""
	}
	for _, e := range cat.Entries {
		if e.OS == osName {
			return e.ImageType
		}
	}
	return ""
}

// safeHostname mirrors quickget-vm's hostname scrubbing: hostnames
// must be DNS-safe or cloud-init/subiquity reject them.
func safeHostname(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '_', r == ' ', r == '.':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
