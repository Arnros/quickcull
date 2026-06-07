package review

import (
	"log/slog"
	"quickcull/internal/utils"
	"runtime"
	"time"
)

// computePolicy determines the number of compute and IO workers.
func (s *Server) computePolicy() (numCompute int, numIOWorkers int) {
	// Mid-range compute concurrency for speed while preventing OOM on massive libraries
	numCompute = runtime.NumCPU() * computeCPUNumerator / computeCPUDenominator
	if numCompute < 1 {
		numCompute = 1
	}

	// Limit IO workers more aggressively to prevent memory spikes on large libraries
	numIOWorkers = runtime.NumCPU() * ioWorkerMultiplier
	if numIOWorkers > ioWorkersMax {
		numIOWorkers = ioWorkersMax
	}
	if numIOWorkers < ioWorkersMin {
		numIOWorkers = ioWorkersMin
	}
	return
}

func (s *Server) schedulerMode() schedulerModeType {
	// Startup/ongoing discovery stays interactive-biased.
	s.progressMu.RLock()
	current := s.progressCur
	total := s.progressTotal
	s.progressMu.RUnlock()
	if total == 0 || current < total {
		s.noteSchedulerMode(schedulerModeActive)
		return schedulerModeActive
	}

	// Navigation/viewport activity keeps active policy briefly.
	last := s.lastUIActivity.Load()
	if last <= 0 {
		s.noteSchedulerMode(schedulerModeIdle)
		return schedulerModeIdle
	}
	if time.Since(time.Unix(0, last)) <= s.uiActiveWindow() {
		s.noteSchedulerMode(schedulerModeActive)
		return schedulerModeActive
	}
	s.noteSchedulerMode(schedulerModeIdle)
	return schedulerModeIdle
}

func (s *Server) schedulerTierPreference(mode schedulerModeType, slot uint64) []queueTier {
	var primary queueTier
	pos := int(slot % schedulerSlotCycle)
	switch mode {
	case schedulerModeActive:
		// 60 / 25 / 15
		switch {
		case pos < 12:
			primary = tierInteractive
		case pos < 17:
			primary = tierWarm
		default:
			primary = tierBulk
		}
	default:
		// idle: shift capacity toward bulk => 20 / 20 / 60
		switch {
		case pos < 4:
			primary = tierInteractive
		case pos < 8:
			primary = tierWarm
		default:
			primary = tierBulk
		}
	}
	return tierFallbackOrder(primary)
}

func tierFallbackOrder(primary queueTier) []queueTier {
	switch primary {
	case tierInteractive:
		return []queueTier{tierInteractive, tierWarm, tierBulk}
	case tierWarm:
		return []queueTier{tierWarm, tierInteractive, tierBulk}
	default:
		return []queueTier{tierBulk, tierWarm, tierInteractive}
	}
}

func (s *Server) uiActiveWindow() time.Duration {
	s.schedulerModeMu.Lock()
	ewma := s.navVelocityEWMA
	s.schedulerModeMu.Unlock()

	if ewma <= 0 {
		return uiActiveWindowBase
	}

	speed := ewma
	if speed < uiVelocityLow {
		speed = uiVelocityLow
	}
	if speed > uiVelocityHigh {
		speed = uiVelocityHigh
	}
	norm := (speed - uiVelocityLow) / (uiVelocityHigh - uiVelocityLow)
	window := float64(uiActiveWindowMin) + norm*float64(uiActiveWindowMax-uiActiveWindowMin)
	if window < float64(uiActiveWindowMin) {
		return uiActiveWindowMin
	}
	if window > float64(uiActiveWindowMax) {
		return uiActiveWindowMax
	}
	return time.Duration(window)
}

