package qelib

import (
	"os"
	"regexp"
	"strings"
)

// HotplugPersist appends a hotplugged device to the VM config so it
// survives reboots (hotplug_persist parity). .conf gets bash-array
// syntax, .toml gets TOML arrays; the key is created when absent.
func HotplugPersist(confPath, kind, val string) error {
	key := "extra_disks"
	if kind == "usb" {
		key = "usb_devices"
	}
	b, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}
	s := string(b)
	if strings.HasSuffix(confPath, ".toml") {
		// (?s) lets the array span lines; index-splicing keeps $
		// sequences in the value literal (ReplaceAllString would
		// expand them as group references and corrupt the conf).
		re := regexp.MustCompile(`(?ms)^` + key + `\s*=\s*\[(.*?)\]`)
		if m := re.FindStringSubmatchIndex(s); m != nil {
			inner := strings.TrimSpace(s[m[2]:m[3]])
			rep := `, "` + val + `"]`
			if inner == "" {
				rep = `"` + val + `"]`
			}
			s = s[:m[0]] + key + ` = [` + inner + rep + s[m[1]:]
		} else {
			s += key + ` = ["` + val + `"]` + "\n"
		}
	} else {
		re := regexp.MustCompile(`(?ms)` + key + `=\((.*?)\)`)
		if m := re.FindStringSubmatchIndex(s); m != nil {
			inner := s[m[2]:m[3]]
			s = s[:m[0]] + key + `=(` + inner + ` "` + val + `")` + s[m[1]:]
		} else {
			s += key + `=("` + val + `")` + "\n"
		}
	}
	return os.WriteFile(confPath, []byte(s), 0o644)
}
