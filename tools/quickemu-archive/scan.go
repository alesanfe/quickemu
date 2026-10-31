// scan.go: the archive scanning pass — validates structure, hashes
// regular members and parses both manifest formats. verify inspects
// the result; extract/import re-open the archive and re-hash while
// staging (a swapped file between passes cannot bypass validation).
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// fileSum records one extracted-eligible member and its sha256.
type fileSum struct{ name, sum string }

// linkEntry records an allowed link member for materialisation after
// every regular file has landed in staging.
type linkEntry struct {
	member, target string // member = cleaned name, target = raw linkname
	hardlink       bool
}

// memberRef is one accepted member — clean name + tar type.
type memberRef struct {
	name string
	typ  byte
}

// scan is the outcome of pass 1.
type scan struct {
	members []memberRef
	files   []fileSum
	links   []linkEntry
	sums    map[string]string
	v1      *v1Manifest
	v2      *v2Manifest
	total   int64 // declared member bytes
	entries int   // member count
	vmName  string
}

// cleanName normalises a member name for identity checks — cleaned,
// slash-separated. Duplicates under this name are rejected.
func cleanName(n string) string { return filepath.ToSlash(filepath.Clean(n)) }

// qeErr carries an envelope error code + message.
type qeErr struct{ code, msg string }

func (e *qeErr) Error() string { return e.msg }

func qerr(code, format string, a ...any) *qeErr {
	return &qeErr{code: code, msg: fmt.Sprintf(format, a...)}
}

// metadata members are small — read bounded. qcow2 data members
// stream through the hasher/extractor; ReadAll would OOM.
const metaLimit = 1 << 20

// openTarGz opens a .tar.gz for one sequential pass. The caller must
// close the returned file; seeking f back to 0 starts a new pass.
func openTarGz(path string) (*os.File, *tar.Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, tar.NewReader(gz), nil
}

// scanArchive performs pass 1: per-member validation (name whitelist,
// type whitelist, link-target rules, count and byte caps, duplicate
// clean names), sha256 over regular members and manifest parsing.
// hash=false skips hashing (inspect only lists declared metadata).
func scanArchive(arch string, hash bool) (*scan, error) {
	f, tr, err := openTarGz(arch)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, qerr("NOT_FOUND", "%s", err.Error())
		}
		return nil, qerr("ARCHIVE_BAD", "%s", err.Error())
	}
	defer f.Close()

	s := &scan{sums: map[string]string{}}
	seen := map[string]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, qerr("ARCHIVE_BAD", "%s", err.Error())
		}
		s.entries++
		if s.entries > maxMembers {
			return nil, qerr("ARCHIVE_BOMB", "member limit %d exceeded", maxMembers)
		}
		s.total += h.Size
		if s.total > maxSize {
			return nil, qerr("ARCHIVE_BOMB", "size limit exceeded")
		}
		if !safeMember(h.Name) {
			return nil, qerr("TAR_SLIP", "unsafe member %s", h.Name)
		}
		clean := cleanName(h.Name)
		if _, dup := seen[clean]; dup {
			return nil, qerr("MANIFEST_DUP", "duplicate member path %s", clean)
		}
		seen[clean] = h.Typeflag
		switch h.Typeflag {
		case tar.TypeReg:
		case tar.TypeDir:
			s.members = append(s.members, memberRef{clean, h.Typeflag})
			continue
		case tar.TypeSymlink, tar.TypeLink:
			hard := h.Typeflag == tar.TypeLink
			if !safeLinkTarget(clean, h.Linkname, hard) {
				return nil, qerr("ARCHIVE_LINK", "unsafe link target %s -> %s", clean, h.Linkname)
			}
			s.links = append(s.links, linkEntry{member: clean, target: h.Linkname, hardlink: hard})
			s.members = append(s.members, memberRef{clean, h.Typeflag})
			continue
		default: // char, block, fifo, xattr, sparse
			return nil, qerr("ARCHIVE_LINK", "unsafe member type %s", clean)
		}
		base := path.Base(clean)
		if metadataFile(base) {
			s.members = append(s.members, memberRef{clean, h.Typeflag})
			body, err := io.ReadAll(io.LimitReader(tr, metaLimit+1))
			if err != nil {
				return nil, qerr("ARCHIVE_BAD", "%s", err.Error())
			}
			if int64(len(body)) != h.Size || len(body) > metaLimit {
				return nil, qerr("ARCHIVE_BAD", "metadata member %s oversized", clean)
			}
			switch base {
			case "manifest.json":
				var m v2Manifest
				if err := json.Unmarshal(body, &m); err != nil {
					return nil, qerr("MANIFEST_BAD", "manifest.json: %s", err.Error())
				}
				s.v2 = &m
				if s.vmName == "" {
					s.vmName = m.VM
				}
			case "quickvm-manifest.json":
				var m v1Manifest
				if json.Unmarshal(body, &m) == nil {
					s.v1 = &m
					if s.vmName == "" {
						s.vmName = m.VM
					}
				}
			case "quickvm-checksums.sha256":
				s.sums = parseSums(string(body))
			}
			continue
		}
		h256 := sha256.New()
		var w io.Writer = io.Discard
		if hash {
			w = h256
		}
		n, err := io.Copy(w, tr)
		if err != nil {
			return nil, qerr("ARCHIVE_BAD", "%s", err.Error())
		}
		if n != h.Size {
			return nil, qerr("ARCHIVE_BAD", "short read on %s", clean)
		}
		fs := fileSum{name: clean}
		if hash {
			fs.sum = hex.EncodeToString(h256.Sum(nil))
		}
		s.files = append(s.files, fs)
		s.members = append(s.members, memberRef{clean, h.Typeflag})
		// The conf member names the VM when no manifest does.
		if s.vmName == "" && strings.HasSuffix(base, ".conf") && !strings.Contains(clean, "/") {
			s.vmName = strings.TrimSuffix(base, ".conf")
		}
	}
	// Hardlink targets name members, not paths on disk — a target
	// absent from the archive (or not a regular member) can never
	// materialise, so reject it here rather than at install time.
	for _, l := range s.links {
		if !l.hardlink {
			continue
		}
		if t, ok := seen[cleanName(l.target)]; !ok || t != tar.TypeReg {
			return nil, qerr("ARCHIVE_LINK", "hardlink %s target not a file member", l.member)
		}
	}
	if s.v1 == nil && s.v2 == nil {
		return nil, qerr("MANIFEST_MISSING", "no manifest in archive")
	}
	return s, nil
}

// verifyManifests applies every manifest rule the archive carries.
func (s *scan) verifyManifests() error {
	if err := s.verifySums(); err != nil {
		return err
	}
	return s.verifyV2()
}
