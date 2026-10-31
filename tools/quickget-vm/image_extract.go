// image_extract — ISO/decompression helpers (split of image.go).
package main

import (
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/kdomanski/iso9660"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

// isoExtractPure: pure-Go ISO9660 walk — finds prefix/vmlinuz and
// prefix/initrd (Rock Ridge names, case-insensitive, ";N" version
// suffix stripped). Replaces the bsdtar/7z/isoinfo dependency for
// standard ISO9660 images; UDF-only images fall back to the tools.
func isoExtractPure(iso, out, prefix string) bool {
	f, err := os.Open(iso)
	if err != nil {
		return false
	}
	defer f.Close()
	img, err := iso9660.OpenImage(f)
	if err != nil {
		return false
	}
	root, err := img.RootDir()
	if err != nil {
		return false
	}
	want := map[string]string{
		"vmlinuz": out + "/vmlinuz",
		"initrd":  out + "/initrd",
	}
	var walk func(dir *iso9660.File) bool
	walk = func(dir *iso9660.File) bool {
		kids, err := dir.GetChildren()
		if err != nil {
			return false
		}
		for _, k := range kids {
			name := strings.ToLower(strings.SplitN(k.Name(), ";", 2)[0])
			if k.IsDir() {
				if name == prefix || prefix == "" {
					if walk(k) {
						return true
					}
				}
				continue
			}
			dst, ok := want[name]
			if !ok {
				continue
			}
			w, err := os.Create(dst)
			if err != nil {
				continue
			}
			_, err = io.Copy(w, k.Reader())
			w.Close()
			if err == nil {
				delete(want, name)
			}
		}
		return len(want) == 0
	}
	return walk(root)
}

// isoExtractBootfiles: extract PREFIX/vmlinuz + PREFIX/initrd from an
// ISO — pure-Go ISO9660 first, bsdtar > 7z > isoinfo as fallback for
// UDF/hybrid images — then normalise the extracted names to OUT root.
func isoExtractBootfiles(iso, out, prefix string) bool {
	if isoExtractPure(iso, out, strings.ToLower(prefix)) {
		return true
	}
	upper := strings.ToUpper(prefix)
	done := false
	if b, err := exec.LookPath("bsdtar"); err == nil {
		done = execCommand(b, "-C", out, "-xf", iso,
			prefix+"/vmlinuz", prefix+"/initrd").Run() == nil
	} else if z, err := exec.LookPath("7z"); err == nil {
		done = execCommand(z, "e", "-o"+out, iso,
			prefix+"/vmlinuz", prefix+"/initrd").Run() == nil
	} else if i, err := exec.LookPath("isoinfo"); err == nil {
		for _, pair := range [][2]string{
			{"/" + upper + "/VMLINUZ.;1", out + "/vmlinuz"},
			{"/" + upper + "/INITRD.;1", out + "/initrd"},
		} {
			data, err := execCommand(i, "-i", iso, "-x", pair[0]).Output()
			if err == nil && len(data) > 0 {
				if os.WriteFile(pair[1], data, 0644) == nil {
					done = true
				}
			}
		}
	}
	if !done {
		fmt.Println("ERROR! Could not read the ISO (not ISO9660) and no bsdtar, 7z or isoinfo fallback is available.")
		return false
	}
	// 7z/bsdtar may place files at OUT root or under PREFIX.
	if st, err := os.Stat(out + "/vmlinuz"); err != nil || st.Size() == 0 {
		if st, err := os.Stat(filepath.Join(out, prefix, "vmlinuz")); err == nil && st.Size() > 0 {
			for _, k := range []string{"vmlinuz", "initrd"} {
				if err := os.Rename(filepath.Join(out, prefix, k), out+"/"+k); err != nil {
					fmt.Println("WARNING! could not move " + k + " into place: " + err.Error())
				}
			}
		}
	}
	vok := fileNonEmpty(out + "/vmlinuz")
	iok := fileNonEmpty(out + "/initrd")
	return vok && iok
}

func fileNonEmpty(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Size() > 0
}

// extract7z: pure-Go 7z extraction (bodgit/sevenzip) — mirrors
// `7z e -o<dir>` flat extraction, no external binary needed.
func extract7z(arc, dir string) bool {
	r, err := sevenzip.OpenReader(arc)
	if err != nil {
		fmt.Println("ERROR! could not read 7z archive: " + err.Error())
		return false
	}
	defer r.Close()
	ok := false
	for _, f := range r.File {
		if f.UncompressedSize > extractMaxMember {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		out, err := os.Create(filepath.Join(dir, filepath.Base(f.Name)))
		if err == nil {
			if _, err := io.Copy(out, rc); err != nil {
				out.Close()
				rc.Close()
				os.Remove(out.Name())
				return false
			}
			if err := out.Close(); err != nil {
				rc.Close()
				os.Remove(out.Name())
				return false
			}
			ok = true
		}
		rc.Close()
	}
	return ok
}

// decompressGZ/XZ/Zstd/BZ2 + extractZip: pure-Go decompressors so the
// pipeline does not depend on host gzip/xz/zstd/bzip2/unzip binaries.
func decompressGZ(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	w, err := os.Create(strings.TrimSuffix(p, ".gz"))
	if err != nil {
		return false
	}
	defer w.Close()
	if _, err := io.Copy(w, gr); err != nil {
		return false
	}
	os.Remove(p)
	return true
}

func decompressBZ2(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	w, err := os.Create(strings.TrimSuffix(p, ".bz2"))
	if err != nil {
		return false
	}
	defer w.Close()
	if _, err := io.Copy(w, bzip2.NewReader(f)); err != nil {
		return false
	}
	os.Remove(p)
	return true
}

func decompressXZ(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	xr, err := xz.NewReader(f)
	if err != nil {
		return false
	}
	w, err := os.Create(strings.TrimSuffix(p, ".xz"))
	if err != nil {
		return false
	}
	defer w.Close()
	if _, err := io.Copy(w, xr); err != nil {
		return false
	}
	os.Remove(p)
	return true
}

// decompressLZMA mirrors decompressXZ for .lzma payloads (nomadbsd).
func decompressLZMA(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	lr, err := lzma.NewReader(f)
	if err != nil {
		return false
	}
	w, err := os.Create(strings.TrimSuffix(p, ".lzma"))
	if err != nil {
		return false
	}
	defer w.Close()
	if _, err := io.Copy(w, lr); err != nil {
		return false
	}
	os.Remove(p)
	return true
}

func decompressZstd(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		return false
	}
	w, err := os.Create(strings.TrimSuffix(p, ".zst"))
	if err != nil {
		return false
	}
	defer w.Close()
	if _, err := io.Copy(w, zr.IOReadCloser()); err != nil {
		return false
	}
	return true
}

// extractMaxMember bounds one archive member's claimed size — a
// remote bomb must not fill the disk (mirrors quickemu-archive).
// Var so tests can lower the bound.
var extractMaxMember = uint64(512 << 30)

func extractZip(zipPath, dest string) bool {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return false
	}
	defer zr.Close()
	for _, zf := range zr.File {
		// Member names come from a remote server — a "../" or absolute
		// member would write outside the VM dir (zip-slip).
		if !filepath.IsLocal(zf.Name) {
			continue
		}
		if zf.UncompressedSize64 > extractMaxMember {
			continue
		}
		d := filepath.Join(dest, zf.Name)
		if zf.FileInfo().IsDir() {
			os.MkdirAll(d, 0755)
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		os.MkdirAll(filepath.Dir(d), 0755)
		w, err := os.Create(d)
		if err == nil {
			// A member that fails mid-copy (CRC mismatch, truncated
			// store) leaves a partial file — fail, don't ship it.
			if _, err := io.Copy(w, rc); err != nil {
				w.Close()
				rc.Close()
				os.Remove(d)
				return false
			}
			if err := w.Close(); err != nil {
				rc.Close()
				os.Remove(d)
				return false
			}
		}
		rc.Close()
	}
	return true
}

// findISO: after an archive unpack, pick up the extracted .iso name
// (Bash: ISO=$(ls -1 $VM_PATH/*.iso | basename)).
func findISO(vmPath, old string) string {
	os.Remove(filepath.Join(vmPath, old))
	m, _ := filepath.Glob(filepath.Join(vmPath, "*.iso"))
	if len(m) > 0 {
		return filepath.Base(m[0])
	}
	return old
}
