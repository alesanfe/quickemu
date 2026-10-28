package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// snapshotInfo: qemu-img info on the disk.
func snapshotInfo(dir string) int {
	disk := confDisk(dir)
	if disk == "" {
		die("NO_DISK", "no disk_img in "+dir)
	}
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		die("NO_QEMU_IMG", "qemu-img not found")
	}
	cmd := execCommand(qi, "info", disk)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return exitCode(cmd.Run())
}

// snapshotListJSON: qemu-img info --output=json snapshot table
// (schemas/v1/vm-snapshots.schema.json shape).
func snapshotListJSON(dir, vmName string) int {
	disk := confDisk(dir)
	snaps := "[]"
	if disk != "" {
		if qi, err := exec.LookPath("qemu-img"); err == nil {
			out, err := execCommand(qi, "info", "--output=json", disk).Output()
			if err == nil {
				var j struct {
					Snapshots []map[string]any `json:"snapshots"`
				}
				if json.Unmarshal(out, &j) == nil && j.Snapshots != nil {
					var norm []map[string]any
					for _, s := range j.Snapshots {
						norm = append(norm, map[string]any{
							"id":            s["id"],
							"name":          s["name"],
							"date":          num(s["date-sec"]) + num(s["date-nsec"])/1e9,
							"vm-state-size": num(s["vm-state-size"]),
						})
					}
					b, _ := json.Marshal(norm)
					snaps = string(b)
				}
			}
		}
	}
	okMap(map[string]any{"vm": vmName, "snapshots": json.RawMessage(snaps)})
	return 0
}

type snapMeta struct {
	ID        string `json:"id"`
	Tag       string `json:"tag"`
	File      string `json:"file"`
	Parent    string `json:"parent"`
	Desc      string `json:"desc"`
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
}

func snapMetaFile(dir string) string { return filepath.Join(dir, "snapshots.json") }

func snapMetaRead(dir string) []snapMeta {
	b, err := os.ReadFile(snapMetaFile(dir))
	if err != nil {
		return nil
	}
	var arr []snapMeta
	if err := json.Unmarshal(b, &arr); err != nil {
		// Corrupt metadata must not silently reset the snapshot
		// registry — preserve the bytes for manual recovery.
		os.WriteFile(snapMetaFile(dir)+".corrupt", b, 0o600)
		fmt.Fprintf(os.Stderr, "WARNING: snapshots.json corrupt (%v) — saved to .corrupt\n", err)
		return nil
	}
	return arr
}

func snapMetaWrite(dir string, arr []snapMeta) {
	tmp := snapMetaFile(dir) + ".tmp"
	if arr == nil {
		arr = []snapMeta{}
	}
	b, _ := json.Marshal(arr)
	if os.WriteFile(tmp, append(b, '\n'), 0644) == nil {
		os.Rename(tmp, snapMetaFile(dir))
	}
}

func snapshotCreateExternal(dir, tag, desc, confPath string) int {
	if tag == "" {
		fmt.Fprintln(os.Stderr, "ERROR! No snapshot tag.")
		return 1
	}
	disk := confDisk(dir)
	if _, err := os.Stat(disk); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", disk)
		return 1
	}
	for _, s := range snapMetaRead(dir) {
		if s.Tag == tag {
			die("SNAPSHOT_CONFLICT", "external snapshot '"+tag+"' already exists")
		}
	}
	name := filepath.Base(dir)
	if c := vmConfPath(dir); c != "" {
		name = strings.TrimSuffix(filepath.Base(c), ".conf")
	}
	// tag lands in the overlay filename AND inside the conf's
	// disk_img="..." — allowlist a filename-safe charset.
	if !snapTagRe.MatchString(tag) {
		die("INVALID_ARGS", "snapshot tag must match [A-Za-z0-9._-]+")
	}
	overlay := filepath.Join(dir, name+"-"+tag+".qcow2")
	if _, err := os.Stat(overlay); err == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s already exists.\n", overlay)
		return 1
	}
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		die("NO_QEMU_IMG", "qemu-img not found")
	}
	fmt_ := "qcow2"
	if f := confGet(dir, "disk_format"); f != "" {
		fmt_ = f
	}
	if execCommand(qi, "create", "-f", "qcow2", "-b", disk, "-F", fmt_, overlay).Run() != nil {
		fmt.Fprintf(os.Stderr, "ERROR! qemu-img failed to create %s.\n", overlay)
		return 1
	}
	// Repoint the conf's disk_img at the overlay. Bash replaces the raw
	// conf value (often a relative path), not the resolved absolute one.
	if confPath != "" {
		if raw := confGet(dir, "disk_img"); raw != "" {
			if b, err := os.ReadFile(confPath); err == nil {
				old := `disk_img="` + raw + `"`
				if strings.Contains(string(b), old) {
					nc := strings.Replace(string(b), old, `disk_img="`+overlay+`"`, 1)
					if err := os.WriteFile(confPath, []byte(nc), 0644); err != nil {
						fmt.Fprintf(os.Stderr, "WARNING: conf repoint failed: %v — disk_img still points at the parent\n", err)
					}
				}
			}
		}
	}
	arr := snapMetaRead(dir)
	arr = append(arr, snapMeta{
		ID:        fmt.Sprintf("snap-%s-%d", time.Now().UTC().Format("20060102150405"), os.Getpid()),
		Tag:       tag,
		File:      overlay,
		Parent:    disk,
		Desc:      desc,
		Type:      "external",
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	})
	snapMetaWrite(dir, arr)
	infof(" - External snapshot '%s' -> %s (parent %s)\n", tag, overlay, disk)
	okMap(map[string]any{"snapshot": tag, "file": overlay, "parent": disk})
	return 0
}

