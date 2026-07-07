package review

import (
	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"quickcull/internal/utils"
	"sort"
)

func (s *Server) applyEvent(ev bus.Event) (bool, bus.Event, error) {
	// Snapshot physical state data BEFORE acquiring appStateMu to avoid lock
	// inversion with RefreshVisibleOrder (state.mu → appStateMu).
	// Also re-acquires state fresh to avoid stale pointer (was captured too early).
	isStruct := isStructuralChange(ev)
	_, isUndo := ev.Payload.(bus.CommandUndoPayload)
	var filesSnapshot []string

	if isStruct && !isUndo {
		state := s.getState()
		if state != nil {
			state.mu.RLock()
			filesSnapshot = make([]string, len(state.files))
			copy(filesSnapshot, state.files)
			state.mu.RUnlock()
		}
	}

	s.appStateMu.Lock()

	if s.appState == nil {
		s.appStateMu.Unlock()
		return false, bus.Event{}, domain.ErrFolderNotFound
	}

	// 1. Reduce state (returns a heap-allocated *AppState)
	nextState, appliedEvent, err := Reduce(s.appState, ev)
	if err != nil {
		s.appStateMu.Unlock()
		utils.LogError("Reducer failed to apply event", "error", err, "type", ev.Type)
		return false, bus.Event{}, err
	}

	// Undo with empty history is a no-op: do not emit/broadcast fake success.
	if _, isUndo := ev.Payload.(bus.CommandUndoPayload); isUndo && appliedEvent.Type == "" {
		s.appStateMu.Unlock()
		return false, bus.Event{}, domain.ErrNothingToUndo
	}

	// 2. For structural changes, re-sync VisibleOrder from the physical state snapshot.
	if isStruct && len(filesSnapshot) > 0 {
		nextState.VisibleOrder = filesSnapshot
	}

	// 3. Commit the new state pointer
	s.appState = nextState
	s.appStateMu.Unlock()

	// Execute physical undo restores OUTSIDE appStateMu to avoid deadlock
	// with RefreshVisibleOrder. RestoreFromTrashAt internally acquires state.mu.Lock().
	s.undoPhysicalRestores(ev, appliedEvent)

	// Persist changes outside the lock.
	s.persistStateChanges(ev, nextState)

	s.Bus.Publish(bus.Event{
		Type:    bus.TypeStateUpdated,
		Payload: bus.StateUpdatedPayload{State: *nextState},
	})

	// Batch metadata updates (star/label/rotate) do not map cleanly to one photo delta.
	// Emit a full state sync so the UI refreshes all changed tiles.
	if payload, ok := appliedEvent.Payload.(bus.CommandBatchPayload); ok && !batchHasStructuralEvents(payload.Events) {
		s.SyncFullState()
		return true, appliedEvent, nil
	}

	// ResetMetadata touches every photo in the collection: send full state so the UI
	// refreshes all photo-level metadata (stars, labels) in addition to global counts.
	if _, ok := appliedEvent.Payload.(bus.CommandResetMetadataPayload); ok {
		s.SyncFullState()
		return true, appliedEvent, nil
	}

	// INTELLIGENT BROADCAST:
	// If it's a simple metadata change, send a DELTA to the UI to save bandwidth.
	// If it's a structural change (Undo, Trash), send the FULL state to ensure consistency.
	if !isStruct {
		if s.broadcastDelta(appliedEvent, *nextState) {
			return true, appliedEvent, nil
		}
	}

	// For structural changes, send SyncState with IsPartial=true and include only affected photos
	affected := affectedPhotosFromEvent(appliedEvent)
	s.broadcastAppStateSelective(nextState, true, affected)

	// For undo of structural changes, files were restored after the logical undo.
	// RefreshVisibleOrder re-syncs VisibleOrder from the now-restored physical state.
	if isUndo {
		s.RefreshVisibleOrder()
	}

	return true, appliedEvent, nil
}

// undoPhysicalRestores restores trashed files for undo operations.
// Must be called outside appStateMu because RestoreFromTrashAt
// internally acquires state.mu.Lock(), which must always be acquired
// before appStateMu (consistent with RefreshVisibleOrder).
func (s *Server) undoPhysicalRestores(ev bus.Event, appliedEvent bus.Event) {
	_, isUndo := ev.Payload.(bus.CommandUndoPayload)
	if !isUndo {
		return
	}

	state := s.getState()
	if state == nil {
		return
	}

	switch p := appliedEvent.Payload.(type) {
	case bus.CommandTrashPhotoPayload:
		utils.LogCore("Undo: Restoring file from trash", "photo", p.PhotoID, "originalIndex", p.OriginalIndex)
		if err := state.RestoreFromTrashAt(p.PhotoID, p.OriginalIndex); err != nil {
			utils.LogWarn("Undo: RestoreFromTrashAt failed", "photo", p.PhotoID, "error", err)
		}
	case bus.CommandBatchPayload:
		type restore struct {
			photoID string
			idx     int
		}
		var toRestore []restore
		for _, subEvent := range p.Events {
			if trashPayload, ok := subEvent.Payload.(bus.CommandTrashPhotoPayload); ok {
				toRestore = append(toRestore, restore{trashPayload.PhotoID, trashPayload.OriginalIndex})
			}
		}
		sort.Slice(toRestore, func(i, j int) bool { return toRestore[i].idx < toRestore[j].idx })
		for _, r := range toRestore {
			utils.LogCore("Undo: Restoring file from trash (batch)", "photo", r.photoID, "originalIndex", r.idx)
			if err := state.RestoreFromTrashAt(r.photoID, r.idx); err != nil {
				utils.LogWarn("Undo: RestoreFromTrashAt failed", "photo", r.photoID, "error", err)
			}
		}
	}
}

func isStructuralChange(ev bus.Event) bool {
	switch ev.Type {
	case bus.TypeCommandTrashPhoto, bus.TypeCommandUndo:
		return true
	case bus.TypeCommandBatch:
		if p, ok := ev.Payload.(bus.CommandBatchPayload); ok {
			return batchHasStructuralEvents(p.Events)
		}
	}
	return false
}

func batchHasStructuralEvents(events []bus.Event) bool {
	for _, e := range events {
		if e.Type == bus.TypeCommandTrashPhoto {
			return true
		}
	}
	return false
}

// affectedPhotosFromEvent extracts all photo IDs affected by a given event or batch event.
func affectedPhotosFromEvent(ev bus.Event) []string {
	var paths []string
	switch p := ev.Payload.(type) {
	case bus.CommandBatchPayload:
		for _, sub := range p.Events {
			if id := photoIDFromPayload(sub.Payload); id != "" {
				paths = append(paths, id)
			}
		}
	default:
		if id := photoIDFromPayload(ev.Payload); id != "" {
			paths = append(paths, id)
		}
	}
	return paths
}
