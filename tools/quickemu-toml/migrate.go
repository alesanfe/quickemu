// migrate.go: `quickemu config migrate` (QE-111) — .conf -> .toml
// with a .conf.bak backup, a unified diff shown before anything is
// written and explicit warnings for lines that cannot migrate. The
// .conf is only ever read as text — never executed.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/renameio/v2"
)

// MigrateResult is what a migrate plan produced — the caller prints
// Diff (before commit), then calls commit() to write.
type MigrateResult struct {
	Out      string   // .toml path written (or would be, for dry-run)
	Backup   string   // .conf.bak path
	Diff     string   // unified diff old .toml -> new .toml
	Warnings []string // non-migratable lines etc.
	Changed  bool     // new content differs from the existing .toml

	conf, toml string // source path and generated TOML text
	dryRun     bool
}

// tomlPathFor maps x.conf -> x.toml (any other extension is kept with
// .toml appended, matching the CLI's TrimSuffix behaviour).
func tomlPathFor(confPath string) string {
	if strings.HasSuffix(confPath, ".conf") {
		return strings.TrimSuffix(confPath, ".conf") + ".toml"
	}
	return confPath + ".toml"
}

// copyFile backups src to dst preserving mode.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, st.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// migrateConf renders TOML for confPath and computes the diff against
// the existing .toml — the caller prints res.Diff BEFORE calling
// commit() so the diff is always shown before anything is written.
// With dryRun set, commit() is a no-op.
func migrateConf(confPath string, dryRun bool) (*MigrateResult, error) {
	src, err := os.ReadFile(confPath)
	if err != nil {
		return nil, err
	}
	doc := parseConf(src)
	res := &MigrateResult{
		Out:      tomlPathFor(confPath),
		Backup:   confPath + ".bak",
		Warnings: doc.warnings,
		toml:     confToTOML(doc),
		conf:     confPath,
		dryRun:   dryRun,
	}
	old, _ := os.ReadFile(res.Out)
	res.Diff = unifiedDiff(filepath.Base(res.Out), old, []byte(res.toml))
	res.Changed = string(old) != res.toml
	return res, nil
}

// commit writes the .conf.bak backup then atomically replaces the
// .toml (fsync + rename via renameio, existing permissions kept).
func (r *MigrateResult) commit() error {
	if r.dryRun {
		return nil
	}
	if err := copyFile(r.conf, r.Backup); err != nil {
		return fmt.Errorf("backup %s: %w", r.Backup, err)
	}
	return renameio.WriteFile(r.Out, []byte(r.toml), 0o644,
		renameio.WithExistingPermissions())
}
