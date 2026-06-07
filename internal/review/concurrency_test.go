package review

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPrioritizeRange_FilmstripWindowGetsWarmPriority(t *testing.T) {
	root, err := os.MkdirTemp("", "quickcull-prirange-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	state := NewState(root, nil)
	for i := 0; i < 50; i++ {
		_ = os.WriteFile(filepath.Join(root, fmt.Sprintf("img_%02d.jpg", i)), []byte("fake-jpeg"), 0644)
		state.AddFile(fmt.Sprintf("img_%02d.jpg", i))
	}
	state.SortFiles()

	srv := NewServer()
	srv.stateMu.Lock()
	srv.state = state
	srv.stateMu.Unlock()
	srv.analysisQueue.Reset()

	// Seed all at bulk priority.
	for i := 0; i < 50; i++ {
		srv.analysisQueue.Push(i, 5)
	}

	center := 25
	filmstripStart := 10
	filmstripEnd := 41
	srv.PrioritizeRange(center, filmstripStart, filmstripEnd)

	// Center must be interactive tier.
	idx, priority, ok := srv.analysisQueue.Pop()
	if !ok {
		t.Fatal("expected item in queue")
	}
	if idx != center {
		t.Fatalf("expected center %d to be highest priority, got idx=%d", center, idx)
	}
	if priority != priorityCenter {
		t.Fatalf("center priority: got %d want %d", priority, priorityCenter)
	}

	// Close queue so drain loop terminates.
	srv.analysisQueue.Close()

	// Drain remaining items and verify filmstrip window items are >= warm min.
	for {
		i, p, ok := srv.analysisQueue.Pop()
		if !ok {
			break
		}
		if i >= filmstripStart && i < filmstripEnd && p < priorityWarmMin {
			t.Errorf("filmstrip item %d has bulk priority %d (want >= %d)", i, p, priorityWarmMin)
		}
	}
}

func TestPrioritizeRange_UpdatesSchedulerTelemetry(t *testing.T) {
	root, err := os.MkdirTemp("", "quickcull-prirange-metrics-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	state := NewState(root, nil)
	for i := 0; i < 30; i++ {
		_ = os.WriteFile(filepath.Join(root, fmt.Sprintf("img_%02d.jpg", i)), []byte("fake-jpeg"), 0644)
		state.AddFile(fmt.Sprintf("img_%02d.jpg", i))
	}
	state.SortFiles()

	srv := NewServer()
	srv.stateMu.Lock()
	srv.state = state
	srv.stateMu.Unlock()
	srv.analysisQueue.Reset()

	center := 12
	srv.PrioritizeRange(center, center-filmstripDefaultRadius, center+filmstripDefaultRadius+1)

	promotions, lastReady, p50, activeMs, idleMs := srv.schedulerTelemetrySnapshot()
	if promotions == 0 {
		t.Fatal("expected nav_promotion_total to be > 0")
	}
	if lastReady != 0 {
		t.Fatalf("expected last_view_ready_ms to start at 0, got %d", lastReady)
	}
	if p50 != 0 {
		t.Fatalf("expected p50_view_ready_ms to start at 0, got %d", p50)
	}
	if activeMs != 0 || idleMs != 0 {
		t.Fatalf("expected mode durations to start at 0, got active=%d idle=%d", activeMs, idleMs)
	}

	if _, ok := srv.consumePromotionTimestamp(center); !ok {
		t.Fatal("expected center promotion timestamp to be tracked")
	}
}

func TestRecordViewReady_InteractiveOnly(t *testing.T) {
	srv := NewServer()
	index := 7
	srv.navPromotedIndices.Store(index, time.Now().Add(-15*time.Millisecond).UnixNano())

	// Bulk-tier item should not consume/update latency.
	srv.recordViewReady(index, priorityWarmMin-1)
	if _, ok := srv.consumePromotionTimestamp(index); !ok {
		t.Fatal("expected promotion timestamp to remain for bulk-tier item")
	}

	// Re-seed and use interactive priority: should consume and set latency.
	srv.navPromotedIndices.Store(index, time.Now().Add(-20*time.Millisecond).UnixNano())
	srv.recordViewReady(index, priorityCenter)
	if _, ok := srv.consumePromotionTimestamp(index); ok {
		t.Fatal("expected promotion timestamp to be consumed for interactive item")
	}
	_, lastReady, p50, _, _ := srv.schedulerTelemetrySnapshot()
	if lastReady <= 0 {
		t.Fatalf("expected positive last_view_ready_ms, got %d", lastReady)
	}
	if p50 <= 0 {
		t.Fatalf("expected positive p50_view_ready_ms, got %d", p50)
	}
}

func TestSchedulerTierPreference_ActiveAndIdle(t *testing.T) {
	srv := NewServer()

	active := srv.schedulerTierPreference(schedulerModeActive, 0)
	if len(active) != 3 || active[0] != tierInteractive {
		t.Fatalf("active slot 0 should prefer interactive, got %v", active)
	}

	activeWarm := srv.schedulerTierPreference(schedulerModeActive, 13)
	if len(activeWarm) != 3 || activeWarm[0] != tierWarm {
		t.Fatalf("active slot 13 should prefer warm, got %v", activeWarm)
	}

	idle := srv.schedulerTierPreference(schedulerModeIdle, 19)
	if len(idle) != 3 || idle[0] != tierBulk {
		t.Fatalf("idle slot 19 should prefer bulk, got %v", idle)
	}
}

func TestSchedulerMode_UsesActivityAndProgress(t *testing.T) {
	srv := NewServer()

	if mode := srv.schedulerMode(); mode != "active" {
		t.Fatalf("expected active mode on startup, got %q", mode)
	}

	srv.progressMu.Lock()
	srv.progressCur = 100
	srv.progressTotal = 100
	srv.progressMu.Unlock()
	if mode := srv.schedulerMode(); mode != "idle" {
		t.Fatalf("expected idle mode after completion without activity, got %q", mode)
	}

	srv.lastUIActivity.Store(time.Now().UnixNano())
	if mode := srv.schedulerMode(); mode != "active" {
		t.Fatalf("expected active mode with recent UI activity, got %q", mode)
	}
}

func TestUIActiveWindow_AdaptsToNavigationVelocity(t *testing.T) {
	srv := NewServer()

	if got := srv.uiActiveWindow(); got != uiActiveWindowBase {
		t.Fatalf("expected base window %v, got %v", uiActiveWindowBase, got)
	}

	base := time.Now()
	srv.noteNavigationActivity(base)
	srv.noteNavigationActivity(base.Add(250 * time.Millisecond)) // ~4 photos/s
	fast := srv.uiActiveWindow()
	if fast <= uiActiveWindowBase {
		t.Fatalf("expected fast navigation window > base (%v), got %v", uiActiveWindowBase, fast)
	}

	srv.noteNavigationActivity(base.Add(250 * time.Millisecond).Add(3 * time.Second)) // ~0.33 photos/s
	slow := srv.uiActiveWindow()
	if slow >= fast {
		t.Fatalf("expected slow navigation window < fast window, got slow=%v fast=%v", slow, fast)
	}
	if slow < uiActiveWindowMin || slow > uiActiveWindowMax {
		t.Fatalf("expected slow window within bounds [%v,%v], got %v", uiActiveWindowMin, uiActiveWindowMax, slow)
	}
}

func TestViewReadyP50_MedianWindow(t *testing.T) {
	srv := NewServer()
	samples := []int64{30, 10, 20, 40, 50}
	for _, v := range samples {
		srv.recordViewReadySample(v)
	}
	if got := srv.viewReadyP50(); got != 30 {
		t.Fatalf("expected median 30, got %d", got)
	}
}

func TestSchedulerModeDurations_AccumulateOnTransitions(t *testing.T) {
	srv := NewServer()

	srv.noteSchedulerMode("active")
	time.Sleep(5 * time.Millisecond)
	srv.noteSchedulerMode("idle")
	time.Sleep(5 * time.Millisecond)
	srv.noteSchedulerMode("active")

	activeMs, idleMs := srv.schedulerModeDurationsSnapshot()
	if activeMs <= 0 {
		t.Fatalf("expected active duration > 0, got %d", activeMs)
	}
	if idleMs <= 0 {
		t.Fatalf("expected idle duration > 0, got %d", idleMs)
	}
}

func TestConsumePromotionTimestamp_ConcurrentWorkers(t *testing.T) {
	srv := NewServer()
	index := 42
	srv.navPromotedIndices.Store(index, time.Now().UnixNano())

	const workers = 8
	results := make([]bool, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, ok := srv.consumePromotionTimestamp(index)
			results[n] = ok
		}(i)
	}
	wg.Wait()

	consumed := 0
	for _, ok := range results {
		if ok {
			consumed++
		}
	}
	if consumed != 1 {
		t.Fatalf("expected exactly 1 worker to consume the timestamp, got %d", consumed)
	}
}

func TestAnalysisYielding(t *testing.T) {
	t.Skip("Superseded by Viewport Attention Stream logic")
	// Create a temporary directory with fake photos.
	root, err := os.MkdirTemp("", "quickcull-yield-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	for i := 0; i < 50; i++ {
		_ = os.WriteFile(filepath.Join(root, fmt.Sprintf("img_%02d.jpg", i)), []byte("fake-jpeg"), 0644)
	}

	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}

	state := srv.getState()
	if state == nil {
		t.Fatal("expected non-nil state")
	}

	// Seed the queue with low-priority background tasks.
	for i := 0; i < state.Len(); i++ {
		srv.analysisQueue.Push(i, 0)
	}

	// Simulate UI priority task: user jumps to photo 45.
	srv.PrioritizeRange(45, 45-filmstripDefaultRadius, 45+filmstripDefaultRadius+1)

	if !srv.analysisQueue.HasUrgentTask() {
		t.Fatal("expected urgent task in queue after prioritization")
	}

	idx, _, ok := srv.analysisQueue.Pop()
	if !ok {
		t.Fatal("expected at least one task in queue")
	}
	if idx != 45 {
		t.Fatalf("expected prioritized index 45 first, got %d", idx)
	}
}
