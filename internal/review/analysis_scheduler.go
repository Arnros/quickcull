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
}

func newAnalysisScheduler() *analysisScheduler {
	return &analysisScheduler{}
}

func (s *analysisScheduler) BeginLoadLifecycle() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		slog.Info("AnalysisScheduler: cancelling previous analysis for new load lifecycle", "loadID", s.currentLoadID)
		s.cancel()
		s.cancel = nil
	}

	s.currentLoadID++
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
	slog.Info("AnalysisScheduler: analysis started", "loadID", s.currentLoadID)

	return subCtx, true
}

// Cancel stops any in-progress analysis started by this scheduler.
func (s *analysisScheduler) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
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
