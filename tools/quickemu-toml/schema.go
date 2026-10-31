// schema.go: the quickemu TOML schema v1 table — which keys are
// declarative, what value types they accept and how a value is
// checked. Diagnosis happens in diagnose.go; this file is pure data
// plus the value-kind predicates.
package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// schemaVersion is the only supported TOML schema version. It must
// appear as `schema_version = 1` at the document root (doc.go §v1).
const schemaVersion = 1

// legacyVersionKey: the version field early migrators emitted — still
// honoured as a deprecated alias for schema_version.
const legacyVersionKey = "formatVersion"

// valKind classifies how a declarative key's value is validated.
type valKind int

const (
	kindStr     valKind = iota // any non-empty string
	kindInt                    // integer (TOML int or digit string)
	kindPort                   // integer in 1..65535
	kindSize                   // storage/RAM size: "4G", "512M", "2048"…
	kindBool                   // on|off|true|false|yes|no
	kindMAC                    // IEEE-802 MAC, six colon-separated octets
	kindPath                   // filesystem path — non-empty, no controls
	kindEnum                   // fixed whitelist (enumVals)
	kindNet                    // network mode or host interface name
	kindFwdList                // port_forwards: HOST:GUEST entries
	kindStrList                // usb_devices/extra_disks: string list
	kindAny                    // QE-060: [[share]] tables — shape-checked by launch
)

// keySpec pairs a declarative key with its value kind and, for
// kindEnum, the accepted words.
type keySpec struct {
	kind valKind
	enum []string
}

// declarativeKeys: every key a .conf may express (the tomlKeyOrder
// set plus the keys the CLI/launch layers understand). Order matches
// tomlKeyOrder; extras follow in the order callers typically emit.
var declarativeKeys = map[string]keySpec{
	"guest_os":            {kindStr, nil},
	"arch":                {kindEnum, []string{"x86_64", "aarch64"}},
	"disk_img":            {kindPath, nil},
	"disk_format":         {kindEnum, []string{"qcow2", "raw", "vmdk", "vdi", "vhdx", "vhd", "vpc"}},
	"disk_size":           {kindSize, nil},
	"iso":                 {kindPath, nil},
	"img":                 {kindPath, nil},
	"fixed_iso":           {kindPath, nil},
	"floppy":              {kindPath, nil},
	"ram":                 {kindSize, nil},
	"cpu_cores":           {kindInt, nil},
	"cpu_model":           {kindStr, nil},
	"cpu_pinning":         {kindStr, nil},
	"network":             {kindNet, nil},
	"network_preset":      {kindStr, nil},
	"bridge":              {kindStr, nil},
	"macaddr":             {kindMAC, nil},
	"ssh_port":            {kindPort, nil},
	"spice_port":          {kindPort, nil},
	"display":             {kindEnum, []string{"sdl", "gtk", "spice", "spice-app", "vnc", "cocoa", "none"}},
	"monitor":             {kindStr, nil},
	"serial":              {kindStr, nil},
	"sound_card":          {kindEnum, []string{"ac97", "es1370", "ich9-intel-hda", "intel-hda", "sb16", "usb-audio", "virtio-sound-pci", "none"}},
	"sound_duplex":        {kindEnum, []string{"hda-duplex", "hda-micro", "hda-output"}},
	"public_dir":          {kindStr, nil}, // "none" is a legal value
	"share_dirs":          {kindStr, nil}, // QE-060: space-separated extra shares
	"share_tags":          {kindStr, nil}, // mount tags matching share_dirs
	"share_in_dirs":       {kindStr, nil}, // QE-061: guest→host inboxes
	"share":               {kindAny, nil}, // QE-060: [[share]] tables
	"seed":                {kindPath, nil},
	"kernel":              {kindPath, nil},
	"initrd":              {kindPath, nil},
	"boot_args":           {kindStr, nil},
	"boot":                {kindEnum, []string{"efi", "legacy"}},
	"tpm":                 {kindBool, nil},
	"secureboot":          {kindBool, nil},
	"gl":                  {kindBool, nil},
	"width":               {kindInt, nil},
	"height":              {kindInt, nil},
	"vnc_listen":          {kindStr, nil},
	"monitor_cmd":         {kindStr, nil},
	"port_forwards":       {kindFwdList, nil},
	"gpu_passthrough":     {kindBool, nil},
	"viewer":              {kindStr, nil},
	"access":              {kindStr, nil},
	"ssh_access":          {kindStr, nil},
	"fullscreen":          {kindBool, nil},
	"braille":             {kindBool, nil},
	"keyboard":            {kindStr, nil},
	"keyboard_layout":     {kindStr, nil},
	"mouse":               {kindStr, nil},
	"usb_controller":      {kindStr, nil},
	"usb_devices":         {kindStrList, nil},
	"usb_passthrough":     {kindStrList, nil},
	"extra_disks":         {kindStrList, nil},
	"extra_args":          {kindStr, nil},
	"macos_release":       {kindStr, nil},
	"snapshot":            {kindBool, nil},
	"ignore_msrs":         {kindBool, nil},
	"ignore_tsc_warning":  {kindBool, nil},
	"monitor_telnet_host": {kindStr, nil},
	"monitor_telnet_port": {kindPort, nil},
	"serial_telnet_host":  {kindStr, nil},
	"serial_telnet_port":  {kindPort, nil},
	"profile":             {kindStr, nil},
}

