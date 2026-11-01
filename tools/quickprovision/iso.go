// iso.go: ISO9660 extraction — pull the casper kernel/initrd out of
// an Ubuntu server ISO so the installer can boot via -kernel/-initrd
// with the autoinstall seed, no media-swap needed mid-install.
// Mirrors quickget-vm's isoExtractBootfiles walk.
package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// listISODir returns the basenames of regular files whose parent dir
// normalises to dirPrefix — the read side of extractBootfiles' walk,
// used to discover which kernel flavour an installer image ships.
func listISODir(isoPath, dirPrefix string) ([]string, error) {
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
	var names []string
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
			if parentMatch {
				names = append(names, name)
			}
		}
		return nil
	}
	if err := walk(root, dirPrefix == ""); err != nil {
		return nil, err
	}
	return names, nil
}

// alpineKernelFlavor returns the kernel suffix the ISO ships —
// "virt" for alpine-virt, "lts" for standard/extended — by pairing
// vmlinuz-* with initramfs-* under boot/.
func alpineKernelFlavor(isoPath string) (string, error) {
	names, err := listISODir(isoPath, "boot")
	if err != nil {
		return "", err
	}
	f, ok := pickAlpineFlavor(names, strings.ToLower(filepath.Base(isoPath)))
	if !ok {
		return "", fmt.Errorf("no vmlinuz-*/initramfs-* pair under boot/ in %s", isoPath)
	}
	return f, nil
}

// pickAlpineFlavor derives the flavour from a boot/ name list: a pair
// exists only where vmlinuz-<x> and initramfs-<x> both appear. The ISO
// filename usually names its flavour (alpine-virt-*.iso), which
// decides when several kernels coexist. ISO9660 base names cannot
// hold '-' — a reader skipping the Rock Ridge/Joliet extensions
// reports VMLINUZ_VIRT — so names are folded to alnum first.
func pickAlpineFlavor(names []string, isoBase string) (string, bool) {
	kernels, initrds := map[string]bool{}, map[string]bool{}
	for _, n := range names {
		d := dirNorm(n)
		if s, ok := strings.CutPrefix(d, "vmlinuz"); ok && s != "" {
			kernels[s] = true
		}
		if s, ok := strings.CutPrefix(d, "initramfs"); ok && s != "" {
			initrds[s] = true
		}
	}
	var pairs []string
	for k := range kernels {
		if initrds[k] {
			pairs = append(pairs, k)
		}
	}
	if len(pairs) == 0 {
		return "", false
	}
	sort.Strings(pairs)
	for _, p := range pairs {
		if strings.Contains(isoBase, "-"+p) {
			return p, true
		}
	}
	return pairs[0], true
}

// injectInitramfsFile appends a file to a gzipped initramfs cpio. The
// kernel unpacks concatenated archives, so the payload is added as a
// second newc archive after the original stream — no walk or rewrite
// of the existing entries needed.
func injectInitramfsFile(initrd, name string, data []byte, mode int64) error {
	f, err := os.Open(initrd)
	if err != nil {
		return err
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return fmt.Errorf("initramfs %s: %w", initrd, err)
	}
	raw, err := io.ReadAll(zr)
	zr.Close()
	f.Close()
	if err != nil {
		return fmt.Errorf("initramfs %s: %w", initrd, err)
	}

	var buf bytes.Buffer
	buf.Write(raw)
	padTo(&buf, 512)
	writeNewcEntry(&buf, name, data, mode)
	writeNewcEntry(&buf, "TRAILER!!!", nil, 0)
	padTo(&buf, 512)

	tmp := initrd + ".inj"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err := zw.Write(buf.Bytes()); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := zw.Close(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, initrd)
}

// padTo zero-pads the buffer up to a multiple of n — cpio archives
// start on 512-byte boundaries, entry fields align to 4.
func padTo(buf *bytes.Buffer, n int) {
	for buf.Len()%n != 0 {
		buf.WriteByte(0)
	}
}

// writeNewcEntry emits one SVR4-newc header + name + data block. The
// thirteen hex fields follow "070701": ino, mode, uid, gid, nlink,
// mtime, size, dev-major/minor, rdev-major/minor, namesize, check.
func writeNewcEntry(buf *bytes.Buffer, name string, data []byte, mode int64) {
	fmt.Fprintf(buf, "070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
		1, 0o100000|mode, 0, 0, 1, 0, len(data), 0, 0, 0, 0, len(name)+1, 0)
	buf.WriteString(name)
	buf.WriteByte(0)
	padTo(buf, 4)
	buf.Write(data)
	padTo(buf, 4)
}

// alpineRelease extracts the repository tag ("v3.22") from an
// alpine-virt-3.22.7-x86_64.iso style name so the written
// repositories match the release actually being installed.
var alpineReleaseRe = regexp.MustCompile(`(\d+)\.(\d+)`)

func alpineRelease(isoPath string) string {
	if m := alpineReleaseRe.FindStringSubmatch(
		filepath.Base(isoPath)); m != nil {
		return "v" + m[1] + "." + m[2]
	}
	return "latest-stable"
}
