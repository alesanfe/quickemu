// metrics_test.go — QE-101: Prometheus exposition shape, label
// escaping and live-VM counters.
package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetricsEndpoint(t *testing.T) {
	dir := t.TempDir()
	old := dataDir
	dataDir = dir
	t.Cleanup(func() { dataDir = old })
	// One stopped VM dir + one ops record.
	vm := filepath.Join(dir, "vms", "alpha")
	os.MkdirAll(filepath.Join(vm, "ops"), 0o755)
	os.WriteFile(filepath.Join(vm, "ops", "op-1.json"), []byte("{}"), 0o644)

	w := httptest.NewRecorder()
	handleMetrics(w, httptest.NewRequest("GET", "/metrics", nil))
	body := w.Body.String()
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("content-type %q", ct)
	}
	for _, want := range []string{
		"quickemu_vms_total 1",
		`quickemu_vm_info{state="stopped",vm="alpha"} 1`,
		"quickemu_vms_running 0",
		`quickemu_vm_ops{vm="alpha"} 1`,
		"quickemu_ops_total 1",
		"quickemu_api_info{version=",
		"quickemu_host_mem_total_bytes",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestMetEsc(t *testing.T) {
	if got := metEsc(`a"b\c`); got != `a\"b\\c` {
		t.Fatalf("esc %q", got)
	}
	if got := metEsc("a\nb"); got != `a\nb` {
		t.Fatalf("nl %q", got)
	}
}
