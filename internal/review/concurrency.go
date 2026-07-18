package review

import (
	"hash/fnv"
	"sync"
)

// numShards is the number of per-file mutex buckets.
// Power-of-two enables a cheap bitmask modulo. 1024 shards cost ~8 KB of
// memory while reducing lock contention to near zero for typical workloads.
const numShards = 1024

// fileShards bounds memory by using a fixed number of mutexes instead of one per file.
var fileShards [numShards]sync.Mutex

// getFileLock returns a mutex for a specific file path.
func getFileLock(path string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(path))
	return &fileShards[h.Sum32()%numShards]
}
