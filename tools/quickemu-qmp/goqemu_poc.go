//go:build poc

// POC for digitalocean/go-qemu (ADR-0001 decision gate): wraps the
// third-party monitor behind our own client shape so its types never
// leak into the contract. Build with -tags poc to compare.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/digitalocean/go-qemu/qmp"
)

// pocExecute mirrors runExecute against the go-qemu monitor to verify
// behavioural parity before any adoption decision.
func pocExecute(socket, command string, timeout time.Duration) int {
	mon, err := qmp.NewSocketMonitor("unix", socket, timeout)
	if err != nil {
		return emit("qmp.execute", nil, "QMP_UNAVAILABLE", err.Error())
	}
	ctx := context.Background()
	if err := mon.Connect(ctx); err != nil {
		return emit("qmp.execute", nil, "QMP_GREETING", err.Error())
	}
	defer mon.Disconnect()
	out, err := mon.Run(ctx, command)
	if err != nil {
		return emit("qmp.execute", nil, "QMP_ERROR", err.Error())
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	fmt.Println(string(out))
	_ = m
	return 0
}
