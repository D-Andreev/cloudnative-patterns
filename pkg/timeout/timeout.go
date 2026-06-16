// Package timeout wraps functions so callers stop waiting once a context deadline is exceeded.
package timeout

import "context"

type response[T any] struct {
	result T
	err    error
}

// SlowFunction performs work for a request without accepting a context.
type SlowFunction[A, T any] func(req A) (T, error)

// WithContext performs work for a request and respects context cancellation.
type WithContext[A, T any] func(context.Context, A) (T, error)

// Timeout wraps a SlowFunction with context-aware waiting.
type Timeout[A, T any] struct {
}

// NewTimeout returns a Timeout.
func NewTimeout[A, T any]() (*Timeout[A, T], error) {
	return &Timeout[A, T]{}, nil
}

// TimeoutFn wraps f so the returned WithContext waits for f in a goroutine until
// either f returns or ctx is done. When the context expires first, the zero value
// of T and ctx.Err() are returned; f may still run to completion in the background.
func (t *Timeout[A, T]) TimeoutFn(f SlowFunction[A, T]) WithContext[A, T] {
	return func(ctx context.Context, args A) (T, error) {
		ch := make(chan response[T], 1)

		go func() {
			result, err := f(args)
			resp := response[T]{result, err}
			ch <- resp
		}()

		select {
		case res := <-ch:
			return res.result, res.err
		case <-ctx.Done():
			var zeroT T
			return zeroT, ctx.Err()
		}
	}
}
