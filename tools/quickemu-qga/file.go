// file.go: guest-file-* based transfers — push writes the guest file
// in base64 blocks, pull reads it back the same way. Transfers bound
// their size (a hostile or endless guest file cannot fill the disk),
// local writes are atomic (temp file + rename), and an optional
// restricted mode rejects guest paths that should never be touched.
package qga

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TransferOpts bounds a push/pull operation.
type TransferOpts struct {
	// MaxBytes caps the transfer (default 1GiB when <= 0). Push
	// refuses oversized local files up front; Pull stops mid-read.
	MaxBytes int64
	// Restricted rejects dangerous guest paths (system dirs,
	// traversal, device nodes) — for callers accepting remote input.
	Restricted bool
	// Block is the per-request payload in bytes (default 48KiB — a
	// base64-friendly size well under the message cap).
	Block int
}

func (o *TransferOpts) norm() {
	if o.MaxBytes <= 0 {
		o.MaxBytes = 1 << 30
	}
	if o.Block <= 0 || o.Block > 1<<20 {
		o.Block = 48 << 10
	}
}

// TransferResult reports what moved.
type TransferResult struct {
	Bytes  int64
	SHA256 string // hex digest of the transferred content
	Trunc  bool   // pull hit MaxBytes mid-file
}

// DefaultMaxTransfer is the pull/push ceiling when the caller sets
// none (1GiB).
const DefaultMaxTransfer = int64(1 << 30)

// SafeGuestPath reports whether p is acceptable under Restricted
// mode: no traversal, no control characters, and outside well-known
// system locations on both POSIX and Windows guests.
func SafeGuestPath(p string) bool {
	if p == "" {
		return false
	}
	if strings.ContainsAny(p, "\x00\r\n") {
		return false
	}
	if strings.Contains(p, "..") {
		return false
	}
	lp := strings.ToLower(strings.ReplaceAll(p, "\\", "/"))
	bad := []string{
		"/etc", "/boot", "/dev", "/proc", "/sys", "/sbin", "/bin",
		"/usr", "/lib", "/lib64", "/snap", "/run/", "/var/lib",
		"/var/log", "/root", "c:/windows", "c:/program files",
		"c:/programdata", "c:/users/*/windows",
	}
	if lp == "/" {
		return false
	}
	for _, b := range bad {
		if strings.HasSuffix(b, "/") {
			if strings.HasPrefix(lp, b) || lp == strings.TrimSuffix(b, "/") {
				return false
			}
			continue
		}
		if lp == b || strings.HasPrefix(lp, b+"/") {
			return false
		}
	}
	return true
}

// fileOp runs a guest-file-* command and decodes the reply.
func (cl *Client) fileOp(ctx context.Context, cmd string, args map[string]any, out any) error {
	return cl.typed(ctx, cmd, args, out)
}

// FileOpen opens a guest path (mode "r" or "w+") and returns the
// agent's handle.
func (cl *Client) FileOpen(ctx context.Context, path, mode string) (int, error) {
	var h int
	if err := cl.fileOp(ctx, "guest-file-open",
		map[string]any{"path": path, "mode": mode}, &h); err != nil {
		return -1, err
	}
	return h, nil
}

// FileClose closes a handle; errors are reported but usually
// ignorable (the handle dies with the agent).
func (cl *Client) FileClose(ctx context.Context, handle int) error {
	return cl.fileOp(ctx, "guest-file-close",
		map[string]any{"handle": handle}, &json.RawMessage{})
}

// fileRead is one guest-file-read reply.
type fileRead struct {
	Count int    `json:"count"`
	Buf   string `json:"buf-b64"`
	EOF   bool   `json:"eof"`
}

// fileWrite is one guest-file-write reply.
type fileWrite struct {
	Count int  `json:"count"`
	EOF   bool `json:"eof"`
}

// Push streams a local file into the guest in base64 blocks. On any
// error the guest handle is closed; partial guest writes are the
// agent's to keep (there is no guest-side temp+rename primitive), so
// callers get the error to decide whether to retry.
func (cl *Client) Push(ctx context.Context, local, guestPath string, o TransferOpts) (*TransferResult, error) {
	o.norm()
	if o.Restricted && !SafeGuestPath(guestPath) {
		return nil, fmt.Errorf("%w: refusing guest path %q in restricted mode", ErrBadResponse, guestPath)
	}
	f, err := os.Open(local)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > o.MaxBytes {
		return nil, fmt.Errorf("%w: %s is %d bytes (limit %d)", ErrMessageTooBig, local, st.Size(), o.MaxBytes)
	}
	h, err := cl.FileOpen(ctx, guestPath, "w+")
	if err != nil {
		return nil, err
	}
	res := &TransferResult{}
	hs := sha256.New()
	fail := func(err error) (*TransferResult, error) {
		cl.FileClose(context.Background(), h)
		return res, err
	}
	buf := make([]byte, o.Block)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			var wr fileWrite
			werr := cl.fileOp(ctx, "guest-file-write", map[string]any{
				"handle": h, "buf-b64": base64.StdEncoding.EncodeToString(buf[:n]),
			}, &wr)
			if werr != nil {
				return fail(werr)
			}
			if wr.Count != n {
				return fail(fmt.Errorf("%w: guest-file-write accepted %d of %d bytes", ErrBadResponse, wr.Count, n))
			}
			hs.Write(buf[:n])
			res.Bytes += int64(n)
		}
		if rerr != nil {
			if rerr != io.EOF {
				return fail(rerr)
			}
			break
		}
	}
	if err := cl.FileClose(ctx, h); err != nil {
		return res, err
	}
	res.SHA256 = hex.EncodeToString(hs.Sum(nil))
	return res, nil
}

