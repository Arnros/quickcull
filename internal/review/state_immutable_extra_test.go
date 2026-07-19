package review

import (
	"quickcull/internal/bus"
	"testing"
)

func TestReduceMetadataActionSharesVisibleOrderAndPreservesStore(t *testing.T) {
	state := benchmarkAppState(30000)
	next, _, err := Reduce(state, bus.Event{Payload: bus.CommandToggleStarPayload{PhotoID: state.VisibleOrder[0], Starred: true}})
	if err != nil {
		t.Fatal(err)
	}
	if &state.VisibleOrder[0] != &next.VisibleOrder[0] {
		t.Fatal("metadata action copied VisibleOrder")
	}
	before, _ := state.photo(state.VisibleOrder[0])
	after, _ := next.photo(next.VisibleOrder[0])
	if before.IsStarred || !after.IsStarred {
		t.Fatalf("immutable update failed: before=%+v after=%+v", before, after)
	}
}

func TestApplyUndoCharacterizationSingleEvents(t *testing.T) {
	tests := []struct {
		name       string
		photo      Photo
		event      bus.Event
		counts     [4]int
		wantPhoto  Photo
		wantCounts [4]int
	}{
		{
			name:       "star true to false does not make count negative",
			photo:      Photo{ID: "p.jpg", IsStarred: true},
			event:      bus.Event{Type: bus.TypeCommandToggleStar, Payload: bus.CommandToggleStarPayload{PhotoID: "p.jpg", OldStarred: false}},
			wantPhoto:  Photo{ID: "p.jpg"},
			wantCounts: [4]int{},
		},
		{
			name:       "star false to true increments count",
			photo:      Photo{ID: "p.jpg"},
			event:      bus.Event{Type: bus.TypeCommandToggleStar, Payload: bus.CommandToggleStarPayload{PhotoID: "p.jpg", OldStarred: true}},
			wantPhoto:  Photo{ID: "p.jpg", IsStarred: true},
			wantCounts: [4]int{1, 0, 0, 0},
		},
		{
			name:       "label set to none decrements count",
			photo:      Photo{ID: "p.jpg", Label: 4},
			event:      bus.Event{Type: bus.TypeCommandLabelPhoto, Payload: bus.CommandLabelPhotoPayload{PhotoID: "p.jpg", OldLabel: 0}},
			counts:     [4]int{0, 1, 0, 0},
			wantPhoto:  Photo{ID: "p.jpg"},
			wantCounts: [4]int{},
		},
		{
			name:       "label none to set increments count",
			photo:      Photo{ID: "p.jpg"},
			event:      bus.Event{Type: bus.TypeCommandLabelPhoto, Payload: bus.CommandLabelPhotoPayload{PhotoID: "p.jpg", OldLabel: 2}},
			wantPhoto:  Photo{ID: "p.jpg", Label: 2},
			wantCounts: [4]int{0, 1, 0, 0},
		},
		{
			name:       "trash true to false decrements count",
			photo:      Photo{ID: "p.jpg", IsTrashed: true},
			event:      bus.Event{Type: bus.TypeCommandTrashPhoto, Payload: bus.CommandTrashPhotoPayload{PhotoID: "p.jpg", OldIsTrashed: false}},
			counts:     [4]int{0, 0, 1, 0},
			wantPhoto:  Photo{ID: "p.jpg"},
			wantCounts: [4]int{},
		},
		{
			name:       "trash false to true increments count",
			photo:      Photo{ID: "p.jpg"},
			event:      bus.Event{Type: bus.TypeCommandTrashPhoto, Payload: bus.CommandTrashPhotoPayload{PhotoID: "p.jpg", OldIsTrashed: true}},
			wantPhoto:  Photo{ID: "p.jpg", IsTrashed: true},
			wantCounts: [4]int{0, 0, 1, 0},
		},
		{
			name:       "exact rotation restores stored value",
			photo:      Photo{ID: "p.jpg", Rotation: 90},
			event:      bus.Event{Type: bus.TypeCommandRotatePhoto, Payload: bus.CommandRotatePhotoPayload{PhotoID: "p.jpg", Direction: rotationReset, OldRotation: 180}},
			counts:     [4]int{0, 0, 0, 1},
			wantPhoto:  Photo{ID: "p.jpg", Rotation: 180},
			wantCounts: [4]int{0, 0, 0, 1},
		},
		{
			name:       "legacy right rotation is inverted",
			photo:      Photo{ID: "p.jpg", Rotation: 90},
			event:      bus.Event{Type: bus.TypeCommandRotatePhoto, Payload: bus.CommandRotatePhotoPayload{PhotoID: "p.jpg", Direction: rotationRight}},
			counts:     [4]int{0, 0, 0, 1},
			wantPhoto:  Photo{ID: "p.jpg"},
			wantCounts: [4]int{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := AppState{
				Photos:       map[string]Photo{"p.jpg": tc.photo},
				History:      []bus.Event{tc.event},
				UndoLen:      1,
				StarredCount: tc.counts[0],
				LabeledCount: tc.counts[1],
				TrashedCount: tc.counts[2],
				RotatedCount: tc.counts[3],
			}

			got, _, err := Reduce(&state, bus.Event{Type: bus.TypeCommandUndo, Payload: bus.CommandUndoPayload{}})
			if err != nil {
				t.Fatalf("undo: %v", err)
			}
			if got.materializePhotos()["p.jpg"] != tc.wantPhoto {
				t.Fatalf("photo = %+v, want %+v", got.materializePhotos()["p.jpg"], tc.wantPhoto)
			}
			gotCounts := [4]int{got.StarredCount, got.LabeledCount, got.TrashedCount, got.RotatedCount}
			if gotCounts != tc.wantCounts {
				t.Fatalf("counts = %v, want %v", gotCounts, tc.wantCounts)
			}
		})
	}
}

