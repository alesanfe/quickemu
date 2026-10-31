// journal.go: the import recovery journal (QE-090). An import writes
// <vm>.import.journal before staging anything, flips it to
// "installing" once the staged tree is verified, and deletes it after
// the final renames. A crash mid-import leaves the journal behind —
// `quickemu-archive recover` either completes a fully-staged import
// or cleans a partial one, so no half-written VM ever sits in dest.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type journal struct {
	Version  int      `json:"journal_version"`
	VM       string   `json:"vm,omitempty"`
	DestDir  string   `json:"dest_dir"`
	State    string   `json:"state"` // extracting | installing
	Staging  string   `json:"staging"`
	ConfTmp  string   `json:"conf_tmp,omitempty"`
	ConfDest string   `json:"conf_dest,omitempty"`
	Members  []string `json:"members"`           // installed-relative paths
	Entries  []string `json:"entries,omitempty"` // generic-extract top-level entries
	Force    bool     `json:"force,omitempty"`
	Created  string   `json:"created"`
}

const (
	jStateExtracting = "extracting"
	jStateInstalling = "installing"
)

// writeJournal persists j atomically (tmp+rename) at journalPath.
func writeJournal(journalPath string, j *journal) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	tmp := journalPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, journalPath)
}

func loadJournal(p string) (*journal, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var j journal
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// installStaged performs the journalised final step: every staged
// entry is renamed into the destination. With force, an existing
// destination entry is removed first; without it the install refuses
// to clobber. The caller has already flipped the journal to
// "installing".
func installStaged(j *journal) error {
	if j.VM != "" {
		// VM import: staging becomes <dest>/<vm>, conf_tmp becomes
		// <dest>/<vm>.conf — two atomic renames, journal covers the gap.
		dst := filepath.Join(j.DestDir, j.VM)
		if err := installOne(j.Staging, dst, j.Force); err != nil {
			return err
		}
		if j.ConfTmp != "" && j.ConfDest != "" {
			if err := installOne(j.ConfTmp, filepath.Join(j.DestDir, j.ConfDest), j.Force); err != nil {
				return err
			}
		}
		return nil
	}
	// Generic extract: every top-level staged entry installs into dest.
	ents, err := os.ReadDir(j.Staging)
	if err != nil {
		return err
	}
	for _, e := range ents {
		src := filepath.Join(j.Staging, e.Name())
		dst := filepath.Join(j.DestDir, e.Name())
		if err := installOne(src, dst, j.Force); err != nil {
			return err
		}
	}
	return nil
}

func installOne(src, dst string, force bool) error {
	if _, err := os.Lstat(dst); err == nil {
		if !force {
			return fmt.Errorf("%s exists (use --force to overwrite)", dst)
		}
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	return os.Rename(src, dst)
}

// stagedComplete reports whether the journal's recorded member set is
// fully present in the staging dir — the precondition for completing
// an interrupted install rather than cleaning it up.
func stagedComplete(j *journal) bool {
	if j.VM != "" {
		if st, err := os.Stat(j.Staging); err != nil || !st.IsDir() {
			return false
		}
		for _, m := range j.Members {
			if _, err := os.Lstat(filepath.Join(j.Staging, filepath.FromSlash(m))); err != nil {
				return false
			}
		}
		if j.ConfTmp != "" {
			if _, err := os.Lstat(j.ConfTmp); err != nil {
				return false
			}
		}
		return true
	}
	st, err := os.Stat(j.Staging)
	return err == nil && st.IsDir()
}

// recoverDest scans dest for import journals and orphan staging dirs.
// Journal in "installing" with a complete staging tree → finish the
// renames; otherwise everything staged is removed. Orphan
// *.import.tmp/.qe-extract-*.tmp dirs with no journal are removed.
func recoverDest(dest string) ([]map[string]string, error) {
	var actions []map[string]string
	act := func(what, p string) {
		actions = append(actions, map[string]string{"action": what, "path": p})
	}
	ents, err := os.ReadDir(dest)
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".import.journal") && !strings.HasSuffix(name, ".qe-extract.journal") {
			continue
		}
		jp := filepath.Join(dest, name)
		j, err := loadJournal(jp)
		if err != nil {
			os.Remove(jp)
			act("removed-corrupt-journal", name)
			continue
		}
		if j.State == jStateInstalling && stagedComplete(j) {
			if err := installStaged(j); err != nil {
				// Cannot finish — clean rather than leave a half VM.
				os.RemoveAll(j.Staging)
				if j.ConfTmp != "" {
					os.Remove(j.ConfTmp)
				}
				os.Remove(jp)
				act("cleaned-incomplete", name)
				continue
			}
			os.Remove(jp)
			act("completed", name)
			continue
		}
		os.RemoveAll(j.Staging)
		if j.ConfTmp != "" {
			os.Remove(j.ConfTmp)
		}
		os.Remove(jp)
		act("cleaned-partial", name)
	}
	// Orphan staging dirs (journal already gone or never written).
	ents, err = os.ReadDir(dest)
	if err != nil {
		return actions, nil
	}
	for _, e := range ents {
		name := e.Name()
		orphan := strings.HasSuffix(name, ".import.tmp") ||
			strings.HasSuffix(name, ".import.conf") ||
			strings.HasSuffix(name, ".journal.tmp") ||
			(strings.HasPrefix(name, ".qe-extract-") && strings.HasSuffix(name, ".tmp"))
		if !orphan || strings.HasSuffix(name, ".journal") {
			continue
		}
		os.RemoveAll(filepath.Join(dest, name))
		act("removed-orphan", name)
	}
	return actions, nil
}
