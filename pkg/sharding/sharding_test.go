package sharding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestREADMEUsage(t *testing.T) {
	sm := NewShardedMap[string, int](8)

	sm.Set("user:42", 100)
	assert.Equal(t, 100, sm.Get("user:42"))
	assert.True(t, sm.Contains("user:42"))

	sm.Delete("user:42")
	assert.False(t, sm.Contains("user:42"))
	assert.Equal(t, 0, sm.Get("user:42"))
}

func TestGetSetDeleteContains(t *testing.T) {
	sm := NewShardedMap[string, string](4)

	sm.Set("a", "alpha")
	sm.Set("b", "beta")

	assert.Equal(t, "alpha", sm.Get("a"))
	assert.Equal(t, "beta", sm.Get("b"))
	assert.True(t, sm.Contains("a"))
	assert.True(t, sm.Contains("b"))
	assert.False(t, sm.Contains("missing"))

	sm.Delete("a")
	assert.False(t, sm.Contains("a"))
	assert.Equal(t, "", sm.Get("a"))
	assert.True(t, sm.Contains("b"))
}

func TestGetMissingKeyReturnsZeroValue(t *testing.T) {
	sm := NewShardedMap[int, int](2)

	assert.Equal(t, 0, sm.Get(99))
	assert.False(t, sm.Contains(99))
}

func TestSetOverwritesExistingValue(t *testing.T) {
	sm := NewShardedMap[string, int](4)

	sm.Set("key", 1)
	sm.Set("key", 2)

	assert.Equal(t, 2, sm.Get("key"))
	assert.True(t, sm.Contains("key"))
}

func TestDeleteMissingKeyIsNoOp(t *testing.T) {
	sm := NewShardedMap[string, int](4)

	sm.Set("present", 1)
	sm.Delete("absent")

	assert.True(t, sm.Contains("present"))
	assert.Equal(t, 1, sm.Get("present"))
}
