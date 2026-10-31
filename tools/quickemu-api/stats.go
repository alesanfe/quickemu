// stats.go — QE-083 structured stats: per-VM /proc sampling (rss,
// cpu, io, uptime), QMP query-stats for guest-visible netdev counters
// and host-side mem/load/uptime inventory. All procfs access goes
// through procDir so tests point it at fixtures.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// procDir / procHz are the procfs seams: tests point procDir at a
// fixture tree; procHz is USER_HZ (jiffies/sec) — 100 on every Linux
// quickemu hosts on, overridable for fixtures.
var (
	procDir = "/proc"
	procHz  = int64(100)
)

// procSample is one reading of a QEMU process' counters.
type procSample struct {
	OK      bool
	State   string
	Utime   int64 // jiffies
	Stime   int64 // jiffies
	StartJ  int64 // jiffies since boot
	Threads int64
	RssB    int64 // bytes
	ReadB   int64 // /proc/<pid>/io read_bytes
	WriteB  int64 // write_bytes
}

// readProc samples /proc/<pid>/stat (+io) for pid. Field layout after
// the closing ')' of comm: [0]state … [11]utime [12]stime …
// [17]num_threads [19]starttime [21]rss(pages).
func readProc(pid int) procSample {
	var s procSample
	b, err := os.ReadFile(filepath.Join(procDir, fmt.Sprint(pid), "stat"))
	if err != nil {
		return s
	}
	txt := string(b)
	i := strings.LastIndexByte(txt, ')')
	if i < 0 || i+2 >= len(txt) {
		return s
	}
	f := strings.Fields(txt[i+2:])
	if len(f) < 22 {
		return s
	}
	s.OK = true
	s.State = f[0]
	s.Utime, _ = strconv.ParseInt(f[11], 10, 64)
	s.Stime, _ = strconv.ParseInt(f[12], 10, 64)
	s.Threads, _ = strconv.ParseInt(f[17], 10, 64)
	s.StartJ, _ = strconv.ParseInt(f[19], 10, 64)
	pages, _ := strconv.ParseInt(f[21], 10, 64)
	if pages > 0 {
		s.RssB = pages * int64(os.Getpagesize())
	}
	if io, err := os.ReadFile(filepath.Join(procDir, fmt.Sprint(pid), "io")); err == nil {
		for _, l := range strings.Split(string(io), "\n") {
			if v, ok := strings.CutPrefix(l, "read_bytes:"); ok {
				s.ReadB, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			}
			if v, ok := strings.CutPrefix(l, "write_bytes:"); ok {
				s.WriteB, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			}
		}
	}
	return s
}

// procUptimeS: host uptime seconds from /proc/uptime (0 unreadable).
func procUptimeS() float64 {
	b, err := os.ReadFile(filepath.Join(procDir, "uptime"))
	if err != nil {
		return 0
	}
	var up float64
	fmt.Sscanf(string(b), "%f", &up)
	return up
}

// uptimeFor: how long pid has been alive, derived from its starttime
// jiffies + host uptime (both in the same clock domain).
func uptimeFor(s procSample) float64 {
	up := procUptimeS()
	if up <= 0 || !s.OK || s.StartJ <= 0 {
		return 0
	}
	u := up - float64(s.StartJ)/float64(procHz)
	if u < 0 {
		return 0
	}
	return u
}

// cpuPct computes percent CPU between two samples over wall seconds —
// top-style (may exceed 100 on multi-vcpu guests). When only one
// sample exists, d==0 falls back to the lifetime average.
func cpuPct(a, b procSample, wallS float64) float64 {
	if !b.OK {
		return 0
	}
	dj := float64(b.Utime + b.Stime)
	if a.OK {
		dj -= float64(a.Utime + a.Stime)
		if dj < 0 {
			dj = 0
		}
	}
	var denom float64
	if a.OK && wallS > 0 {
		denom = wallS
	} else if up := uptimeFor(b); up > 0 {
		denom = up
	}
	if denom <= 0 {
		return 0
	}
	return dj / float64(procHz) / denom * 100
}

// qmpQueryStats asks quickemu-qmp for query-stats on the VM's socket
// and aggregates the netdev provider counters (guest-visible traffic).
// ok=false when the channel or tool is unavailable — stats degrades,
// never fails the request.
func qmpQueryStats(sock string) (map[string]any, bool) {
	if st, err := os.Stat(sock); err != nil || st.Mode()&os.ModeSocket == 0 {
		return nil, false
	}
	payload, _ := json.Marshal(map[string]any{"execute": "query-stats"})
	out, err := toolT("quickemu-qmp", 8*time.Second, nil,
		"execute", "--socket", sock, "--command", string(payload))
	if err != nil {
		return nil, false
	}
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &env) != nil ||
		!env.Success || len(env.Data) == 0 {
		return nil, false
	}
	var providers []struct {
		Provider string             `json:"provider"`
		Stats    map[string]float64 `json:"stats"`
	}
	if json.Unmarshal(env.Data, &providers) != nil {
		return nil, false
	}
	rxB, txB, rxP, txP := 0.0, 0.0, 0.0, 0.0
	ifaces := 0
	for _, p := range providers {
		if p.Provider != "netdev" {
			continue
		}
		ifaces++
		for k, v := range p.Stats {
			switch k {
			case "rx-bytes":
				rxB += v
			case "tx-bytes":
				txB += v
			case "rx-packets":
				rxP += v
			case "tx-packets":
				txP += v
			}
		}
	}
	return map[string]any{
		"rx_bytes": int64(rxB), "tx_bytes": int64(txB),
		"rx_packets": int64(rxP), "tx_packets": int64(txP),
		"interfaces": ifaces,
	}, true
}

