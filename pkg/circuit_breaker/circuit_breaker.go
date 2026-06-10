// Package circuitbreaker wraps functions with a circuit breaker that opens after
// repeated failures, fast-fails while open, and probes recovery in half-open state.
package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
)

// BreakerErrResponse is returned when the circuit is open or a half-open probe
// slot is unavailable. Use errors.Is to detect it.
var BreakerErrResponse = errors.New("service unavailable")

// State represents the current phase of the circuit breaker.
type State int

const (
	// Closed allows all requests through to the wrapped circuit.
	Closed State = iota
	// Open rejects requests until the retry cooldown elapses.
	Open
	// HalfOpen allows a single probe request to test recovery.
	HalfOpen
)

// String returns a lowercase name for the state.
func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Circuit is a function that may fail and is protected by the breaker.
type Circuit[T any] func(context.Context) (T, error)

// IsFailureFunc reports whether an error from the wrapped circuit should count
// toward opening the breaker.
type IsFailureFunc func(err error) bool

// Settings configures a Breaker.
type Settings struct {
	// IsFailure decides which downstream errors increment the failure count.
	// Required.
	IsFailure IsFailureFunc `validate:"required"`
	// Threshold is the number of consecutive failures before the circuit opens.
	// Must be at least 1.
	Threshold int `validate:"required,min=1"`
}

// Breaker executes a Circuit with open, closed, and half-open state transitions.
type Breaker[T any] struct {
	isFailure     IsFailureFunc
	mu            sync.Mutex
	state         State
	threshold     int
	failures      int
	last          time.Time
	probeInFlight bool
}

// NewBreaker validates settings and returns a breaker in the closed state.
func NewBreaker[T any](settings Settings) (*Breaker[T], error) {
	validate := validator.New(validator.WithRequiredStructEnabled())
	err := validate.Struct(settings)
	if err != nil {
		return nil, err
	}

	return &Breaker[T]{
		isFailure: settings.IsFailure,
		state:     Closed,
		threshold: settings.Threshold,
	}, nil
}

// State returns the current breaker state.
func (b *Breaker[T]) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *Breaker[T]) retryAt() time.Time {
	d := max(b.failures-b.threshold, 0)
	return b.last.Add(time.Duration(2<<d) * time.Second)
}

func (b *Breaker[T]) tryAcquireProbe() bool {
	switch b.state {
	case Closed:
		return true
	case Open:
		if !time.Now().After(b.retryAt()) {
			return false
		}
		b.state = HalfOpen
		fallthrough
	case HalfOpen:
		if b.probeInFlight {
			return false
		}
		b.probeInFlight = true
		return true
	default:
		return false
	}
}

// BreakerFn wraps circuit and returns a function that enforces breaker semantics.
// Call BreakerFn once and reuse the returned Circuit; do not call BreakerFn on
// every request.
func (b *Breaker[T]) BreakerFn(circuit Circuit[T]) Circuit[T] {
	return func(ctx context.Context) (T, error) {
		b.mu.Lock()
		if !b.tryAcquireProbe() {
			b.mu.Unlock()
			var zero T
			return zero, BreakerErrResponse
		}
		b.mu.Unlock()

		response, err := circuit(ctx)

		b.mu.Lock()
		defer b.mu.Unlock()

		b.probeInFlight = false
		b.last = time.Now()

		if b.isFailure(err) {
			b.failures++
			if b.state == HalfOpen || b.failures >= b.threshold {
				b.state = Open
			}
			return response, err
		}

		b.failures = 0
		b.state = Closed
		return response, nil
	}
}