// sectionNames: the only TOML tables the schema defines. [extra]
// holds preserved unknown conf keys; [runtime] holds volatile state
// that is never emitted back to .conf.
var sectionNames = map[string]bool{"extra": true, "runtime": true}

// listKeys: keys whose .conf spelling is a bash array `k=(a b)` and
// whose TOML spelling is a string array.
var listKeys = map[string]bool{
	"port_forwards": true, "usb_devices": true,
	"usb_passthrough": true, "extra_disks": true,
}

var (
	macRe  = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)
	sizeRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?\s*[KMGTPE]i?B?$`)
	fwdRe  = regexp.MustCompile(`^([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:)?[0-9]+:[0-9]+$`)
	ifRe   = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)
	keyRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// strVal flattens a decoded TOML scalar to the string form .conf
// values take; returns ok=false for containers/unsupported types.
func strVal(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case int64:
		return strconv.FormatInt(t, 10), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case bool:
		if t {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}

// checkValue validates one decoded value for a declarative key. It
// returns a message on failure ("" when valid). TOML-native types are
// normalised via strVal first so `ssh_port = 22` and
// `ssh_port = "22"` are equivalent.
func checkValue(key string, spec keySpec, v any) string {
	// List kinds accept a single string or an array of strings.
	if spec.kind == kindAny {
		return "" // non-scalar containers validate in the launch layer
	}
	if spec.kind == kindFwdList || spec.kind == kindStrList {
		var items []string
		switch t := v.(type) {
		case string:
			items = strings.Fields(t)
		case []any:
			for _, e := range t {
				s, ok := e.(string)
				if !ok {
					return fmt.Sprintf("%s: list elements must be strings", key)
				}
				items = append(items, s)
			}
		default:
			return fmt.Sprintf("%s: expected a string or list of strings, got %T", key, v)
		}
		if spec.kind == kindFwdList {
			for _, it := range items {
				if !fwdRe.MatchString(it) {
					return fmt.Sprintf("%s: %q is not a valid port forward (expected host:guest port numbers)", key, it)
				}
			}
		}
		return ""
	}
	s, ok := strVal(v)
	if !ok {
		return fmt.Sprintf("%s: expected a scalar value, got %T", key, v)
	}
	if s == "" {
		return fmt.Sprintf("%s: value must not be empty", key)
	}
	switch spec.kind {
	case kindStr:
		return ""
	case kindInt:
		if !digitsOnly(s) {
			return fmt.Sprintf("%s: expected an integer, got %q", key, s)
		}
	case kindPort:
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Sprintf("%s: expected a port number in 1-65535, got %q", key, s)
		}
	case kindSize:
		if !sizeRe.MatchString(s) && !digitsOnly(s) {
			return fmt.Sprintf("%s: expected a size like \"4G\" or \"512M\", got %q", key, s)
		}
	case kindBool:
		switch strings.ToLower(s) {
		case "on", "off", "true", "false", "yes", "no":
		default:
			return fmt.Sprintf("%s: expected on|off, got %q", key, s)
		}
	case kindMAC:
		if !macRe.MatchString(s) {
			return fmt.Sprintf("%s: %q is not a valid MAC address", key, s)
		}
	case kindPath:
		if strings.ContainsRune(s, 0) || strings.ContainsAny(s, "\r\n") {
			return fmt.Sprintf("%s: path contains control characters", key)
		}
	case kindEnum:
		for _, e := range spec.enum {
			if s == e {
				return ""
			}
		}
		return fmt.Sprintf("%s: %q is not recognised (want one of %s)", key, s, strings.Join(spec.enum, ", "))
	case kindNet:
		switch s {
		case "none", "user", "restrict", "passt", "slirp", "bridge":
			return ""
		}
		if !ifRe.MatchString(s) {
			return fmt.Sprintf("%s: %q is not a network mode or interface name", key, s)
		}
	}
	return ""
}

// knownKey reports whether key is declarative (root-level conf key).
func knownKey(key string) bool { _, ok := declarativeKeys[key]; return ok }
