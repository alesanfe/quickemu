package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// backupDir: <vmdir>/backups — backups reuse the export format.
func backupDir(dir string) string {
	return filepath.Join(dir, "backups")
}

// backupVm: export into <vmdir>/backups/<utc>-<vm>.quickvm.tar.gz.
func backupVm(dir string) int {
	bdir := backupDir(dir)
	if err := os.MkdirAll(bdir, 0755); err != nil {
		die("BACKUP_FAILED", err.Error())
	}
	out := filepath.Join(bdir, time.Now().UTC().Format("20060102T150405Z")+"-"+filepath.Base(dir)+".quickvm.tar.gz")
	return exportVm(dir, out)
}

// backupList: enumerate *.quickvm.tar.gz in the backup dir.
func backupList(dir string) int {
	bdir := backupDir(dir)
	m, _ := filepath.Glob(filepath.Join(bdir, "*.quickvm.tar.gz"))
	if outputJSON {
		items := []map[string]any{}
		for _, f := range m {
			st, _ := os.Stat(f)
			var sz int64
			if st != nil {
				sz = st.Size()
			}
			items = append(items, map[string]any{"file": f, "size": sz})
		}
		okMap(map[string]any{"backups": items})
		return 0
	}
	info("Backups in " + bdir + ":")
	for _, f := range m {
		info(" - " + filepath.Base(f))
	}
	return 0
}

// backupPrune: keep the newest N backups.
func backupPrune(dir string, keep int) int {
	if keep <= 0 {
		keep = 5
	}
	bdir := backupDir(dir)
	m, _ := filepath.Glob(filepath.Join(bdir, "*.quickvm.tar.gz"))
	// newest-first by modtime
	slices.SortFunc(m, func(a, b string) int {
		sa, _ := os.Stat(a)
		sb, _ := os.Stat(b)
		if sa == nil || sb == nil {
			return strings.Compare(b, a)
		}
		return sb.ModTime().Compare(sa.ModTime())
	})
	for i, f := range m {
		if i >= keep {
			os.Remove(f)
			info(" - Pruned " + filepath.Base(f))
		}
	}
	ok("{}")
	return 0
}

// backupVerify: manifest + checksums must exist and validate.
func backupVerify(fpath string) int {
	f, err := os.Open(fpath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! Backup "+fpath+" not found.")
		return 1
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! Not a gzip tar.")
		return 1
	}
	tr := tar.NewReader(gz)
	sums := map[string]string{}
	hashes := map[string]string{}
	hasManifest := false
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if h.Name == "quickvm-checksums.sha256" {
			b, _ := io.ReadAll(io.LimitReader(tr, 1<<20))
			for _, l := range strings.Split(string(b), "\n") {
				flds := strings.Fields(l)
				if len(flds) == 2 {
					sums[flds[1]] = flds[0]
				}
			}
			continue
		}
		if h.Name == "quickvm-manifest.json" {
			hasManifest = true
		}
		// Hash while streaming — ReadAll per member OOMs on real
		// backups (qcow2 payloads are GBs).
		hh := sha256.New()
		io.Copy(hh, tr)
		hashes[h.Name] = fmt.Sprintf("%x", hh.Sum(nil))
	}
	if !hasManifest {
		fmt.Fprintln(os.Stderr, "ERROR! No quickvm-manifest.json (not a verified backup).")
		return 1
	}
	bad := false
	for name, want := range sums {
		if hashes[name] != want {
			bad = true
		}
	}
	if bad {
		fmt.Fprintln(os.Stderr, "ERROR! Backup "+fpath+" failed verification.")
		return 1
	}
	info(" - Backup " + fpath + " verified OK.")
	return 0
}

// backupRestore: verify then import into dest.
func backupRestore(fpath, dest string) int {
	if backupVerify(fpath) != 0 {
		return 1
	}
	return importVm(fpath, dest)
}
