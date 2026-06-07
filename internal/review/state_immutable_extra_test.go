package review

import (
	"testing"
	"quickcull/internal/bus"
)

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
	if state1.Photos["p.jpg"].Label != 3 {
		t.Fatalf("label want 3, got %d", state1.Photos["p.jpg"].Label)
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
	if state2.Photos["p.jpg"].Label != 0 {
		t.Fatalf("label after undo want 0, got %d", state2.Photos["p.jpg"].Label)
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
	if !state1.Photos["p.jpg"].IsStarred {
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
	if state2.Photos["p.jpg"].IsStarred {
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
	if state2.Photos["a.jpg"].IsStarred || state2.Photos["b.jpg"].Label != 0 {
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
