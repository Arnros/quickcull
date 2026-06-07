package review

import (
	"context"
	"hash/fnv"
	"runtime"
	"sync"

	"golang.org/x/sync/semaphore"
)

// semaphoreCPUFraction is the fraction of CPU cores reserved for background analysis.
// Keeping 25% free prevents background work from making the UI feel sluggish.
const semaphoreCPUFraction = 0.75

// numShards is the number of per-file mutex buckets.
// Power-of-two enables a cheap bitmask modulo. 1024 shards cost ~8 KB of
// memory while reducing lock contention to near zero for typical workloads.
const numShards = 1024

var (
	// fileShards bounds memory by using a fixed number of mutexes instead of one per file.
	fileShards [numShards]sync.Mutex

	// globalSemaphore limits the number of concurrent background operations.
	globalSemaphore *semaphore.Weighted
)

func init() {
	ncpu := runtime.NumCPU()
	limit := int64(float64(ncpu) * semaphoreCPUFraction)
	if limit < 1 {
		limit = 1
	}
	globalSemaphore = semaphore.NewWeighted(limit)
}

// getFileLock returns a mutex for a specific file path.
func getFileLock(path string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(path))
	return &fileShards[h.Sum32()%numShards]
}

// AcquireWorker acquires a worker slot from the global pool.
func AcquireWorker(ctx context.Context) error {
	return globalSemaphore.Acquire(ctx, 1)
}

// ReleaseWorker releases a worker slot back to the global pool.
func ReleaseWorker() {
	globalSemaphore.Release(1)
}
