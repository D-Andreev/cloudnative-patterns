// Throttle limits the frequency of a function invocation to some maximum number for a unit of time
package throttle

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/D-Andreev/cloudnative-patterns/internal/queue"
	"github.com/go-playground/validator/v10"
)

var tooManyCalls = errors.New("too many calls")

// Settings for Throttle
type Settings struct {
	// The maximum times the function will be called for the given time period
	Maximum uint `validate:"min=1"`
	// The time window in which the function will be called at most Maximum times
	Duration time.Duration `validate:"min=1ms"`
	// Refill bucket by some amount every Duration
	Refill uint `validate:"min=1"`
}

// Effector executes downstream work for a request and may fail.
type Effector[A, T any] func(ctx context.Context, req A) (T, error)

// Throttle executes an Effector Maximum times for Duration
type Throttle[A, T any] struct {
	maximum           uint
	duration          time.Duration
	refill            uint
	tokens            uint
	once              sync.Once
	mu                sync.Mutex
	lastSuccessfulRes T
	queue             queue.Queue[A]
}

// NewThrottle validates settings and returns a Throttle.
func NewThrottle[A, T any](settings Settings) (*Throttle[A, T], error) {
	validate := validator.New(validator.WithRequiredStructEnabled())
	err := validate.Struct(settings)
	if err != nil {
		return nil, err
	}
	return &Throttle[A, T]{
		maximum:  settings.Maximum,
		duration: settings.Duration,
		refill:   settings.Refill,
		tokens:   settings.Maximum,
	}, nil
}

// WithError wraps effector with throttle behavior.
// The Effector will be executed at most Maximum times for given Duration
// If the Effector is throttled an error is returned
func (throttle *Throttle[A, T]) WithError(e Effector[A, T]) Effector[A, T] {
	return func(ctx context.Context, req A) (T, error) {
		if ctx.Err() != nil {
			var zeroT T
			return zeroT, ctx.Err()
		}

		throttle.once.Do(func() {
			ticker := time.NewTicker(throttle.duration)

			go func() {
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						throttle.mu.Lock()
						throttle.tokens = min(throttle.tokens+throttle.refill, throttle.maximum)
						throttle.mu.Unlock()
					}
				}
			}()
		})

		throttle.mu.Lock()
		defer throttle.mu.Unlock()

		if throttle.tokens <= 0 {
			var zeroT T
			return zeroT, tooManyCalls
		}

		throttle.tokens--

		return e(ctx, req)
	}
}

// WithReplay wraps effector with throttle behavior.
// The Effector will be executed at most Maximum times for given Duration
// If the Effector is throttled the last successful response is returned
func (throttle *Throttle[A, T]) WithReplay(e Effector[A, T]) Effector[A, T] {
	return func(ctx context.Context, req A) (T, error) {
		if ctx.Err() != nil {
			var zeroT T
			return zeroT, ctx.Err()
		}

		throttle.once.Do(func() {
			ticker := time.NewTicker(throttle.duration)

			go func() {
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						throttle.mu.Lock()
						throttle.tokens = min(throttle.tokens+throttle.refill, throttle.maximum)
						throttle.mu.Unlock()
					}
				}
			}()
		})

		throttle.mu.Lock()
		defer throttle.mu.Unlock()

		if throttle.tokens <= 0 {
			return throttle.lastSuccessfulRes, nil
		}

		throttle.tokens--

		res, err := e(ctx, req)
		if err == nil {
			throttle.lastSuccessfulRes = res
		}

		return res, err
	}
}

// WithQueue wraps effector with throttle behavior.
// The Effector will be executed at most Maximum times for given Duration
// If the Effector is throttled the requests are put in a queue and invoked when tokens are available
func (throttle *Throttle[A, T]) WithQueue(e Effector[A, T]) Effector[A, T] {
	return func(ctx context.Context, req A) (T, error) {
		if ctx.Err() != nil {
			var zeroT T
			return zeroT, ctx.Err()
		}

		throttle.once.Do(func() {
			ticker := time.NewTicker(throttle.duration)

			go func() {
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						throttle.mu.Lock()
						throttle.tokens = min(throttle.tokens+throttle.refill, throttle.maximum)
						throttle.mu.Unlock()
					}
				}
			}()
		})

		throttle.mu.Lock()
		defer throttle.mu.Unlock()

		if throttle.tokens <= 0 {
			throttle.queue.Enqueue(req)
			var zeroT T
			return zeroT, tooManyCalls
		}

		throttle.tokens--

		if throttle.queue.IsEmpty() {
			return e(ctx, req)
		}

		reqFromQueue, err := throttle.queue.Dequeue()
		if err != nil {
			return e(ctx, req)
		}

		throttle.queue.Enqueue(req)
		return e(ctx, reqFromQueue)
	}
}
