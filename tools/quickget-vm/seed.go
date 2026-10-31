// tools/quickget-vm/seed.go: cloud-init/NoCloud seed ISO + hostname/username sanitizers
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// safeUsername: $USER is interpolated into cloud-init YAML and a sudoers
// echo — keep POSIX login-name chars only; a value that sanitizes to
// nothing or an illegal first char falls back to "quickemu".
func safeUsername(u string) string {
	var b strings.Builder
	for _, r := range u {
		if r == '_' || r == '-' || (r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" || len(s) > 32 || s[0] == '-' || (s[0] >= '0' && s[0] <= '9') {
		return "quickemu"
	}
	return s
}

// safeHostname: the VM dir becomes a cloud-init hostname — basename,
// lowercased, non [a-z0-9-] folded to '-'.
func safeHostname(p string) string {
	var b strings.Builder
	for _, r := range filepath.Base(p) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		default:
			b.WriteRune('-')
		}
	}
	if s := strings.Trim(b.String(), "-"); s != "" {
		return s
	}
	return "vm"
}

// genSeedISO: gen_seed_iso parity — write user-data + meta-data and
// build a NoCloud seed.iso in pure Go (writeISO9660/diskfs — replaces
// the cloud-localds > genisoimage > xorrisofs > mkisofs chain), then
// append seed="..." to the conf.
func genSeedISO(vmPath, confFile string) int {
	userName := safeUsername(os.Getenv("USER"))
	hostName := safeHostname(vmPath)
	sshKey := ""
	for _, k := range []string{"id_ed25519.pub", "id_rsa.pub"} {
		if b, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".ssh", k)); err == nil {
			sshKey = strings.TrimSpace(string(b))
			break
		}
	}
	userData := filepath.Join(vmPath, "user-data")
	var ud strings.Builder
	ud.WriteString("#cloud-config\nhostname: " + hostName + "\nmanage_etc_hosts: true\nusers:\n  - name: " + userName +
		"\n    sudo: ALL=(ALL) NOPASSWD:ALL\n    groups: sudo\n    shell: /bin/bash\n    lock_passwd: false\n    plain_text_passwd: quickemu\n")
	if sshKey != "" {
		ud.WriteString("    ssh_authorized_keys:\n      - " + sshKey + "\n")
	}
	ud.WriteString("ssh_pwauth: true\npackage_update: true\n")
	if err := os.WriteFile(userData, []byte(ud.String()), 0644); err != nil {
		fmt.Println(err)
		return 1
	}
	metaData := filepath.Join(vmPath, "meta-data")
	if _, err := os.Stat(metaData); err != nil {
		if err := os.WriteFile(metaData, []byte("instance-id: "+hostName+"\nlocal-hostname: "+hostName+"\n"), 0644); err != nil {
			fmt.Println(err)
			return 1
		}
	}
	seed := filepath.Join(vmPath, "seed.iso")
	// cloud-localds parity: a cidata seed ISO is just an ISO9660 image
	// (volid=cidata, Rock Ridge + Joliet) holding user-data/meta-data.
	if err := writeISO9660(seed, "cidata", map[string]string{
		"/user-data": userData, "/meta-data": metaData,
	}); err != nil {
		fmt.Println("ERROR! Could not build seed.iso: " + err.Error())
		return 1
	}
	if _, err := os.Stat(seed); err == nil {
		// The seed line must land in the conf or the ISO is orphaned —
		// a nil f (open error) made Fprintf a silent no-op and still
		// printed success.
		f, err := os.OpenFile(confFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Println("ERROR! Could not record seed.iso in " + confFile + ": " + err.Error())
			return 1
		}
		if _, err := fmt.Fprintf(f, "seed=\"%s\"\n", seed); err != nil {
			f.Close()
			fmt.Println("ERROR! Could not record seed.iso in " + confFile + ": " + err.Error())
			return 1
		}
		if err := f.Close(); err != nil {
			fmt.Println("ERROR! Could not record seed.iso in " + confFile + ": " + err.Error())
			return 1
		}
		fmt.Println(" - NoCloud seed.iso created (user: " + userName + ", password: quickemu)")
	}
	return 0
}
