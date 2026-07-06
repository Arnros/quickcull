package review

import (
	"fmt"
	"log/slog"

	"quickcull/internal/bus"
)

// Rotation arithmetic constants.
const (
	rotationStep    = 90
	rotationModulus = 360
)

// History capacity cap: holding the last 100 events is cheap even at 30,000 photos.
const maxHistoryLen = 100

// noLabel is the sentinel value meaning a photo carries no colour label.
const noLabel = 0

// Rotation direction tokens used in CommandRotatePhotoPayload.
const (
	rotationLeft  = "left"
	rotationRight = "right"
	rotationReset = "reset"
)

// Metadata reset scope tokens used in CommandResetMetadataPayload.
const (
	scopeStars  = "stars"
	scopeLabels = "labels"
	scopeAll    = "all"
)

// Photo represents the immutable metadata for a single photo in the domain.
type Photo struct {
	ID        string // typically the relPath
	IsStarred bool
	Rotation  int
	Label     int
	IsTrashed bool
	// We can extend this with FileType, Size, Date without mutating
}

// AppState is the IMMUTABLE representation of the application's current view.
type AppState struct {
	Root     string
	CacheDir string

	// IsPartial indicates this state snapshot excludes the Photos map
	// for performance reasons (large libraries).
	IsPartial bool `json:"is_partial,omitempty"`

	// Fast lookup for O(1) property checks
	Photos map[string]Photo

	// The current ordered list of photo IDs (relPaths) visible in the grid/filmstrip.
	// This slice should never be mutated in-place; always reassigned.
	VisibleOrder []string

	// Basic statistics
	TrashedCount int
	StarredCount int
	LabeledCount int
	RotatedCount int

	// Event Sourcing History (Optional for Undo, but core to the design)
	// For memory constraints with 30,000 photos, holding the last 100 events is cheap.
	History []bus.Event

	// UndoLen is the number of events currently in the history.
	// Used for efficient UI synchronization without transmitting the full journal.
	UndoLen int `json:"undoLen"`

	// The pointer to the State before any events were applied, used for Rebuilding during Undo.
	InitialState *AppState
}

// RecalculateCounts audits the entire Photos map to update StarredCount, LabeledCount, and RotatedCount.
func (s *AppState) RecalculateCounts() {
	s.StarredCount = 0
	s.LabeledCount = 0
	s.RotatedCount = 0
	for _, id := range s.VisibleOrder {
		photo, ok := s.Photos[id]
		if !ok || photo.IsTrashed {
			continue
		}
		if photo.IsStarred {
			s.StarredCount++
		}
		if photo.Label > noLabel {
			s.LabeledCount++
		}
		if photo.Rotation != 0 {
			s.RotatedCount++
		}
	}
}

// Clone creates a shallow copy of the state, but deeply copies the maps and slices
// that we expect to mutate in the Reducer to guarantee immutability of the old state.
func (s *AppState) Clone(includePhotos bool) AppState {
	next := AppState{
		Root:         s.Root,
		CacheDir:     s.CacheDir,
		TrashedCount: s.TrashedCount,
		StarredCount: s.StarredCount,
		LabeledCount: s.LabeledCount,
		RotatedCount: s.RotatedCount,
		IsPartial:    s.IsPartial,
	}

	// Clone Photos Map (selective)
	if includePhotos && s.Photos != nil {
		next.Photos = make(map[string]Photo, len(s.Photos))
		for k, v := range s.Photos {
			next.Photos[k] = v
		}
	}

	// Clone Visible Slice
	if s.VisibleOrder != nil {
		next.VisibleOrder = make([]string, len(s.VisibleOrder))
		copy(next.VisibleOrder, s.VisibleOrder)
	}

	// Clone History
	if s.History != nil {
		next.History = make([]bus.Event, len(s.History))
		copy(next.History, s.History)
	}

	next.InitialState = s.InitialState

	return next
}