func TestApplyUndoCharacterizationBatchUsesReverseOrder(t *testing.T) {
	batch := bus.Event{Type: bus.TypeCommandBatch, Payload: bus.CommandBatchPayload{Events: []bus.Event{
		{Type: bus.TypeCommandToggleStar, Payload: bus.CommandToggleStarPayload{PhotoID: "p.jpg", OldStarred: false}},
		{Type: bus.TypeCommandToggleStar, Payload: bus.CommandToggleStarPayload{PhotoID: "p.jpg", OldStarred: true}},
		{Type: bus.TypeCommandLabelPhoto, Payload: bus.CommandLabelPhotoPayload{PhotoID: "p.jpg", OldLabel: 0}},
		{Type: bus.TypeCommandLabelPhoto, Payload: bus.CommandLabelPhotoPayload{PhotoID: "p.jpg", OldLabel: 3}},
	}}}
	state := AppState{
		Photos:  map[string]Photo{"p.jpg": {ID: "p.jpg"}},
		History: []bus.Event{batch},
		UndoLen: 1,
	}

	got, _, err := Reduce(&state, bus.Event{Type: bus.TypeCommandUndo, Payload: bus.CommandUndoPayload{}})
	if err != nil {
		t.Fatalf("undo batch: %v", err)
	}
	if photo := got.materializePhotos()["p.jpg"]; photo.IsStarred || photo.Label != noLabel {
		t.Fatalf("reverse-order undo did not restore original photo: %+v", photo)
	}
	if got.StarredCount != 0 || got.LabeledCount != 0 {
		t.Fatalf("reverse-order undo counts = starred:%d labeled:%d", got.StarredCount, got.LabeledCount)
	}
}