// snapshotTree: render the overlay chain from metadata.
func snapshotTree(dir string) int {
	arr := snapMetaRead(dir)
	disk := confDisk(dir)
	if outputJSON {
		b, _ := json.Marshal(arr)
		okMap(map[string]any{"active": disk, "snapshots": json.RawMessage(b)})
		return 0
	}
	name := filepath.Base(dir)
	if c := vmConfPath(dir); c != "" {
		name = strings.TrimSuffix(filepath.Base(c), ".conf")
	}
	infof("Snapshot chain for %s (active: %s):\n", name, disk)
	for _, s := range arr {
		infof(" - %s\n   %s <- %s\n", s.Tag, s.File, s.Parent)
	}
	if len(arr) == 0 {
		info(" - (no external snapshots)")
	}
	return 0
}

// snapshotVerify: every registered overlay must exist and its
// qemu-img backing must match the recorded parent.
func snapshotVerify(dir string) int {
	rc := 0
	for _, s := range snapMetaRead(dir) {
		if _, err := os.Stat(s.File); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR! Snapshot overlay %s missing.\n", s.File)
			rc = 1
			continue
		}
		if s.Parent != "" {
			out, err := execCommand("qemu-img", "info", s.File).Output()
			actual := ""
			if err == nil {
				for _, l := range strings.Split(string(out), "\n") {
					if strings.HasPrefix(l, "backing file: ") {
						actual = strings.TrimPrefix(l, "backing file: ")
					}
				}
			}
			if actual != s.Parent {
				fmt.Fprintf(os.Stderr, "ERROR! %s backs %s, expected %s.\n", s.File, actual, s.Parent)
				rc = 1
			}
		}
	}
	if rc == 0 {
		info(" - Snapshot chain verified.")
	}
	return rc
}

// snapshotConsolidate: commit a leaf overlay into its parent and
// remove the metadata node (snapshot_consolidate parity).
func snapshotConsolidate(dir, tag string) int {
	if tag == "" {
		fmt.Fprintln(os.Stderr, "ERROR! 'snapshot consolidate' needs a tag.")
		return 1
	}
	arr := snapMetaRead(dir)
	var node *snapMeta
	for i := range arr {
		if arr[i].Tag == tag {
			node = &arr[i]
		}
		if arr[i].Parent == tag {
			die("SNAPSHOT_CONFLICT", "'"+tag+"' has children — consolidate leaves only")
		}
	}
	if node == nil {
		die("SNAPSHOT_CONFLICT", "external snapshot '"+tag+"' not found")
	}
	// qemu-img commit on a live chain corrupts the backing file —
	// same guard as the internal snapshot ops.
	vmStoppedOrDie(dir, "consolidating snapshots")
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		die("NO_QEMU_IMG", "qemu-img not found")
	}
	if execCommand(qi, "commit", node.File).Run() != nil {
		die("CONSOLIDATE_FAILED", "qemu-img commit failed")
	}
	os.Remove(node.File)
	var rest []snapMeta
	for _, s := range arr {
		if s.Tag != tag {
			rest = append(rest, s)
		}
	}
	snapMetaWrite(dir, rest)
	infof(" - Consolidated '%s' into %s.\n", tag, node.Parent)
	okMap(map[string]any{"consolidated": tag, "into": node.Parent})
	return 0
}
