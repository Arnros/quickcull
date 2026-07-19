package review

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"quickcull/internal/utils"
)

func (s *Server) startBackgroundAnalysis(ctx context.Context) {
	subCtx, started := s.analysisSched.TryStart(ctx)
	if !started {
		return
	}

	utils.SafeGo(func() {
		defer s.analysisSched.Done()
		s.runBackgroundAnalysis(subCtx)
	})
}

func (s *Server) runBackgroundAnalysis(ctx context.Context) {
	state := s.getState()
	if state == nil {
		return
	}
	startedAt := time.Now()

	numCompute, numIOWorkers := s.computePolicy()
	computeSem := make(chan struct{}, numCompute)

	var hashDeferred atomic.Bool
	hashDeferred.Store(s.currentHashDeferPolicy())
	s.setAnalysisPerf(numIOWorkers, hashDeferred.Load())

	issueCollector := newAnalysisIssueCollector(2)
	stopIssueCollection := setActiveAnalysisIssueCollector(issueCollector)
	defer stopIssueCollection()

	s.startGCWatchdog(ctx)

	var wg sync.WaitGroup
	processed := 0
	var progressMu sync.Mutex
	lastEmitAt := time.Now()

	processors := DefaultProcessors()

	slog.Info("BackgroundAnalysis: starting",
		"io_workers", numIOWorkers,
		"compute_sem", numCompute,
		"hash_deferred", hashDeferred.Load(),
	)

	// Start IO workers
	s.runAnalysisWorkerLoop(ctx, numIOWorkers, computeSem, processors, &hashDeferred, &progressMu, &processed, &lastEmitAt, &wg)

	wg.Wait()

	result := "completed"
	if ctx.Err() != nil {
		result = "cancelled"
	}
	s.progressMu.RLock()
	total := s.progressTotal
	s.progressMu.RUnlock()
	logAnalysisLifecycleSummary(analysisLifecycleSummary{
		Result:         result,
		Duration:       time.Since(startedAt),
		Processed:      processed,
		Total:          total,
		IOWorkers:      numIOWorkers,
		ComputeWorkers: numCompute,
		HashDeferred:   hashDeferred.Load(),
	})

	// Final Telemetry & Issues
	s.logAnalysisSummary(processed, issueCollector)

	// Notify completion
	time.Sleep(50 * time.Millisecond)
	s.broadcast("analysis:complete", map[string]any{
		"total": processed,
	})

	s.logFinalMemoryStats(processed)
}

func (s *Server) logAnalysisSummary(processed int, collector *analysisIssueCollector) {
	interactive, warm, bulk := s.analysisQueue.DepthByTier()
	promotions, lastReady, medianReady, activeMs, idleMs := s.schedulerTelemetrySnapshot()
	collector.LogSummary()
	utils.LogAnalysis("BackgroundAnalysis: complete",
		"processed", processed,
		"queue_interactive", interactive,
		"queue_warm", warm,
		"queue_bulk", bulk,
		"nav_promotion_total", promotions,
		"last_view_ready_ms", lastReady,
		"p50_view_ready_ms", medianReady,
		"active_mode_ms", activeMs,
		"idle_mode_ms", idleMs,
	)
}

func (s *Server) logFinalMemoryStats(processed int) {
	s.progressMu.RLock()
	finalTotal := s.progressTotal
	s.progressMu.RUnlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	utils.LogAnalysis("Background analysis complete",
		"processed", processed,
		"total", finalTotal,
		"heap_alloc_mb", m.HeapAlloc/1024/1024,
		"sys_mb", m.Sys/1024/1024,
		"num_gc", m.NumGC)
}

func (s *Server) analysisProgress() (current int, total int, thumbCur int, thumbTotal int) {
	s.progressMu.RLock()
	defer s.progressMu.RUnlock()
	return s.progressCur, s.progressTotal, s.thumbCur, s.thumbTotal
}
