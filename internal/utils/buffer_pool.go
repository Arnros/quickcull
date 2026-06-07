package utils

import (
	"sync"
)

// BufferPool provides a pool of byte slices to reduce GC pressure.
// It uses sync.Pool to reuse buffers for I/O operations.
type BufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates a new pool with buffers of the given size.
func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, size)
				return &b
			},
		},
	}
}

// Get returns a byte slice from the pool.
func (p *BufferPool) Get() []byte {
	return *(p.pool.Get().(*[]byte))
}

// Put returns a byte slice to the pool.
func (p *BufferPool) Put(b []byte) {
	p.pool.Put(&b)
}

var (
	// DefaultBufferPool is a pool for 32KB buffers, suitable for most I/O.
	DefaultBufferPool = NewBufferPool(32 * 1024)

	// LargeBufferPool is a pool for 1MB buffers, suitable for larger image processing tasks.
	LargeBufferPool = NewBufferPool(1 * 1024 * 1024)
)
