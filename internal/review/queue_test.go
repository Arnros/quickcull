package review

import (
	"testing"
	"time"
)

func TestAnalysisQueuePopBlocksOnEmpty(t *testing.T) {
	q := NewAnalysisQueue()

	done := make(chan struct{})
	go func() {
		_, _, ok := q.Pop()
		if ok {
			t.Errorf("expected ok=false after Close()")
		}
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Pop should have blocked on empty queue")
	case <-time.After(50 * time.Millisecond):
		// Success: it's blocking
	}

	q.Close()

	select {
	case <-done:
		// Success: unblocked after Close
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Pop failed to unblock after Close")
	}
}

func TestAnalysisQueuePushPopPriority(t *testing.T) {
	q := NewAnalysisQueue()
	q.Push(1, 10)
	q.Push(2, 50)
	q.Push(3, 20)

	got, _, ok := q.Pop()
	if !ok || got != 2 {
		t.Fatalf("expected highest-priority index 2, got %d (ok=%v)", got, ok)
	}
}

func TestAnalysisQueue_PopReturnsPriority(t *testing.T) {
	q := NewAnalysisQueue()
	q.Push(5, 95)
	idx, priority, ok := q.Pop()
	if !ok || idx != 5 || priority != 95 {
		t.Fatalf("got idx=%d priority=%d ok=%v", idx, priority, ok)
	}
}

func TestAnalysisQueueHasUrgentTaskEmpty(t *testing.T) {
	q := NewAnalysisQueue()
	if q.HasUrgentTask() {
		t.Fatal("expected no urgent item in empty queue")
	}
}

func TestAnalysisQueueHasUrgentTaskLowPriority(t *testing.T) {
	q := NewAnalysisQueue()
	q.Push(1, 50)
	if q.HasUrgentTask() {
		t.Fatal("expected no urgent item below priority 90")
	}
}

func TestAnalysisQueueHasUrgentTaskHighPriority(t *testing.T) {
	q := NewAnalysisQueue()
	q.Push(1, 90)
	if !q.HasUrgentTask() {
		t.Fatal("expected urgent item at priority 90")
	}
}

func TestAnalysisQueue_DepthByTier(t *testing.T) {
	q := NewAnalysisQueue()
	q.Push(0, 95) // interactive
	q.Push(1, 60) // warm
	q.Push(2, 10) // bulk
	interactive, warm, bulk := q.DepthByTier()
	if interactive != 1 || warm != 1 || bulk != 1 {
		t.Fatalf("got (%d,%d,%d) want (1,1,1)", interactive, warm, bulk)
	}
}

func TestAnalysisQueue_DecayBoostedPriorities(t *testing.T) {
	q := NewAnalysisQueue()
	q.Push(1, 95) // interactive (boosted)
	q.Push(2, 60) // warm (boosted)
	q.Push(3, 10) // bulk (not boosted)

	// Force staleness for boosted entries.
	q.mu.Lock()
	q.set[1].boostedAtMs = time.Now().Add(-3 * time.Second).UnixMilli()
	q.set[2].boostedAtMs = time.Now().Add(-3 * time.Second).UnixMilli()
	q.mu.Unlock()

	decayed := q.DecayBoostedPriorities(1 * time.Second)
	if decayed != 2 {
		t.Fatalf("expected 2 decayed items, got %d", decayed)
	}

	_, p1, ok := q.Pop()
	if !ok {
		t.Fatal("expected first pop")
	}
	if p1 != 10 {
		t.Fatalf("expected bulk priority first after decay, got %d", p1)
	}

	_, p2, ok := q.Pop()
	if !ok {
		t.Fatal("expected second pop")
	}
	if p2 != 0 {
		t.Fatalf("expected decayed priority 0, got %d", p2)
	}
}

func TestAnalysisQueue_DecayBoostedPrioritiesKeepsFreshItems(t *testing.T) {
	q := NewAnalysisQueue()
	q.Push(1, 95) // boosted and fresh

	decayed := q.DecayBoostedPriorities(5 * time.Second)
	if decayed != 0 {
		t.Fatalf("expected no decay for fresh item, got %d", decayed)
	}
	_, priority, ok := q.Pop()
	if !ok {
		t.Fatal("expected pop")
	}
	if priority != 95 {
		t.Fatalf("expected priority unchanged, got %d", priority)
	}
}

func TestAnalysisQueue_PopWithTierPreference(t *testing.T) {
	q := NewAnalysisQueue()
	q.Push(1, 95) // interactive
	q.Push(2, 60) // warm
	q.Push(3, 10) // bulk

	idx, priority, ok := q.PopWithTierPreference([]queueTier{tierWarm, tierInteractive, tierBulk})
	if !ok {
		t.Fatal("expected pop ok")
	}
	if idx != 2 || priority != 60 {
		t.Fatalf("expected warm item first, got idx=%d priority=%d", idx, priority)
	}
}

func TestAnalysisQueue_PopWithTierPreferenceFallback(t *testing.T) {
	q := NewAnalysisQueue()
	q.Push(1, 95) // interactive

	idx, priority, ok := q.PopWithTierPreference([]queueTier{tierBulk, tierWarm})
	if !ok {
		t.Fatal("expected pop ok")
	}
	// No bulk/warm available: falls back to global highest (interactive).
	if idx != 1 || priority != 95 {
		t.Fatalf("expected fallback to interactive, got idx=%d priority=%d", idx, priority)
	}
}

func TestAnalysisQueue_PopWithTierPreferenceOnClosedQueue(t *testing.T) {
	q := NewAnalysisQueue()
	q.Close()
	_, _, ok := q.PopWithTierPreference([]queueTier{tierInteractive, tierWarm, tierBulk})
	if ok {
		t.Fatal("expected ok=false on closed empty queue")
	}
}
