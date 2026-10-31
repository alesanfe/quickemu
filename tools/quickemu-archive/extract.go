// extract.go: pass 2 — staged extraction. Nothing is written to the
// destination directly: members land in a private staging dir inside
// it (same filesystem → the final os.Rename is atomic), staged
// content is re-verified against the pass-1 hashes, links materialise
// only after every regular file exists, and the journal records the
// transition into "installing" before the renames.
package main

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// freeSpace is the statfs seam — tests stub it to force the
// disk-full path without needing a full filesystem.
var freeSpace = statfsFree

// mkdirTemp is the staging-dir seam.
var mkdirTemp = os.MkdirTemp

// extracted maps the layout policy: where an archive member lands
// relative to the staging root.
type extracted struct {
	staging string            // staging root
	rel     map[string]string // clean member name -> staging-relative path
	conf    string            // clean conf member name routed aside (vmMode)
}

// layoutRel maps a clean member-namespace path to its staging-relative
// path under the VM layout: "<vm>/x" → "x", "<vm>.conf" is routed
// outside the staging dir (ok=false — it must never resolve inside),
// and any other nested path is an IMPORT_LAYOUT violation. Generic
// mode (vmMode off) keeps paths verbatim.
func layoutRel(clean, vm string, vmMode bool) (string, bool) {
	if !vmMode {
		return clean, clean != ""
	}
	if clean == vm || clean == vm+".conf" {
		return "", false // the dir root itself / the routed conf
	}
	strip := vm + "/"
	if strings.HasPrefix(clean, strip) {
		return clean[len(strip):], true
	}
	if strings.Contains(clean, "/") {
		return "", false
	}
	return clean, true
}

// layOut computes the staging-relative path of every member.
func (s *scan) layOut(staging, vm string, vmMode bool) (*extracted, error) {
	ex := &extracted{staging: staging, rel: map[string]string{}}
	for _, m := range s.members {
		clean := m.name
		if vmMode && clean == vm+".conf" {
			ex.conf = clean
			continue
		}
		if metadataFile(path.Base(clean)) {
			continue // consumed during verification, not installed
		}
		rel, ok := layoutRel(clean, vm, vmMode)
		if !ok {
			if vmMode && clean == vm {
				continue // the <vm>/ root dir entry itself
			}
			return nil, qerr("IMPORT_LAYOUT", "member %s outside %s/", clean, vm)
		}
		if !safeMember(rel) || !underRoot(staging, rel) {
			return nil, qerr("TAR_SLIP", "unsafe member %s", clean)
		}
		ex.rel[clean] = rel
	}
	return ex, nil
}

// stageArchive pass 2: re-open the archive, stream every staged
// member under ex.staging, then materialise the registered links.
func (s *scan) stageArchive(arch string, ex *extracted, vm string, vmMode bool) error {
	f, tr, err := openTarGz(arch)
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return qerr("ARCHIVE_BAD", "%s", err.Error())
		}
		clean := cleanName(h.Name)
		if metadataFile(path.Base(clean)) {
			continue
		}
		if ex.conf != "" && clean == ex.conf {
			if h.Typeflag == tar.TypeReg {
				// The conf stages as a sidecar next to the staging dir.
				if err := stageFile(tr, ex.staging+".conf", "", 0o600, h.Size); err != nil {
					return err
				}
			}
			continue
		}
		rel, ok := ex.rel[clean]
		if !ok {
			continue // link member (deferred) or layout-dropped
		}
		dst := filepath.Join(ex.staging, filepath.FromSlash(rel))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return qerr("EXTRACT_FAIL", "%s", err.Error())
			}
		case tar.TypeReg:
			if err := stageFile(tr, dst, ex.staging, fs.FileMode(h.Mode)&0o777, h.Size); err != nil {
				return err
			}
		}
	}
	return s.materialiseLinks(ex, vm, vmMode)
}

// stageFile streams one member to dst. When root is non-empty the
// resolved parent dir must stay under it — a symlinked component
// escaping the lexical gates is caught by EvalSymlinks containment.
func stageFile(r io.Reader, dst, root string, mode fs.FileMode, want int64) error {
	if mode == 0 {
		mode = 0o600
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return qerr("EXTRACT_FAIL", "%s", err.Error())
	}
	if root != "" {
		if real, err := filepath.EvalSymlinks(dir); err == nil && !underRoot(root, mustRel(root, real)) {
			return qerr("TAR_SLIP", "symlinked parent escapes staging: %s", dir)
		}
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return qerr("EXTRACT_FAIL", "%s", err.Error())
	}
	n, err := io.Copy(out, r)
	if cerr := out.Close(); cerr != nil && err == nil {
		return qerr("EXTRACT_FAIL", "%s", cerr.Error())
	}
	if err != nil {
		return qerr("EXTRACT_FAIL", "%s", err.Error())
	}
	if n != want {
		return qerr("ARCHIVE_BAD", "short read on %s", dst)
	}
	return nil
}

