// oci.go — QE-092: OCI image layout push/pull for VM exports.
//
// The layout follows the OCI Image Layout spec so any OCI tooling
// (skopeo copy oci:… docker:…, podman load) can ferry the image to a
// real registry; the push/pull here works purely on the filesystem —
// no network, no registry auth, fully deterministic for tests.
//
//	blobs/sha256/<tgz>          layer = the .quickvm.tar.gz export
//	blobs/sha256/<config>       config = vm name + v2 manifest data
//	blobs/sha256/<manifest>     OCI image manifest
//	index.json + oci-layout
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ociLayerMedia  = "application/vnd.quickemu.vm.layer.v1.tar+gzip"
	ociConfMedia   = "application/vnd.quickemu.vm.config.v1+json"
	ociManifestM   = "application/vnd.oci.image.manifest.v1+json"
	ociLayoutVer   = "1.0.0"
	ociRefDefault  = "quickemu-vm:latest"
	ociRefAnnotKey = "org.opencontainers.image.ref.name"
)

type ociDescriptor struct {
	MediaType string            `json:"mediaType"`
	Digest    string            `json:"digest"`
	Size      int64             `json:"size"`
	Annots    map[string]string `json:"annotations,omitempty"`
}

// refNameOk: OCI ref components land in directory and file names.
var refNameOkRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func refNameOk(s string) bool { return refNameOkRe.MatchString(s) && s != "." && s != ".." }

// ociBlob writes data as a content-addressed blob and returns its
// descriptor.
func ociBlob(layoutDir, media string, data []byte) (ociDescriptor, error) {
	sum := sha256.Sum256(data)
	hexsum := hex.EncodeToString(sum[:])
	p := filepath.Join(layoutDir, "blobs", "sha256", hexsum)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return ociDescriptor{}, err
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return ociDescriptor{}, err
	}
	return ociDescriptor{MediaType: media,
		Digest: "sha256:" + hexsum, Size: int64(len(data))}, nil
}

// ociPush packs <arch> as an OCI layout under <dest>/<ref-dir>.
// ref may carry a tag (name:tag); the layout lands in dest/name.
func ociPush(arch, dest, ref string) int {
	if ref == "" {
		ref = ociRefDefault
	}
	// Exactly one optional name:tag split; both sides carry the
	// filename-safe charset (the name becomes the layout dir).
	parts := strings.Split(ref, ":")
	if len(parts) > 2 || !refNameOk(parts[0]) ||
		(len(parts) == 2 && !refNameOk(parts[1])) {
		return die("OCI_BAD_REF", "ref must be a bare name or name:tag")
	}
	name := parts[0]
	layout := filepath.Join(dest, name)
	if _, err := os.Stat(layout); err == nil {
		return die("OCI_EXISTS", "layout exists: "+layout+" (remove it or pick another ref)")
	}
	// Layer = the archive bytes.
	tgz, err := os.ReadFile(arch)
	if err != nil {
		return die("ARCHIVE_BAD", err.Error())
	}
	layerDesc, err := ociBlob(layout, ociLayerMedia, tgz)
	if err != nil {
		return die("OCI_WRITE", err.Error())
	}
	// Config = inspect data — the v2 manifest travels with the image.
	s, _ := scanArchive(arch, false)
	vmName := ""
	created := ""
	if s != nil {
		vmName = s.vmName
		if s.v2 != nil {
			created = s.v2.CreatedAt
		} else if s.v1 != nil {
			created = s.v1.CreatedAt
		}
	}
	cfg, _ := json.Marshal(map[string]any{
		"architecture": "x86_64", "os": "linux",
		"created": created, "vm": vmName, "quickemu_format": "quickvm.tar.gz",
	})
	cfgDesc, err := ociBlob(layout, ociConfMedia, cfg)
	if err != nil {
		return die("OCI_WRITE", err.Error())
	}
	manifest, _ := json.Marshal(map[string]any{
		"schemaVersion": 2, "mediaType": ociManifestM,
		"config": cfgDesc, "layers": []ociDescriptor{layerDesc},
	})
	manDesc, err := ociBlob(layout, ociManifestM, manifest)
	if err != nil {
		return die("OCI_WRITE", err.Error())
	}
	manDesc.Annots = map[string]string{ociRefAnnotKey: ref}
	index, _ := json.Marshal(map[string]any{
		"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json",
		"manifests": []ociDescriptor{manDesc},
	})
	if err := os.WriteFile(filepath.Join(layout, "index.json"), index, 0o644); err != nil {
		return die("OCI_WRITE", err.Error())
	}
	if err := os.WriteFile(filepath.Join(layout, "oci-layout"),
		[]byte(`{"imageLayoutVersion":"`+ociLayoutVer+`"}`), 0o644); err != nil {
		return die("OCI_WRITE", err.Error())
	}
	d, _ := json.Marshal(map[string]any{
		"pushed": ref, "layout": layout,
		"digest": manDesc.Digest, "size": layerDesc.Size, "vm": vmName})
	return ok(string(d))
}

