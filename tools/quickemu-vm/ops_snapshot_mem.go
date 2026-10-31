// ops_snapshot_mem.go — QE-021/022: in-disk snapshot registry
// entries (vmstate snapshots are created by the CLI via monitor
// savevm; quickemu-vm only records them) and the scheduled
// snapshot policy engine (snapshot_auto conf key).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------- QE-021: registry ops for monitor-created snapshots ----------

// snapshotRegister records a vmstate snapshot created through the
// live monitor (savevm) so tree/status/policy see its type. The
// File field stays empty — the state lives inside the disk image,
// not in an overlay file.
func snapshotRegister(dir, tag, typ, desc string) int {
	if !snapTagOk(tag) {
		die("INVALID_ARGS", "snapshot tag must match [A-Za-z0-9._-]+ and not start with '-'")
	}
	if typ != "vmstate" && typ != "disk" {
		die("INVALID_ARGS", "snapshot type must be vmstate|disk")
	}
	arr := snapMetaRead(dir)
	for _, s := range arr {
		if s.Tag == tag {
			die("SNAPSHOT_CONFLICT", "snapshot '"+tag+"' already registered")
		}
	}
	arr = append(arr, snapMeta{
		ID:        fmt.Sprintf("snap-%s-%d", time.Now().UTC().Format("20060102150405"), os.Getpid()),
		Tag:       tag,
		Desc:      desc,
		Type:      typ,
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	})
	snapMetaWrite(dir, arr)
	okMap(map[string]any{"registered": tag, "type": typ})
	return 0
}

// snapshotUnregister drops a registry entry — used after delvm.
// Unknown tags are a no-op (the monitor delete already removed the
// state; the sidecar may never have registered it).
func snapshotUnregister(dir, tag string) int {
	var rest []snapMeta
	for _, s := range snapMetaRead(dir) {
		if s.Tag != tag {
			rest = append(rest, s)
		}
	}
	snapMetaWrite(dir, rest)
	okMap(map[string]any{"unregistered": tag})
	return 0
}

// ---------- QE-022: scheduled snapshot policy ----------

// snapPeriod is one retention bucket: a cadence stamp format plus a
// max keep count. The stamp makes "is a new snapshot due" a pure
// string compare — two snaps in the same hour share a stamp.
type snapPeriod struct {
	Name  string // hourly|daily|weekly
	Keep  int    // how many auto-<period>-* snaps to retain
	Stamp string // time.Format layout producing the bucket key
}

// snapPeriods: the cadence ladder. Stamp layouts collide only
// within the same bucket: hourly truncates to the hour, daily to
// the day, weekly to ISO year-week.
var snapPeriods = map[string]snapPeriod{
	"hourly": {Name: "hourly", Keep: 4, Stamp: "2006010215"},
	"daily":  {Name: "daily", Keep: 7, Stamp: "20060102"},
	"weekly": {Name: "weekly", Keep: 2, Stamp: "2006W02"},
}

// snapAutoParse reads snapshot_auto="hourly:4,daily:7,weekly:2"
// (or "off"/""). A bare period name keeps its default retention.
func snapAutoParse(spec string) ([]snapPeriod, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "off" {
		return nil, nil
	}
	var out []snapPeriod
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		name, keep := tok, 0
		if i := strings.IndexByte(tok, ':'); i >= 0 {
			name = tok[:i]
			n, err := strconv.Atoi(tok[i+1:])
			if err != nil || n < 1 || n > 99 {
				return nil, fmt.Errorf("bad retention %q in snapshot_auto", tok[i+1:])
			}
			keep = n
		}
		p, ok := snapPeriods[name]
		if !ok {
			return nil, fmt.Errorf("unknown period %q in snapshot_auto (hourly|daily|weekly)", name)
		}
		if keep > 0 {
			p.Keep = keep
		}
		out = append(out, p)
	}
	return out, nil
}

// snapInternalList: `qemu-img snapshot -l --output=json` — the
// internal snapshot table of an offline disk.
func snapInternalList(disk string) []struct {
	Name   string `json:"name"`
	DateMs int64  `json:"date-sec"`
} {
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		return nil
	}
	out, err := execCommand(qi, "snapshot", "-l", "--output=json", disk).Output()
	if err != nil {
		return nil
	}
	var arr []struct {
		Name   string `json:"name"`
		DateMs int64  `json:"date-sec"`
	}
	if json.Unmarshal(out, &arr) != nil {
		return nil
	}
	return arr
}

// snapAutoName: auto-<period>-<stamp> — deterministic so "due" is
// name-level and re-runs inside one bucket are idempotent.
func snapAutoName(p snapPeriod, t time.Time) string {
	return "auto-" + p.Name + "-" + t.UTC().Format(p.Stamp)
}

// snapshotPolicy implements `snapshot-policy <dir> <verb> [--dry]`.
// Verbs: run (create due + prune), status (policy + counts), units
// (systemd user timer fragment).
func snapshotPolicy(dir, verb string, dry bool) int {
	disk := confDisk(dir)
	spec := confGet(dir, "snapshot_auto")
	periods, err := snapAutoParse(spec)
	if err != nil {
		die("INVALID_CONF", err.Error())
	}
	switch verb {
	case "status":
		return snapPolicyStatus(dir, spec, periods, disk)
	case "units":
		return snapPolicyUnits(dir)
	case "run":
		return snapPolicyRun(dir, periods, disk, dry)
	default:
		die("INVALID_ARGS", "snapshot-policy verb must be run|status|units")
	}
	return 1
}

