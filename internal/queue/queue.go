package queue

import "errors"

// Queue data structure
type Queue[T any] []T

// Add element to the queue
func (q *Queue[T]) Enqueue(val T) {
	*q = append(*q, val)
}

// Checks if queue is empty
func (q *Queue[T]) IsEmpty() bool {
	return len(*q) == 0
}

// Removes an element from the queue
func (q *Queue[T]) Dequeue() (T, error) {
	if q.IsEmpty() {
		var zeroT T
		return zeroT, errors.New("Queue is empty")
	}

	value := (*q)[0]
	*q = (*q)[1:]

	return value, nil
}

// Get the size of the queue
func (q *Queue[T]) Size() int {
	return len(*q)
}
