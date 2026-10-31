// stats_test.go — QE-083 stats coverage: /proc fixture sampling,
// cpu/uptime derivation, QMP query-stats aggregation, per-VM and
// host-side stat shapes, statfs probing.
package main

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// withProc points the procDir seam at a fixture tree.
func withProc(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := procDir
	procDir = dir
	t.Cleanup(func() { procDir = old })
	return dir
}

// writeProcStat drops a synthetic /proc/<pid>/stat. Field layout
// after ')': [0]state … [11]utime [12]stime [17]num_threads
// [19]starttime [21]rss(pages).
func writeProcStat(t *testing.T, proc string, pid int, body string) {
	t.Helper()
	d := filepath.Join(proc, strconv.Itoa(pid))
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "stat"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// statLine builds a stat record: utime/stime/threads/startJ/rss as
// given, state S, fillers elsewhere — 22 fields after ')'.
func statLine(pid int, utime, stime, threads, startJ, rss int64) string {
	f := []string{
		"S",                          // 0 state
		"1", "1", "1", "0", "0", "0", // 1-6
		"0", "0", "0", "0", // 7-10
		strconv.FormatInt(utime, 10), // 11 utime
		strconv.FormatInt(stime, 10), // 12 stime
		"0", "0",                     // 13-14
		"20", "0", // 15-16
		strconv.FormatInt(threads, 10), // 17 num_threads
		"0",                            // 18
		strconv.FormatInt(startJ, 10),  // 19 starttime
		"999",                          // 20 vsize
		strconv.FormatInt(rss, 10),     // 21 rss pages
	}
	return strconv.Itoa(pid) + " (qemu) " + strings.Join(f, " ") + "\n"
}

func TestReadProc(t *testing.T) {
	proc := withProc(t)
	writeProcStat(t, proc, 4242, statLine(4242, 300, 100, 7, 12345, 200))
	os.WriteFile(filepath.Join(proc, "4242", "io"),
		[]byte("rchar: 5\nwchar: 6\nread_bytes: 4096\nwrite_bytes: 8192\n"), 0o644)

	s := readProc(4242)
	if !s.OK || s.State != "S" || s.Utime != 300 || s.Stime != 100 ||
		s.Threads != 7 || s.StartJ != 12345 || s.ReadB != 4096 || s.WriteB != 8192 {
		t.Fatalf("readProc %+v", s)
	}
	if s.RssB != 200*int64(os.Getpagesize()) {
		t.Fatalf("rss %d", s.RssB)
	}
	// Missing stat → !OK.
	if s := readProc(9999); s.OK {
		t.Fatal("missing pid must give !OK")
	}
	// No ')' → !OK.
	writeProcStat(t, proc, 4243, "garbage-no-paren\n")
	if s := readProc(4243); s.OK {
		t.Fatal("no-paren stat must give !OK")
	}
	// Paren but too few fields → !OK.
	writeProcStat(t, proc, 4244, "4244 (x) S 1 2 3\n")
	if s := readProc(4244); s.OK {
		t.Fatal("short stat must give !OK")
	}
	// No io file → stat still parses, io counters stay zero.
	writeProcStat(t, proc, 4245, statLine(4245, 1, 2, 3, 4, 5))
	if s := readProc(4245); !s.OK || s.ReadB != 0 || s.WriteB != 0 {
		t.Fatalf("no-io %+v", s)
	}
}

func TestProcUptimeS(t *testing.T) {
	proc := withProc(t)
	os.WriteFile(filepath.Join(proc, "uptime"), []byte("200.50 100.25\n"), 0o644)
	if got := procUptimeS(); got != 200.50 {
		t.Fatalf("uptime %v", got)
	}
	// Missing → 0.
	proc2 := withProc(t)
	_ = proc2
	if got := procUptimeS(); got != 0 {
		t.Fatalf("missing uptime %v", got)
	}
}

func TestUptimeFor(t *testing.T) {
	proc := withProc(t)
	os.WriteFile(filepath.Join(proc, "uptime"), []byte("200.00 0\n"), 0o644)
	old := procHz
	procHz = 100
	t.Cleanup(func() { procHz = old })

	// startJ=12345 → 123.45s since boot → alive 76.55s.
	s := procSample{OK: true, StartJ: 12345}
	if got := uptimeFor(s); got <= 76 || got >= 77 {
		t.Fatalf("uptimeFor %v", got)
	}
	if got := uptimeFor(procSample{}); got != 0 {
		t.Fatalf("not-ok sample %v", got)
	}
	if got := uptimeFor(procSample{OK: true, StartJ: 0}); got != 0 {
		t.Fatalf("zero startJ %v", got)
	}
	// startJ beyond the uptime → clamped to 0.
	if got := uptimeFor(procSample{OK: true, StartJ: 99999999}); got != 0 {
		t.Fatalf("future start %v", got)
	}
	// Unreadable uptime → 0.
	os.Remove(filepath.Join(proc, "uptime"))
	if got := uptimeFor(procSample{OK: true, StartJ: 1}); got != 0 {
		t.Fatalf("no-uptime %v", got)
	}
}

func TestCPUPct(t *testing.T) {
	proc := withProc(t)
	os.WriteFile(filepath.Join(proc, "uptime"), []byte("1000.00 0\n"), 0o644)
	old := procHz
	procHz = 100
	t.Cleanup(func() { procHz = old })

	// b not OK → 0.
	if got := cpuPct(procSample{}, procSample{}, 1); got != 0 {
		t.Fatalf("dead b %v", got)
	}
	// Two samples over 10s wall: 300 jiffies → 3s cpu → 30%.
	a := procSample{OK: true, Utime: 50, Stime: 50}
	b := procSample{OK: true, Utime: 300, Stime: 100, StartJ: 50000}
	if got := cpuPct(a, b, 10); got != 30 {
		t.Fatalf("delta cpu %v", got)
	}
	// Counters moving backwards clamp to 0.
	a2 := procSample{OK: true, Utime: 9999}
	if got := cpuPct(a2, b, 10); got != 0 {
		t.Fatalf("backwards %v", got)
	}
	// One sample (a !OK): lifetime average — 400 jiffies = 4s cpu over
	// 1000-500 s uptime ≈ 0.8%.
	if got := cpuPct(procSample{}, b, 0); got <= 0.7 || got >= 0.9 {
		t.Fatalf("lifetime cpu %v", got)
	}
	// a OK but wall=0 → falls back to the lifetime denom.
	if got := cpuPct(a, b, 0); got <= 0 {
		t.Fatalf("wall-zero %v", got)
	}
	// No usable denom → 0.
	os.Remove(filepath.Join(proc, "uptime"))
	if got := cpuPct(procSample{}, procSample{OK: true}, 0); got != 0 {
		t.Fatalf("no-denom %v", got)
	}
}

// unixSock creates a bound unix socket file (mode check fodder).
func unixSock(t *testing.T, path string) net.Listener {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("unix sock: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

func TestQMPQueryStats(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "vm-qmp.socket")

	// No socket → degraded, not failed.
	if _, ok := qmpQueryStats(sock); ok {
		t.Fatal("missing socket must be not-ok")
	}
	// A non-socket file is rejected too.
	os.WriteFile(sock, []byte("x"), 0o644)
	if _, ok := qmpQueryStats(sock); ok {
		t.Fatal("non-socket must be not-ok")
	}
	os.Remove(sock)
	ln := unixSock(t, sock)
	_ = ln

	// No quickemu-qmp on PATH → not-ok.
	t.Setenv("PATH", t.TempDir())
	if _, ok := qmpQueryStats(sock); ok {
		t.Fatal("missing tool must be not-ok")
	}

	// Tool fails → not-ok.
	stubBin(t, "quickemu-qmp", "#!/bin/sh\nexit 1\n")
	if _, ok := qmpQueryStats(sock); ok {
		t.Fatal("failing tool must be not-ok")
	}
	// Bad JSON / unsuccessful envelope → not-ok.
	stubBin(t, "quickemu-qmp", "#!/bin/sh\necho garbage\n")
	if _, ok := qmpQueryStats(sock); ok {
		t.Fatal("garbage envelope must be not-ok")
	}
	stubBin(t, "quickemu-qmp", "#!/bin/sh\necho '{\"success\":false}'\n")
	if _, ok := qmpQueryStats(sock); ok {
		t.Fatal("unsuccessful envelope must be not-ok")
	}
	// Data that does not unmarshal into provider rows → not-ok.
	stubBin(t, "quickemu-qmp", "#!/bin/sh\necho '{\"success\":true,\"data\":{\"x\":1}}'\n")
	if _, ok := qmpQueryStats(sock); ok {
		t.Fatal("wrong data shape must be not-ok")
	}
	// Netdev providers aggregate; other providers are ignored.
	payload := `{"success":true,"data":[` +
		`{"provider":"netdev","stats":{"rx-bytes":1000,"tx-bytes":2000,"rx-packets":10,"tx-packets":20}},` +
		`{"provider":"netdev","stats":{"rx-bytes":5,"tx-bytes":5,"rx-packets":1,"tx-packets":1}},` +
		`{"provider":"kvm","stats":{"x":1}}` +
		`]}`
	stubBin(t, "quickemu-qmp", "#!/bin/sh\necho '"+payload+"'\n")
	net1, ok := qmpQueryStats(sock)
	if !ok {
		t.Fatal("netdev stats must be ok")
	}
	if net1["rx_bytes"].(int64) != 1005 || net1["tx_bytes"].(int64) != 2005 ||
		net1["rx_packets"].(int64) != 11 || net1["tx_packets"].(int64) != 21 ||
		net1["interfaces"].(int) != 2 {
		t.Fatalf("aggregate %v", net1)
	}
	// Only non-netdev providers → ok with zeroed counters.
	payload = `{"success":true,"data":[{"provider":"kvm","stats":{"x":1}}]}`
	stubBin(t, "quickemu-qmp", "#!/bin/sh\necho '"+payload+"'\n")
	net2, ok := qmpQueryStats(sock)
	if !ok || net2["interfaces"].(int) != 0 {
		t.Fatalf("non-netdev %v %v", net2, ok)
	}
}

func TestVMStats(t *testing.T) {
	dir := t.TempDir()
	proc := withProc(t)
	os.WriteFile(filepath.Join(proc, "uptime"), []byte("500.00 0\n"), 0o644)

	// Dead VM → base fields only.
	ev := vmEvidence{State: "stopped", Pid: 0, Alive: false}
	out := vmStats(dir, "vm", ev, nil, time.Now())
	if out["vm"] != "vm" || out["state"] != "stopped" || out["alive"] != false ||
		out["qmp"] != false || out["qga"] != false {
		t.Fatalf("dead vm %v", out)
	}

	// Live VM with a /proc fixture.
	writeProcStat(t, proc, 4242, statLine(4242, 300, 100, 7, 10000, 100))
	os.WriteFile(filepath.Join(proc, "4242", "io"),
		[]byte("read_bytes: 4096\nwrite_bytes: 8192\n"), 0o644)
	ev = vmEvidence{State: "running", Pid: 4242, Alive: true}
	out = vmStats(dir, "vm", ev, nil, time.Now())
	if out["state"] != "running" || out["threads"] != int64(7) ||
		out["disk_rd_b"] != int64(4096) || out["disk_wr_b"] != int64(8192) ||
		out["uptime_s"] != int64(400) {
		t.Fatalf("live vm %v", out)
	}
	if want := math2(float64(100*os.Getpagesize()) / (1 << 20)); out["rss_mb"] != want {
		t.Fatalf("rss %v want %v", out["rss_mb"], want)
	}

	// qga marker present → qga flag flips. The check treats a
	// non-socket agent file as present (unlike the qmp socket probe).
	os.WriteFile(filepath.Join(dir, "vm-agent.sock"), []byte("x"), 0o644)
	out = vmStats(dir, "vm", ev, nil, time.Now())
	if out["qga"] != true {
		t.Fatalf("qga %v", out)
	}
	// A real socket at the agent path is NOT counted by this check.
	os.Remove(filepath.Join(dir, "vm-agent.sock"))
	unixSock(t, filepath.Join(dir, "vm-agent.sock"))
	out = vmStats(dir, "vm", ev, nil, time.Now())
	if out["qga"] != false {
		t.Fatalf("socket-agent %v", out)
	}

	// qmp socket + stub tool → net counters + qmp flag.
	unixSock(t, filepath.Join(dir, "vm-qmp.socket"))
	payload := `{"success":true,"data":[{"provider":"netdev","stats":{"rx-bytes":1,"tx-bytes":2}}]}`
	stubBin(t, "quickemu-qmp", "#!/bin/sh\necho '"+payload+"'\n")
	out = vmStats(dir, "vm", ev, nil, time.Now())
	if out["qmp"] != true || out["net"] == nil {
		t.Fatalf("qmp %v", out)
	}

	// A previous sample makes cpu_pct a delta instead of lifetime.
	prev := &procSample{OK: true, Utime: 100, Stime: 50, StartJ: 10000}
	out = vmStats(dir, "vm", ev, prev, time.Now().Add(-10*time.Second))
	if out["cpu_pct"].(float64) <= 0 {
		t.Fatalf("prev-sample cpu %v", out)
	}

	// Live evidence but unreadable proc stat → early return.
	ev2 := vmEvidence{State: "running", Pid: 7777, Alive: true}
	out = vmStats(dir, "vm2", ev2, nil, time.Now())
	if out["cpu_pct"] != 0 || out["threads"] != 0 {
		t.Fatalf("unreadable-proc %v", out)
	}
}

func TestPrevOrZero(t *testing.T) {
	if p := prevOrZero(nil); p == nil || p.OK {
		t.Fatal("nil → zero sample")
	}
	s := &procSample{OK: true}
	if prevOrZero(s) != s {
		t.Fatal("non-nil must pass through")
	}
}

func TestMath2(t *testing.T) {
	for in, want := range map[float64]float64{
		-1: 0, 0: 0, -0.5: 0,
		1.006: 1.01, 2.567: 2.57, 10: 10, 0.004: 0,
	} {
		if got := math2(in); got != want {
			t.Errorf("math2(%v)=%v want %v", in, got, want)
		}
	}
}

func TestMemInfo(t *testing.T) {
	proc := withProc(t)
	os.WriteFile(filepath.Join(proc, "meminfo"),
		[]byte("MemTotal: 16384000 kB\nMemFree: 100 kB\nMemAvailable: 8192000 kB\n"), 0o644)
	tot, avail := memInfo()
	if tot != 16384000 || avail != 8192000 {
		t.Fatalf("memInfo %d %d", tot, avail)
	}
	// No MemAvailable → gopsutil seam fallback.
	os.WriteFile(filepath.Join(proc, "meminfo"), []byte("MemTotal: 100 kB\n"), 0o644)
	old := qelibHostFreeMemMB
	qelibHostFreeMemMB = func() int { return 512 }
	t.Cleanup(func() { qelibHostFreeMemMB = old })
	tot, avail = memInfo()
	if tot != 100 || avail != 512*1024 {
		t.Fatalf("fallback memInfo %d %d", tot, avail)
	}
	// Missing file + zero fallback → zeros.
	proc2 := withProc(t)
	_ = proc2
	qelibHostFreeMemMB = func() int { return 0 }
	tot, avail = memInfo()
	if tot != 0 || avail != 0 {
		t.Fatalf("empty memInfo %d %d", tot, avail)
	}
}

func TestLoadAvg(t *testing.T) {
	proc := withProc(t)
	os.WriteFile(filepath.Join(proc, "loadavg"), []byte("0.50 1.25 2.75 3/400 12345\n"), 0o644)
	l1, l5, l15 := loadAvg()
	if l1 != 0.50 || l5 != 1.25 || l15 != 2.75 {
		t.Fatalf("loadAvg %v %v %v", l1, l5, l15)
	}
	proc2 := withProc(t)
	_ = proc2
	if l1, l5, l15 := loadAvg(); l1 != 0 || l5 != 0 || l15 != 0 {
		t.Fatalf("missing loadavg %v %v %v", l1, l5, l15)
	}
}

func TestHostStats(t *testing.T) {
	dir := withDataDir(t)
	proc := withProc(t)
	os.WriteFile(filepath.Join(proc, "meminfo"),
		[]byte("MemTotal: 8000 kB\nMemAvailable: 4000 kB\n"), 0o644)
	os.WriteFile(filepath.Join(proc, "loadavg"), []byte("1.00 2.00 3.00 x\n"), 0o644)
	os.WriteFile(filepath.Join(proc, "uptime"), []byte("999.00 0\n"), 0o644)

	// One live VM (the test pid), one stopped.
	live := filepath.Join(dir, "vms", "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"pid": os.Getpid()})
	os.WriteFile(filepath.Join(live, "live.runtime.json"), b, 0o644)
	dead := filepath.Join(dir, "vms", "dead")
	if err := os.MkdirAll(dead, 0o755); err != nil {
		t.Fatal(err)
	}

	got := hostStats()
	if got["vms_total"] != 2 || got["vms_running"] != 1 {
		t.Fatalf("vms %v", got)
	}
	if got["mem_total_mb"] != int64(8000/1024) || got["mem_avail_mb"] != int64(4000/1024) {
		t.Fatalf("mem %v", got)
	}
	if got["mem_used_pct"] != 50.0 {
		t.Fatalf("used pct %v", got)
	}
	if got["load1"] != 1.0 || got["load5"] != 2.0 || got["load15"] != 3.0 {
		t.Fatalf("load %v", got)
	}
	if got["uptime_s"] != int64(999) {
		t.Fatalf("uptime %v", got)
	}
	if got["data_dir_free_ok"] != true || got["data_dir_free_kb"].(int64) <= 0 {
		t.Fatalf("statfs %v", got)
	}
	if got["cpu_cores"].(int) < 1 {
		t.Fatalf("cores %v", got)
	}

	// No meminfo → used_pct stays 0 (no div-by-zero).
	proc2 := withProc(t)
	_ = proc2
	old := qelibHostFreeMemMB
	qelibHostFreeMemMB = func() int { return 0 }
	t.Cleanup(func() { qelibHostFreeMemMB = old })
	got = hostStats()
	if got["mem_used_pct"] != 0.0 {
		t.Fatalf("no-meminfo pct %v", got)
	}
}

func TestStatfsAvailKB(t *testing.T) {
	kb, ok := statfsAvailKB(t.TempDir())
	if !ok || kb <= 0 {
		t.Fatalf("tempdir statfs %d %v", kb, ok)
	}
	kb, ok = statfsAvailKB(filepath.Join(t.TempDir(), "missing"))
	if ok || kb != 0 {
		t.Fatalf("missing statfs %d %v", kb, ok)
	}
}

// The /v1/stats handler delegates to hostStats — fixture proc +
// a VM set exercise the merged path through the mux.
func TestStatsEndpointWithFixtures(t *testing.T) {
	dir := withDataDir(t)
	proc := withProc(t)
	os.WriteFile(filepath.Join(proc, "meminfo"), []byte("MemTotal: 4096 kB\nMemAvailable: 2048 kB\n"), 0o644)
	os.WriteFile(filepath.Join(proc, "loadavg"), []byte("0.10 0.20 0.30 x\n"), 0o644)
	os.WriteFile(filepath.Join(proc, "uptime"), []byte("60.00 0\n"), 0o644)
	if err := os.MkdirAll(filepath.Join(dir, "vms", "v"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, httptest.NewRequest("GET", "/v1/stats", nil))
	if rec.Code != 200 {
		t.Fatalf("stats %d %s", rec.Code, rec.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	d := m["data"].(map[string]any)
	if d["vms_total"] != 1.0 || d["vms_running"] != 0.0 {
		t.Fatalf("stats data %v", d)
	}
	if d["mem_total_mb"] != 4.0 {
		t.Fatalf("mem %v", d)
	}
}