func (s *Server) noteSchedulerMode(mode schedulerModeType) {
	nowMs := time.Now().UnixMilli()
	s.schedulerModeMu.Lock()
	defer s.schedulerModeMu.Unlock()

	if s.schedulerModeName == "" {
		s.schedulerModeName = mode
		s.schedulerModeSince = nowMs
		return
	}
	if s.schedulerModeName == mode {
		return
	}

	elapsed := nowMs - s.schedulerModeSince
	if elapsed > 0 {
		switch s.schedulerModeName {
		case schedulerModeActive:
			s.schedulerActiveMs += elapsed
		case schedulerModeIdle:
			s.schedulerIdleMs += elapsed
		}
	}

	s.schedulerModeName = mode
	s.schedulerModeSince = nowMs
}

func (s *Server) noteNavigationActivity(now time.Time) {
	nowNs := now.UnixNano()
	s.lastUIActivity.Store(nowNs)

	s.schedulerModeMu.Lock()
	defer s.schedulerModeMu.Unlock()

	if s.lastNavAtNs > 0 {
		delta := float64(nowNs-s.lastNavAtNs) / float64(time.Second)
		if delta > 0 {
			instant := 1.0 / delta
			if s.navVelocityEWMA <= 0 {
				s.navVelocityEWMA = instant
			} else {
				s.navVelocityEWMA = uiVelocityAlpha*instant + (1-uiVelocityAlpha)*s.navVelocityEWMA
			}
		}
	}
	s.lastNavAtNs = nowNs
}

func (s *Server) setAnalysisPerf(ioWorkers int, hashDeferred bool) {
	s.perfMu.Lock()
	s.lastIOWorkers = ioWorkers
	s.hashDeferred = hashDeferred
	s.perfMu.Unlock()
}

func (s *Server) perfSnapshot() (ioWorkers int, hashDeferred bool, cacheMetaGC int, cacheHashGC int, cacheDerivedGC int) {
	s.perfMu.RLock()
	defer s.perfMu.RUnlock()
	return s.lastIOWorkers, s.hashDeferred, s.cacheMetaGC, s.cacheHashGC, s.cacheDerivedGC
}

// PrioritizeRange promotes center + neighbors to the interactive tier and the
// filmstrip window [filmstripStart, filmstripEnd) to the warm tier.
func (s *Server) PrioritizeRange(center, filmstripStart, filmstripEnd int) {
	utils.LogNav("PrioritizeRange: start", "center", center, "start", filmstripStart, "end", filmstripEnd)
	state := s.getState()
	if state == nil {
		return
	}
	s.noteNavigationActivity(time.Now())
	total := state.Len()

	promoted := 0

	// Interactive tier: center + immediate neighbors
	s.analysisQueue.Push(center, priorityCenter)
	promoted++
	for i := 1; i <= priorityNeighborRadius; i++ {
		if center+i < total {
			s.analysisQueue.Push(center+i, priorityNeighborBase-i)
			promoted++
		}
		if center-i >= 0 {
			s.analysisQueue.Push(center-i, priorityNeighborBase-i)
			promoted++
		}
	}

	// Warm tier: rest of filmstrip window
	for i := filmstripStart; i < filmstripEnd && i < total; i++ {
		if i < 0 {
			continue
		}
		if i >= center-priorityNeighborRadius && i <= center+priorityNeighborRadius {
			continue // already promoted above
		}
		s.analysisQueue.Push(i, priorityFilmstripWarm)
		promoted++
	}
	s.recordPromotion(center, promoted)

	interactive, warm, bulk := s.analysisQueue.DepthByTier()
	totalPromotions, _, _, _, _ := s.schedulerTelemetrySnapshot()
	slog.Debug("PrioritizeRange", "center", center,
		"filmstrip_start", filmstripStart, "filmstrip_end", filmstripEnd,
		"promoted_count", promoted,
		"nav_promotion_total", totalPromotions,
		"queue_interactive", interactive, "queue_warm", warm, "queue_bulk", bulk)
}

func (s *Server) recordPromotion(center, promoted int) {
	if promoted <= 0 {
		return
	}
	s.navPromotionTotal.Add(uint64(promoted))
	s.navPromotedIndices.Store(center, time.Now().UnixNano())
}
