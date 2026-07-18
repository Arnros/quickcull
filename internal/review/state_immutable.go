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
			copiedEvents[i] = applySingleEvent(&nextState, subEvent)
		}
		event.Payload = bus.CommandBatchPayload{Events: copiedEvents}
	default:
		event = applySingleEvent(&nextState, event)
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

// applySingleEvent applies one photo metadata event and returns the event with
// its undo fields populated. Unsupported events leave the state unchanged.
func applySingleEvent(state *AppState, event bus.Event) bus.Event {
	switch payload := event.Payload.(type) {
	case bus.CommandToggleStarPayload:
		applyToggleStar(state, payload.PhotoID, payload.Starred)
	case bus.CommandTrashPhotoPayload:
		if photo, ok := state.Photos[payload.PhotoID]; ok {
			payload.OldIsTrashed = photo.IsTrashed
			event.Payload = payload
		}
		applyTrashPhoto(state, payload.PhotoID)
	case bus.CommandLabelPhotoPayload:
		applyLabelPhoto(state, payload.PhotoID, payload.Label)
	case bus.CommandRotatePhotoPayload:
		if photo, ok := state.Photos[payload.PhotoID]; ok {
			payload.OldRotation = photo.Rotation
			event.Payload = payload
		}
		applyRotatePhoto(state, payload.PhotoID, payload.Direction)
	}
	return event
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

	applyUndoEvent(&nextState, newPhotos, undoneEvent)

	nextState.Photos = newPhotos
	return &nextState, undoneEvent, nil
}

// applyUndoEvent restores the metadata captured by one event. Batch events are
// traversed in reverse so multiple changes to the same photo unwind correctly.
func applyUndoEvent(state *AppState, photos map[string]Photo, event bus.Event) {
	switch payload := event.Payload.(type) {
	case bus.CommandToggleStarPayload:
		before, after, ok := updatePhoto(photos, payload.PhotoID, func(photo *Photo) {
			photo.IsStarred = payload.OldStarred
		})
		if ok {
			adjustUndoCount(&state.StarredCount, before.IsStarred, after.IsStarred)
		}
	case bus.CommandTrashPhotoPayload:
		before, after, ok := updatePhoto(photos, payload.PhotoID, func(photo *Photo) {
			photo.IsTrashed = payload.OldIsTrashed
		})
		if ok {
			adjustUndoCount(&state.TrashedCount, before.IsTrashed, after.IsTrashed)
		}
	case bus.CommandLabelPhotoPayload:
		before, after, ok := updatePhoto(photos, payload.PhotoID, func(photo *Photo) {
			photo.Label = payload.OldLabel
		})
		if ok {
			adjustUndoCount(&state.LabeledCount, before.Label > noLabel, after.Label > noLabel)
		}
	case bus.CommandRotatePhotoPayload:
		before, after, ok := updatePhoto(photos, payload.PhotoID, func(photo *Photo) {
			photo.Rotation = undoRotation(*photo, payload)
		})
		if ok {
			adjustUndoCount(&state.RotatedCount, before.Rotation != 0, after.Rotation != 0)
		}
	case bus.CommandBatchPayload:
		for i := len(payload.Events) - 1; i >= 0; i-- {
			applyUndoEvent(state, photos, payload.Events[i])
		}
	}
}

func updatePhoto(photos map[string]Photo, photoID string, update func(*Photo)) (Photo, Photo, bool) {
	before, ok := photos[photoID]
	if !ok {
		return Photo{}, Photo{}, false
	}
	after := before
	update(&after)
	photos[photoID] = after
	return before, after, true
}

func adjustUndoCount(count *int, before, after bool) {
	switch {
	case !before && after:
		*count++
	case before && !after && *count > 0:
		*count--
	}
}

func undoRotation(photo Photo, payload bus.CommandRotatePhotoPayload) int {
	if payload.OldRotation != 0 || payload.Direction == rotationReset {
		return payload.OldRotation
	}

	switch payload.Direction {
	case rotationLeft:
		return (photo.Rotation + rotationStep) % rotationModulus
	case rotationRight:
		rotation := (photo.Rotation - rotationStep) % rotationModulus
		if rotation < 0 {
			rotation += rotationModulus
		}
		return rotation
	default:
		return photo.Rotation
	}
}

// applyToggleStar sets the starred state of a photo and keeps StarredCount in sync.
func applyToggleStar(s *AppState, photoID string, starred bool) {
	before, after, ok := updatePhoto(s.Photos, photoID, func(photo *Photo) {
		photo.IsStarred = starred
	})
	if !ok {
		return
	}
	adjustCount(&s.StarredCount, before.IsStarred, after.IsStarred)
	slog.Debug("Reducer: Applied ToggleStar", "photo", photoID, "starred", after.IsStarred)
}

// applyTrashPhoto flips the trashed state of a single photo and keeps TrashedCount in sync.
func applyTrashPhoto(s *AppState, photoID string) {
	before, after, ok := updatePhoto(s.Photos, photoID, func(photo *Photo) {
		photo.IsTrashed = !photo.IsTrashed
	})
	if !ok {
		return
	}
	adjustCount(&s.TrashedCount, before.IsTrashed, after.IsTrashed)
	slog.Debug("Reducer: Applied TrashPhoto", "photo", photoID, "trashed", after.IsTrashed)
}

// applyLabelPhoto sets a new label on a photo and keeps LabeledCount in sync.
func applyLabelPhoto(s *AppState, photoID string, label int) {
	before, after, ok := updatePhoto(s.Photos, photoID, func(photo *Photo) {
		photo.Label = label
	})
	if !ok {
		return
	}
	adjustCount(&s.LabeledCount, before.Label > noLabel, after.Label > noLabel)
	slog.Debug("Reducer: Applied LabelPhoto", "photo", photoID, "label", label)
}

// applyRotatePhoto adjusts a photo's rotation by rotationStep degrees in the given direction.
func applyRotatePhoto(s *AppState, photoID, direction string) {
	before, after, ok := updatePhoto(s.Photos, photoID, func(photo *Photo) {
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
	})
	if !ok {
		return
	}
	adjustUndoCount(&s.RotatedCount, before.Rotation != 0, after.Rotation != 0)
	slog.Debug("Reducer: Applied RotatePhoto", "photo", photoID, "rotation", after.Rotation)
}

func adjustCount(count *int, before, after bool) {
	if !before && after {
		*count++
	} else if before && !after {
		*count--
	}
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
