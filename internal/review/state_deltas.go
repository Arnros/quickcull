package review

import (
	"encoding/json"
	"log/slog"
	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"quickcull/internal/persistence"
)

// Delta change-map keys included in SyncDelta payloads.
const (
	// deltaKeyIsStarred is the change-map key for the starred flag.
	deltaKeyIsStarred = "IsStarred"
	// deltaKeyLabel is the change-map key for the color label.
	deltaKeyLabel = "Label"
	// deltaKeyRotation is the change-map key for the rotation value.
	deltaKeyRotation = "Rotation"
	// deltaKeyStats is the change-map key that carries global count statistics.
	deltaKeyStats = "_stats"
)

// Stats sub-keys embedded under deltaKeyStats in SyncDelta payloads.
const (
	statsKeyStarred = "starred"
	statsKeyLabeled = "labeled"
	statsKeyTrashed = "trashed"
	statsKeyUndoLen = "undoLen"
)

// broadcastDelta sends a SyncDelta event for metadata-only changes.
// Returns true if a delta was broadcast, false if the caller should fall through to structural sync.
func (s *Server) broadcastDelta(appliedEvent bus.Event, nextState AppState) bool {
	var photoID string
	changes := make(map[string]any)

	switch p := appliedEvent.Payload.(type) {
	case bus.CommandToggleStarPayload:
		photoID = p.PhotoID
		changes[deltaKeyIsStarred] = nextState.Photos[photoID].IsStarred
	case bus.CommandLabelPhotoPayload:
		photoID = p.PhotoID
		changes[deltaKeyLabel] = nextState.Photos[photoID].Label
	case bus.CommandRotatePhotoPayload:
		photoID = p.PhotoID
		changes[deltaKeyRotation] = nextState.Photos[photoID].Rotation
	}

	if photoID == "" {
		return false
	}

	// Also sync the global counts in the delta.
	changes[deltaKeyStats] = map[string]int{
		statsKeyStarred: nextState.StarredCount,
		statsKeyLabeled: nextState.LabeledCount,
		statsKeyTrashed: nextState.TrashedCount,
		statsKeyUndoLen: len(nextState.History),
	}
	s.broadcast(eventSyncDelta, bus.StateDeltaPayload{
		PhotoID: photoID,
		Changes: changes,
	})
	return true
}

func photoIDFromPayload(payload any) string {
	switch p := payload.(type) {
	case bus.CommandTrashPhotoPayload:
		return p.PhotoID
	case bus.CommandToggleStarPayload:
		return p.PhotoID
	case bus.CommandLabelPhotoPayload:
		return p.PhotoID
	case bus.CommandRotatePhotoPayload:
		return p.PhotoID
	case map[string]any:
		if v, ok := p["PhotoID"].(string); ok {
			return v
		}
		if v, ok := p["photoID"].(string); ok {
			return v
		}
	}
	return ""
}

func (s *Server) persistStateChanges(ev bus.Event, state *AppState) {
	if s.persistence == nil || state == nil {
		return
	}

	folderID := domain.GetFolderID(state.Root)
	writer := s.asyncPersistenceWriter()

	switch payload := ev.Payload.(type) {
	case bus.CommandToggleStarPayload,
		bus.CommandLabelPhotoPayload,
		bus.CommandTrashPhotoPayload,
		bus.CommandRotatePhotoPayload:
		s.persistSinglePhoto(folderID, photoIDFromPayload(payload), state, writer)
	case bus.CommandBatchPayload:
		// Multiple photos changed: rebuild full folder metadata.
		s.persistFullFolder(folderID, state, writer)
	case bus.CommandUndoPayload:
		// For Undo, rebuild and save full folder metadata to stay consistent.
		s.persistFullFolder(folderID, state, writer)
	case bus.CommandResetMetadataPayload:
		s.persistFullFolder(folderID, state, writer)
	}

	// ALWAYS persist the entire undo history to DB.
	s.persistHistory(folderID, state, writer)
}

// persistSinglePhoto writes metadata for one photo to the store.
func (s *Server) persistSinglePhoto(folderID, photoID string, state *AppState, writer *asyncMetadataWriter) {
	p, ok := state.Photos[photoID]
	if !ok {
		return
	}
	meta := persistence.PhotoMetadata{
		IsStarred: p.IsStarred,
		Label:     p.Label,
		Rotation:  p.Rotation,
		IsTrashed: p.IsTrashed,
	}
	if writer != nil {
		writer.enqueueSingle(folderID, photoID, meta)
		return
	}
	if err := s.persistence.SavePhotoMetadata(folderID, photoID, meta); err != nil {
		slog.Error("Failed to persist photo metadata", "folder", folderID, "photo", photoID, "error", err)
	}
}

// persistFullFolder writes metadata for every photo in the state to the store.
func (s *Server) persistFullFolder(folderID string, state *AppState, writer *asyncMetadataWriter) {
	metadata := make(map[string]persistence.PhotoMetadata, len(state.Photos))
	for id, p := range state.Photos {
		metadata[id] = persistence.PhotoMetadata{
			IsStarred: p.IsStarred,
			Label:     p.Label,
			Rotation:  p.Rotation,
			IsTrashed: p.IsTrashed,
		}
	}
	if writer != nil {
		writer.enqueueFullFolder(folderID, metadata)
		return
	}
	if err := s.persistence.SaveFolderMetadata(folderID, metadata); err != nil {
		slog.Error("Failed to persist folder metadata after undo", "folder", folderID, "error", err)
	}
}

// persistHistory serializes and writes the undo history to the store.
func (s *Server) persistHistory(folderID string, state *AppState, writer *asyncMetadataWriter) {
	historyData, err := json.Marshal(state.History)
	if err != nil {
		slog.Error("Failed to marshal undo history", "folder", folderID, "error", err)
		return
	}
	if writer != nil {
		writer.enqueueHistory(folderID, historyData)
		return
	}
	if err := s.persistence.SaveHistory(folderID, historyData); err != nil {
		slog.Error("Failed to persist undo history", "folder", folderID, "error", err)
	}
}
