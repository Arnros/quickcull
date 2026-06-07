package review

import "os"

func (a *App) emitStateUpdate() {
	a.doEmit()
}

func (a *App) doEmit() {
	stats := a.snapshotStats()
	current, total, thumbCur, thumbTotal := a.server.analysisProgress()
	a.server.broadcast("state:update", StateUpdatePayload{
		Stats: stats,
		Analysis: ProgressCounts{
			Current: current,
			Total:   total,
		},
		Thumbnails: ProgressCounts{
			Current: thumbCur,
			Total:   thumbTotal,
		},
	})
}

// finalizeAction handles common cleanup and response generation after a structural change.
func (a *App) finalizeAction(state *State, newTotal int, targetIndex int, changedPaths []string) ActionResponse {
	for _, p := range changedPaths {
		a.server.cache.DropPath(p)
		a.dropDerivedCacheForSource(p)
	}
	a.server.invalidateBurstCache()

	// RefreshVisibleOrder internally calls s.broadcast("SyncState", ...).
	// This is the authoritative full state update for structural changes.
	a.finalizeStructuralChange()

	nextIndex := targetIndex
	if nextIndex >= newTotal {
		nextIndex = newTotal - 1
	}
	if nextIndex < 0 {
		nextIndex = 0
	}

	return ActionResponse{
		Stats: a.snapshotStats(),
		Total: newTotal,
		Index: nextIndex,
		Ok:    true,
	}
}

func (a *App) finalizeStructuralChange() {
	a.server.RefreshVisibleOrder()
	a.emitStateUpdate()
}

func (a *App) dropDerivedCacheForSource(src string) {
	if src == "" || a.server.cacheDir == "" {
		return
	}
	if thumbPath, err := ThumbCachePathForSource(src, a.server.cacheDir); err == nil {
		_ = os.Remove(thumbPath)
	}
	// We only remove the processed file if it exists. 
	// We DON'T call GetMetadata here because it would trigger a re-extraction 
	// which is exactly what we want to avoid during a cache drop.
}
