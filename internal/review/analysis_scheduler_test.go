package review

import (
	"context"
	"testing"
	"time"
)

func TestAnalysisScheduler_SingleStartPerLoad(t *testing.T) {
	scheduler := newAnalysisScheduler(nil, nil)
	scheduler.BeginLoadLifecycle()

	subCtx, started := scheduler.TryStart(context.Background())
	if !started {
		t.Fatal("expected first start in lifecycle to succeed")
	}

	if _, startedAgain := scheduler.TryStart(context.Background()); startedAgain {
		t.Fatal("expected second start in same lifecycle to be ignored")
	}

	scheduler.BeginLoadLifecycle()

	select {
	case <-subCtx.Done():
		// expected: new lifecycle cancels prior run context
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected previous lifecycle context to be canceled")
	}

	if _, startedNewLoad := scheduler.TryStart(context.Background()); !startedNewLoad {
		t.Fatal("expected start to succeed for new lifecycle")
	}
}

func TestAnalysisScheduler_DynamicHashDeferForLargeFolder(t *testing.T) {
	srv := NewServer()

	srv.progressMu.Lock()
	srv.progressTotal = 0
	srv.progressMu.Unlock()

	if srv.currentHashDeferPolicy() {
		t.Fatal("expected hash defer to be disabled when discovered total is 0")
	}

	srv.progressMu.Lock()
	srv.progressTotal = hashDeferThreshold + 1
	srv.progressMu.Unlock()

	if !srv.currentHashDeferPolicy() {
		t.Fatalf("expected hash defer to be enabled when discovered total exceeds %d", hashDeferThreshold)
	}
}

// TestAnalysisScheduler_CancelThenWaitReturnsPromptly locks in the P0-2 fix:
// when an analysis goroutine is started, then blocked on an empty queue, the
// Cancel() → Wait() sequence (used by ResetAppCache) must return within a
// bounded deadline — otherwise the UI freezes.
//
// Without the queue's WakeAndCancel hook, workers idle on cond.Wait never see
// ctx.Done(), so runBackgroundAnalysis's inner wg.Wait() never returns, so
// Done() is never called, so scheduler.Wait() hangs.
func TestAnalysisScheduler_CancelThenWaitReturnsPromptly(t *testing.T) {
	q := NewAnalysisQueue()
	scheduler := newAnalysisScheduler(q.WakeAndCancel, q.Reset)
	scheduler.BeginLoadLifecycle()

	ctx, started := scheduler.TryStart(context.Background())
	if !started {
		t.Fatal("expected TryStart to succeed")
	}

	// Simulate runBackgroundAnalysis: starts workers stuck on the empty queue,
	// then exits when the worker loop returns. Done() is deferred off the same
	// goroutine so Wait() can only return once the worker has actually exited.
	go func() {
		defer scheduler.Done()
		// One worker pop that will block until cancelled.
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if _, _, ok := q.PopWithTierPreference([]queueTier{tierInteractive, tierWarm, tierBulk}); !ok {
					return
				}
			}
		}
	}()

	// Give the worker a moment to enter cond.Wait().
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	scheduler.Cancel()
	scheduler.Wait()
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("Cancel+Wait took %v, expected < 500ms (UI would freeze)", elapsed)
	}
}

// TestAnalysisScheduler_CancelIsIdempotent ensures multiple Cancel calls are
// safe (ResetAppCache + Quit may both call Cancel).
func TestAnalysisScheduler_CancelIsIdempotent(t *testing.T) {
	q := NewAnalysisQueue()
	scheduler := newAnalysisScheduler(q.WakeAndCancel, q.Reset)
	scheduler.BeginLoadLifecycle()
	_, _ = scheduler.TryStart(context.Background())

	scheduler.Cancel()
	scheduler.Cancel() // must not panic
	scheduler.Cancel() // must not panic
}
