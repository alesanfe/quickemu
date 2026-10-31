// shares.go — QE-060/061: multiple [[share]] entries + a dedicated
// guest→host direction. The primary public_dir keeps the virtiofs
// preference; extra shares always emit 9p pairs (one virtiofsd per
// extra dir is not worth the socket/pid bookkeeping).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// shareEntry: one extra host directory exported to the guest.
// Dir is the semantic direction: "out" shares host→guest like the
// classic public_dir; "in" is the QE-061 inbox — where the guest
// drops files for the host (drop-in folder).
type shareEntry struct {
	Path string
	Tag  string
	Dir  string // "out" | "in"
}

var shareTagRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,31}$`)

// shareEntries gathers extra shares from the flat conf keys:
//
//	share_dirs="p1 p2"      extra host→guest shares
//	share_tags="t1 t2"      optional mount tags (default: basename)
//	share_in_dirs="q1 q2"   guest→host inbox shares (QE-061)
//
// TOML [[share]] tables flatten into the same keys by parseTomlConf.
func shareEntries(c Conf) []shareEntry {
	var out []shareEntry
	dirs := strings.Fields(c.get("share_dirs", ""))
	tags := strings.Fields(c.get("share_tags", ""))
	for i, d := range dirs {
		tag := ""
		if i < len(tags) {
			tag = tags[i]
		}
		out = append(out, shareEntry{Path: d, Tag: tag, Dir: "out"})
	}
	for i, d := range strings.Fields(c.get("share_in_dirs", "")) {
		tag := "qe-inbox"
		if i > 0 {
			tag = fmt.Sprintf("qe-inbox-%d", i)
		}
		out = append(out, shareEntry{Path: d, Tag: tag, Dir: "in"})
	}
	return out
}

// validateShares: existence + tag sanity — a bad path warns and is
// dropped (same tolerance as resolvePublic); a bad tag refuses the
// share outright (tags land in mount_tag position).
func validateShares(c Conf) []shareEntry {
	seen := map[string]bool{}
	var out []shareEntry
	for _, s := range shareEntries(c) {
		if st, err := os.Stat(s.Path); err != nil || !st.IsDir() {
			fmt.Fprintf(os.Stderr, " - WARNING! Share directory does not exist: %s\n", s.Path)
			continue
		}
		if s.Tag == "" {
			s.Tag = filepath.Base(s.Path)
		}
		if !shareTagRe.MatchString(s.Tag) {
			fmt.Fprintf(os.Stderr, " - WARNING! Share tag %q must match [A-Za-z0-9._-]{1,31} — skipped.\n", s.Tag)
			continue
		}
		if seen[s.Tag] {
			fmt.Fprintf(os.Stderr, " - WARNING! Duplicate share tag %q — skipped.\n", s.Tag)
			continue
		}
		seen[s.Tag] = true
		out = append(out, s)
	}
	return out
}

// extraShareArgs emits -fsdev/-device pairs for the validated extra
// shares. fsdev ids continue from fsdev0 (the primary public_dir
// pair) so the two never collide.
func extraShareArgs(c Conf) []string {
	var a []string
	for i, s := range validateShares(c) {
		id := fmt.Sprintf("fsdev%d", i+1)
		a = append(a,
			"-fsdev", "local,id="+id+",path="+qemuOptEsc(s.Path)+",security_model=mapped-xattr",
			"-device", "virtio-9p-pci,fsdev="+id+",mount_tag="+qemuOptEsc(s.Tag))
		if s.Dir == "in" {
			fmt.Printf(" - 9P:       Guest->Host inbox: mount -t 9p -o trans=virtio,version=9p2000.L %s ~/%s\n",
				s.Tag, filepath.Base(s.Path))
		} else {
			fmt.Printf(" - 9P:       On guest: sudo mount -t 9p -o trans=virtio,version=9p2000.L %s ~/%s\n",
				s.Tag, filepath.Base(s.Path))
		}
	}
	return a
}
