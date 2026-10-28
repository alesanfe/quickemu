package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func isRuntimeArtifact(name string) bool {
	for _, e := range runtimeExts {
		if strings.HasSuffix(name, e) || strings.Contains(name, ".lock") {
			return true
		}
	}
	return false
}

// exportVm: tar.gz of <name>.conf + <name>/ (runtime artifacts
// excluded) + quickvm-manifest.json + quickvm-checksums.sha256 —
// byte-layout parity with export_vm. The conf is the real file the
// user passed (QE_VMCONF), not <dir>.conf: a conf may live inside or
// beside the VM dir (Bash resolves it after its pushd into VMPATH).
func exportVm(dir, out string) int {
	name := filepath.Base(dir)
	parent := filepath.Dir(dir)
	conf := filepath.Join(parent, name+".conf")
	if qc := os.Getenv("QE_VMCONF"); qc != "" {
		if abs, err := filepath.Abs(qc); err == nil {
			qc = abs
		}
		if _, err := os.Stat(qc); err == nil {
			conf = qc
			parent = filepath.Dir(conf)
			name = strings.TrimSuffix(filepath.Base(conf), filepath.Ext(conf))
		}
	}
	if _, err := os.Stat(conf); err != nil {
		die("NO_CONF", "no "+conf)
	}
	// Member list: conf + every non-runtime file under the VM dir
	// (<parent>/<name>/ — Bash only archives the convention dir named
	// after the VM, present or not).
	var members []string
	members = append(members, name+".conf")
	vdir := filepath.Join(parent, name)
	if st, err := os.Stat(vdir); err == nil && st.IsDir() {
		filepath.WalkDir(vdir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(parent, p)
			if !isRuntimeArtifact(d.Name()) {
				members = append(members, filepath.ToSlash(rel))
			}
			return nil
		})
	}
	slices.Sort(members[1:])
	if out == "" {
		out = name + ".quickvm.tar.gz"
	}
	f, err := os.Create(out)
	if err != nil {
		die("EXPORT_FAILED", err.Error())
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	// Checksum file accumulated while streaming members.
	var sums strings.Builder
	writeFile := func(arcName, diskPath string) error {
		f, err := os.Open(diskPath)
		if err != nil {
			return err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return err
		}
		// Stream through the hasher — whole-file reads OOM on qcow2s.
		h := sha256.New()
		if err := tw.WriteHeader(&tar.Header{Name: arcName, Mode: 0644,
			Size: st.Size(), ModTime: st.ModTime()}); err != nil {
			return err
		}
		if _, err := io.Copy(io.MultiWriter(h, tw), f); err != nil {
			return err
		}
		sums.WriteString(fmt.Sprintf("%x  %s\n", h.Sum(nil), arcName))
		return nil
	}
	for _, m := range members {
		if err := writeFile(m, filepath.Join(parent, filepath.FromSlash(m))); err != nil {
			die("EXPORT_FAILED", err.Error())
		}
	}
	mb, _ := json.Marshal(map[string]any{
		"formatVersion": 1, "quickemuVersion": vmVersion(),
		"createdAt": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"vm":        name, "checksums": "quickvm-checksums.sha256"})
	manifest := string(mb) + "\n"
	tw.WriteHeader(&tar.Header{Name: "quickvm-manifest.json", Mode: 0644, Size: int64(len(manifest))})
	tw.Write([]byte(manifest))
	sumb := sums.String()
	tw.WriteHeader(&tar.Header{Name: "quickvm-checksums.sha256", Mode: 0644, Size: int64(len(sumb))})
	tw.Write([]byte(sumb))
	infof(" - Exported %s to %s\n", name, out)
	okMap(map[string]any{"export": out})
	return 0
}

func vmVersion() string { return qelib.QEVersion }

