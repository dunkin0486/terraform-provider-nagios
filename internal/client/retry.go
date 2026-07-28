package client

import (
	"context"
	"time"
)

// RetryUntilFound polls fn a bounded number of times with a fixed backoff,
// tolerating Nagios XI's own eventual-consistency window right after a write
// (a create/update can return success before the object is visible on a
// subsequent read). It returns the first non-nil result. If every attempt
// returns (nil, nil), that's treated as genuinely not-found and returned as
// such - this only smooths over transient absence immediately after a write,
// it never converts a real not-found into an error, and it never turns a
// real error into a false not-found.
//
// Callers should only use this immediately after a create/update, not from a
// plain Read/refresh path - a resource that was genuinely deleted outside of
// Terraform should surface as not-found on the next plan without a multi-
// second stall on every refresh.
func RetryUntilFound[T any](ctx context.Context, attempts int, backoff time.Duration, fn func() (*T, error)) (*T, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		v, err := fn()
		if err != nil {
			lastErr = err
		} else if v != nil {
			return v, nil
		} else {
			lastErr = nil
		}

		if i < attempts-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return nil, lastErr
}
