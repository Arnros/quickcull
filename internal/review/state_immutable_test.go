package review

import (
	"testing"

	"quickcull/internal/bus"
)

func TestReduceToggleStar(t *testing.T) {
	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", IsStarred: false},
			"photo2.jpg": {ID: "photo2.jpg", IsStarred: true},
		},
		VisibleOrder: []string{"photo1.jpg", "photo2.jpg"},
	}

	// 1. Create the event
	event := bus.Event{
		Type: bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{
			PhotoID: "photo1.jpg", Starred: true, OldStarred: false,
		},
	}

	// 2. Reduce
	nextState, _, err := Reduce(&initialState, event)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}

	// 3. Verify Immutability: the INITIAL state must NOT have changed.
	if initialState.materializePhotos()["photo1.jpg"].IsStarred != false {
		t.Errorf("Immutability broken! Initial state was mutated.")
	}

	// 4. Verify Correctness: the NEW state MUST reflect the change.
	if nextState.materializePhotos()["photo1.jpg"].IsStarred != true {
		t.Errorf("Reducer failed to toggle star in next state.")
	}

	// 5. Verify History
	if len(nextState.History) != 1 {
		t.Errorf("Reducer failed to append to history. Got len %d", len(nextState.History))
	}
}

func TestReduceTrashPhoto(t *testing.T) {
	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", IsTrashed: false},
			"photo2.jpg": {ID: "photo2.jpg", IsTrashed: true},
		},
		TrashedCount: 1,
		VisibleOrder: []string{"photo1.jpg", "photo2.jpg"},
	}

	event := bus.Event{
		Type: bus.TypeCommandTrashPhoto,
		Payload: bus.CommandTrashPhotoPayload{
			PhotoID: "photo1.jpg",
		},
	}

	nextState, _, err := Reduce(&initialState, event)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}

	// Verify Immutability
	if initialState.materializePhotos()["photo1.jpg"].IsTrashed != false {
		t.Errorf("Immutability broken! Initial state was mutated.")
	}
	if initialState.TrashedCount != 1 {
		t.Errorf("Immutability broken! Initial count was mutated.")
	}

	// Verify Correctness
	if nextState.materializePhotos()["photo1.jpg"].IsTrashed != true {
		t.Errorf("Reducer failed to trash photo in next state.")
	}
	if nextState.TrashedCount != 2 {
		t.Errorf("Reducer failed to update trashed count.")
	}
}

func TestReduceLabelPhoto(t *testing.T) {
	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", Label: 0},
			"photo2.jpg": {ID: "photo2.jpg", Label: 2},
		},
		VisibleOrder: []string{"photo1.jpg", "photo2.jpg"},
	}

	event := bus.Event{
		Type: bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{
			PhotoID: "photo1.jpg",
			Label:   3,
		},
	}

	nextState, _, err := Reduce(&initialState, event)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}

	// Verify Immutability
	if initialState.materializePhotos()["photo1.jpg"].Label != 0 {
		t.Errorf("Immutability broken! Initial state was mutated.")
	}

	// Verify Correctness
	if nextState.materializePhotos()["photo1.jpg"].Label != 3 {
		t.Errorf("Reducer failed to label photo in next state.")
	}
}

func TestReduceRotatePhoto(t *testing.T) {
	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", Rotation: 90},
		},
	}

	eventLeft := bus.Event{
		Type: bus.TypeCommandRotatePhoto,
		Payload: bus.CommandRotatePhotoPayload{
			PhotoID:   "photo1.jpg",
			Direction: "left",
		},
	}

	nextState, _, err := Reduce(&initialState, eventLeft)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}

	if initialState.materializePhotos()["photo1.jpg"].Rotation != 90 {
		t.Errorf("Immutability broken!")
	}

	if nextState.materializePhotos()["photo1.jpg"].Rotation != 0 {
		t.Errorf("Expected 0 after rotating left from 90, got %d", nextState.materializePhotos()["photo1.jpg"].Rotation)
	}

	eventRight := bus.Event{
		Type: bus.TypeCommandRotatePhoto,
		Payload: bus.CommandRotatePhotoPayload{
			PhotoID:   "photo1.jpg",
			Direction: "right",
		},
	}

	nextRightState, _, _ := Reduce(nextState, eventRight)
	if nextRightState.materializePhotos()["photo1.jpg"].Rotation != 90 {
		t.Errorf("Expected 90 after rotating right from 0, got %d", nextRightState.materializePhotos()["photo1.jpg"].Rotation)
	}
}