func TestApplyUndoCharacterizationMissingPhotoIsIgnored(t *testing.T) {
	event := bus.Event{Type: bus.TypeCommandTrashPhoto, Payload: bus.CommandTrashPhotoPayload{PhotoID: "missing.jpg", OldIsTrashed: true}}
	state := AppState{Photos: map[string]Photo{}, History: []bus.Event{event}, UndoLen: 1, TrashedCount: 2}

	got, undone, err := Reduce(&state, bus.Event{Type: bus.TypeCommandUndo, Payload: bus.CommandUndoPayload{}})
	if err != nil {
		t.Fatalf("undo missing photo: %v", err)
	}
	if undone.Type != bus.TypeCommandTrashPhoto || got.TrashedCount != 2 || got.photoCount() != 0 {
		t.Fatalf("missing photo undo changed state: undone=%q count=%d photos=%v", undone.Type, got.TrashedCount, got.Photos)
	}
}

// ── Undo variants ──────────────────────────────────────────────────────────

func TestReduceUndoLabel(t *testing.T) {
	state0 := AppState{
		Photos: map[string]Photo{
			"p.jpg": {ID: "p.jpg", Label: 0},
		},
		VisibleOrder: []string{"p.jpg"},
	}

	state1, _, err := Reduce(&state0, bus.Event{
		Type:    bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: "p.jpg", Label: 3, OldLabel: 0},
	})
	if err != nil {
		t.Fatalf("label: %v", err)
	}
	if state1.materializePhotos()["p.jpg"].Label != 3 {
		t.Fatalf("label want 3, got %d", state1.materializePhotos()["p.jpg"].Label)
	}
	if state1.LabeledCount != 1 {
		t.Fatalf("labeled count want 1, got %d", state1.LabeledCount)
	}

	state2, undone, err := Reduce(state1, bus.Event{Type: bus.TypeCommandUndo, Payload: bus.CommandUndoPayload{}})
	if err != nil {
		t.Fatalf("undo label: %v", err)
	}
	if undone.Type != bus.TypeCommandLabelPhoto {
		t.Fatalf("undone event type want Label, got %q", undone.Type)
	}
	if state2.materializePhotos()["p.jpg"].Label != 0 {
		t.Fatalf("label after undo want 0, got %d", state2.materializePhotos()["p.jpg"].Label)
	}
	if state2.LabeledCount != 0 {
		t.Fatalf("labeled count after undo want 0, got %d", state2.LabeledCount)
	}
}

func TestReduceUndoStar(t *testing.T) {
	state0 := AppState{
		Photos: map[string]Photo{
			"p.jpg": {ID: "p.jpg", IsStarred: false},
		},
		VisibleOrder: []string{"p.jpg"},
	}

	state1, _, err := Reduce(&state0, bus.Event{
		Type:    bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{PhotoID: "p.jpg", Starred: true, OldStarred: false},
	})
	if err != nil {
		t.Fatalf("star: %v", err)
	}
	if !state1.materializePhotos()["p.jpg"].IsStarred {
		t.Fatal("want starred")
	}
	if state1.StarredCount != 1 {
		t.Fatalf("starred count want 1, got %d", state1.StarredCount)
	}

	state2, undone, err := Reduce(state1, bus.Event{Type: bus.TypeCommandUndo, Payload: bus.CommandUndoPayload{}})
	if err != nil {
		t.Fatalf("undo star: %v", err)
	}
	if undone.Type != bus.TypeCommandToggleStar {
		t.Fatalf("undone event type want Star, got %q", undone.Type)
	}
	if state2.materializePhotos()["p.jpg"].IsStarred {
		t.Fatal("want NOT starred after undo")
	}
	if state2.StarredCount != 0 {
		t.Fatalf("starred count after undo want 0, got %d", state2.StarredCount)
	}
}

