package review

import (
	"quickcull/internal/utils"
	"time"
)

type syncSummary struct {
	Result            string
	Duration          time.Duration
	Photos            int
	Chunks            int
	FullMapSuppressed bool
}

func logSyncSummary(summary syncSummary) {
	utils.LogCore("SyncFullState: lifecycle summary",
		"result", summary.Result,
		"duration_ms", summary.Duration.Milliseconds(),
		"photos", summary.Photos,
		"chunks", summary.Chunks,
		"full_map_suppressed", summary.FullMapSuppressed,
	)
}

// SyncFullState broadcasts the state in chunks for large libraries to prevent OOM.
func (s *Server) SyncFullState() {
	startedAt := time.Now()
	result := "completed"
	photoCount := 0
	chunks := 0
	fullMapSuppressed := false
	defer func() {
		if r := recover(); r != nil {
			result = "failed"
			utils.LogError("SyncFullState panic", "error", r)
		}
		logSyncSummary(syncSummary{
			Result:            result,
			Duration:          time.Since(startedAt),
			Photos:            photoCount,
			Chunks:            chunks,
			FullMapSuppressed: fullMapSuppressed,
		})
	}()

	s.appStateMu.RLock()
	if s.appState == nil {
		s.appStateMu.RUnlock()
		return
	}

	photoCount = s.appState.photoCount()
	isLarge := photoCount > largeLibraryThreshold
	fullMapSuppressed = isLarge

	// 1. Initial authoritative sync (structural start)
	utils.LogCore("SyncFullState: starting delivery", "total", photoCount, "isLarge", isLarge)
	s.broadcastAppState(s.appState, true, false)

	if photoCount == 0 {
		s.appStateMu.RUnlock()
		return
	}

	// 2. Capture keys for chunked delivery
	keys := s.snapshotPhotoKeys()
	s.appStateMu.RUnlock()

	// 3. Stream photo metadata in chunks
	chunks = s.streamPhotoChunks(keys)

	// 4. Final authoritative update
	s.finalizeSync(isLarge)

	// Memory pressure is handled by the threshold-based watchdog. Forcing a
	// collection after every sync creates avoidable pauses during navigation.
	utils.LogCore("SyncFullState: delivery complete")
}

// snapshotPhotoKeys captures a stable list of photo IDs for chunked delivery.
// Must be called with appStateMu held (at least RLock).
func (s *Server) snapshotPhotoKeys() []string {
	keys := make([]string, 0, s.appState.photoCount())
	s.appState.rangePhotos(func(id string, _ Photo) bool {
		keys = append(keys, id)
		return true
	})
	return keys
}

// streamPhotoChunks delivers photo metadata in timed batches.
func (s *Server) streamPhotoChunks(keys []string) int {
	chunks := 0
	for i := 0; i < len(keys); i += syncChunkSize {
		end := i + syncChunkSize
		if end > len(keys) {
			end = len(keys)
		}

		// Build chunk under short lock
		chunk := make(map[string]Photo, end-i)
		s.appStateMu.RLock()
		if s.appState != nil {
			for _, k := range keys[i:end] {
				if p, ok := s.appState.photo(k); ok {
					chunk[k] = p
				}
			}
		}
		s.appStateMu.RUnlock()

		s.broadcast("sync:state:photos", map[string]any{
			"photos": chunk,
			"index":  i,
			"total":  len(keys),
			"isLast": end == len(keys),
		})
		chunks++

		time.Sleep(syncChunkInterval)
	}
	return chunks
}

// finalizeSync sends the final authoritative state update.
func (s *Server) finalizeSync(isLarge bool) {
	s.appStateMu.RLock()
	if s.appState != nil {
		if !isLarge {
			// Small library: send final full authoritative state
			s.broadcastAppState(s.appState, false, true)
			s.appStateMu.RUnlock()
		} else {
			// Large library: final structural confirmation (empty photos map)
			utils.LogCore("SyncFullState: library is large, final full map suppressed to prevent OOM")
			s.broadcastAppState(s.appState, true, false)
			s.appStateMu.RUnlock()
			// Must call RefreshVisibleOrder OUTSIDE appStateMu lock to avoid inversion with stateMu
			s.RefreshVisibleOrder()
		}
	} else {
		s.appStateMu.RUnlock()
	}
}

// RefreshVisibleOrder updates the immutable appState's VisibleOrder to match the current legacy state order.
func (s *Server) RefreshVisibleOrder() {
	state := s.getState()
	if state == nil {
		return
	}

	visibleOrder, trashedCount := state.SnapshotVisibleState()

	s.appStateMu.Lock()
	defer s.appStateMu.Unlock()

	if s.appState == nil {
		return
	}

	// Only the structural fields change here. Keep the immutable photo store and
	// history shared instead of cloning either collection on every navigation.
	next := *s.appState
	next.Photos = nil
	next.pendingPhotos = nil
	next.VisibleOrder = visibleOrder
	next.TrashedCount = trashedCount

	s.appState = &next

	// Standardize on SyncState with IsPartial=true
	s.broadcastAppState(s.appState, true, false)
}

// ReconcileScannedFiles makes the immutable application state authoritative for
// a freshly scanned file set, preserving metadata only for photos that remain.
func (s *Server) ReconcileScannedFiles(files []string) {
	state := s.getState()
	if state == nil {
		return
	}

	// TrashedCount acquires and releases state.mu internally. Complete that read
	// before taking appStateMu to preserve the stateMu -> appStateMu discipline.
	trashedCount := state.TrashedCount()

	s.appStateMu.Lock()
	if s.appState == nil {
		s.appStateMu.Unlock()
		return
	}

	// Reconciliation replaces the photo store below, so copying the current
	// visible order or materializing its photos would be wasted work.
	next := *s.appState
	next.Photos = nil
	next.pendingPhotos = nil
	next.IsPartial = false
	next.VisibleOrder = append([]string(nil), files...)
	photos := make(map[string]Photo, len(files))
	for _, id := range files {
		if photo, ok := s.appState.photo(id); ok {
			photos[id] = photo
		} else {
			photos[id] = Photo{ID: id}
		}
	}
	next.photos = newPhotoStore(photos)
	next.Photos = nil
	next.TrashedCount = trashedCount
	next.RecalculateCounts()
	s.appState = &next
	isLarge := next.photoCount() > largeLibraryThreshold
	s.appStateMu.Unlock()

	if isLarge {
		s.SyncFullState()
		return
	}
	snapshot := BuildSyncSnapshot(next, true)
	s.broadcast(eventSyncState, snapshot)
}

// broadcastAppState assembles and emits a SyncState payload.
func (s *Server) broadcastAppState(state *AppState, isPartial bool, includePhotos bool) {
	if state == nil {
		return
	}
	snapshot := BuildSyncSnapshot(*state, includePhotos)
	snapshot.IsPartial = isPartial
	s.broadcast(eventSyncState, snapshot)
}

// broadcastAppStateSelective assembles and emits a SyncState payload with selective photos metadata.
func (s *Server) broadcastAppStateSelective(state *AppState, isPartial bool, affectedPhotos []string) {
	if state == nil {
		return
	}
	snapshot := BuildSyncSnapshotSelective(*state, isPartial, affectedPhotos)
	s.broadcast(eventSyncState, snapshot)
}