// Pull reads a guest file and writes it atomically to local: the
// payload lands in <local>.tmp-<pid> and is renamed only after the
// whole file (or the size cap) is in place — an error mid-transfer
// removes the temp file rather than leaving a truncated artifact.
func (cl *Client) Pull(ctx context.Context, guestPath, local string, o TransferOpts) (*TransferResult, error) {
	o.norm()
	if o.Restricted && !SafeGuestPath(guestPath) {
		return nil, fmt.Errorf("%w: refusing guest path %q in restricted mode", ErrBadResponse, guestPath)
	}
	h, err := cl.FileOpen(ctx, guestPath, "r")
	if err != nil {
		return nil, err
	}
	tmp := local + ".tmp-qga"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		cl.FileClose(context.Background(), h)
		return nil, err
	}
	res := &TransferResult{}
	hs := sha256.New()
	fail := func(err error) (*TransferResult, error) {
		out.Close()
		os.Remove(tmp)
		cl.FileClose(context.Background(), h)
		return res, err
	}
	for {
		var rd fileRead
		if err := cl.fileOp(ctx, "guest-file-read", map[string]any{
			"handle": h, "count": o.Block,
		}, &rd); err != nil {
			return fail(err)
		}
		if rd.Count > 0 {
			b, derr := base64.StdEncoding.DecodeString(rd.Buf)
			if derr != nil {
				return fail(fmt.Errorf("%w: bad buf-b64 in guest-file-read", ErrBadResponse))
			}
			if len(b) > rd.Count {
				b = b[:rd.Count]
			}
			if res.Bytes+int64(len(b)) > o.MaxBytes {
				b = b[:o.MaxBytes-res.Bytes]
				res.Trunc = true
			}
			if _, werr := out.Write(b); werr != nil {
				return fail(werr)
			}
			hs.Write(b)
			res.Bytes += int64(len(b))
		}
		if rd.EOF || res.Trunc || res.Bytes >= o.MaxBytes {
			break
		}
		if rd.Count == 0 {
			// No data and no EOF — the agent is stalling; wait a tick
			// and let ctx bound the total time rather than spin.
			select {
			case <-ctx.Done():
				return fail(fmt.Errorf("%w: guest-file-read stalled", ErrTimeout))
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	if err := cl.FileClose(ctx, h); err != nil {
		out.Close()
		os.Remove(tmp)
		return res, err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return res, err
	}
	if err := os.Rename(tmp, local); err != nil {
		os.Remove(tmp)
		return res, err
	}
	res.SHA256 = hex.EncodeToString(hs.Sum(nil))
	return res, nil
}

// VerifyPull re-reads a just-pushed guest file and returns its
// SHA-256 so callers can compare against the pushed digest — the
// agent has no hashing command, so verification is a second read.
func (cl *Client) VerifyPull(ctx context.Context, guestPath string, o TransferOpts) (string, error) {
	o.norm()
	tmp, err := os.CreateTemp("", "qga-verify-*")
	if err != nil {
		return "", err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	res, err := cl.Pull(ctx, guestPath, tmp.Name(), o)
	if err != nil {
		return "", err
	}
	return res.SHA256, nil
}

// CleanGuestPath reports p after rejecting anything SafeGuestPath
// would refuse AND anything that is not an absolute POSIX or Windows
// path — the stricter read for operators who want canonical targets.
func CleanGuestPath(p string) bool {
	if !SafeGuestPath(p) {
		return false
	}
	if strings.HasPrefix(p, "/") {
		return !strings.Contains(p, "\x00")
	}
	// Windows absolute: drive letter or UNC.
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		return (p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')
	}
	return strings.HasPrefix(p, "\\\\")
}

// PathIsUnder reports whether child lives inside dir after lexical
// cleaning — used to keep restricted-mode pulls inside chosen roots.
func PathIsUnder(dir, child string) bool {
	d := filepath.Clean(dir)
	c := filepath.Clean(child)
	if c == d {
		return true
	}
	return strings.HasPrefix(c, d+string(filepath.Separator))
}
