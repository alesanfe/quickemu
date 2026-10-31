// tools/quickget-vm/overlay.go: qcow2 overlay disks on a shared,
// read-only base-image cache. The downloaded cloud image becomes an
// immutable base (mode 0444 — NEVER written again); each VM gets a
// thin qcow2 overlay (-b base -F qcow2). qemu-img does the format
// work; the chain is validated so a deleted/corrupt base or a
// re-written backing file fails loudly instead of corrupting the VM.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// baseCacheDir is where pristine base images live, shared between VMs:
// $QE_BASE_CACHE or XDG (~/.cache/quickemu/base). Var for tests.
var baseCacheDir = func() string {
	if p := os.Getenv("QE_BASE_CACHE"); p != "" {
		return p
	}
	return filepath.Join(qelib.XDGCache(), "quickemu", "base")
}

// qemuImg wraps `qemu-img` (LookPath + execCommand seam).
func qemuImg(args ...string) error {
	bin, err := qemuImgPath()
	if err != nil {
		return err
	}
	return execCommand(bin, args...).Run()
}

var qemuImgPath = func() (string, error) {
	return execLookPath("qemu-img")
}

// execLookPath is a seam so tests can drop qemu-img entirely.
var execLookPath = func(name string) (string, error) {
	return exec.LookPath(name)
}

// qemuImgJSON runs `qemu-img info --output=json` and decodes the image
// descriptor; nil info + nil error only when parsing fails soft.
func qemuImgJSON(path string) (map[string]any, error) {
	bin, err := qemuImgPath()
	if err != nil {
		return nil, err
	}
	out, err := execCommand(bin, "info", "--output=json", path).Output()
	if err != nil {
		return nil, fmt.Errorf("qemu-img info %s: %v", path, err)
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("qemu-img info %s: unreadable output", path)
	}
	return v, nil
}

// backingChain returns the chain entries from `qemu-img info
// --backing-chain --output=json` (array, overlay first, base last).
// Older qemu-img builds that reject the combination fall back to
// walking per-level descriptors via backing-filename.
func backingChain(overlay string) ([]map[string]any, error) {
	bin, err := qemuImgPath()
	if err != nil {
		return nil, err
	}
	out, err := execCommand(bin, "info", "--output=json", "--backing-chain", overlay).Output()
	if err == nil {
		var arr []map[string]any
		if json.Unmarshal(out, &arr) == nil && len(arr) > 0 {
			return arr, nil
		}
	}
	// Per-level walk: each descriptor's full-backing-filename leads to
	// the next level until the chain bottoms out.
	var chain []map[string]any
	cur := overlay
	for depth := 0; depth < 32; depth++ {
		info, err := qemuImgJSON(cur)
		if err != nil {
			return nil, err
		}
		chain = append(chain, info)
		next, _ := info["full-backing-filename"].(string)
		if next == "" {
			next, _ = info["backing-filename"].(string)
		}
		if next == "" {
			return chain, nil
		}
		if !filepath.IsAbs(next) {
			next = filepath.Join(filepath.Dir(cur), next)
		}
		cur = next
	}
	return nil, fmt.Errorf("%s: backing chain too deep (loop?)", overlay)
}

// checkBackingOverlay validates the qcow2 chain under overlay:
//   - every level parses as an image qemu-img accepts
//   - every level below the top exists, non-empty, and is READ-ONLY
//     (base images are chmod 0444 — a writable base is a contract break)
//   - no level reports a qcow2 corrupt flag
func checkBackingOverlay(overlay string) error {
	chain, err := backingChain(overlay)
	if err != nil {
		return err
	}
	for i, lv := range chain {
		name, _ := lv["filename"].(string)
		if name == "" {
			name = overlay
		}
		st, err := os.Stat(name)
		if err != nil {
			return fmt.Errorf("backing chain broken: %s missing", name)
		}
		if st.Size() == 0 {
			return fmt.Errorf("backing chain broken: %s is empty", name)
		}
		if i > 0 && st.Mode().Perm()&0o222 != 0 {
			return fmt.Errorf("base image %s is writable — overlays must back onto a read-only base", name)
		}
		if fs, _ := lv["format-specific"].(map[string]any); fs != nil {
			if data, _ := fs["data"].(map[string]any); data != nil {
				if c, _ := data["corrupt"].(bool); c {
					return fmt.Errorf("backing chain broken: %s reports a corrupt flag", name)
				}
			}
		}
	}
	return nil
}

// qcowMagic checks the QFI\xfb signature — a sanity gate before trusting
// qemu-img with a file that is supposed to be a qcow2 base.
func qcowMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	var hdr [4]byte
	_, err = f.Read(hdr[:])
	f.Close()
	return err == nil && hdr == [4]byte{'Q', 'F', 'I', 0xfb}
}

// installBase moves src into the base cache as name, makes it read-only
// and returns the base path. An existing base wins — a second download
// of the same (os, release, arch) is discarded, never overwriting a
// base other VMs may already overlay.
func installBase(src, name string) (string, error) {
	dir := baseCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Join(dir, name)
	if st, err := os.Stat(base); err == nil && st.Size() > 0 {
		if !qcowMagic(base) {
			return "", fmt.Errorf("cached base %s is not a qcow2 — refusing to overlay a corrupt image", base)
		}
		os.Remove(src)
		return base, nil
	}
	if err := qelib.MoveFile(src, base); err != nil {
		return "", err
	}
	// Read-only FOREVER: a backing file written after overlays attach
	// silently corrupts every VM on top of it.
	if err := os.Chmod(base, 0o444); err != nil {
		return "", fmt.Errorf("could not protect base image %s: %v", base, err)
	}
	return base, nil
}

