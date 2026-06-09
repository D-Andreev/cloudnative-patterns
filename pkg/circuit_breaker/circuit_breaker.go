package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

var BreakerErrResponse = errors.New("service unavailable")

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

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

type Circuit[T any] func(context.Context) (T, error)

type Breaker[T any] struct {
	isFailure     func(err error) bool
	mu            sync.Mutex
	state         State
	threshold     int
	failures      int
	last          time.Time
	probeInFlight bool
}

func NewBreaker[T any](isFailure func(err error) bool, threshold int) *Breaker[T] {
	return &Breaker[T]{
		isFailure: isFailure,
		state:     Closed,
		threshold: threshold,
	}
}

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
