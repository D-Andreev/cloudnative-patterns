package fanout

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestREADMEUsage(t *testing.T) {
	fo := NewFanOut[int]()
	source := make(chan int)

	dests := fo.Split(source, 3)

	var (
		mu       sync.Mutex
		received []int
		wg       sync.WaitGroup
	)

	for _, dest := range dests {
		wg.Add(1)
		go func(dest Destination[int]) {
			defer wg.Done()
			for v := range dest {
				mu.Lock()
				received = append(received, v)
				mu.Unlock()
			}
		}(dest)
	}

	for i := range 10 {
		source <- i
	}
	close(source)

	wg.Wait()
	assert.ElementsMatch(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, received)
}

func TestFanOut(t *testing.T) {
	fanOut := NewFanOut[int]()
	source := make(chan int)
	dests := fanOut.Split(source, 3)
	require.Len(t, dests, 3)
	var (
		mu       sync.Mutex
		received []int
		wg       sync.WaitGroup
	)
	for _, dest := range dests {
		wg.Add(1)
		go func(dest Destination[int]) {
			defer wg.Done()
			for v := range dest {
				mu.Lock()
				received = append(received, v)
				mu.Unlock()
			}
		}(dest)
	}
	for i := range 10 {
		source <- i
	}
	close(source)
	wg.Wait()
	assert.Len(t, received, 10)
	assert.ElementsMatch(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, received)
}
