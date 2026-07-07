package review

import (
	"context"
	"log/slog"
	"sync"
)

type analysisScheduler struct {
	mu            sync.Mutex
	currentLoadID uint64
	startedLoadID uint64
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	onCancel      func() // unblocks queue waiters (WakeAndCancel)
	onReset       func() // re-arms the queue for a new load (Reset)
}

func newAnalysisScheduler(onCancel, onReset func()) *analysisScheduler {
	return &analysisScheduler{onCancel: onCancel, onReset: onReset}
}

func (s *analysisScheduler) BeginLoadLifecycle() {
	s.mu.Lock()
	if s.cancel != nil {
		slog.Info("AnalysisScheduler: cancelling previous analysis for new load lifecycle", "loadID", s.currentLoadID)
		s.cancel()
		s.cancel = nil
	}
	onReset := s.onReset
	s.currentLoadID++
	s.mu.Unlock()

	// Re-arm the queue outside the scheduler lock: Cancel() may be running
	// concurrently and would otherwise deadlock on the same lock.
	if onReset != nil {
		onReset()
	}
}

func (s *analysisScheduler) TryStart(ctx context.Context) (context.Context, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentLoadID == 0 {
		// Backward compatibility for callers that start analysis without
		// opening a lifecycle explicitly.
		s.currentLoadID = 1
	}
	if s.startedLoadID == s.currentLoadID {
		slog.Debug("AnalysisScheduler: analysis already started for this load", "loadID", s.currentLoadID)
		return nil, false
	}

	if s.cancel != nil {
		slog.Info("AnalysisScheduler: restarting analysis", "loadID", s.currentLoadID)
		s.cancel()
	}
	subCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.startedLoadID = s.currentLoadID
	s.wg.Add(1)
	slog.Info("AnalysisScheduler: analysis started", "loadID", s.currentLoadID)

	return subCtx, true
}

// Wait blocks until the analysis goroutine for the current load has exited.
func (s *analysisScheduler) Wait() {
	s.wg.Wait()
}

// Done decrements the scheduler WaitGroup. Called by the analysis goroutine
// when runBackgroundAnalysis returns.
func (s *analysisScheduler) Done() {
	s.wg.Done()
}

// Cancel stops any in-progress analysis started by this scheduler.
// Also wakes idle queue waiters so Pop returns ok=false promptly.
func (s *analysisScheduler) Cancel() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	onCancel := s.onCancel
	s.mu.Unlock()
	if onCancel != nil {
		onCancel()
	}
}

func shouldDeferHashForDiscoveredTotal(total int) bool {
	return total > hashDeferThreshold
}

func (s *Server) currentHashDeferPolicy() bool {
	s.progressMu.RLock()
	total := s.progressTotal
	s.progressMu.RUnlock()
	return shouldDeferHashForDiscoveredTotal(total)
}
