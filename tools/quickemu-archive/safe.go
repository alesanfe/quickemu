// safe.go: the tar-slip gates — member-name and link-target
// validation shared by verify, extract and import. QE-090 tightens
// the original whitelist: post-IsLocal residuals (absolute paths,
// surviving "..", backslash/drive-letter and NUL smuggling) are
// rejected outright, and links are no longer blanket-refused — only
// links whose targets stay inside the archive root pass.
package main

import (
	"archive/tar"
	"path"
	"path/filepath"
	"strings"
)

const maxMembers = 100000
const maxDepth = 16
const maxSize = int64(512 << 30) // 512GiB total

// memberKinds worth materialising; every other typeflag is refused.
func allowedType(t byte) bool {
	switch t {
	case tar.TypeReg, tar.TypeDir, tar.TypeSymlink, tar.TypeLink:
		return true
	}
	return false
}

// suspiciousChars are never legal in an archive member or link target:
// '\' smuggles traversal on Windows hosts, ':' is drive-letter/ADS,
// NUL truncates paths in C-level filesystem calls.
const suspiciousChars = "\\:\x00"

// safeMember is the first tar-slip gate: absolute paths, drive
// letters, residual ".." segments and runaway depth are rejected —
// on top of filepath.IsLocal, not instead of it, because a hostile
// archive may be authored for a different OS's separator rules.
func safeMember(name string) bool {
	// Empty names and "." slip past lexical checks but are not local
	// paths — reject before anything else.
	if name == "" || name == "." {
		return false
	}
	if !filepath.IsLocal(name) {
		return false
	}
	if strings.HasPrefix(name, "/") || strings.ContainsAny(name, suspiciousChars) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	// Residual ".." anywhere post-Clean: filepath.Clean collapses
	// interior "a/../b" into "b" (safe), so a surviving segment is
	// always an escape attempt.
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." {
			return false
		}
	}
	return strings.Count(clean, "/") < maxDepth
}

// safeLinkTarget reports whether a symlink/hardlink member's target
// resolves inside the archive root. Symlink targets are relative to
// the member's directory (POSIX); hardlink targets name another
// member relative to the archive root. Both must be local paths with
// no escape segments — links pointing outside the VM are refused.
func safeLinkTarget(memberName, linkname string, hardlink bool) bool {
	if linkname == "" || linkname == "." {
		return false
	}
	if !filepath.IsLocal(linkname) {
		return false
	}
	if strings.HasPrefix(linkname, "/") || strings.ContainsAny(linkname, suspiciousChars) {
		return false
	}
	lc := filepath.ToSlash(linkname)
	var tgt string
	if hardlink {
		tgt = path.Clean(lc) // root-relative member name
	} else {
		tgt = path.Clean(path.Join(path.Dir(filepath.ToSlash(memberName)), lc))
	}
	if tgt == "." || tgt == ".." || strings.HasPrefix(tgt, "../") {
		return false
	}
	for _, seg := range strings.Split(tgt, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// linkTargetPath returns the staging-relative path a symlink target
// resolves to, used at materialisation time to re-verify the target
// stays under the staging root.
func linkTargetPath(memberName, linkname string) string {
	return path.Clean(path.Join(path.Dir(filepath.ToSlash(memberName)),
		filepath.ToSlash(linkname)))
}

// underRoot resolves rel against root and verifies containment —
// post-join defence in depth on top of the lexical gates.
func underRoot(root, rel string) bool {
	joined := filepath.Join(root, rel)
	r, err := filepath.Rel(root, joined)
	if err != nil {
		return false
	}
	return r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}
