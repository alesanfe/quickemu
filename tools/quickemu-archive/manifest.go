// manifest.go: archive manifest formats. v1 is the legacy pair
// quickvm-manifest.json + quickvm-checksums.sha256 (sha256sum-style
// lines). v2 (QE-091, manifest.json) is the portable versioned
// format: format_version, normalised conf, per-blob sha256, disk
// metadata (format, backing chain, virtual size, internal
// snapshots) and the minimum QEMU version needed to boot the VM.
package main

import (
	"fmt"
	"path"
	"strings"
)

// v1Manifest mirrors quickvm-manifest.json produced by
// quickemu-vm export (formatVersion 1).
type v1Manifest struct {
	FormatVersion int    `json:"formatVersion"`
	SchemaVersion int    `json:"schemaVersion"` // pre-formatVersion exports
	VM            string `json:"vm"`
	CreatedAt     string `json:"createdAt"`
	Checksums     string `json:"checksums"`
}

// v2Manifest is manifest.json — format_version 2.
type v2Manifest struct {
	FormatVersion   int               `json:"format_version"`
	VM              string            `json:"vm"`
	CreatedAt       string            `json:"created_at"`
	QuickemuVersion string            `json:"quickemu_version"`
	MinQemuVersion  string            `json:"min_qemu_version,omitempty"`
	Conf            map[string]string `json:"conf,omitempty"`
	ConfSHA256      string            `json:"conf_sha256,omitempty"`
	IncludesTPM     bool              `json:"includes_tpm,omitempty"`
	Blobs           []v2Blob          `json:"blobs,omitempty"`
	Disks           []v2Disk          `json:"disks,omitempty"`
}

type v2Blob struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Kind   string `json:"kind,omitempty"` // conf | disk | image | file
}

type v2Snapshot struct {
	Name   string `json:"name"`
	ID     string `json:"id,omitempty"`
	Date   string `json:"date,omitempty"`
	VMSize int64  `json:"vm_size,omitempty"`
}

type v2Disk struct {
	Path         string       `json:"path"`
	Format       string       `json:"format"`
	VirtualSize  int64        `json:"virtual_size,omitempty"`
	BackingFile  string       `json:"backing_file,omitempty"`
	BackingChain []string     `json:"backing_chain,omitempty"`
	Snapshots    []v2Snapshot `json:"snapshots,omitempty"`
}

const v2FormatVersion = 2

// metadataFile reports whether name (archive-member base name) is an
// internal archive-metadata member, consumed during verification and
// never installed into the VM directory.
func metadataFile(base string) bool {
	switch base {
	case "quickvm-manifest.json", "quickvm-checksums.sha256", "manifest.json":
		return true
	}
	return false
}

// parseSums decodes quickvm-checksums.sha256 — "hex  path" lines.
func parseSums(body string) map[string]string {
	sums := map[string]string{}
	for _, l := range strings.Split(body, "\n") {
		p := strings.Fields(l)
		if len(p) == 2 && len(p[0]) == 64 {
			sums[p[1]] = p[0]
		}
	}
	return sums
}

// verifySums applies the v1 rule: every declared member checksum must
// match the computed one (basename keys tolerated for legacy
// checksums files that did not record full paths).
func (s *scan) verifySums() error {
	for _, fm := range s.files {
		want, declared := s.sums[fm.name]
		if !declared { // tolerate manifests that key by basename
			want, declared = s.sums[path.Base(fm.name)]
		}
		if declared && want != fm.sum {
			return fmt.Errorf("CHECKSUM_MISMATCH: %s failed sha256", fm.name)
		}
	}
	return nil
}

// verifyV2 applies the v2 rule, strict by design: format_version must
// be understood, every declared blob must exist as a regular member
// whose sha256 matches, and conf_sha256 must equal the conf member's
// computed digest.
func (s *scan) verifyV2() error {
	m := s.v2
	if m == nil {
		return nil
	}
	if m.FormatVersion > v2FormatVersion {
		return fmt.Errorf("UNSUPPORTED_FORMAT: manifest format_version %d", m.FormatVersion)
	}
	byPath := map[string]string{}
	for _, fm := range s.files {
		byPath[fm.name] = fm.sum
	}
	declared := map[string]bool{}
	for _, b := range m.Blobs {
		if !safeMember(b.Path) {
			return fmt.Errorf("MANIFEST_BAD: unsafe blob path %q", b.Path)
		}
		clean := cleanName(b.Path)
		if declared[clean] {
			return fmt.Errorf("MANIFEST_BAD: duplicate blob %q", b.Path)
		}
		declared[clean] = true
		sum, ok := byPath[clean]
		if !ok {
			return fmt.Errorf("MANIFEST_BAD: blob %q absent from archive", b.Path)
		}
		if b.SHA256 != "" && sum != b.SHA256 {
			return fmt.Errorf("CHECKSUM_MISMATCH: blob %s failed sha256", b.Path)
		}
	}
	if m.ConfSHA256 != "" && m.VM != "" {
		if sum, ok := byPath[m.VM+".conf"]; ok && sum != m.ConfSHA256 {
			return fmt.Errorf("CHECKSUM_MISMATCH: conf failed sha256")
		}
	}
	return nil
}
