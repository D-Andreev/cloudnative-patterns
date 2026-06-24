package fanin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestREADMEUsage(t *testing.T) {
	fi := NewFanIn[int]()

	sourceA := make(chan int)
	sourceB := make(chan int)

	go func() {
		defer close(sourceA)
		sourceA <- 1
		sourceA <- 2
	}()

	go func() {
		defer close(sourceB)
		sourceB <- 10
		sourceB <- 20
	}()

	dest := fi.Funnel(sourceA, sourceB)

	var got []int
	for v := range dest {
		got = append(got, v)
	}

	assert.ElementsMatch(t, []int{1, 2, 10, 20}, got)
}

func TestREADMEFunnelSliceSpread(t *testing.T) {
	fi := NewFanIn[int]()

	sourceA := make(chan int)
	sourceB := make(chan int)

	go func() {
		defer close(sourceA)
		sourceA <- 1
		sourceA <- 2
	}()

	go func() {
		defer close(sourceB)
		sourceB <- 10
		sourceB <- 20
	}()

	sources := []Source[int]{sourceA, sourceB}
	dest := fi.Funnel(sources...)

	var got []int
	for v := range dest {
		got = append(got, v)
	}

	assert.ElementsMatch(t, []int{1, 2, 10, 20}, got)
}

func TestFanIn(t *testing.T) {
	fanIn := NewFanIn[int]()
	var sources []Source[int]
	expectedOutput := []int{
		1, 1, 1,
		2, 2, 2,
		3, 3, 3,
		4, 4, 4,
		5, 5, 5,
	}

	for range 3 {
		ch := make(chan int)
		sources = append(sources, ch)

		go func() {
			defer close(ch)

			for i := 1; i <= 5; i++ {
				ch <- i
				time.Sleep(time.Second)
			}
		}()
	}

	dest := fanIn.Funnel(sources...)
	i := 0
	for d := range dest {
		assert.Equal(t, expectedOutput[i], d)
		i++
	}
}
