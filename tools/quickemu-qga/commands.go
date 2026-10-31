// commands.go: typed wrappers over the raw QGA command surface —
// each maps the agent's "return" payload onto a Go value and keeps
// the argument shape in one place.
package qga

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Ping is guest-ping — proves the agent answers commands. A nil error
// is the whole contract; the reply body is always {}.
func (cl *Client) Ping(ctx context.Context) error {
	_, err := cl.Execute(ctx, "guest-ping", nil)
	return err
}

// SupportedCommand is one entry of guest-info's supported_commands.
type SupportedCommand struct {
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	SuccessResponse bool   `json:"success-response"`
}

// GuestInfo is the guest-info reply: agent version plus the command
// allowlist the running agent was configured with.
type GuestInfo struct {
	Version           string             `json:"version"`
	SupportedCommands []SupportedCommand `json:"supported_commands"`
}

func (cl *Client) Info(ctx context.Context) (*GuestInfo, error) {
	var out GuestInfo
	if err := cl.typed(ctx, "guest-info", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// OSInfo is the guest-get-osinfo reply (fields beyond these vary by
// distribution; the common set is decoded, the rest is ignored).
type OSInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PrettyName    string `json:"pretty-name"`
	Version       string `json:"version"`
	VersionID     string `json:"version-id"`
	KernelRelease string `json:"kernel-release"`
	KernelVersion string `json:"kernel-version"`
	Architecture  string `json:"machine"`
	Variant       string `json:"variant"`
	VariantID     string `json:"variant-id"`
}

func (cl *Client) OSInfo(ctx context.Context) (*OSInfo, error) {
	var out OSInfo
	if err := cl.typed(ctx, "guest-get-osinfo", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HostName wraps guest-get-host-name.
func (cl *Client) HostName(ctx context.Context) (string, error) {
	var out struct {
		HostName string `json:"host-name"`
	}
	if err := cl.typed(ctx, "guest-get-host-name", nil, &out); err != nil {
		return "", err
	}
	return out.HostName, nil
}

// GuestIPAddress is one address entry inside a GuestInterface.
type GuestIPAddress struct {
	Address string `json:"ip-address"`
	Type    string `json:"ip-address-type"` // "ipv4" | "ipv6"
	Prefix  int    `json:"prefix"`
}

// GuestInterface is one guest-network-get-interfaces element.
// "name" may be absent on some guests; hardware-address likewise.
type GuestInterface struct {
	Name         string           `json:"name"`
	HardwareAddr string           `json:"hardware-address"`
	IPAddresses  []GuestIPAddress `json:"ip-addresses"`
}

func (cl *Client) NetInterfaces(ctx context.Context) ([]GuestInterface, error) {
	var out []GuestInterface
	if err := cl.typed(ctx, "guest-network-get-interfaces", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GuestTime wraps guest-get-time — nanoseconds since the Unix epoch.
func (cl *Client) GuestTime(ctx context.Context) (time.Time, error) {
	var ns int64
	if err := cl.typed(ctx, "guest-get-time", nil, &ns); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, ns).UTC(), nil
}

// GuestFSDisk is one backing disk of a GuestFS.
type GuestFSDisk struct {
	Bus           int    `json:"bus"`
	Target        int    `json:"target"`
	Unit          int    `json:"unit"`
	Serial        string `json:"serial"`
	Dev           string `json:"dev"`
	BusType       string `json:"bus-type"`
	PCIController *struct {
		Bus      int `json:"bus"`
		Slot     int `json:"slot"`
		Function int `json:"function"`
	} `json:"pci-controller"`
}

// GuestFS is one guest-get-fsinfo element — a mounted filesystem and
// its usage as the guest sees it.
type GuestFS struct {
	Name       string        `json:"name"`
	Mountpoint string        `json:"mountpoint"`
	Type       string        `json:"type"`
	UsedBytes  int64         `json:"used-bytes"`
	TotalBytes int64         `json:"total-bytes"`
	Disk       []GuestFSDisk `json:"disk"`
}

func (cl *Client) FSInfo(ctx context.Context) ([]GuestFS, error) {
	var out []GuestFS
	if err := cl.typed(ctx, "guest-get-fsinfo", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// typed runs cmd and unmarshals "return" into out — the shared
// decode-and-validate tail of every wrapper.
func (cl *Client) typed(ctx context.Context, cmd string, args map[string]any, out any) error {
	raw, err := cl.Execute(ctx, cmd, args)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: %s reply does not decode: %v", ErrBadResponse, cmd, err)
	}
	return nil
}