// Reduce is the core engine for state transitions.
// It takes the current state and an event, and returns a brand-new state plus the
// event that was just applied (or undone). It never mutates currentState.
func Reduce(currentState *AppState, event bus.Event) (*AppState, bus.Event, error) {
	// 1. Always start from a clean clone to guarantee immutability.
	// Skip Photos copy for undo events: applyUndo creates its own filtered copy.
	includePhotos := true
	if _, isUndo := event.Payload.(bus.CommandUndoPayload); isUndo {
		includePhotos = false
	}
	nextState := currentState.Clone(includePhotos)

	// Special Case: Undo removes the last event from history.
	if _, ok := event.Payload.(bus.CommandUndoPayload); ok {
		return applyUndo(currentState, nextState)
	}

	// 2. Initialize the immutable origin point if this is the first transition.
	if nextState.InitialState == nil {
		firstState := currentState.Clone(true)
		firstState.History = nil
		nextState.InitialState = &firstState
	}

	// 3. Apply mutations based on event type.
	switch payload := event.Payload.(type) {
	case bus.CommandToggleStarPayload:
		applyToggleStar(&nextState, payload.PhotoID, payload.Starred)
	case bus.CommandTrashPhotoPayload:
		if photo, ok := nextState.Photos[payload.PhotoID]; ok {
			payload.OldIsTrashed = photo.IsTrashed
			event.Payload = payload
		}
		applyTrashPhoto(&nextState, payload.PhotoID)
	case bus.CommandLabelPhotoPayload:
		applyLabelPhoto(&nextState, payload.PhotoID, payload.Label)
	case bus.CommandRotatePhotoPayload:
		if photo, ok := nextState.Photos[payload.PhotoID]; ok {
			payload.OldRotation = photo.Rotation
			event.Payload = payload
		}
		applyRotatePhoto(&nextState, payload.PhotoID, payload.Direction)
	case bus.CommandResetMetadataPayload:
		applyResetMetadata(&nextState, payload.Scope)
	case bus.CommandBatchPayload:
		// Deep-copy events to avoid mutating the original event payload.
		// Mutation during Reduce breaks event sourcing purity (replayed events
		// would carry stale OldIsTrashed/OldRotation values).
		copiedEvents := make([]bus.Event, len(payload.Events))
		copy(copiedEvents, payload.Events)
		for i, subEvent := range copiedEvents {
			if _, nested := subEvent.Payload.(bus.CommandBatchPayload); nested {
				return &nextState, event, fmt.Errorf("nested CommandBatch not allowed")
			}
			switch p := subEvent.Payload.(type) {
			case bus.CommandToggleStarPayload:
				applyToggleStar(&nextState, p.PhotoID, p.Starred)
			case bus.CommandLabelPhotoPayload:
				applyLabelPhoto(&nextState, p.PhotoID, p.Label)
			case bus.CommandTrashPhotoPayload:
				if photo, ok := nextState.Photos[p.PhotoID]; ok {
					p.OldIsTrashed = photo.IsTrashed
					copiedEvents[i].Payload = p
				}
				applyTrashPhoto(&nextState, p.PhotoID)
			case bus.CommandRotatePhotoPayload:
				if photo, ok := nextState.Photos[p.PhotoID]; ok {
					p.OldRotation = photo.Rotation
					copiedEvents[i].Payload = p
				}
				applyRotatePhoto(&nextState, p.PhotoID, p.Direction)
			}
		}
		event.Payload = bus.CommandBatchPayload{Events: copiedEvents}
	default:
		// Unsupported event type; state remains unchanged.
	}

	// 4. Append the event to our local historical journal (useful for Undo later).
	nextState.History = append(nextState.History, event)
	if len(nextState.History) > maxHistoryLen {
		trimmed := nextState.History[len(nextState.History)-maxHistoryLen:]
		compact := make([]bus.Event, maxHistoryLen)
		copy(compact, trimmed)
		nextState.History = compact
	}
	nextState.UndoLen = len(nextState.History)

	return &nextState, event, nil
}