func TestReduceUndoBatch(t *testing.T) {
	state0 := AppState{
		Photos: map[string]Photo{
			"a.jpg": {ID: "a.jpg", IsStarred: false},
			"b.jpg": {ID: "b.jpg", Label: 0},
		},
		VisibleOrder: []string{"a.jpg", "b.jpg"},
	}

	batchEv := bus.Event{
		Type: bus.TypeCommandBatch,
		Payload: bus.CommandBatchPayload{
			Events: []bus.Event{
				{Type: bus.TypeCommandToggleStar, Payload: bus.CommandToggleStarPayload{PhotoID: "a.jpg", Starred: true}},
				{Type: bus.TypeCommandLabelPhoto, Payload: bus.CommandLabelPhotoPayload{PhotoID: "b.jpg", Label: 5}},
			},
		},
	}

	state1, _, err := Reduce(&state0, batchEv)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}

	state2, undone, err := Reduce(state1, bus.Event{Type: bus.TypeCommandUndo, Payload: bus.CommandUndoPayload{}})
	if err != nil {
		t.Fatalf("undo batch: %v", err)
	}
	if undone.Type != bus.TypeCommandBatch {
		t.Fatalf("undone event type want Batch, got %q", undone.Type)
	}
	if state2.materializePhotos()["a.jpg"].IsStarred || state2.materializePhotos()["b.jpg"].Label != 0 {
		t.Fatal("state not restored after batch undo")
	}
}

func TestReduceUndoNoOp(t *testing.T) {
	state0 := AppState{
		Photos: map[string]Photo{
			"p.jpg": {ID: "p.jpg", IsStarred: true},
		},
		StarredCount: 1,
		History:      nil,
	}
	state0.InitialState = func() *AppState { c := state0.Clone(true); return &c }()

	result, applied, err := Reduce(&state0, bus.Event{Type: bus.TypeCommandUndo, Payload: bus.CommandUndoPayload{}})
	if err != nil {
		t.Fatalf("undo on empty history should not error, got: %v", err)
	}
	if applied.Type != "" {
		t.Fatalf("applied event type should be empty for no-op undo, got %q", applied.Type)
	}
	if result.StarredCount != 1 {
		t.Errorf("state should be unchanged, got starredCount %d", result.StarredCount)
	}
}

func TestReduceRecursiveBatchProtection(t *testing.T) {
	state0 := AppState{
		Photos: map[string]Photo{"x.jpg": {ID: "x.jpg"}},
	}
	inner := bus.Event{
		Type:    bus.TypeCommandBatch,
		Payload: bus.CommandBatchPayload{Events: []bus.Event{{Type: bus.TypeCommandToggleStar}}},
	}
	outer := bus.Event{
		Type:    bus.TypeCommandBatch,
		Payload: bus.CommandBatchPayload{Events: []bus.Event{inner}},
	}

	_, _, err := Reduce(&state0, outer)
	if err == nil {
		t.Fatal("expected error on nested batch")
	}
}

func TestReduceHistoryTrimming(t *testing.T) {
	state0 := AppState{
		Photos: map[string]Photo{
			"x.jpg": {ID: "x.jpg"},
			"y.jpg": {ID: "y.jpg"},
		},
	}
	firstState := state0.Clone(true)
	firstState.History = nil
	state0.InitialState = &firstState

	// Fill history up to limit
	for i := 0; i < maxHistoryLen; i++ {
		st, _, _ := Reduce(&state0, bus.Event{
			Type:    bus.TypeCommandToggleStar,
			Payload: bus.CommandToggleStarPayload{PhotoID: "x.jpg", Starred: true},
		})
		state0 = *st
	}

	if len(state0.History) != maxHistoryLen {
		t.Fatalf("history len want %d, got %d", maxHistoryLen, len(state0.History))
	}

	// Push one more event to trigger the trim.
	state1, _, err := Reduce(&state0, bus.Event{
		Type:    bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{PhotoID: "y.jpg", Starred: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(state1.History) != maxHistoryLen {
		t.Fatalf("history len after trim want %d, got %d", maxHistoryLen, len(state1.History))
	}
	if state1.History[maxHistoryLen-1].Payload.(bus.CommandToggleStarPayload).PhotoID != "y.jpg" {
		t.Fatal("last event in history should be the most recent one")
	}
}
