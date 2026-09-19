package runtime

import (
	"sync"
)

// BoundedMap is a thread-safe map with a fixed capacity and FIFO eviction.
type BoundedMap[K comparable, V any] struct {
	mu       sync.RWMutex
	capacity int
	entries  map[K]V
	order    []K
}

// NewBoundedMap creates a new BoundedMap with the specified maximum capacity.
func NewBoundedMap[K comparable, V any](capacity int) *BoundedMap[K, V] {
	if capacity <= 0 {
		capacity = 256
	}
	return &BoundedMap[K, V]{
		capacity: capacity,
		entries:  make(map[K]V, capacity),
		order:    make([]K, 0, capacity),
	}
}

// Get retrieves a value by key.
func (m *BoundedMap[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.entries[key]
	return val, ok
}

// Has checks whether a key is present in the map.
func (m *BoundedMap[K, V]) Has(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.entries[key]
	return ok
}

// Set inserts or updates a key-value pair, evicting the oldest key if capacity is reached.
func (m *BoundedMap[K, V]) Set(key K, val V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[key]; exists {
		m.entries[key] = val
		return
	}

	if len(m.entries) >= m.capacity {
		if len(m.order) > 0 {
			oldest := m.order[0]
			m.order = m.order[1:]
			delete(m.entries, oldest)
		}
	}

	m.entries[key] = val
	m.order = append(m.order, key)
}

// Delete removes a key from the map.
func (m *BoundedMap[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[key]; !exists {
		return
	}

	delete(m.entries, key)
	for i, k := range m.order {
		if k == key {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

// Clear removes all entries.
func (m *BoundedMap[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[K]V, m.capacity)
	m.order = m.order[:0]
}

// Len returns the current number of items.
func (m *BoundedMap[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}
