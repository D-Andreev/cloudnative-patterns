package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueueIsEmpty(t *testing.T) {
	var q Queue[int]
	assert.True(t, q.IsEmpty())

	q.Enqueue(1)
	assert.False(t, q.IsEmpty())
}

func TestQueueEnqueueAndSize(t *testing.T) {
	var q Queue[string]

	assert.Equal(t, 0, q.Size())

	q.Enqueue("a")
	q.Enqueue("b")

	assert.Equal(t, 2, q.Size())
}

func TestQueueDequeueFIFO(t *testing.T) {
	var q Queue[int]

	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)

	v1, err := q.Dequeue()
	require.NoError(t, err)
	assert.Equal(t, 1, v1)

	v2, err := q.Dequeue()
	require.NoError(t, err)
	assert.Equal(t, 2, v2)

	assert.Equal(t, 1, q.Size())

	v3, err := q.Dequeue()
	require.NoError(t, err)
	assert.Equal(t, 3, v3)
	assert.True(t, q.IsEmpty())
}

func TestQueueDequeueEmpty(t *testing.T) {
	var q Queue[int]

	v, err := q.Dequeue()
	require.Error(t, err)
	assert.Equal(t, "Queue is empty", err.Error())
	assert.Equal(t, 0, v)
}

func TestQueueDequeueEmptiesQueue(t *testing.T) {
	var q Queue[string]
	q.Enqueue("only")

	v, err := q.Dequeue()
	require.NoError(t, err)
	assert.Equal(t, "only", v)
	assert.True(t, q.IsEmpty())
	assert.Equal(t, 0, q.Size())

	_, err = q.Dequeue()
	require.Error(t, err)
}