// baseImageName builds the canonical cache name for an OS release.
func baseImageName(osName, release, arch string) string {
	if arch == "" {
		arch = hostArch()
	}
	return osName + "-" + release + "-" + arch + ".qcow2"
}

// makeOverlay installs src as (or reuses) the cached base and creates
// the VM's thin overlay at dst. The overlay's backing path is the
// absolute base path so the VM dir stays relocatable.
func makeOverlay(src, dst, baseName string) (base string, err error) {
	base, err = installBase(src, baseName)
	if err != nil {
		return "", err
	}
	if err := qemuImg("create", "-f", "qcow2", "-b", base, "-F", "qcow2", dst); err != nil {
		return "", fmt.Errorf("qemu-img create overlay: %v", err)
	}
	return base, nil
}

// rebaseOverlay swaps an overlay's backing file — explicit and safe:
// the current chain must validate first (rebase on a broken chain only
// deepens the damage) and the new base must be a real qcow2.
// -u (unsafe) is never used: qemu-img rewrites the backing pointer AND
// re-checks, so a mismatching base content is caught by the guest OS
// semantics, but at minimum we refuse to point at a non-qcow2 file.
func rebaseOverlay(overlay, newBase string) error {
	if !qcowMagic(newBase) {
		return fmt.Errorf("%s is not a qcow2 image", newBase)
	}
	st, err := os.Stat(newBase)
	if err != nil {
		return err
	}
	if st.Mode().Perm()&0o222 != 0 {
		return fmt.Errorf("new base %s must be read-only", newBase)
	}
	if err := checkBackingOverlay(overlay); err != nil {
		return err
	}
	abs, err := filepath.Abs(newBase)
	if err != nil {
		return err
	}
	return qemuImg("rebase", "-f", "qcow2", "-F", "qcow2", "-b", abs, overlay)
}

// flattenOverlay produces a standalone copy of overlay (backing chain
// committed into it) — the mandatory step before a VM export so the
// archive does not ship a dangling backing reference.
func flattenOverlay(overlay, out string) error {
	if err := checkBackingOverlay(overlay); err != nil {
		return err
	}
	return qemuImg("convert", "-O", "qcow2", overlay, out)
}

// cmdOverlay implements `quickget-vm overlay <vmdir> <src.qcow2>`:
// install the src image as a shared base (name from --base or the src
// filename) and create <vmdir>/disk.qcow2 on top of it.
func cmdOverlay(args []string) int {
	var vmPath, src, base string
	pos := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--base" && i+1 < len(args) {
			i++
			base = args[i]
			continue
		}
		if strings.HasPrefix(a, "--") {
			fmt.Println("overlay: unsupported argument " + a)
			return 2
		}
		pos = append(pos, a)
	}
	if len(pos) > 0 {
		vmPath = pos[0]
	}
	if len(pos) > 1 {
		src = pos[1]
	}
	if vmPath == "" || src == "" {
		fmt.Println("usage: quickget-vm overlay <vmdir> <src.qcow2> [--base <name>]")
		return 2
	}
	if !qcowMagic(src) {
		fmt.Println("overlay: " + src + " is not a qcow2 image")
		return 1
	}
	if base == "" {
		base = filepath.Base(src)
	}
	if err := os.MkdirAll(vmPath, 0o755); err != nil {
		fmt.Println(err)
		return 1
	}
	dst := filepath.Join(vmPath, "disk.qcow2")
	b, err := makeOverlay(src, dst, base)
	if err != nil {
		fmt.Println("overlay: " + err.Error())
		return 1
	}
	if err := checkBackingOverlay(dst); err != nil {
		fmt.Println("overlay: " + err.Error())
		return 1
	}
	fmt.Println(" - Overlay " + dst + " on base " + b)
	return 0
}

// cmdCheckBase implements `quickget-vm checkbase <overlay.qcow2>` —
// the start-of-VM validation hook: chain intact + every backing level
// read-only and non-corrupt.
func cmdCheckBase(args []string) int {
	if len(args) != 1 {
		fmt.Println("usage: quickget-vm checkbase <overlay.qcow2>")
		return 2
	}
	if err := checkBackingOverlay(args[0]); err != nil {
		fmt.Println("checkbase: " + err.Error())
		return 1
	}
	fmt.Println(" - Backing chain OK: " + args[0])
	return 0
}

// cmdRebase implements `quickget-vm rebase <overlay> <new-base.qcow2>`.
func cmdRebase(args []string) int {
	if len(args) != 2 {
		fmt.Println("usage: quickget-vm rebase <overlay.qcow2> <new-base.qcow2>")
		return 2
	}
	if err := rebaseOverlay(args[0], args[1]); err != nil {
		fmt.Println("rebase: " + err.Error())
		return 1
	}
	fmt.Println(" - Rebased " + args[0] + " onto " + args[1])
	return 0
}

// cmdFlatten implements `quickget-vm flatten <overlay> <out.qcow2>` —
// collapse the chain into a standalone image for export.
func cmdFlatten(args []string) int {
	if len(args) != 2 {
		fmt.Println("usage: quickget-vm flatten <overlay.qcow2> <out.qcow2>")
		return 2
	}
	if err := flattenOverlay(args[0], args[1]); err != nil {
		fmt.Println("flatten: " + err.Error())
		return 1
	}
	fmt.Println(" - Flattened " + args[0] + " -> " + args[1])
	return 0
}
