// hw_pinning.go: CPU pinning: parse/validate/apply host-core affinity
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// parseCPUPinning: the Bash ^[0-9]+(,[0-9]+)*$ validation — a
// comma-separated list of host CPUs, one per guest logical core.
func parseCPUPinning(v string) ([]int, error) {
	if v == "" {
		return nil, nil
	}
	var out []int
	for _, f := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 0 {
			return nil, fmt.Errorf(" - ERROR!  Couldn't parse CPU pinning: '%s', only comma-separated list is supported", v)
		}
		out = append(out, n)
	}
	return out, nil
}

// paramCheckPinning: vm.sh validates the count against
// GUEST_CPU_LOGICAL_CORES during arg assembly (Linux only).
func paramCheckPinning(c Conf) error {
	v := c.get("cpu_pinning", "")
	if v == "" || runtime.GOOS != "linux" {
		return nil
	}
	m, err := parseCPUPinning(v)
	if err != nil {
		return err
	}
	lc := 1
	fmt.Sscanf(c["_lcores"], "%d", &lc)
	if len(m) != lc {
		return fmt.Errorf(" - ERROR!  Number of host cores for pinning should be equal to VM core count (%d != %d)", len(m), lc)
	}
	return nil
}

// applyCPUPinning: configure_cpu_pinning — walks the QEMU task list
// for "CPU <n>/KVM" vCPU threads and pins each to its mapped host CPU
// via taskset (threads of one core share the host CPU).
func applyCPUPinning(pid int, c Conf) {
	mapping, err := parseCPUPinning(c.get("cpu_pinning", ""))
	if err != nil || len(mapping) == 0 || runtime.GOOS != "linux" {
		return
	}
	threads := 1
	fmt.Sscanf(c["_threads"], "%d", &threads)
	tids, err := filepath.Glob(fmt.Sprintf("/proc/%d/task/*", pid))
	if err != nil {
		return
	}
	sort.Strings(tids)
	var guestCPUs []string
	idx := 0
	for _, td := range tids {
		comm, err := os.ReadFile(filepath.Join(td, "comm"))
		if err != nil || !strings.HasPrefix(string(comm), "CPU") {
			continue
		}
		coreIdx := idx / threads
		if coreIdx < len(mapping) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			exec.CommandContext(ctx, "taskset", "-cp", fmt.Sprint(mapping[coreIdx]),
				filepath.Base(td)).Run()
			cancel()
		}
		if idx%threads == 0 {
			guestCPUs = append(guestCPUs, fmt.Sprint(coreIdx))
		}
		idx++
	}
	if idx > 0 {
		fmt.Printf(" - CPU Pinning: Bind guest cores to host cores (%s -> %s)\n",
			strings.Join(guestCPUs, ","), c.get("cpu_pinning", ""))
	}
}
