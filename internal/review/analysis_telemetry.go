package review

import (
	"context"
	"log/slog"
	"quickcull/internal/utils"
	"runtime"
	"sort"
	"time"
)

func (s *Server) startGCWatchdog(ctx context.Context) {
	// Periodic GC and memory logging for large libraries
	utils.SafeGo(func() {
		ticker := time.NewTicker(gcWatchdogInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > highMemoryThreshold {
					slog.Info("High memory detected, forcing GC", "heap_mb", m.HeapAlloc/1024/1024)
					runtime.GC()
				}
			}
		}
	})
}

func (s *Server) emitProgress(currentProcessed int, force bool, lastEmitAt *time.Time) {
	now := time.Now()
	if !force && now.Sub(*lastEmitAt) < progressEmitInterval {
		return
	}
	*lastEmitAt = now

	s.progressMu.RLock()
	currentTotal := s.progressTotal
	s.progressMu.RUnlock()
	promotions, lastReady, medianReady, activeMs, idleMs := s.schedulerTelemetrySnapshot()
	mode := s.schedulerMode()

	s.broadcast("progress", map[string]any{
		"current":                currentProcessed,
		"total":                  currentTotal,
		"nav_promotion_total":    promotions,
		"view_ready_latency":     lastReady,
		"view_ready_latency_p50": medianReady,
		"scheduler_mode":         mode,
		"active_mode_ms":         activeMs,
		"idle_mode_ms":           idleMs,
	})
}

func (s *Server) schedulerTelemetrySnapshot() (promotionTotal uint64, lastReadyMs int64, p50ReadyMs int64, activeModeMs int64, idleModeMs int64) {
	activeModeMs, idleModeMs = s.schedulerModeDurationsSnapshot()
	return s.navPromotionTotal.Load(), s.lastViewReadyMs.Load(), s.viewReadyP50(), activeModeMs, idleModeMs
}

func (s *Server) recordViewReadySample(latency int64) {
	if latency <= 0 {
		return
	}
	s.viewReadyMu.Lock()
	defer s.viewReadyMu.Unlock()
	s.viewReadySamples[s.viewReadyWrite] = latency
	s.viewReadyWrite = (s.viewReadyWrite + 1) % viewReadySampleWindow
	if s.viewReadyCount < viewReadySampleWindow {
		s.viewReadyCount++
	}
}

func (s *Server) viewReadyP50() int64 {
	s.viewReadyMu.Lock()
	if s.viewReadyCount == 0 {
		s.viewReadyMu.Unlock()
		return 0
	}
	count := s.viewReadyCount
	cp := make([]int64, count)
	for i := 0; i < count; i++ {
		idx := (s.viewReadyWrite - count + i + viewReadySampleWindow) % viewReadySampleWindow
		cp[i] = s.viewReadySamples[idx]
	}
	s.viewReadyMu.Unlock()
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	return cp[len(cp)/2]
}

func (s *Server) schedulerModeDurationsSnapshot() (activeMs int64, idleMs int64) {
	nowMs := time.Now().UnixMilli()
	s.schedulerModeMu.Lock()
	defer s.schedulerModeMu.Unlock()

	activeMs = s.schedulerActiveMs
	idleMs = s.schedulerIdleMs
	if s.schedulerModeName == "" || s.schedulerModeSince <= 0 {
		return
	}

	elapsed := nowMs - s.schedulerModeSince
	if elapsed <= 0 {
		return
	}

	switch s.schedulerModeName {
	case schedulerModeActive:
		activeMs += elapsed
	case schedulerModeIdle:
		idleMs += elapsed
	}
	return
}

func (s *Server) recordCacheGC(metaRemoved, hashRemoved, derivedRemoved int) {
	if metaRemoved == 0 && hashRemoved == 0 && derivedRemoved == 0 {
		return
	}
	s.perfMu.Lock()
	s.cacheMetaGC += metaRemoved
	s.cacheHashGC += hashRemoved
	s.cacheDerivedGC += derivedRemoved
	s.perfMu.Unlock()
}
