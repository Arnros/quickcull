package review

import (
	"context"
	"testing"
	"time"
)

func TestAnalysisScheduler_SingleStartPerLoad(t *testing.T) {
	scheduler := newAnalysisScheduler()
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
