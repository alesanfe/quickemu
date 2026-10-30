// ops_snapshot.go — internal qemu-img snapshot ops extracted from
// run()'s op switch (snapshot-create/-apply/-delete share one body).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runSnapshotOp(op, dir string, args []string) int {
	// qemu-img snapshot -c/-a/-d <tag> <disk> — same as vm.sh,
	// including the headline echo printed before the tag check.
	disk0 := confDisk(dir)
	fmt.Println(map[string]string{
		"snapshot-create": "Snapshotting", "snapshot-apply": "Snapshot apply to",
		"snapshot-delete": "Snapshot removal"}[op] + " " + disk0)
	if len(args) < 3 {
		die("INVALID_ARGS", "snapshot op needs a tag")
	}
	tag := args[2]
	// The tag lands in `qemu-img snapshot <flag> <tag>` option
	// position — same allowlist as the external path.
	if !snapTagOk(tag) {
		die("INVALID_ARGS", "snapshot tag must match [A-Za-z0-9._-]+ and not start with '-'")
	}
	// vm_running guard: qemu-img snapshot on a live disk corrupts
	// it — liveness+identity, not a bare pidfile read.
	sname := filepath.Base(dir)
	if c := vmConfPath(dir); c != "" {
		sname = strings.TrimSuffix(filepath.Base(c), ".conf")
	}
	if vmRunningDir(dir, sname) {
		fmt.Fprintln(os.Stderr, " - ERROR! Stop "+sname+" before managing snapshots.")
		return 1
	}
	// --external/-external [desc]: qcow2 overlay chain instead of
	// an internal qemu-img snapshot (snapshot_create_external).
	for _, a := range args[3:] {
		if a == "--external" || a == "-external" {
			if op == "snapshot-create" {
				desc := ""
				for j, b := range args[3:] {
					if b != "--external" && b != "-external" && (j == 0 || args[3+j-1] == "--external" || args[3+j-1] == "-external") {
						desc = b
						break
					}
				}
				return snapshotCreateExternal(dir, tag, desc, vmConfPath(dir))
			}
			die("INVALID_ARGS", "--external only applies to snapshot-create")
		}
	}
	disk := confDisk(dir)
	// snapshot_tag_exists gates: create refuses a used tag,
	// delete refuses a missing one (qe_die SNAPSHOT_CONFLICT /
	// NOT_FOUND parity).
	switch op {
	case "snapshot-create":
		if snapTagExists(disk, tag) {
			die("SNAPSHOT_CONFLICT", "snapshot tag '"+tag+"' already exists in "+disk)
		}
	case "snapshot-delete":
		if !snapTagExists(disk, tag) {
			die("NOT_FOUND", "snapshot '"+tag+"' not found in "+disk)
		}
	}
	if _, err := os.Stat(disk); err != nil || disk == "" {
		fmt.Println(" - NOTE! " + disk + " not found. Doing nothing.")
		return 0
	}
	flag := map[string]string{"snapshot-create": "-c", "snapshot-apply": "-a", "snapshot-delete": "-d"}[op]
	cmd := execCommand("qemu-img", "snapshot", "-q", flag, tag, disk)
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, " - ERROR! Failed to %s snapshot '%s' %s %s\n",
			map[string]string{"snapshot-create": "create", "snapshot-apply": "apply",
				"snapshot-delete": "delete"}[op],
			tag, map[string]string{"snapshot-create": "for", "snapshot-apply": "to",
				"snapshot-delete": "from"}[op], disk)
		return 1
	}
	fmt.Printf(" - %s snapshot '%s' %s %s\n",
		map[string]string{"snapshot-create": "Created", "snapshot-apply": "Applied",
			"snapshot-delete": "Deleted"}[op],
		tag, map[string]string{"snapshot-create": "for", "snapshot-apply": "to",
			"snapshot-delete": "from"}[op], disk)
	okMap(map[string]any{"snapshot": tag, "op": op})
	return 0
}
