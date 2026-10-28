package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// fsRename is a seam for tests (ADR-0009): moveAny must be exercised
// against an injected cross-device failure without needing a second
// mount on the host.
var fsRename = os.Rename

// moveAny moves src to dst like `mv`: rename when possible, and a
// copy+remove fallback when the two paths live on different
// filesystems (EXDEV). The VM dir and the XDG trash can absolutely
// sit on different mounts — a delete-to-trash that only knows
// rename(2) fails there.
func moveAny(src, dst string) error {
	err := fsRename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if st.IsDir() {
		if err := cpDir(src, dst); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}
	if err := cpFile(src, dst, st.Mode()); err != nil {
		return err
	}
	return os.Remove(src)
}

func tarGz(src, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == src {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return nil
		}
		info, _ := d.Info()
		h, _ := tar.FileInfoHeader(info, "")
		h.Name = rel
		tw.WriteHeader(h)
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

func cpDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		t := filepath.Join(dst, rel)
		info, _ := d.Info()
		if d.IsDir() {
			return os.MkdirAll(t, info.Mode())
		}
		return cpFile(p, t, info.Mode())
	})
}

func cpFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