// mustRel resolves p relative to root, "" on failure.
func mustRel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return ".."
	}
	return r
}

// materialiseLinks creates every registered link inside staging.
// Targets were validated at scan time and member names are
// duplicate-free; here the target is re-mapped through the staging
// layout so namespace shifts (vm/ stripping) cannot silently
// redirect a link.
func (s *scan) materialiseLinks(ex *extracted, vm string, vmMode bool) error {
	for _, l := range s.links {
		rel, ok := ex.rel[l.member]
		if !ok {
			continue // link dropped by layout
		}
		dst := filepath.Join(ex.staging, filepath.FromSlash(rel))
		if l.hardlink {
			// Hardlink target names a member relative to the archive
			// root — it must be a staged regular file.
			tgtRel, ok := ex.rel[cleanName(l.target)]
			if !ok {
				return qerr("ARCHIVE_LINK", "hardlink %s target not staged", l.member)
			}
			tgt := filepath.Join(ex.staging, filepath.FromSlash(tgtRel))
			if st, err := os.Lstat(tgt); err != nil || !st.Mode().IsRegular() {
				return qerr("ARCHIVE_LINK", "hardlink %s target not a file", l.member)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return qerr("EXTRACT_FAIL", "%s", err.Error())
			}
			if err := os.Link(tgt, dst); err != nil {
				return qerr("EXTRACT_FAIL", "%s", err.Error())
			}
			continue
		}
		// Symlink: member-namespace target → staging-rel target. The
		// stored link text must still resolve to that staged path —
		// if layout remapped it the link would point elsewhere, so
		// refuse rather than write a redirected link.
		tgtNS := linkTargetPath(l.member, l.target)
		tgtRel, ok := layoutRel(tgtNS, vm, vmMode)
		if !ok {
			return qerr("ARCHIVE_LINK", "symlink %s escapes the VM dir", l.member)
		}
		want, err := filepath.Rel(filepath.Dir(dst),
			filepath.Join(ex.staging, filepath.FromSlash(tgtRel)))
		if err != nil || filepath.ToSlash(want) != filepath.ToSlash(filepath.Clean(l.target)) {
			return qerr("ARCHIVE_LINK", "symlink %s target remapped by layout", l.member)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return qerr("EXTRACT_FAIL", "%s", err.Error())
		}
		if err := os.Symlink(l.target, dst); err != nil {
			return qerr("EXTRACT_FAIL", "%s", err.Error())
		}
	}
	return nil
}

// memberNames returns the staging-relative member list recorded in
// the journal — the set recover must find complete before finishing
// an interrupted install.
func (ex *extracted) memberNames() []string {
	out := make([]string, 0, len(ex.rel))
	for _, rel := range ex.rel {
		out = append(out, rel)
	}
	return out
}

// ensureSpace gates extraction on declared member bytes fitting in
// the filesystem holding dest (QE-090 — checked before any write).
func (s *scan) ensureSpace(dest string) error {
	free, err := freeSpace(dest)
	if err != nil {
		return qerr("SPACE_CHECK", "cannot statfs %s: %s", dest, err.Error())
	}
	if free < s.total {
		return qerr("INSUFFICIENT_SPACE",
			"archive needs %d bytes, %s has %d free", s.total, dest, free)
	}
	return nil
}

// verifyStaged re-hashes every staged regular file and compares it
// with the pass-1 digest — catches write-time corruption before the
// install renames run. Files stream through the hasher; ReadAll
// would OOM on multi-GB members.
func (s *scan) verifyStaged(ex *extracted) error {
	for _, fm := range s.files {
		if fm.sum == "" {
			continue
		}
		var p string
		switch {
		case ex.conf != "" && fm.name == ex.conf:
			p = ex.staging + ".conf"
		default:
			rel, ok := ex.rel[fm.name]
			if !ok {
				continue // metadata members are not staged
			}
			p = filepath.Join(ex.staging, filepath.FromSlash(rel))
		}
		f, err := os.Open(p)
		if err != nil {
			return qerr("EXTRACT_FAIL", "%s", err.Error())
		}
		h256 := sha256.New()
		_, err = io.Copy(h256, f)
		f.Close()
		if err != nil {
			return qerr("EXTRACT_FAIL", "%s", err.Error())
		}
		if got := fmt.Sprintf("%x", h256.Sum(nil)); got != fm.sum {
			return qerr("CHECKSUM_MISMATCH", "staged %s failed sha256", fm.name)
		}
	}
	return nil
}

