// Fan-in multiplexes multiple input channels onto one output channel.
package fanin

import "sync"

// The Source channel type, receive only
type Source[T any] <-chan T

// The Destination channel type
type Destination[T any] chan T

// FanIn struct
type FanIn[T any] struct{}

// NewFanIn returns FanIn struct
func NewFanIn[T any]() *FanIn[T] {
	return &FanIn[T]{}
}

// Funnel accepts N source channels and forwards the values to destination channel.
func (fanIn *FanIn[T]) Funnel(sources ...Source[T]) Destination[T] {
	dest := make(Destination[T])
	wg := sync.WaitGroup{}
	wg.Add(len(sources))

	for _, ch := range sources {
		go func(ch <-chan T) {
			defer wg.Done()
			for n := range ch {
				dest <- n
			}
		}(ch)
	}

	go func() {
		wg.Wait()
		close(dest)
	}()

	return dest
}