func TestReduceRotateReset(t *testing.T) {
	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", Rotation: 270},
		},
	}

	event := bus.Event{
		Type: bus.TypeCommandRotatePhoto,
		Payload: bus.CommandRotatePhotoPayload{
			PhotoID:   "photo1.jpg",
			Direction: "reset",
		},
	}

	nextState, _, _ := Reduce(&initialState, event)
	if nextState.materializePhotos()["photo1.jpg"].Rotation != 0 {
		t.Errorf("Expected 0 after reset, got %d", nextState.materializePhotos()["photo1.jpg"].Rotation)
	}
}

func TestReduceResetMetadataStarsOnly(t *testing.T) {
	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", IsStarred: true, Label: 0},
			"photo2.jpg": {ID: "photo2.jpg", IsStarred: true, Label: 0},
			"photo3.jpg": {ID: "photo3.jpg", IsStarred: false, Label: 3},
		},
		VisibleOrder: []string{"photo1.jpg", "photo2.jpg", "photo3.jpg"},
		StarredCount: 2,
		LabeledCount: 1,
	}

	event := bus.Event{
		Type: bus.TypeCommandResetMetadata,
		Payload: bus.CommandResetMetadataPayload{
			Scope: "stars",
		},
	}

	nextState, _, err := Reduce(&initialState, event)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}

	// Verify all photos have IsStarred=false
	nextState.rangePhotos(func(id string, photo Photo) bool {
		if photo.IsStarred {
			t.Errorf("Expected photo %s to be unstarred after reset stars", id)
		}
		return true
	})

	// Verify StarredCount=0
	if nextState.StarredCount != 0 {
		t.Errorf("Expected StarredCount=0, got %d", nextState.StarredCount)
	}

	// Verify labels are unchanged
	if nextState.materializePhotos()["photo3.jpg"].Label != 3 {
		t.Errorf("Expected label of photo3.jpg to remain 3, got %d", nextState.materializePhotos()["photo3.jpg"].Label)
	}
	if nextState.LabeledCount != 1 {
		t.Errorf("Expected LabeledCount=1 (unchanged), got %d", nextState.LabeledCount)
	}
}

func TestReduceResetMetadataLabelsOnly(t *testing.T) {
	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", IsStarred: false, Label: 2},
			"photo2.jpg": {ID: "photo2.jpg", IsStarred: false, Label: 4},
			"photo3.jpg": {ID: "photo3.jpg", IsStarred: true, Label: 0},
		},
		VisibleOrder: []string{"photo1.jpg", "photo2.jpg", "photo3.jpg"},
		StarredCount: 1,
		LabeledCount: 2,
	}

	event := bus.Event{
		Type: bus.TypeCommandResetMetadata,
		Payload: bus.CommandResetMetadataPayload{
			Scope: "labels",
		},
	}

	nextState, _, err := Reduce(&initialState, event)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}

	// Verify all labels are 0
	nextState.rangePhotos(func(id string, photo Photo) bool {
		if photo.Label != 0 {
			t.Errorf("Expected label of photo %s to be 0 after reset labels, got %d", id, photo.Label)
		}
		return true
	})

	// Verify LabeledCount=0
	if nextState.LabeledCount != 0 {
		t.Errorf("Expected LabeledCount=0, got %d", nextState.LabeledCount)
	}

	// Verify stars are unchanged
	if !nextState.materializePhotos()["photo3.jpg"].IsStarred {
		t.Errorf("Expected photo3.jpg to remain starred")
	}
	if nextState.StarredCount != 1 {
		t.Errorf("Expected StarredCount=1 (unchanged), got %d", nextState.StarredCount)
	}
}

