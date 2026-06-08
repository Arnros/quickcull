package review

import (
	"container/heap"
	"log/slog"
	"sync"
	"time"
)

// Item is something we manage in a priority queue.
type Item struct {
	index       int   // The value of the item; arbitrary.
	priority    int   // The priority of the item in the queue.
	boostedAtMs int64 // last timestamp when item entered interactive/warm tier
	// The index is needed by update and is maintained by the heap.Interface methods.
	heapIndex int
	// bucketIndex is the position in the secondary bucket index for O(1) removal.
	bucketIndex int
}

// A PriorityQueue implements heap.Interface and holds Items.
type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return pq[i].priority > pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].heapIndex = i
	pq[j].heapIndex = j
}

func (pq *PriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*Item)
	item.heapIndex = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil      // avoid memory leak
	item.heapIndex = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// AnalysisQueue is a thread-safe wrapper around PriorityQueue.
type AnalysisQueue struct {
	mu       sync.Mutex
	pq       PriorityQueue
	set      map[int]*Item
	buckets  map[int][]*Item // secondary index by priority for O(1) tier access
	cond     *sync.Cond
	isClosed bool
}

type queueTier int

const (
	tierInteractive queueTier = iota
	tierWarm
	tierBulk
)

func NewAnalysisQueue() *AnalysisQueue {
	q := &AnalysisQueue{
		set:     make(map[int]*Item),
		buckets: make(map[int][]*Item),
	}
	q.cond = sync.NewCond(&q.mu)
	heap.Init(&q.pq)
	slog.Info("AnalysisQueue: created")
	return q
}

func (q *AnalysisQueue) Push(index int, priority int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.isClosed {
		return
	}

	if item, ok := q.set[index]; ok {
		if priority > item.priority {
			q.removeFromBucketLocked(item)
			item.priority = priority
			if priority >= priorityWarmMin {
				item.boostedAtMs = time.Now().UnixMilli()
			}
			heap.Fix(&q.pq, item.heapIndex)
			q.addToBucketLocked(item)
		}
	} else {
		item := &Item{index: index, priority: priority, bucketIndex: -1}
		if priority >= priorityWarmMin {
			item.boostedAtMs = time.Now().UnixMilli()
		}
		q.set[index] = item
		heap.Push(&q.pq, item)
		q.addToBucketLocked(item)
		q.cond.Signal()
	}
}

func (q *AnalysisQueue) addToBucketLocked(item *Item) {
	bucket := q.buckets[item.priority]
	item.bucketIndex = len(bucket)
	q.buckets[item.priority] = append(bucket, item)
}

func (q *AnalysisQueue) removeFromBucketLocked(item *Item) {
	if item.bucketIndex < 0 {
		return
	}
	bucket := q.buckets[item.priority]
	idx := item.bucketIndex
	lastIdx := len(bucket) - 1

	if idx != lastIdx {
		// Swap with last item to keep removal O(1)
		lastItem := bucket[lastIdx]
		bucket[idx] = lastItem
		lastItem.bucketIndex = idx
	}

	q.buckets[item.priority] = bucket[:lastIdx]
	item.bucketIndex = -1
}

func (q *AnalysisQueue) Pop() (index int, priority int, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.pq) == 0 && !q.isClosed {
		q.cond.Wait()
	}

	if len(q.pq) == 0 {
		return 0, 0, false
	}

	item := heap.Pop(&q.pq).(*Item)
	q.removeFromBucketLocked(item)
	delete(q.set, item.index)
	return item.index, item.priority, true
}

// PopWithTierPreference pops an item using tier preference order, falling back
// to highest-priority global pop when no preferred tier is available.
func (q *AnalysisQueue) PopWithTierPreference(preferred []queueTier) (index int, priority int, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.pq) == 0 && !q.isClosed {
		q.cond.Wait()
	}
	if len(q.pq) == 0 {
		return 0, 0, false
	}

	for _, tier := range preferred {
		if item, found := q.popTierLocked(tier); found {
			return item.index, item.priority, true
		}
	}

	// Fallback to global highest priority if no preferred tier item found
	item := heap.Pop(&q.pq).(*Item)
	q.removeFromBucketLocked(item)
	delete(q.set, item.index)
	return item.index, item.priority, true
}

// DepthByTier returns the number of queued items in each priority tier.
func (q *AnalysisQueue) DepthByTier() (interactive, warm, bulk int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.pq {
		switch {
		case item.priority >= priorityInteractiveMin:
			interactive++
		case item.priority >= priorityWarmMin:
			warm++
		default:
			bulk++
		}
	}
	return
}

// DecayBoostedPriorities demotes stale warm/interactive items to bulk priority.
// Returns the number of items decayed.
func (q *AnalysisQueue) DecayBoostedPriorities(maxAge time.Duration) int {
	if maxAge <= 0 {
		return 0
	}
	nowMs := time.Now().UnixMilli()
	maxAgeMs := maxAge.Milliseconds()

	q.mu.Lock()
	defer q.mu.Unlock()

	decayed := 0
	toDecay := make([]*Item, 0, 8)
	for _, item := range q.pq {
		if item.priority < priorityWarmMin || item.boostedAtMs <= 0 {
			continue
		}
		if nowMs-item.boostedAtMs <= maxAgeMs {
			continue
		}
		toDecay = append(toDecay, item)
	}

	for _, item := range toDecay {
		q.removeFromBucketLocked(item)
		item.priority = 0
		item.boostedAtMs = 0
		heap.Fix(&q.pq, item.heapIndex)
		q.addToBucketLocked(item)
		decayed++
	}
	return decayed
}

// popTierLocked finds the highest-priority item in the requested tier
// using the bucketed secondary index. O(p) where p is the priority range
// (typically 0-100), avoiding the O(n) linear scan.
func (q *AnalysisQueue) popTierLocked(tier queueTier) (*Item, bool) {
	min, max := q.tierRange(tier)

	// Scan buckets from highest priority in tier down to min.
	for p := max; p >= min; p-- {
		bucket := q.buckets[p]
		if len(bucket) == 0 {
			continue
		}

		// Take the last item (O(1) pop from slice)
		item := bucket[len(bucket)-1]
		q.buckets[p] = bucket[:len(bucket)-1]
		item.bucketIndex = -1

		// Remove from heap (O(log n))
		heap.Remove(&q.pq, item.heapIndex)
		delete(q.set, item.index)
		return item, true
	}

	return nil, false
}

func (q *AnalysisQueue) tierRange(tier queueTier) (min, max int) {
	switch tier {
	case tierInteractive:
		return priorityInteractiveMin, 1000 // use high enough max
	case tierWarm:
		return priorityWarmMin, priorityInteractiveMin - 1
	case tierBulk:
		return 0, priorityWarmMin - 1
	default:
		return 1, 0 // empty range
	}
}

func (q *AnalysisQueue) HasUrgentTask() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pq) > 0 && q.pq[0].priority >= priorityInteractiveMin
}

func (q *AnalysisQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.isClosed = true
	q.cond.Broadcast()
	slog.Info("AnalysisQueue: closed", "count", len(q.pq))
}

func (q *AnalysisQueue) Reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	slog.Info("AnalysisQueue: reset", "count", len(q.pq))
	q.isClosed = false
	q.pq = PriorityQueue{}
	q.set = make(map[int]*Item)
	q.buckets = make(map[int][]*Item)
	heap.Init(&q.pq)
}
