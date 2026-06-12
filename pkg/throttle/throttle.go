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

// Effector is the function executed by Throttle.
type Effector[T any] func(ctx context.Context) (T, error)

// Throttle executes an Effector Maximum times for Duration
type Throttle[T any] struct {
	maximum           uint
	duration          time.Duration
	refill            uint
	tokens            uint
	once              sync.Once
	mu                sync.Mutex
	lastSuccessfulRes T
	queue             queue.Queue[context.Context]
}

// NewThrottle validates settings and returns a Retry.
func NewThrottle[T any](settings Settings) (*Throttle[T], error) {
	validate := validator.New(validator.WithRequiredStructEnabled())
	err := validate.Struct(settings)
	if err != nil {
		return nil, err
	}
	return &Throttle[T]{
		maximum:  settings.Maximum,
		duration: settings.Duration,
		refill:   settings.Refill,
		tokens:   settings.Maximum,
	}, nil
}

// ThrottleFnWithError wraps effector with throttle behavior.
// The Effector will be executed at most Maximum times for given Duration
// If the Effector is throttled an error is returned
func (throttle *Throttle[T]) ThrottleFnWithError(e Effector[T]) Effector[T] {
	return func(ctx context.Context) (T, error) {
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

		return e(ctx)
	}
}

// ThrottleFnWithReplay wraps effector with throttle behavior.
// The Effector will be executed at most Maximum times for given Duration
// If the Effector is throttled the last successful response is returned
func (throttle *Throttle[T]) ThrottleFnWithReplay(e Effector[T]) Effector[T] {
	return func(ctx context.Context) (T, error) {
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

		res, err := e(ctx)
		if err == nil {
			throttle.lastSuccessfulRes = res
		}

		return res, err
	}
}

// ThrottleFnWithQueue wraps effector with throttle behavior.
// The Effector will be executed at most Maximum times for given Duration
// If the Effector is throttled the requests are put in a queue and invoked when tokens are available
func (throttle *Throttle[T]) ThrottleFnWithQueue(e Effector[T]) Effector[T] {
	return func(ctx context.Context) (T, error) {
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
			throttle.queue.Enqueue(ctx)
			var zeroT T
			return zeroT, tooManyCalls
		}

		throttle.tokens--

		if throttle.queue.IsEmpty() {
			return e(ctx)
		}

		ctxFromQueue, err := throttle.queue.Dequeue()
		if err != nil {
			// This should never happen, but if queue is empty just execute the current ctx
			return e(ctx)
		}

		throttle.queue.Enqueue(ctx)
		return e(ctxFromQueue)
	}
}
