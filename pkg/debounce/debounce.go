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
	// FunctionFirst runs the inner function on the first call; subsequent calls within
	// Duration return the cached result from that first execution.
	FunctionFirst DebounceType = iota
	// FunctionLast waits Duration after the last call before running the inner function.
	FunctionLast
)

// Settings for the Debounce
type Settings struct {
	// Duration is the debounce window. Defaults to 300ms when unset.
	Duration time.Duration `validate:"min=1ms"`
	// DebounceType selects first or last debounce behavior.
	DebounceType DebounceType `validate:"min=0,max=1"`
}

// Circuit executes downstream work for a request and may fail.
type Circuit[A, T any] func(context.Context, A) (T, error)

// Debounce limits call frequency of a function
type Debounce[A, T any] struct {
	mu       sync.Mutex
	duration time.Duration

	threshold     time.Time
	cachedResult  T
	cachedErr     error
	firstInFlight bool
	firstDone     chan struct{}

	timer      *time.Timer
	lastCancel context.CancelFunc
}

// NewDebounce validates settings and returns a Debounce
func NewDebounce[A, T any](settings Settings) (*Debounce[A, T], error) {
	validate := validator.New(validator.WithRequiredStructEnabled())
	err := validate.Struct(settings)
	if err != nil {
		return nil, err
	}

	settings = normalizeSettings(settings)

	return &Debounce[A, T]{
		duration: settings.Duration,
	}, nil
}

func normalizeSettings(s Settings) Settings {
	if s.Duration <= 0 {
		s.Duration = defaultDuration
	}
	return s
}

// DebounceFirstFn wraps circuit and returns a function that debounces function-first style.
// The first call in a window runs circuit; later calls within Duration return the same result.
// Call DebounceFirstFn once and reuse the returned Circuit.
func (deb *Debounce[A, T]) DebounceFirstFn(circuit Circuit[A, T]) Circuit[A, T] {
	return func(ctx context.Context, req A) (T, error) {
		deb.mu.Lock()
		if time.Now().Before(deb.threshold) {
			if deb.firstInFlight {
				done := deb.firstDone
				deb.mu.Unlock()
				<-done
				deb.mu.Lock()
				result, err := deb.cachedResult, deb.cachedErr
				deb.mu.Unlock()
				return result, err
			}
			result, err := deb.cachedResult, deb.cachedErr
			deb.mu.Unlock()
			return result, err
		}

		deb.firstInFlight = true
		deb.firstDone = make(chan struct{})
		deb.threshold = time.Now().Add(deb.duration)
		deb.mu.Unlock()

		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		result, err := circuit(runCtx, req)

		deb.mu.Lock()
		deb.cachedResult = result
		deb.cachedErr = err
		deb.firstInFlight = false
		close(deb.firstDone)
		deb.mu.Unlock()

		return result, err
	}
}

// DebounceLastFn wraps circuit and returns a function that debounces function-last style.
// Each call resets the timer; circuit runs once after Duration passes with no new calls.
// Superseded callers receive ctx.Err() when a newer call replaces them.
// Call DebounceLastFn once and reuse the returned Circuit.
func (deb *Debounce[A, T]) DebounceLastFn(circuit Circuit[A, T]) Circuit[A, T] {
	return func(ctx context.Context, req A) (T, error) {
		deb.mu.Lock()

		if deb.lastCancel != nil {
			deb.lastCancel()
		}
		if deb.timer != nil {
			deb.timer.Stop()
		}

		runCtx, cancel := context.WithCancel(ctx)
		deb.lastCancel = cancel

		resCh := make(chan struct {
			result T
			err    error
		}, 1)

		deb.timer = time.AfterFunc(deb.duration, func() {
			result, err := circuit(runCtx, req)
			resCh <- struct {
				result T
				err    error
			}{result, err}

			deb.mu.Lock()
			deb.timer = nil
			deb.lastCancel = nil
			deb.mu.Unlock()
		})

		deb.mu.Unlock()

		select {
		case res := <-resCh:
			return res.result, res.err
		case <-runCtx.Done():
			var zero T
			return zero, runCtx.Err()
		}
	}
}
