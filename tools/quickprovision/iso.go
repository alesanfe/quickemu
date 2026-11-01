// iso.go: ISO9660 extraction — pull the casper kernel/initrd out of
// an Ubuntu server ISO so the installer can boot via -kernel/-initrd
// with the autoinstall seed, no media-swap needed mid-install.
// Mirrors quickget-vm's isoExtractBootfiles walk.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kdomanski/iso9660"
)

// dirNorm folds an ISO9660 dir name to alnum lowercase — the base
// descriptor cannot hold '.' or '_' reliably (genisoimage maps them,
// diskfs truncates at the dot), so "install.amd" may surface as
// INSTALL, INSTALL_AMD or INSTALLAM depending on the tool that wrote
// the ISO. Normalised matching survives all of those.
func dirNorm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// dirMatch reports whether the on-disc dir name is an acceptable
// rendering of dirPrefix after normalisation — exact match, or one
// contains the other (covers truncation both ways).
func dirMatch(discName, dirPrefix string) bool {
	if dirPrefix == "" {
		return true
	}
	d, p := dirNorm(discName), dirNorm(dirPrefix)
	return d == p || strings.Contains(d, p) || strings.Contains(p, d)
}

// extractBootfiles copies the wanted basenames found under dirPrefix
// (e.g. "casper") into outDir — ISO9660 names may carry ";1" version
// suffixes, stripped before matching. Every directory is walked and
// candidates are ranked: a file whose parent dir normalises to the
// prefix beats a same-named file elsewhere on the disc (the gtk
// installer's initrd.gz must not shadow the text installer's).
// Returns basename→dest map.
func extractBootfiles(isoPath, dirPrefix, outDir string, want []string) (map[string]string, error) {
	f, err := os.Open(isoPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := iso9660.OpenImage(f)
	if err != nil {
		return nil, err
	}
	root, err := img.RootDir()
	if err != nil {
		return nil, err
	}
	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[w] = true
	}
	type cand struct {
		dst     string // extracted path
		inMatch bool   // parent dir matched the prefix
	}
	cands := map[string][]cand{}
	var walk func(dir *iso9660.File, parentMatch bool) error
	walk = func(dir *iso9660.File, parentMatch bool) error {
		kids, err := dir.GetChildren()
		if err != nil {
			return err
		}
		for _, k := range kids {
			name := strings.ToLower(strings.SplitN(k.Name(), ";", 2)[0])
			if k.IsDir() {
				if err := walk(k, dirMatch(name, dirPrefix)); err != nil {
					return err
				}
				continue
			}
			if !wantSet[name] {
				continue
			}
			dst := filepath.Join(outDir, name)
			// duplicate basenames: first candidate keeps the slot —
			// the better parent still wins by insertion order below.
			w, err := os.Create(dst)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, k.Reader())
			w.Close()
			if err != nil {
				return err
			}
			if parentMatch || dirPrefix == "" {
				// matching parents prepend so they win selection.
				cands[name] = append([]cand{{dst: dst, inMatch: true}}, cands[name]...)
			} else {
				cands[name] = append(cands[name], cand{dst: dst})
			}
		}
		return nil
	}
	if err := walk(root, dirPrefix == ""); err != nil {
		return nil, err
	}
	got := map[string]string{}
	for _, w := range want {
		cs := cands[w]
		if len(cs) == 0 {
			return got, fmt.Errorf("%s not found under %s/ in %s", w, dirPrefix, isoPath)
		}
		got[w] = cs[0].dst
	}
	return got, nil
}
