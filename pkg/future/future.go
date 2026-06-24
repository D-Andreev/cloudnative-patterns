// Future provides a placeholder for a value that's not known yet
package future

import (
	"context"
	"sync"
)

// Future represents asynchronous work that may not have completed yet.
type Future[T any] interface {
	// Result blocks until the work finishes and returns its value or error.
	// Result is safe to call from multiple goroutines; the outcome is read once.
	Result() (T, error)
}

// Task performs work asynchronously and should respect ctx cancellation.
type Task[T any] func(ctx context.Context) (T, error)

type outcome[T any] struct {
	val T
	err error
}
type future[T any] struct {
	once sync.Once
	out  outcome[T]
	ch   <-chan outcome[T]
}

// Async runs task in a new goroutine and returns immediately.
// The task receives ctx; cancel ctx to stop work that respects it.
// If Result is never called, the goroutine still completes without leaking:
// the result channel is buffered.
func Async[T any](ctx context.Context, task Task[T]) Future[T] {
	ch := make(chan outcome[T], 1)
	go func() {
		val, err := task(ctx)
		ch <- outcome[T]{val: val, err: err}
	}()
	return &future[T]{ch: ch}
}

func (f *future[T]) Result() (T, error) {
	f.once.Do(func() {
		f.out = <-f.ch
	})
	return f.out.val, f.out.err
}