// applyUndo reverses the most recent event in the history of nextState.
// currentState is returned unchanged when there is nothing to undo.
func applyUndo(currentState *AppState, nextState AppState) (*AppState, bus.Event, error) {
	if len(nextState.History) == 0 {
		return currentState, bus.Event{}, nil // nothing to undo
	}

	undoneEvent := nextState.History[len(nextState.History)-1]
	nextState.History = nextState.History[:len(nextState.History)-1]
	nextState.UndoLen = len(nextState.History)

	// Create one Photos copy for the entire undo operation to avoid
	// duplicating the allocation in every sub-case. When Clone skipped
	// photos for undo-fast-path, populate from the current state.
	if nextState.Photos == nil {
		nextState.Photos = make(map[string]Photo, len(currentState.Photos))
		for k, v := range currentState.Photos {
			nextState.Photos[k] = v
		}
	}
	newPhotos := make(map[string]Photo, len(nextState.Photos))
	for k, v := range nextState.Photos {
		newPhotos[k] = v
	}

	switch payload := undoneEvent.Payload.(type) {
	case bus.CommandToggleStarPayload:
		photo, ok := newPhotos[payload.PhotoID]
		if !ok {
			break
		}
		wasStarred := photo.IsStarred
		newPhoto := photo
		newPhoto.IsStarred = payload.OldStarred
		newPhotos[payload.PhotoID] = newPhoto
		if newPhoto.IsStarred && !wasStarred {
			nextState.StarredCount++
		} else if !newPhoto.IsStarred && wasStarred {
			if nextState.StarredCount > 0 {
				nextState.StarredCount--
			}
		}
	case bus.CommandTrashPhotoPayload:
		photo, ok := newPhotos[payload.PhotoID]
		if !ok {
			break
		}
		wasTrash := photo.IsTrashed
		newPhoto := photo
		newPhoto.IsTrashed = payload.OldIsTrashed
		newPhotos[payload.PhotoID] = newPhoto
		if newPhoto.IsTrashed && !wasTrash {
			nextState.TrashedCount++
		} else if !newPhoto.IsTrashed && wasTrash {
			if nextState.TrashedCount > 0 {
				nextState.TrashedCount--
			}
		}
	case bus.CommandLabelPhotoPayload:
		photo, ok := newPhotos[payload.PhotoID]
		if !ok {
			break
		}
		newLabel := photo.Label
		oldLabel := payload.OldLabel
		newPhoto := photo
		newPhoto.Label = oldLabel
		newPhotos[payload.PhotoID] = newPhoto

		if newLabel > noLabel && oldLabel == noLabel {
			if nextState.LabeledCount > 0 {
				nextState.LabeledCount--
			}
		} else if newLabel == noLabel && oldLabel > noLabel {
			nextState.LabeledCount++
		}
	case bus.CommandRotatePhotoPayload:
		photo, ok := newPhotos[payload.PhotoID]
		if !ok {
			break
		}
		wasRotated := photo.Rotation != 0
		newPhoto := photo
		if payload.OldRotation != 0 || payload.Direction == rotationReset {
			// Exact undo using stored old rotation value
			newPhoto.Rotation = payload.OldRotation
		} else {
			// Legacy fallback: invert direction
			switch payload.Direction {
			case rotationLeft:
				newPhoto.Rotation = (photo.Rotation + rotationStep) % rotationModulus
			case rotationRight:
				newPhoto.Rotation = (photo.Rotation - rotationStep) % rotationModulus
				if newPhoto.Rotation < 0 {
					newPhoto.Rotation += rotationModulus
				}
			default:
				break
			}
		}
		isRotated := newPhoto.Rotation != 0
		newPhotos[payload.PhotoID] = newPhoto
		if !wasRotated && isRotated {
			nextState.RotatedCount++
		} else if wasRotated && !isRotated {
			if nextState.RotatedCount > 0 {
				nextState.RotatedCount--
			}
		}
	case bus.CommandBatchPayload:
		// Undo all sub-events in reverse order using the already-created newPhotos map.
		for i := len(payload.Events) - 1; i >= 0; i-- {
			subEvent := payload.Events[i]
			switch p := subEvent.Payload.(type) {
			case bus.CommandToggleStarPayload:
				photo, ok := newPhotos[p.PhotoID]
				if !ok {
					continue
				}
				wasStarred := photo.IsStarred
				newPhoto := photo
				newPhoto.IsStarred = p.OldStarred
				newPhotos[p.PhotoID] = newPhoto
				if newPhoto.IsStarred && !wasStarred {
					nextState.StarredCount++
				} else if !newPhoto.IsStarred && wasStarred {
					if nextState.StarredCount > 0 {
						nextState.StarredCount--
					}
				}
			case bus.CommandLabelPhotoPayload:
				photo, ok := newPhotos[p.PhotoID]
				if !ok {
					continue
				}
				newLabel := photo.Label
				newPhoto := photo
				newPhoto.Label = p.OldLabel
				newPhotos[p.PhotoID] = newPhoto
				if newLabel > noLabel && p.OldLabel == noLabel {
					if nextState.LabeledCount > 0 {
						nextState.LabeledCount--
					}
				} else if newLabel == noLabel && p.OldLabel > noLabel {
					nextState.LabeledCount++
				}
			case bus.CommandTrashPhotoPayload:
				photo, ok := newPhotos[p.PhotoID]
				if !ok {
					continue
				}
				wasTrash := photo.IsTrashed
				newPhoto := photo
				newPhoto.IsTrashed = p.OldIsTrashed
				newPhotos[p.PhotoID] = newPhoto
				if newPhoto.IsTrashed && !wasTrash {
					nextState.TrashedCount++
				} else if !newPhoto.IsTrashed && wasTrash {
					if nextState.TrashedCount > 0 {
						nextState.TrashedCount--
					}
				}
			case bus.CommandRotatePhotoPayload:
				photo, ok := newPhotos[p.PhotoID]
				if !ok {
					continue
				}
				wasRotated := photo.Rotation != 0
				newPhoto := photo
				if p.OldRotation != 0 || p.Direction == rotationReset {
					newPhoto.Rotation = p.OldRotation
				} else {
					switch p.Direction {
					case rotationLeft:
						newPhoto.Rotation = (photo.Rotation + rotationStep) % rotationModulus
					case rotationRight:
						newPhoto.Rotation = (photo.Rotation - rotationStep) % rotationModulus
						if newPhoto.Rotation < 0 {
							newPhoto.Rotation += rotationModulus
						}
					}
				}
				isRotated := newPhoto.Rotation != 0
				newPhotos[p.PhotoID] = newPhoto
				if !wasRotated && isRotated {
					nextState.RotatedCount++
				} else if wasRotated && !isRotated {
					if nextState.RotatedCount > 0 {
						nextState.RotatedCount--
					}
				}
			}
		}
	}

	nextState.Photos = newPhotos
	return &nextState, undoneEvent, nil
}

