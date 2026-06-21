// Fan-out evenly distributes messages from an input channel to multiple output channels
package fanout

// The Source channel type, receive only
type Source[T any] <-chan T

// The Destination channel type, receive only
type Destination[T any] <-chan T

// FanOut struct
type FanOut[T any] struct{}

// NewFanOut returns FanOut struct
func NewFanOut[T any]() *FanOut[T] {
	return &FanOut[T]{}
}

// A function that accepts source channel and immediately returns Destinations.
// Any input from Source will be output to Destination.
func (fanOut *FanOut[T]) Split(source Source[T], n int) []Destination[T] {
	var dests []Destination[T]

	for range n {
		ch := make(chan T)
		dests = append(dests, ch)

		go func() {
			defer close(ch)

			for val := range source {
				ch <- val
			}
		}()
	}

	return dests
}