func snapPolicyStatus(dir, spec string, periods []snapPeriod, disk string) int {
	type per struct {
		Period string `json:"period"`
		Keep   int    `json:"keep"`
		Have   int    `json:"have"`
		Latest string `json:"latest"`
	}
	var report []per
	counts := map[string][]string{}
	for _, s := range snapInternalList(disk) {
		for _, p := range periods {
			if strings.HasPrefix(s.Name, "auto-"+p.Name+"-") {
				counts[p.Name] = append(counts[p.Name], s.Name)
			}
		}
	}
	for _, p := range periods {
		names := counts[p.Name]
		sort.Strings(names)
		latest := ""
		if len(names) > 0 {
			latest = names[len(names)-1]
		}
		report = append(report, per{p.Name, p.Keep, len(names), latest})
	}
	if outputJSON {
		b, _ := json.Marshal(report)
		okMap(map[string]any{"policy": spec, "periods": json.RawMessage(b)})
		return 0
	}
	if len(periods) == 0 {
		infof("Snapshot policy: off (snapshot_auto=%q)\n", spec)
		return 0
	}
	infof("Snapshot policy: %s\n", spec)
	for _, r := range report {
		infof(" - %s: keep %d, have %d, latest %s\n", r.Period, r.Keep, r.Have, r.Latest)
	}
	return 0
}

// snapPolicyUnits emits a systemd --user timer+service pair that
// runs `quickemu --vm <conf> --snapshot-policy` on each cadence.
func snapPolicyUnits(dir string) int {
	conf := vmConfPath(dir)
	info(`# /etc/systemd/user/quickemu-snapshot.timer — enable with:
#   systemctl --user enable --now quickemu-snapshot.timer
[Unit]
Description=Quickemu scheduled snapshots

[Timer]
OnCalendar=hourly
Persistent=true

[Install]
WantedBy=timers.target

# /etc/systemd/user/quickemu-snapshot.service
[Unit]
Description=Quickemu snapshot policy run

[Service]
Type=oneshot
ExecStart=/usr/bin/quickemu --vm ` + conf + ` --snapshot-policy
`)
	return 0
}

// snapPolicyRun creates the due auto snapshots (qemu-img snapshot
// -c on the offline disk) and prunes buckets past their keep.
func snapPolicyRun(dir string, periods []snapPeriod, disk string, dry bool) int {
	if len(periods) == 0 {
		info("Snapshot policy: off — nothing to do.")
		okMap(map[string]any{"policy": "off", "created": 0, "pruned": 0})
		return 0
	}
	if disk == "" {
		die("NO_DISK", "no disk_img in "+dir)
	}
	if _, err := os.Stat(disk); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s not found.\n", disk)
		return 1
	}
	// qemu-img snapshot on a live image corrupts it — same guard as
	// the manual snapshot ops. Live machines use --memory instead.
	vmStoppedOrDie(dir, "running snapshot policy")
	now := time.Now().UTC()
	existing := map[string]bool{}
	var byPeriod = map[string][]string{}
	for _, s := range snapInternalList(disk) {
		existing[s.Name] = true
		for _, p := range periods {
			if strings.HasPrefix(s.Name, "auto-"+p.Name+"-") {
				byPeriod[p.Name] = append(byPeriod[p.Name], s.Name)
			}
		}
	}
	var create, del []string
	for _, p := range periods {
		want := snapAutoName(p, now)
		if !existing[want] {
			create = append(create, want)
		}
		names := append(byPeriod[p.Name], want)
		sort.Strings(names)
		for len(names) > p.Keep {
			del = append(del, names[0])
			names = names[1:]
		}
	}
	if dry {
		fmt.Printf(" - Plan: create %v, delete %v\n", create, del)
		okMap(map[string]any{"dry": true, "create": create, "delete": del})
		return 0
	}
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		die("NO_QEMU_IMG", "qemu-img not found")
	}
	rc, created, pruned := 0, 0, 0
	for _, tag := range create {
		if execCommand(qi, "snapshot", "-q", "-c", tag, disk).Run() != nil {
			fmt.Fprintf(os.Stderr, "ERROR! Failed to create snapshot '%s' for %s\n", tag, disk)
			rc = 1
			continue
		}
		fmt.Printf(" - Created scheduled snapshot '%s' for %s\n", tag, disk)
		created++
	}
	for _, tag := range del {
		if execCommand(qi, "snapshot", "-q", "-d", tag, disk).Run() != nil {
			fmt.Fprintf(os.Stderr, "ERROR! Failed to prune snapshot '%s' from %s\n", tag, disk)
			rc = 1
			continue
		}
		fmt.Printf(" - Pruned scheduled snapshot '%s' from %s\n", tag, disk)
		pruned++
	}
	cname := filepath.Base(dir)
	if c := vmConfPath(dir); c != "" {
		cname = strings.TrimSuffix(filepath.Base(c), ".conf")
	}
	vmEvent(dir, cname, "vm.snapshot-policy",
		map[string]any{"created": created, "pruned": pruned})
	okMap(map[string]any{"created": created, "pruned": pruned})
	return rc
}
