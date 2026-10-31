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

// extractBootfiles copies the wanted basenames found under dirPrefix
// (e.g. "casper") into outDir — ISO9660 names may carry ";1" version
// suffixes, stripped before matching. Returns basename→dest map.
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
	got := map[string]string{}
	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[w] = true
	}
	var walk func(dir *iso9660.File) error
	walk = func(dir *iso9660.File) error {
		kids, err := dir.GetChildren()
		if err != nil {
			return err
		}
		for _, k := range kids {
			name := strings.ToLower(strings.SplitN(k.Name(), ";", 2)[0])
			if k.IsDir() {
				if name == dirPrefix || dirPrefix == "" {
					if err := walk(k); err != nil {
						return err
					}
				}
				continue
			}
			if !wantSet[name] || got[name] != "" {
				continue
			}
			dst := filepath.Join(outDir, name)
			w, err := os.Create(dst)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, k.Reader())
			w.Close()
			if err != nil {
				return err
			}
			got[name] = dst
		}
		return nil
	}
	if err := walk(root); err != nil {
		return got, err
	}
	for _, w := range want {
		if got[w] == "" {
			return got, fmt.Errorf("%s not found under %s/ in %s", w, dirPrefix, isoPath)
		}
	}
	return got, nil
}
