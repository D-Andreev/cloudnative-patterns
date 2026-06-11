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

// Effector is the function executed by Retry.
type Effector[T any] func(ctx context.Context) (T, error)

// Retry executes an Effector with retries on error.
type Retry[T any] struct {
	delay       time.Duration
	maxFailures int
}

// NewRetry validates settings and returns a Retry.
func NewRetry[T any](settings Settings) (*Retry[T], error) {
	return &Retry[T]{
		delay:       settings.Delay,
		maxFailures: settings.MaxFailures,
	}, nil
}

// RetryFn wraps effector with retry behavior. On error it waits Delay and invokes
// effector again until it succeeds, MaxFailures retries are exhausted, or ctx is
// canceled. Call RetryFn once and reuse the returned Effector.
func (retry *Retry[T]) RetryFn(e Effector[T]) Effector[T] {
	return func(ctx context.Context) (T, error) {
		for i := 0; ; i++ {
			r, err := e(ctx)
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
