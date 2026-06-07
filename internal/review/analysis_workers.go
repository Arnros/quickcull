package review

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"quickcull/internal/utils"
)

func (s *Server) runAnalysisWorkerLoop(ctx context.Context, numIOWorkers int, computeSem chan struct{}, processors []MediaProcessor, hashDeferred *atomic.Bool, progressMu *sync.Mutex, processed *int, lastEmitAt *time.Time, wg *sync.WaitGroup) {
	for w := 0; w < numIOWorkers; w++ {
		wg.Add(1)
		utils.SafeGo(func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					if !s.processNextAnalysisTask(ctx, numIOWorkers, computeSem, processors, hashDeferred, progressMu, processed, lastEmitAt) {
						return
					}
				}
			}
		})
	}
}

func (s *Server) processNextAnalysisTask(ctx context.Context, numIOWorkers int, computeSem chan struct{}, processors []MediaProcessor, hashDeferred *atomic.Bool, progressMu *sync.Mutex, processed *int, lastEmitAt *time.Time) bool {
	slot := s.scheduleSlot.Add(1) - 1
	mode := s.schedulerMode()
	if slot%promotionDecayCheckEvery == 0 {
		decayed := s.analysisQueue.DecayBoostedPriorities(promotionDecayTTL)
		if decayed > 0 {
			slog.Debug("Scheduler: decayed stale boosted items", "count", decayed)
		}
	}
	idx, itemPriority, ok := s.analysisQueue.PopWithTierPreference(s.schedulerTierPreference(mode, slot))
	if !ok {
		return false
	}
	// Check if we should yield to urgent UI tasks before processing.
	if idx%urgentCheckEvery == 0 {
		if s.analysisQueue.HasUrgentTask() {
			s.analysisQueue.Push(idx, 0)
			time.Sleep(urgentYieldInterval)
			return true
		}
	}
	s.recordViewReady(idx, itemPriority)

	state := s.getState()
	if state == nil {
		return true
	}

	path, err := state.AbsPath(idx)
	if err == nil {
		currentHashDeferred := s.currentHashDeferPolicy()
		if currentHashDeferred != hashDeferred.Load() {
			hashDeferred.Store(currentHashDeferred)
			s.setAnalysisPerf(numIOWorkers, currentHashDeferred)
		}

		// --- Execute plugin-based processing ---
		pctx := &ProcessorContext{
			Cache:              s.cache,
			CacheDir:           s.cacheDir,
			ComputeSem:         computeSem,
			SkipBackgroundHash: currentHashDeferred,
			IsThumbnailEager:   true,
			SkipHeavyThumbnail: itemPriority < priorityWarmMin,
		}

		proc := GetProcessorFor(path, processors)
		if proc != nil {
			if err := proc.Process(ctx, path, pctx); err != nil {
				if !recordAnalysisIssue(analysisIssueProcessor, path, err) {
					slog.Debug("processor error", "path", path, "error", err)
				}
			}
		}

		// Bursts (photo-specific)
		if proc != nil {
			meta := s.cache.GetMetadata(path)
			if meta != nil && meta.Date != "" {
				if _, ok := s.burstCache.Load(idx); !ok {
					s.getBurstInfo(idx, path)
				}
			}

			// Live Duplicate Detection (if pHash was computed)
			if !currentHashDeferred {
				s.checkForLiveDuplicates(idx)
			}
		}
	}

	progressMu.Lock()
	*processed++
	currentProcessed := *processed
	progressMu.Unlock()

	s.progressMu.Lock()
	s.progressCur = currentProcessed
	currentTotal := s.progressTotal
	s.progressMu.Unlock()

	reportEvery := progressReportEvery
	if currentTotal > 0 && currentTotal <= smallFolderThreshold {
		reportEvery = progressReportEverySmall
	}
	if currentProcessed%reportEvery == 0 || currentProcessed >= currentTotal {
		s.emitProgress(currentProcessed, currentProcessed >= currentTotal, lastEmitAt)
	}

	return true
}

func (s *Server) recordViewReady(index int, itemPriority int) {
	if itemPriority < priorityInteractiveMin {
		return
	}
	ts, ok := s.consumePromotionTimestamp(index)
	if !ok {
		return
	}
	latency := time.Since(time.Unix(0, ts)).Milliseconds()
	if latency < 0 {
		return
	}
	s.lastViewReadyMs.Store(latency)
	s.recordViewReadySample(latency)
	slog.Debug("BackgroundAnalysis: view-ready",
		"index", index,
		"latency_ms", latency,
		"priority", itemPriority,
	)
}

func (s *Server) consumePromotionTimestamp(index int) (int64, bool) {
	v, ok := s.navPromotedIndices.LoadAndDelete(index)
	if !ok {
		return 0, false
	}
	ts, ok := v.(int64)
	if !ok {
		return 0, false
	}
	return ts, true
}

func (s *Server) checkForLiveDuplicates(idx int) {
	state := s.getState()
	if state == nil {
		return
	}

	checkEvery := dupCheckEvery
	if state.Len() > dupLargeLibraryThreshold {
		checkEvery = dupCheckEveryLarge
	}

	if idx%checkEvery != 0 {
		return
	}

	groups := s.cache.GetDuplicateGroups(state, dupSimilarityThreshold)
	if len(groups) == 0 {
		return
	}

	s.duplicateGroupsMu.Lock()
	oldLen := len(s.duplicateGroups)
	s.duplicateGroups = groups
	newLen := len(groups)
	s.duplicateGroupsMu.Unlock()

	// Only emit if we found more groups and throttled
	if newLen > oldLen {
		now := time.Now().UnixNano()
		last := s.lastDupEmit.Load()
		if now-last > int64(dupEmitCooldown) {
			s.lastDupEmit.Store(now)
			s.broadcast("duplicates:found", map[string]any{
				"count": newLen,
			})
		}
	}
}
