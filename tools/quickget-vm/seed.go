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

// genSeedISO is the zero-option wrapper kept for callers that build a
// stock seed (autoinstall path).
func genSeedISO(vmPath, confFile string) int {
	return genSeedISOOpts(vmPath, confFile, &seedOptions{})
}

// genSeedISOOpts: gen_seed_iso parity — write user-data + meta-data (+
// optional network-config) and build a NoCloud seed.iso in pure Go
// (writeISO9660/diskfs — replaces the cloud-localds > genisoimage >
// xorrisofs > mkisofs chain), then append seed="..." to the conf.
//
// user-data comes from the generated base template, merged with the
// --user-data overlay when given (merge contract in cloudinit.go).
// On regeneration (--regen-seed) the redacted diff vs the previous
// user-data/meta-data is printed BEFORE the files are overwritten —
// key material and passwords never reach the log.
func genSeedISOOpts(vmPath, confFile string, o *seedOptions) int {
	if o == nil {
		o = &seedOptions{}
	}
	hostName := safeHostname(vmPath)
	userName := ""
	if us := o.buildUsers(); len(us) > 0 {
		userName = us[0].name
	}
	userData := filepath.Join(vmPath, "user-data")
	ud, err := o.userDataWithOverlay(hostName)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	if o.regen {
		if old, err := os.ReadFile(userData); err == nil && string(old) != ud {
			fmt.Println("user-data changes:")
			fmt.Println(diffSeedDocs(string(old), ud))
		} else if err == nil {
			fmt.Println("user-data unchanged")
		}
	}
	if err := os.WriteFile(userData, []byte(ud), 0644); err != nil {
		fmt.Println(err)
		return 1
	}
	metaData := filepath.Join(vmPath, "meta-data")
	if md, err := o.metaDataWithOverlay(); err != nil {
		fmt.Println(err)
		return 1
	} else if md != "" {
		if o.regen {
			if old, err := os.ReadFile(metaData); err == nil && string(old) != md {
				fmt.Println("meta-data changes:")
				fmt.Println(diffSeedDocs(string(old), md))
			}
		}
		if err := os.WriteFile(metaData, []byte(md), 0644); err != nil {
			fmt.Println(err)
			return 1
		}
	} else if _, err := os.Stat(metaData); err != nil {
		def := "instance-id: " + hostName + "\nlocal-hostname: " + hostName + "\n"
		if err := os.WriteFile(metaData, []byte(def), 0644); err != nil {
			fmt.Println(err)
			return 1
		}
	}
	files := map[string]string{
		"/user-data": userData, "/meta-data": metaData,
	}
	if o.networkFile != "" {
		dst := filepath.Join(vmPath, "network-config")
		if err := copyFile(o.networkFile, dst); err != nil {
			fmt.Println("--network-config: " + err.Error())
			return 1
		}
		files["/network-config"] = dst
	}
	seed := filepath.Join(vmPath, "seed.iso")
	// diskfs.Create opens the target O_EXCL — regen must remove the old
	// ISO first or the rebuild fails on the pre-existing file.
	if _, err := os.Stat(seed); err == nil && o.regen {
		os.Remove(seed)
	}
	// cloud-localds parity: a cidata seed ISO is just an ISO9660 image
	// (volid=cidata, Rock Ridge + Joliet) holding the seed files.
	if err := writeISO9660(seed, "cidata", files); err != nil {
		fmt.Println("ERROR! Could not build seed.iso: " + err.Error())
		return 1
	}
	if _, err := os.Stat(seed); err == nil {
		// The seed line must land in the conf or the ISO is orphaned —
		// a nil f (open error) made Fprintf a silent no-op and still
		// printed success. On regen the seed= line already exists.
		// The conf is only READ when it is a sane regular file — a
		// symlink to /dev/full or another non-regular target would
		// otherwise stream endlessly into memory (OOM).
		needsLine := true
		if st, err := os.Stat(confFile); err == nil && st.Mode().IsRegular() && st.Size() < 1<<20 {
			if b, err := os.ReadFile(confFile); err == nil {
				for _, l := range strings.Split(string(b), "\n") {
					if strings.HasPrefix(l, "seed=") {
						needsLine = false
						break
					}
				}
			}
		}
		if needsLine {
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
		}
		fmt.Println(" - NoCloud seed.iso created (user: " + userName + ", password: quickemu)")
	}
	return 0
}
