package main

import (
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

func cpDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		t := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(t, info.Mode())
		}
		// Symlinks are copied as links (mv/cp -r parity): opening the
		// entry would dereference it — dragging the whole target tree
		// into the copy or dying on a dir target mid-move.
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			os.MkdirAll(filepath.Dir(t), 0755)
			os.Remove(t)
			return os.Symlink(target, t)
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
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
