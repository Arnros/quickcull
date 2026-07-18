package review

import (
	"quickcull/internal/utils"
	"runtime"
	"time"
)

// SyncFullState broadcasts the state in chunks for large libraries to prevent OOM.
func (s *Server) SyncFullState() {
	defer func() {
		if r := recover(); r != nil {
			utils.LogError("SyncFullState panic", "error", r)
		}
	}()

	s.appStateMu.RLock()
	if s.appState == nil {
		s.appStateMu.RUnlock()
		return
	}

	photoCount := len(s.appState.Photos)
	isLarge := photoCount > largeLibraryThreshold

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
	s.streamPhotoChunks(keys)

	// 4. Final authoritative update
	s.finalizeSync(isLarge)

	// 5. Cleanup
	runtime.GC()
	utils.LogCore("SyncFullState: delivery complete and memory flushed")
}

// snapshotPhotoKeys captures a stable list of photo IDs for chunked delivery.
// Must be called with appStateMu held (at least RLock).
func (s *Server) snapshotPhotoKeys() []string {
	keys := make([]string, 0, len(s.appState.Photos))
	for k := range s.appState.Photos {
		keys = append(keys, k)
	}
	return keys
}

// streamPhotoChunks delivers photo metadata in timed batches.
func (s *Server) streamPhotoChunks(keys []string) {
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
				if p, ok := s.appState.Photos[k]; ok {
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

		time.Sleep(syncChunkInterval)
	}
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

	next := s.appState.Clone(true)
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

	next := s.appState.Clone(false)
	next.IsPartial = false
	next.VisibleOrder = append([]string(nil), files...)
	next.Photos = make(map[string]Photo, len(files))
	for _, id := range files {
		if photo, ok := s.appState.Photos[id]; ok {
			next.Photos[id] = photo
		} else {
			next.Photos[id] = Photo{ID: id}
		}
	}
	next.TrashedCount = trashedCount
	next.RecalculateCounts()
	s.appState = &next
	isLarge := len(next.Photos) > largeLibraryThreshold
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