// ociReadManifest resolves the layout's index.json → image manifest.
func ociReadManifest(layout string) (map[string]any, ociDescriptor, error) {
	b, err := os.ReadFile(filepath.Join(layout, "index.json"))
	if err != nil {
		return nil, ociDescriptor{}, fmt.Errorf("no index.json: %w", err)
	}
	var idx struct {
		Manifests []ociDescriptor `json:"manifests"`
	}
	if json.Unmarshal(b, &idx) != nil || len(idx.Manifests) == 0 {
		return nil, ociDescriptor{}, fmt.Errorf("index.json has no manifests")
	}
	desc := idx.Manifests[0]
	mb, err := os.ReadFile(ociBlobPath(layout, desc.Digest))
	if err != nil {
		return nil, ociDescriptor{}, fmt.Errorf("manifest blob: %w", err)
	}
	var man map[string]any
	if json.Unmarshal(mb, &man) != nil {
		return nil, ociDescriptor{}, fmt.Errorf("manifest blob is not JSON")
	}
	return man, desc, nil
}

// ociBlobPath maps a "sha256:<hex>" digest to its blob path, refusing
// anything that is not the expected digest form (the digest is
// attacker-controlled input from index.json).
func ociBlobPath(layout, digest string) string {
	const p = "sha256:"
	if !strings.HasPrefix(digest, p) {
		return ""
	}
	h := digest[len(p):]
	if len(h) != 64 || strings.ContainsAny(h, "/\\") {
		return ""
	}
	return filepath.Join(layout, "blobs", "sha256", h)
}

// ociLayers extracts the layer descriptors of an image manifest.
func ociLayers(man map[string]any) []ociDescriptor {
	var out []ociDescriptor
	raw, _ := man["layers"].([]any)
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, ociDescriptor{
				MediaType: strOf(m["mediaType"]),
				Digest:    strOf(m["digest"]),
				Size:      int64(numOf(m["size"])),
			})
		}
	}
	return out
}

func strOf(v any) string { s, _ := v.(string); return s }
func numOf(v any) float64 {
	f, _ := v.(float64)
	return f
}

// ociVerifyBlob checks a blob's sha256 before it is consumed — a
// corrupted or hostile layout must not hand back the wrong bytes.
func ociVerifyBlob(layout string, d ociDescriptor) ([]byte, error) {
	p := ociBlobPath(layout, d.Digest)
	if p == "" {
		return nil, fmt.Errorf("invalid digest %q", d.Digest)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	if "sha256:"+hex.EncodeToString(sum[:]) != d.Digest {
		return nil, fmt.Errorf("CHECKSUM_MISMATCH: blob %s", d.Digest)
	}
	return b, nil
}

// ociInspect prints the layout's refs, digests and layer info.
func ociInspect(layout string) int {
	man, desc, err := ociReadManifest(layout)
	if err != nil {
		return die("OCI_BAD", err.Error())
	}
	layers := ociLayers(man)
	var lj []map[string]any
	for _, l := range layers {
		lj = append(lj, map[string]any{
			"mediaType": l.MediaType, "digest": l.Digest, "size": l.Size})
	}
	d, _ := json.Marshal(map[string]any{
		"layout": layout, "digest": desc.Digest,
		"ref":    desc.Annots[ociRefAnnotKey],
		"layers": lj, "config": man["config"]})
	return ok(string(d))
}

// ociPull materialises the layer blob into <dest>/<ref>.quickvm.tar.gz
// after digest verification; --import then unpacks it through the
// hardened import path.
func ociPull(layout, dest, ref string, doImport, force bool) int {
	if ref == "" {
		// Resolve the single annotated ref when not given.
		_, desc, err := ociReadManifest(layout)
		if err != nil {
			return die("OCI_BAD", err.Error())
		}
		ref = desc.Annots[ociRefAnnotKey]
		if ref == "" {
			ref = "quickvm"
		}
	}
	man, _, err := ociReadManifest(layout)
	if err != nil {
		return die("OCI_BAD", err.Error())
	}
	layers := ociLayers(man)
	if len(layers) == 0 {
		return die("OCI_BAD", "manifest has no layers")
	}
	blob, err := ociVerifyBlob(layout, layers[0])
	if err != nil {
		return die("OCI_BAD", err.Error())
	}
	base := ref
	if i := strings.IndexByte(base, ':'); i >= 0 {
		base = base[:i]
	}
	base = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, base)
	if base == "" || base == "." || base == ".." {
		return die("OCI_BAD_REF", "ref produces an unsafe file name")
	}
	out := filepath.Join(dest, base+".quickvm.tar.gz")
	if _, err := os.Stat(out); err == nil && !force {
		return die("OCI_EXISTS", out+" exists (--force to overwrite)")
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return die("OCI_WRITE", err.Error())
	}
	if err := os.WriteFile(out, blob, 0o644); err != nil {
		return die("OCI_WRITE", err.Error())
	}
	if !doImport {
		d, _ := json.Marshal(map[string]any{
			"pulled": ref, "archive": out, "bytes": len(blob)})
		return ok(string(d))
	}
	exn, err := runExtraction(out, dest, true, force)
	if err != nil {
		if qe, ok := err.(*qeErr); ok {
			return die(qe.code, qe.msg)
		}
		return die("EXTRACT_FAIL", err.Error())
	}
	d, _ := json.Marshal(map[string]any{
		"pulled": ref, "import": exn.scan.vmName, "dest": dest})
	return ok(string(d))
}