// vmStats gathers the QE-083 shape for one VM.
func vmStats(dir, name string, ev vmEvidence, prev *procSample, prevAt time.Time) map[string]any {
	out := map[string]any{
		"vm": name, "state": ev.State, "pid": ev.Pid, "alive": ev.Alive,
		"qmp": false, "qga": false,
		"cpu_pct": 0, "rss_mb": 0, "threads": 0,
		"disk_rd_b": 0, "disk_wr_b": 0, "uptime_s": 0,
		"net": nil,
	}
	if st, err := os.Stat(filepath.Join(dir, name+"-agent.sock")); err == nil &&
		st.Mode()&os.ModeSocket == 0 {
		out["qga"] = true
	}
	if !ev.Alive || ev.Pid <= 0 {
		return out
	}
	s := readProc(ev.Pid)
	if !s.OK {
		return out
	}
	var wall float64
	if prev != nil && prev.OK {
		wall = time.Since(prevAt).Seconds()
	}
	out["cpu_pct"] = math2(cpuPct(*prevOrZero(prev), s, wall))
	out["rss_mb"] = math2(float64(s.RssB) / (1 << 20))
	out["threads"] = s.Threads
	out["disk_rd_b"] = s.ReadB
	out["disk_wr_b"] = s.WriteB
	out["uptime_s"] = int64(uptimeFor(s))
	if net, ok := qmpQueryStats(filepath.Join(dir, name+"-qmp.socket")); ok {
		out["net"] = net
		out["qmp"] = true
	}
	return out
}

func prevOrZero(p *procSample) *procSample {
	if p == nil {
		return &procSample{}
	}
	return p
}

// math2 rounds to 2 decimals for stable API output.
func math2(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return float64(int64(v*100+0.5)) / 100
}

// memInfo: total/available kB from /proc/meminfo (Linux); available
// falls back to the gopsutil-backed qelib helper on hosts without
// procfs so macOS still reports a real number.
func memInfo() (totalKB, availKB int64) {
	b, err := os.ReadFile(filepath.Join(procDir, "meminfo"))
	if err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			switch {
			case strings.HasPrefix(l, "MemTotal:"):
				fmt.Sscanf(l, "MemTotal: %d kB", &totalKB)
			case strings.HasPrefix(l, "MemAvailable:"):
				fmt.Sscanf(l, "MemAvailable: %d kB", &availKB)
			}
		}
	}
	if availKB <= 0 {
		if mb := qelibHostFreeMemMB(); mb > 0 {
			availKB = int64(mb) * 1024
		}
	}
	return
}

// qelibHostFreeMemMB is a seam over qelib.HostFreeMemMB so tests can
// pin the fallback path.
var qelibHostFreeMemMB = func() int { return qelib.HostFreeMemMB() }

// loadAvg parses /proc/loadavg (empty on non-Linux hosts).
func loadAvg() (float64, float64, float64) {
	b, err := os.ReadFile(filepath.Join(procDir, "loadavg"))
	if err != nil {
		return 0, 0, 0
	}
	var l1, l5, l15 float64
	fmt.Sscanf(string(b), "%f %f %f", &l1, &l5, &l15)
	return l1, l5, l15
}

// hostStats: /v1/host/stats payload.
func hostStats() map[string]any {
	tot, avail := memInfo()
	l1, l5, l15 := loadAvg()
	running := 0
	for _, e := range collectVMs() {
		ev := probeVM(e.Dir, e.Name, e.Recorded)
		if ev.Alive {
			running++
		}
	}
	freeKB, freeOK := statfsAvailKB(dataDir)
	usedPct := 0.0
	if tot > 0 {
		usedPct = math2(float64(tot-avail) / float64(tot) * 100)
	}
	out := map[string]any{
		"cpu_cores":     runtime.NumCPU(),
		"mem_total_mb":  tot / 1024,
		"mem_avail_mb":  avail / 1024,
		"mem_used_pct":  usedPct,
		"load1":         l1, "load5": l5, "load15": l15,
		"uptime_s":      int64(procUptimeS()),
		"vms_total":     len(collectVMs()),
		"vms_running":   running,
		"data_dir_free_kb": freeKB,
		"data_dir_free_ok": freeOK,
	}
	return out
}