// importVm: safe extract of a .quickvm.tar.gz into the cwd —
// anti-bomb limits, path whitelist, manifest/checksum verification.
func importVm(in, destDir string) int {
	st, err := os.Stat(in)
	if err != nil || st.IsDir() {
		fmt.Fprintln(os.Stderr, "ERROR! '--import' needs an existing .tar.gz file.")
		return 1
	}
	f, err := os.Open(in)
	if err != nil {
		die("IMPORT_FAILED", err.Error())
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! Not a gzip tar.")
		return 1
	}
	// Pass 1: enumerate + validate members (same limits as Bash).
	tr := tar.NewReader(gz)
	var members []string
	base := ""
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		members = append(members, h.Name)
		if len(members) > 100000 {
			die("UNSAFE_ARCHIVE", fmt.Sprintf("Archive has >100000 members."))
		}
		if strings.HasPrefix(h.Name, "/") || h.Name == ".." ||
			strings.HasPrefix(h.Name, "../") || strings.Contains(h.Name, "/..") {
			die("UNSAFE_ARCHIVE", "Unsafe path in archive: '"+h.Name+"'")
		}
		if strings.Count(h.Name, "/") > 16 {
			die("UNSAFE_ARCHIVE", "Path too deep in archive: '"+h.Name+"'")
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
			die("UNSAFE_ARCHIVE", "Archive contains links or special files; refusing import.")
		}
		if base == "" && strings.HasSuffix(h.Name, ".conf") {
			base = strings.TrimSuffix(filepath.Base(h.Name), ".conf")
		}
	}
	f.Close()
	if base == "" {
		fmt.Fprintln(os.Stderr, "ERROR! No .conf found inside "+in+".")
		return 1
	}
	if base == "." || base == ".." || strings.ContainsAny(base, "/") || strings.HasPrefix(base, "-") {
		fmt.Fprintln(os.Stderr, "ERROR! Unsafe VM name '"+base+"' in archive.")
		return 1
	}
	if _, err := os.Stat(filepath.Join(destDir, base+".conf")); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", base)
		return 1
	}
	if _, err := os.Stat(filepath.Join(destDir, base)); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", base)
		return 1
	}
	// Pass 2: extract to a private staging dir.
	f2, _ := os.Open(in)
	defer f2.Close()
	gz2, _ := gzip.NewReader(f2)
	tr2 := tar.NewReader(gz2)
	tmp, err := os.MkdirTemp("", "quickemu-import.")
	if err != nil {
		die("IMPORT_FAILED", err.Error())
	}
	defer os.RemoveAll(tmp)
	for {
		h, err := tr2.Next()
		if err != nil {
			break
		}
		dest := filepath.Join(tmp, h.Name)
		if h.FileInfo().IsDir() {
			os.MkdirAll(dest, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(dest), 0755)
		w, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, h.FileInfo().Mode())
		if err != nil {
			fmt.Fprintln(os.Stderr, "ERROR! Failed to extract "+in+".")
			return 1
		}
		io.Copy(w, tr2)
		w.Close()
	}
	// Manifest + checksum verification.
	mf := filepath.Join(tmp, "quickvm-manifest.json")
	if b, err := os.ReadFile(mf); err == nil {
		var m struct {
			FormatVersion int    `json:"formatVersion"`
			VM            string `json:"vm"`
		}
		json.Unmarshal(b, &m)
		if m.FormatVersion > 1 {
			die("UNSAFE_ARCHIVE", fmt.Sprintf("Unsupported manifest formatVersion '%d'.", m.FormatVersion))
		}
		if m.VM != "" && m.VM != base {
			die("IMPORT_CONFLICT", fmt.Sprintf("Manifest vm '%s' does not match '%s'.", m.VM, base))
		}
		sf := filepath.Join(tmp, "quickvm-checksums.sha256")
		sb, err := os.ReadFile(sf)
		if err != nil {
			die("UNSAFE_ARCHIVE", "Manifest present but checksums missing.")
		}
		for _, l := range strings.Split(string(sb), "\n") {
			flds := strings.Fields(l)
			if len(flds) != 2 {
				continue
			}
			mb, err := os.ReadFile(filepath.Join(tmp, filepath.FromSlash(flds[1])))
			if err != nil || fmt.Sprintf("%x", sha256.Sum256(mb)) != flds[0] {
				die("CHECKSUM_MISMATCH", "Archive checksum verification failed.")
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, " - WARNING! %s has no manifest; integrity is not verified.\n", in)
	}
	// Install: move staged entries into destDir.
	ents, _ := os.ReadDir(tmp)
	for _, e := range ents {
		src := filepath.Join(tmp, e.Name())
		dst := filepath.Join(destDir, e.Name())
		if err := os.Rename(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! Failed to install %s.\n", e.Name())
			return 1
		}
	}
	infof(" - Imported %s from %s\n", base, in)
	fmt.Fprintf(os.Stderr, " - WARNING! %s.conf is executed as code on launch; review it before running the VM.\n", base)
	okMap(map[string]any{"import": base})
	return 0
}