func TestReduceResetMetadataAll(t *testing.T) {
	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", IsStarred: true, Label: 2},
			"photo2.jpg": {ID: "photo2.jpg", IsStarred: true, Label: 0},
			"photo3.jpg": {ID: "photo3.jpg", IsStarred: false, Label: 5},
		},
		VisibleOrder: []string{"photo1.jpg", "photo2.jpg", "photo3.jpg"},
		StarredCount: 2,
		LabeledCount: 2,
	}

	event := bus.Event{
		Type: bus.TypeCommandResetMetadata,
		Payload: bus.CommandResetMetadataPayload{
			Scope: "all",
		},
	}

	nextState, _, err := Reduce(&initialState, event)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}

	// Verify all photos have IsStarred=false and Label=0
	nextState.rangePhotos(func(id string, photo Photo) bool {
		if photo.IsStarred {
			t.Errorf("Expected photo %s to be unstarred after reset all", id)
		}
		if photo.Label != 0 {
			t.Errorf("Expected label of photo %s to be 0 after reset all, got %d", id, photo.Label)
		}
		return true
	})

	// Verify StarredCount=0 and LabeledCount=0
	if nextState.StarredCount != 0 {
		t.Errorf("Expected StarredCount=0, got %d", nextState.StarredCount)
	}
	if nextState.LabeledCount != 0 {
		t.Errorf("Expected LabeledCount=0, got %d", nextState.LabeledCount)
	}
}

func TestReduceHistoryBoundary(t *testing.T) {
	// Build initial history with maxHistoryLen entries
	history := make([]bus.Event, maxHistoryLen)
	for i := range history {
		history[i] = bus.Event{
			Type:    bus.TypeCommandToggleStar,
			Payload: bus.CommandToggleStarPayload{PhotoID: "filler.jpg", Starred: true, OldStarred: false},
		}
	}

	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", IsStarred: false},
			"filler.jpg": {ID: "filler.jpg", IsStarred: false},
		},
		VisibleOrder: []string{"photo1.jpg", "filler.jpg"},
		History:      history,
	}
	// Set InitialState to avoid it being set to nil-clone on first Reduce
	firstState := initialState.Clone(true)
	firstState.History = nil
	initialState.InitialState = &firstState

	event := bus.Event{
		Type: bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{
			PhotoID: "photo1.jpg", Starred: true, OldStarred: false,
		},
	}

	nextState, _, err := Reduce(&initialState, event)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}

	// History must not exceed maxHistoryLen
	if len(nextState.History) != maxHistoryLen {
		t.Errorf("Expected History length to be capped at %d, got %d", maxHistoryLen, len(nextState.History))
	}
}

func TestPhotoStoreSnapshot_FallbackFromPhotosMap(t *testing.T) {
	photos := map[string]Photo{
		"a.jpg": {ID: "a.jpg", IsStarred: true},
		"b.jpg": {ID: "b.jpg"},
	}
	state := &AppState{Photos: photos}

	// photo(missing) returns false
	if _, ok := state.photo("missing.jpg"); ok {
		t.Error("expected false for missing photo")
	}

	// photo(existing) returns the correct value
	p, ok := state.photo("a.jpg")
	if !ok || !p.IsStarred {
		t.Error("expected starred photo a.jpg")
	}

	// photoCount returns the correct count even when s.photos is nil
	if n := state.photoCount(); n != 2 {
		t.Errorf("photoCount = %d, want 2", n)
	}

	// rangePhotos visits all photos exactly once
	visited := make(map[string]bool)
	state.rangePhotos(func(id string, _ Photo) bool {
		visited[id] = true
		return true
	})
	if len(visited) != 2 || !visited["a.jpg"] || !visited["b.jpg"] {
		t.Errorf("rangePhotos missed photos: %v", visited)
	}
}

func TestPhotoStoreSnapshot_WithPhotoStore(t *testing.T) {
	photos := map[string]Photo{
		"a.jpg": {ID: "a.jpg", IsStarred: true},
		"b.jpg": {ID: "b.jpg"},
	}
	state := &AppState{photos: newPhotoStore(photos)}

	// Must prefer s.photos over s.Photos even if both are set
	state.Photos = map[string]Photo{"a.jpg": {ID: "a.jpg", IsStarred: false}}

	if p, ok := state.photo("a.jpg"); !ok || !p.IsStarred {
		t.Error("expected s.photos to take priority over s.Photos")
	}
}

