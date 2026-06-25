// Sharding splits a large map into multiple partitions, each with its own
// read/write lock, to reduce lock contention when many goroutines access
// different keys concurrently.
package sharding

import (
	"hash/fnv"
	"reflect"
	"sync"
)

// Shard is one partition of a ShardedMap. Each shard owns a map and serializes
// access to it with an embedded RWMutex.
type Shard[K comparable, V any] struct {
	sync.RWMutex
	items map[K]V
}

// ShardedMap routes keys to shards using an FNV-1a hash of the key's string
// representation. Operations only lock the shard that owns the key.
type ShardedMap[K comparable, V any] []*Shard[K, V]

// NewShardedMap creates a map split across nShards partitions.
func NewShardedMap[K comparable, V any](nShards int) ShardedMap[K, V] {
	shards := make([]*Shard[K, V], nShards)

	for i := range nShards {
		shard := make(map[K]V)
		shards[i] = &Shard[K, V]{items: shard}
	}

	return shards
}

func (m ShardedMap[K, V]) getShardIndex(key K) int {
	str := reflect.ValueOf(key).String()
	hash := fnv.New32a()
	hash.Write([]byte(str))
	sum := int(hash.Sum32())
	return sum % len(m)
}

func (m ShardedMap[K, V]) getShard(key K) *Shard[K, V] {
	index := m.getShardIndex(key)
	return m[index]
}

// Get returns the value stored under key. When key is absent, it returns the
// zero value of V.
func (m ShardedMap[K, V]) Get(key K) V {
	shard := m.getShard(key)
	shard.RLock()
	defer shard.RUnlock()

	return shard.items[key]
}

// Set stores value under key, replacing any previous value.
func (m ShardedMap[K, V]) Set(key K, value V) {
	shard := m.getShard(key)
	shard.Lock()
	defer shard.Unlock()

	shard.items[key] = value
}

// Delete removes key from the map. Missing keys are ignored.
func (m ShardedMap[K, V]) Delete(key K) {
	shard := m.getShard(key)
	shard.Lock()
	defer shard.Unlock()

	delete(shard.items, key)
}

// Contains reports whether key is present in the map.
func (m ShardedMap[K, V]) Contains(key K) bool {
	shard := m.getShard(key)
	shard.RLock()
	defer shard.RUnlock()

	_, ok := shard.items[key]
	return ok
}