// applyToggleStar sets the starred state of a photo and keeps StarredCount in sync.
func applyToggleStar(s *AppState, photoID string, starred bool) {
	photo, ok := s.Photos[photoID]
	if !ok {
		return
	}
	wasStarred := photo.IsStarred
	photo.IsStarred = starred
	s.Photos[photoID] = photo
	if photo.IsStarred && !wasStarred {
		s.StarredCount++
	} else if !photo.IsStarred && wasStarred {
		s.StarredCount--
	}
	slog.Debug("Reducer: Applied ToggleStar", "photo", photoID, "starred", photo.IsStarred)
}

// applyTrashPhoto flips the trashed state of a single photo and keeps TrashedCount in sync.
func applyTrashPhoto(s *AppState, photoID string) {
	photo, ok := s.Photos[photoID]
	if !ok {
		return
	}
	photo.IsTrashed = !photo.IsTrashed
	s.Photos[photoID] = photo
	if photo.IsTrashed {
		s.TrashedCount++
	} else {
		s.TrashedCount--
	}
	slog.Debug("Reducer: Applied TrashPhoto", "photo", photoID, "trashed", photo.IsTrashed)
}

// applyLabelPhoto sets a new label on a photo and keeps LabeledCount in sync.
func applyLabelPhoto(s *AppState, photoID string, label int) {
	photo, ok := s.Photos[photoID]
	if !ok {
		return
	}
	oldLabel := photo.Label
	photo.Label = label
	s.Photos[photoID] = photo
	if oldLabel == noLabel && label > noLabel {
		s.LabeledCount++
	} else if oldLabel > noLabel && label == noLabel {
		s.LabeledCount--
	}
	slog.Debug("Reducer: Applied LabelPhoto", "photo", photoID, "label", label)
}

// applyRotatePhoto adjusts a photo's rotation by rotationStep degrees in the given direction.
func applyRotatePhoto(s *AppState, photoID, direction string) {
	photo, ok := s.Photos[photoID]
	if !ok {
		return
	}
	wasRotated := photo.Rotation != 0
	switch direction {
	case rotationLeft:
		photo.Rotation = (photo.Rotation - rotationStep) % rotationModulus
		if photo.Rotation < 0 {
			photo.Rotation += rotationModulus
		}
	case rotationRight:
		photo.Rotation = (photo.Rotation + rotationStep) % rotationModulus
	case rotationReset:
		photo.Rotation = 0
	}
	isRotated := photo.Rotation != 0
	s.Photos[photoID] = photo

	if !wasRotated && isRotated {
		s.RotatedCount++
	} else if wasRotated && !isRotated {
		if s.RotatedCount > 0 {
			s.RotatedCount--
		}
	}
	slog.Debug("Reducer: Applied RotatePhoto", "photo", photoID, "rotation", photo.Rotation)
}

// applyResetMetadata clears star and/or label metadata across all photos based on scope.
func applyResetMetadata(s *AppState, scope string) {
	clearStars := scope == scopeStars || scope == scopeAll
	clearLabels := scope == scopeLabels || scope == scopeAll

	for id, photo := range s.Photos {
		if clearStars {
			photo.IsStarred = false
		}
		if clearLabels {
			photo.Label = noLabel
		}
		s.Photos[id] = photo
	}
	if clearStars {
		s.StarredCount = 0
	}
	if clearLabels {
		s.LabeledCount = 0
	}
	// Note: We don't reset rotation here as it's not part of stars/labels scope.
	// If scope == scopeAll, should we reset rotation? Current logic says no.
	slog.Debug("Reducer: Applied ResetMetadata", "scope", scope)
}
