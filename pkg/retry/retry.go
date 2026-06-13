// Package retry wraps functions with automatic retries after failure.
// Failed attempts are separated by a configurable delay until success,
// MaxFailures is reached, or the context is canceled.
package retry

import (
	"context"
	"time"
)

// Settings configures a Retry.
type Settings struct {
	// Delay is the wait between retry attempts after a failed invocation.
	Delay time.Duration
	// MaxFailures is the maximum number of retries after the first failed attempt.
	// A value of 0 means no retries; 3 means up to four invocations on persistent failure.
	MaxFailures int
}

// Effector executes downstream work for a request and may fail.
type Effector[A, T any] func(ctx context.Context, req A) (T, error)

// Retry executes an Effector with retries on error.
type Retry[A, T any] struct {
	delay       time.Duration
	maxFailures int
}

// NewRetry returns a Retry.
func NewRetry[A, T any](settings Settings) (*Retry[A, T], error) {
	return &Retry[A, T]{
		delay:       settings.Delay,
		maxFailures: settings.MaxFailures,
	}, nil
}

// Wrap wraps effector with retry behavior. On error it waits Delay and invokes
// effector again until it succeeds, MaxFailures retries are exhausted, or ctx is
// canceled. Call Wrap once and reuse the returned Effector.
func (retry *Retry[A, T]) Wrap(e Effector[A, T]) Effector[A, T] {
	return func(ctx context.Context, req A) (T, error) {
		for i := 0; ; i++ {
			r, err := e(ctx, req)
			if err == nil || i >= retry.maxFailures {
				return r, err
			}

			select {
			case <-time.After(retry.delay):
			case <-ctx.Done():
				var zeroT T
				return zeroT, ctx.Err()
			}
		}
	}
}