// extraction holds the finished operation's state for the caller's
// result payload.
type extraction struct {
	scan    *scan
	ex      *extracted
	journal *journal
	jpath   string
}

// runExtraction is the shared pipeline for extract and import:
// recover leftovers → scan → manifest verify → space gate → stage →
// links → staged re-verify → journal flip → install → delete
// journal. In vmMode the VM name is derived from the archive itself
// (manifest vm or the <vm>.conf member) — never from the filename.
// Handled failures clean staging; only a hard crash leaves the
// journal for recover.
func runExtraction(arch, dest string, vmMode, force bool) (*extraction, error) {
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return nil, qerr("EXTRACT_FAIL", "%s", err.Error())
	}
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return nil, qerr("EXTRACT_FAIL", "%s", err.Error())
	}
	if vmMode {
		// Leftover .import.tmp state from a crashed import is resolved
		// first — never stack a new staging dir on top of it.
		if _, err := recoverDest(destAbs); err != nil {
			return nil, qerr("EXTRACT_FAIL", "recover: %s", err.Error())
		}
	}
	s, err := scanArchive(arch, true)
	if err != nil {
		return nil, err
	}
	if err := s.verifyManifests(); err != nil {
		return nil, err
	}
	vm := s.vmName
	if !vmMode {
		vm = "" // the manifest vm only drives the VM-aware layout
	}
	if vmMode {
		if vm == "" {
			return nil, qerr("MANIFEST_MISSING", "archive names no VM")
		}
		if vm == "." || vm == ".." || strings.ContainsAny(vm, "/\\") {
			return nil, qerr("IMPORT_LAYOUT", "unsafe VM name %s", vm)
		}
		if !force {
			for _, p := range []string{
				filepath.Join(destAbs, vm),
				filepath.Join(destAbs, vm+".conf"),
			} {
				if _, err := os.Lstat(p); err == nil {
					return nil, qerr("IMPORT_CONFLICT",
						"%s exists (use --force to overwrite)", p)
				}
			}
		}
	}
	if err := s.ensureSpace(destAbs); err != nil {
		return nil, err
	}

	var staging, jname string
	if vmMode {
		staging = filepath.Join(destAbs, vm+".import.tmp")
		jname = vm + ".import.journal"
	} else {
		tmp, err := mkdirTemp(destAbs, ".qe-extract-*.tmp")
		if err != nil {
			return nil, qerr("EXTRACT_FAIL", "%s", err.Error())
		}
		staging = tmp
		jname = strings.TrimSuffix(filepath.Base(tmp), ".tmp") + ".qe-extract.journal"
	}
	j := &journal{
		Version: 1, VM: vm, DestDir: destAbs, State: jStateExtracting,
		Staging: staging, Force: force,
		Created: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	if vmMode {
		j.ConfTmp = staging + ".conf"
		j.ConfDest = vm + ".conf"
	}
	jpath := filepath.Join(destAbs, jname)
	if err := writeJournal(jpath, j); err != nil {
		if !vmMode {
			os.RemoveAll(staging)
		}
		return nil, qerr("EXTRACT_FAIL", "journal: %s", err.Error())
	}
	fail := func(e error) (*extraction, error) {
		os.RemoveAll(staging)
		if j.ConfTmp != "" {
			os.Remove(j.ConfTmp)
		}
		os.Remove(jpath)
		return nil, e
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return fail(qerr("EXTRACT_FAIL", "%s", err.Error()))
	}
	ex, err := s.layOut(staging, vm, vmMode)
	if err != nil {
		return fail(err)
	}
	exn := &extraction{scan: s, ex: ex, journal: j, jpath: jpath}
	if err := s.stageArchive(arch, ex, vm, vmMode); err != nil {
		return fail(err)
	}
	if err := s.verifyStaged(ex); err != nil {
		return fail(err)
	}
	j.Members = ex.memberNames()
	j.State = jStateInstalling
	if err := writeJournal(jpath, j); err != nil {
		return fail(qerr("EXTRACT_FAIL", "journal: %s", err.Error()))
	}
	if err := installStaged(j); err != nil {
		return fail(qerr("EXTRACT_FAIL", "%s", err.Error()))
	}
	os.Remove(jpath)
	return exn, nil
}
