// power.go: guest power operations — the agent-side shutdown
// request. The ACPI fallback and the force-escalation live in the CLI
// (they need the monitor channel, which is not a QGA concern).
package qga

import (
	"context"
	"fmt"
)

// ShutdownMode is the guest-shutdown mode vocabulary.
type ShutdownMode string

const (
	ShutdownPowerdown ShutdownMode = "powerdown" // ACPI-ish poweroff
	ShutdownHalt      ShutdownMode = "halt"      // halt, leave VM running
	ShutdownReboot    ShutdownMode = "reboot"
)

// Shutdown issues guest-shutdown. The agent acknowledges the request
// before initiating it — a nil error means the guest accepted, not
// that it already powered off; callers confirm via VM state/monitor.
func (cl *Client) Shutdown(ctx context.Context, mode ShutdownMode) error {
	switch mode {
	case ShutdownPowerdown, ShutdownHalt, ShutdownReboot:
	default:
		return fmt.Errorf("%w: unknown shutdown mode %q", ErrBadResponse, mode)
	}
	_, err := cl.Execute(ctx, "guest-shutdown",
		map[string]any{"mode": string(mode)})
	return err
}
