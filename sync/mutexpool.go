package sync

import (
	"sync"
	"sync/atomic"
)

// RefMutex represents a mutex with reference counting.
type RefMutex struct {
	Mutex    sync.Mutex
	refCount int32
}

// MutexPool manages a collection of RefMutex instances indexed by key.
type MutexPool[K comparable] struct {
	mu    sync.RWMutex
	locks map[K]*RefMutex
}

// NewMutexPool creates a new Pool.
func NewMutexPool[K comparable]() *MutexPool[K] {
	return &MutexPool[K]{
		locks: make(map[K]*RefMutex),
	}
}

// getOrAdd returns the RefMutex for the given key, creating it if necessary.
// Increases reference count atomically.
func (p *MutexPool[K]) getOrAdd(key K) *RefMutex {
	p.mu.RLock()
	if rm, ok := p.locks[key]; ok {
		p.mu.RUnlock()
		atomic.AddInt32(&rm.refCount, 1)
		return rm
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if rm, ok := p.locks[key]; ok {
		atomic.AddInt32(&rm.refCount, 1)
		return rm
	}

	rm := &RefMutex{refCount: 1}
	p.locks[key] = rm
	return rm
}

// release decreases the reference count and removes the mutex if zero.
func (p *MutexPool[K]) release(key K, rm *RefMutex) {
	newCount := atomic.AddInt32(&rm.refCount, -1)
	if newCount > 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if current, exists := p.locks[key]; !exists || current != rm || atomic.LoadInt32(&current.refCount) > 0 {
		return
	}

	delete(p.locks, key)
}

// WithLock acquires the mutex associated with the given key and executes fn.
func (p *MutexPool[K]) WithLock(key K, fn func()) {
	if fn == nil {
		return
	}

	rm := p.getOrAdd(key)
	rm.Mutex.Lock()
	defer rm.Mutex.Unlock()
	defer p.release(key, rm)

	fn()
}
