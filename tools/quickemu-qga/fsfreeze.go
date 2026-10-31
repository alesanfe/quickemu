// fsfreeze.go: guest-fsfreeze-* wrappers and the WithFreeze helper —
// freeze the guest filesystems, run a function (snapshot, backup...),
// and always thaw again, with a watchdog so a stuck call cannot leave
// the guest frozen forever.
package qga

import (
	"context"
	"fmt"
	"time"
)

// FSFStatus is the guest-fsfreeze-status vocabulary.
type FSFStatus string

const (
	FSFThawed     FSFStatus = "thawed"
	FSFFrozen     FSFStatus = "frozen"
	FSFInProgress FSFStatus = "in-progress"
)

// FSFreezeStatus wraps guest-fsfreeze-status.
func (cl *Client) FSFreezeStatus(ctx context.Context) (FSFStatus, error) {
	var s string
	if err := cl.typed(ctx, "guest-fsfreeze-status", nil, &s); err != nil {
		return "", err
	}
	return FSFStatus(s), nil
}

// FSFreeze wraps guest-fsfreeze-freeze — returns the number of
// filesystems the agent reports as frozen.
func (cl *Client) FSFreeze(ctx context.Context) (int, error) {
	var n int
	if err := cl.typed(ctx, "guest-fsfreeze-freeze", nil, &n); err != nil {
		return -1, err
	}
	return n, nil
}

// FSThaw wraps guest-fsfreeze-thaw — returns the number of
// filesystems the agent reports as thawed.
func (cl *Client) FSThaw(ctx context.Context) (int, error) {
	var n int
	if err := cl.typed(ctx, "guest-fsfreeze-thaw", nil, &n); err != nil {
		return -1, err
	}
	return n, nil
}

// WithFreeze freezes the guest filesystems, runs fn, and thaws —
// guaranteed even when fn returns an error or the command channel
// drops mid-flight. Two independent safeguards:
//
//   - a deferred thaw runs unconditionally after fn returns;
//   - a watchdog timer thaws on its own once watchdog expires, so a
//     hung fn (or a wedged client) cannot freeze the guest forever.
//
// watchdog <= 0 defaults to 2 minutes. A thaw failure is reported
// when fn itself succeeded — fn's error takes precedence otherwise.
func (cl *Client) WithFreeze(ctx context.Context, watchdog time.Duration,
	fn func(ctx context.Context) error) error {
	if watchdog <= 0 {
		watchdog = 2 * time.Minute
	}
	if _, err := cl.FSFreeze(ctx); err != nil {
		return err
	}
	// The watchdog fires exactly once: past the deadline it thaws
	// without waiting for fn — best-effort, errors intentionally
	// dropped (the deferred thaw still runs and reports its error).
	done := make(chan struct{})
	fired := make(chan struct{}, 1)
	go func() {
		t := time.NewTimer(watchdog)
		defer t.Stop()
		select {
		case <-done:
		case <-t.C:
			thawCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_, _ = cl.thawBestEffort(thawCtx)
			cancel()
			fired <- struct{}{}
		}
	}()
	err := fn(ctx)
	close(done)
	thawCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, terr := cl.thawBestEffort(thawCtx)
	cancel()
	select {
	case <-fired:
		// The watchdog already thawed: report it so the caller knows
		// fn outlived its freeze window.
		if err == nil {
			err = fmt.Errorf("%w: freeze watchdog fired before function completed", ErrTimeout)
		}
	default:
	}
	if err != nil {
		return err
	}
	return terr
}

// thawBestEffort retries the thaw once on failure — a frozen guest is
// worse than a doubled command.
func (cl *Client) thawBestEffort(ctx context.Context) (int, error) {
	n, err := cl.FSThaw(ctx)
	if err != nil {
		select {
		case <-ctx.Done():
			return -1, err
		case <-time.After(500 * time.Millisecond):
		}
		return cl.FSThaw(ctx)
	}
	return n, nil
}
