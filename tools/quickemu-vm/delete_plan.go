// tools/quickemu-vm/delete_plan.go: delete-plan itemization + plan-id
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// outputJSON toggles the v1 envelope vs human text (--format json).
var outputJSON bool

// deletePlanItems enumerates dir contents excluding runtime artifacts —
// mirrors delete_plan_items' find with the same exclusions and C-locale
// sort order.
func deletePlanItems(dir string) []string {
	excl := map[string]bool{
		".lock": true, ".lock.shared": true, ".runtime.json": true,
		".pid": true, ".sock": true, ".socket": true, ".ports": true,
		".paused": true, ".spice": true, ".monitor": true,
	}
	// Bash lists ${VM} and `find ${VMDIR}` output — paths relative to
	// its post-pushd cwd. Emit cwd-relative spellings for parity.
	cwd, _ := os.Getwd()
	rel := func(p string) string {
		if r, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
		return p
	}
	items := []string{}
	conf := vmConfPath(dir)
	if conf != "" {
		items = append(items, rel(conf))
	}
	filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return nil
		}
		// Ext() alone can't see compound suffixes (.runtime.json),
		// so exclude by suffix match against the artifact list.
		base := filepath.Base(p)
		for e := range excl {
			if e == filepath.Ext(p) || e == base || strings.HasSuffix(base, e) {
				return nil
			}
		}
		items = append(items, rel(p))
		return nil
	})
	// LC_ALL=C sort: bytewise; the conf first then sorted rest (Bash
	// lists ${VM} first, then the find output sorted).
	if len(items) > 1 {
		rest := items[1:]
		for i := 0; i < len(rest); i++ {
			for j := i + 1; j < len(rest); j++ {
				if rest[j] < rest[i] {
					rest[i], rest[j] = rest[j], rest[i]
				}
			}
		}
	}
	return items
}

// deletePlanID: sha256 over the canonical plan serialization —
// "vm=<name>\n" then "<path>|<size>|<mtime>|<type>" per item.
// validVMName: a VM name becomes a sibling dir plus <name>.conf —
// it must stay a single safe path component.
func validVMName(n string) bool {
	return n != "" && n != "." && n != ".." &&
		!strings.ContainsAny(n, `/\`) && !strings.HasPrefix(n, "-") &&
		strings.IndexFunc(n, unicode.IsControl) < 0
}

func deletePlanID(name string, items []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "vm=%s\n", name)
	for _, it := range items {
		line := it
		if st, err := os.Lstat(it); err == nil {
			kind := "file"
			switch {
			case st.IsDir():
				kind = "directory"
			case st.Mode()&os.ModeSymlink != 0:
				kind = "symbolic link"
			}
			line += fmt.Sprintf("|%d|%s|%s", st.Size(), st.ModTime().Format("2006-01-02 15:04:05.000000000 -0700"), kind)
		}
		b.WriteString(line + "\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("sha256:%x", sum)
}

// deletePlanDependents: qcow2s in dir whose backing_file resolves
// inside the same dir (deleting it would orphan the child).
func deletePlanDependents(dir string) []string {
	var out []string
	if _, err := exec.LookPath("qemu-img"); err != nil {
		return out
	}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !strings.HasSuffix(p, ".qcow2") || d.IsDir() {
			return nil
		}
		b, err := execCommand("qemu-img", "info", "--output=json", p).Output()
		if err != nil {
			return nil
		}
		var j struct {
			Backing string `json:"backing-filename"`
		}
		if json.Unmarshal(b, &j) != nil || j.Backing == "" {
			return nil
		}
		bf := j.Backing
		if strings.HasPrefix(bf, dir+"/") || strings.HasPrefix(bf, "./") || strings.HasPrefix(bf, filepath.Base(dir)+"/") {
			out = append(out, p+" -> "+bf)
		}
		return nil
	})
	return out
}
