// metrics.go — QE-101: Prometheus text exposition at /metrics.
// Built from the same on-disk evidence as /v1/stats (procfs + runtime
// records) — the scrape must never shell out or block on QMP.
package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// metEsc: Prometheus label values escape \ " and newline only.
func metEsc(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return strings.ReplaceAll(s, "\n", `\n`)
}

func metLine(b *strings.Builder, name string, labels map[string]string, v any) {
	b.WriteString(name)
	if len(labels) > 0 {
		var keys []string
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(b, `%s="%s"`, k, metEsc(labels[k]))
		}
		b.WriteByte('}')
	}
	fmt.Fprintf(b, " %v\n", v)
}

// handleMetrics renders the text exposition format (v0.0.4).
func handleMetrics(w http.ResponseWriter, _ *http.Request) {
	var b strings.Builder
	vms := collectVMs()
	running := 0
	metLine(&b, "quickemu_vms_total", nil, len(vms))
	for _, e := range vms {
		ev := probeVM(e.Dir, e.Name, e.Recorded)
		lbl := map[string]string{"vm": e.Name, "state": ev.State}
		metLine(&b, "quickemu_vm_info", lbl, 1)
		if ev.Alive {
			running++
			if s := readProc(ev.Pid); s.OK {
				metLine(&b, "quickemu_vm_rss_bytes", map[string]string{"vm": e.Name}, s.RssB)
				metLine(&b, "quickemu_vm_threads", map[string]string{"vm": e.Name}, s.Threads)
				metLine(&b, "quickemu_vm_uptime_seconds", map[string]string{"vm": e.Name}, int64(uptimeFor(s)))
				metLine(&b, "quickemu_vm_disk_read_bytes", map[string]string{"vm": e.Name}, s.ReadB)
				metLine(&b, "quickemu_vm_disk_write_bytes", map[string]string{"vm": e.Name}, s.WriteB)
			}
		}
	}
	metLine(&b, "quickemu_vms_running", nil, running)
	// Ops journal counts per VM — the journal files themselves are
	// the durable record; count without parsing bodies.
	opsTotal := 0
	for _, e := range vms {
		ents, _ := filepath.Glob(filepath.Join(e.Dir, "ops", "*.json"))
		opsTotal += len(ents)
		metLine(&b, "quickemu_vm_ops", map[string]string{"vm": e.Name}, len(ents))
	}
	metLine(&b, "quickemu_ops_total", nil, opsTotal)
	tot, avail := memInfo()
	metLine(&b, "quickemu_host_mem_total_bytes", nil, tot*1024)
	metLine(&b, "quickemu_host_mem_available_bytes", nil, avail*1024)
	metLine(&b, "quickemu_host_uptime_seconds", nil, int64(procUptimeS()))
	metLine(&b, "quickemu_api_info",
		map[string]string{"version": qelib.QEVersion}, 1)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(200)
	fmt.Fprint(w, b.String())
}