func TestRecalculateCounts_WithOnlyPhotosMap(t *testing.T) {
	// The bug: when s.photos is nil, photo() creates a new photoStore from
	// s.Photos on EVERY call. RecalculateCounts must handle this correctly
	// (not O(n²), but at minimum produce the right counts).
	photos := map[string]Photo{
		"a.jpg": {ID: "a.jpg", IsStarred: true, Label: 3},
		"b.jpg": {ID: "b.jpg"},
		"c.jpg": {ID: "c.jpg", Label: 1, Rotation: 90},
	}
	state := &AppState{
		Photos:       photos,
		VisibleOrder: []string{"a.jpg", "b.jpg", "c.jpg"},
	}

	state.RecalculateCounts()

	if state.StarredCount != 1 {
		t.Errorf("StarredCount = %d, want 1", state.StarredCount)
	}
	if state.LabeledCount != 2 {
		t.Errorf("LabeledCount = %d, want 2", state.LabeledCount)
	}
	if state.RotatedCount != 1 {
		t.Errorf("RotatedCount = %d, want 1", state.RotatedCount)
	}
	if state.photos != nil {
		t.Errorf("RecalculateCounts must not set s.photos")
	}
}

func TestRecalculateCounts_WithPhotoStore(t *testing.T) {
	// The efficient path: s.photos is set, photo() uses it directly.
	photos := map[string]Photo{
		"a.jpg": {ID: "a.jpg", IsStarred: true, Label: 3},
		"b.jpg": {ID: "b.jpg"},
		"c.jpg": {ID: "c.jpg", Label: 1, Rotation: 90, IsTrashed: true},
	}
	state := &AppState{
		photos:       newPhotoStore(photos),
		VisibleOrder: []string{"a.jpg", "b.jpg", "c.jpg"},
	}

	state.RecalculateCounts()

	if state.StarredCount != 1 {
		t.Errorf("StarredCount = %d, want 1", state.StarredCount)
	}
	if state.LabeledCount != 1 {
		t.Errorf("LabeledCount = %d, want 1 (c.jpg is trashed)", state.LabeledCount)
	}
	if state.RotatedCount != 0 {
		t.Errorf("RotatedCount = %d, want 0 (c.jpg is trashed)", state.RotatedCount)
	}
}

func TestReduceUndo(t *testing.T) {
	initialState := AppState{
		Photos: map[string]Photo{
			"photo1.jpg": {ID: "photo1.jpg", IsStarred: false},
		},
	}

	eventStar := bus.Event{
		Type: bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{
			PhotoID: "photo1.jpg", Starred: true, OldStarred: false,
		},
	}

	state1, _, err := Reduce(&initialState, eventStar)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}

	if state1.materializePhotos()["photo1.jpg"].IsStarred != true {
		t.Errorf("Expected photo to be starred")
	}

	if len(state1.History) != 1 {
		t.Errorf("Expected 1 event in history, got %d", len(state1.History))
	}
	if state1.InitialState == nil {
		t.Errorf("Expected InitialState to be initialized")
	}

	eventUndo := bus.Event{
		Type:    bus.TypeCommandUndo,
		Payload: bus.CommandUndoPayload{},
	}

	state2, undoneEvent, err := Reduce(state1, eventUndo)
	if err != nil {
		t.Fatalf("Reduce returned error for Undo: %v", err)
	}

	if undoneEvent.Type != bus.TypeCommandToggleStar {
		t.Errorf("Expected undoneEvent to be ToggleStar, got %v", undoneEvent.Type)
	}

	if state2.materializePhotos()["photo1.jpg"].IsStarred != false {
		t.Errorf("Expected photo to be UNSTARRED after Undo")
	}

	if len(state2.History) != 0 {
		t.Errorf("Expected 0 events in history after undoing the only event, got %d", len(state2.History))
	}

	if state2.InitialState == nil {
		t.Errorf("Expected InitialState to survive after Undo rebuild")
	}
}
