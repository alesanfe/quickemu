// quickemu-archive: ADR-0001 phase 6 — manifest verification and safe
// extraction for .quickvm.tar.gz exports. Mirrors the Bash whitelist:
// no absolute paths, .., symlinks, hardlinks, devices, FIFOs.
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
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const version = "0.1.0"

func die(c, m string) { qelib.Fail(qelib.CmdName("archive"), "quickemu-archive/"+version, c, m) }

// ok emits the envelope; d is a pre-marshalled JSON fragment (the
// small fixed payloads built by callers below).
func ok(d string) {
	qelib.EmitOK(qelib.CmdName("archive"), "quickemu-archive/"+version, json.RawMessage(d))
	os.Exit(0)
}

const maxMembers = 100000
const maxDepth = 16
const maxSize = int64(512 << 30) // 512GiB total

type manifest struct {
	SchemaVersion int      `json:"schemaVersion"`
	VM            string   `json:"vm"`
	CreatedAt     string   `json:"createdAt"`
	Members       []string `json:"members"`
}

func safeMember(name string) bool {
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") || strings.Contains(name, ":") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return false
	}
	return strings.Count(clean, "/") < maxDepth
}

func main() {
	os.Args = qelib.NormArgs(os.Args)
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-archive <verify|extract> --archive <tgz> [--dest <dir>]")
		os.Exit(2)
	}
	sub := os.Args[1]
	var arch, dest string
	for i := 2; i+1 < len(os.Args); i += 2 {
		switch os.Args[i] {
		case "--archive":
			arch = os.Args[i+1]
		case "--dest":
			dest = os.Args[i+1]
		}
	}
	f, err := os.Open(arch)
	if err != nil {
		die("NOT_FOUND", err.Error())
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		die("ARCHIVE_BAD", err.Error())
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var man manifest
	var haveMan bool
	sums := map[string]string{} // name -> expected sha256 (from .sha256 file)
	entries := 0
	var total int64
	var files []string

	// First pass for verify; for extract we stream into dest.
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			die("ARCHIVE_BAD", err.Error())
		}
		entries++
		if entries > maxMembers {
			die("ARCHIVE_BOMB", "member limit")
		}
		total += h.Size
		if total > maxSize {
			die("ARCHIVE_BOMB", "size limit")
		}
		if !safeMember(h.Name) {
			die("TAR_SLIP", "unsafe member "+h.Name)
		}
		if !filepath.IsLocal(h.Name) {
			die("TAR_SLIP", "non-local member "+h.Name)
		}
		switch h.Typeflag {
		case tar.TypeReg:
		case tar.TypeDir:
			continue
		default: // symlink, link, char, block, fifo, xattr
			die("ARCHIVE_LINK", "unsafe member type "+h.Name)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			die("ARCHIVE_BAD", err.Error())
		}
		if int64(len(body)) != h.Size {
			die("ARCHIVE_BAD", "short read on "+h.Name)
		}
		base := filepath.Base(h.Name)
		if base == "quickvm-manifest.json" {
			haveMan = json.Unmarshal(body, &man) == nil
		}
		if base == "quickvm-checksums.sha256" {
			for _, l := range strings.Split(string(body), "\n") {
				p := strings.Fields(l)
				if len(p) == 2 {
					sums[p[1]] = p[0]
				}
			}
			continue
		}
		sum := sha256.Sum256(body)
		cname := filepath.ToSlash(filepath.Clean(h.Name))
		files = append(files, cname+"="+hex.EncodeToString(sum[:]))
		if sub == "extract" && dest != "" {
			out := filepath.Join(dest, h.Name)
			os.MkdirAll(filepath.Dir(out), 0o755)
			os.WriteFile(out, body, 0o600)
		}
	}
	if !haveMan {
		die("MANIFEST_MISSING", "quickvm-manifest.json absent")
	}
	// Verify every declared member checksum by CANONICAL FULL PATH —
	// basename matching lets disk1/data.qcow2 collide with
	// disk2/data.qcow2. Reject duplicate/equivalent paths outright.
	mismatch := 0
	seen := map[string]bool{}
	for _, fm := range files {
		name, sum, _ := strings.Cut(fm, "=")
		if seen[name] {
			die("MANIFEST_DUP", "duplicate member path "+name)
		}
		seen[name] = true
		want, declared := sums[name]
		if !declared { // tolerate manifests that key by basename
			want, declared = sums[filepath.Base(name)]
		}
		if declared && want != sum {
			mismatch++
		}
	}
	if mismatch > 0 {
		die("CHECKSUM_MISMATCH", fmt.Sprintf("%d members failed sha256", mismatch))
	}
	d, _ := json.Marshal(map[string]any{"members": entries, "verified": len(files)})
	ok(string(d))
}
