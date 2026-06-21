package fanin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
