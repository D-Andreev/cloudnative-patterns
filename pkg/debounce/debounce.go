// Package debounce limits the frequence of a function invocation.
// Depending on the Type you specify either the first or the last in a cluster of calls is actually performed.
package debounce

import (
	"context"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
)

const defaultDuration = 300 * time.Millisecond

// Type of debounce (last or first)
type DebounceType int

const (
	// Inner function is called and subsequent requests are ignored for duration D
	FunctionFirst DebounceType = iota
	// Function last will wait for duration D after series of calls to call the inner function
	FunctionLast
)

// Settings for the Debounce
type Settings struct {
	// Duration to wait before making the actual calls. If not passed default is 1s.
	// This applies only when DebouncyType is FunctionLast
	Duration time.Duration `validate:"min=1ms"`
	// Debounce type, first or last
	DebounceType DebounceType `validate:"min=0,max=1"`
}

// Circuit is the inner function that's executed in debounce
type Circuit[T any] func(context.Context) (T, error)

// Debounce limits call frequency of a function
type Debounce[T any] struct {
	mu                sync.Mutex
	duration          time.Duration
	debounceType      DebounceType
	threshold         time.Time
	fnFirstLastCtx    context.Context
	fnFirstLastCancel context.CancelFunc
	timer             *time.Timer
	fnLastCtx         context.Context
	fnLastCancel      context.CancelFunc
}

// NewDebounce validates settings and returns a Debounce
func NewDebounce[T any](settings Settings) (*Debounce[T], error) {
	validate := validator.New(validator.WithRequiredStructEnabled())
	err := validate.Struct(settings)
	if err != nil {
		return nil, err
	}

	settings = normalizeSettings(settings)

	return &Debounce[T]{
		duration:     settings.Duration,
		debounceType: settings.DebounceType,
	}, nil
}

func normalizeSettings(s Settings) Settings {
	if s.Duration <= 0 {
		s.Duration = defaultDuration
	}
	return s
}

// DebounceFirstFn wraps circuit and returns a function that debounces function first style.
// Call DebounceFirstFn once and reuse the returned Circuit; do not call DebounceFirstFn on every request.
func (deb *Debounce[T]) DebounceFirstFn(circuit Circuit[T]) Circuit[T] {
	return func(ctx context.Context) (T, error) {
		deb.mu.Lock()

		if time.Now().Before(deb.threshold) {
			deb.fnFirstLastCancel()
			deb.mu.Unlock()
			var zeroT T
			return zeroT, nil
		}

		deb.fnFirstLastCtx, deb.fnFirstLastCancel = context.WithCancel(ctx)
		deb.threshold = time.Now().Add(deb.duration)

		deb.mu.Unlock()

		result, err := circuit(deb.fnFirstLastCtx)
		return result, err
	}
}

// DebounceLasttFn wraps circuit and returns a function that debounces function last style.
// Call DebounceLasttFn once and reuse the returned Circuit; do not call DebounceLasttFn on every request.
func (deb *Debounce[T]) DebounceLasttFn(circuit Circuit[T]) Circuit[T] {
	return func(ctx context.Context) (T, error) {
		deb.mu.Lock()

		if deb.timer != nil {
			deb.timer.Stop()
			deb.fnLastCancel()
			deb.mu.Unlock()
			var zeroT T
			return zeroT, nil
		}

		deb.fnLastCtx, deb.fnLastCancel = context.WithCancel(ctx)
		ch := make(chan struct {
			result T
			err    error
		}, 1)

		deb.timer = time.AfterFunc(deb.duration, func() {
			r, e := circuit(ctx)
			ch <- struct {
				result T
				err    error
			}{r, e}
		})

		deb.mu.Unlock()

		select {
		case res := <-ch:
			return res.result, res.err
		case <-deb.fnLastCtx.Done():
			var zeroT T
			return zeroT, deb.fnLastCtx.Err()
		}
	}
}
