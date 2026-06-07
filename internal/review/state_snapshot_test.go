package review

import (
	"testing"

	"quickcull/internal/bus"
)

func TestSyncFullState_EmitsImmutableSnapshot(t *testing.T) {
	srv := NewServer()
	srv.appState = &AppState{
		Root: "/tmp/root",
		Photos: map[string]Photo{
			"a.jpg": {ID: "a.jpg", IsStarred: false},
			"b.jpg": {ID: "b.jpg", IsStarred: true},
		},
		VisibleOrder: []string{"a.jpg", "b.jpg"},
		History: []bus.Event{
			{
				Type:    bus.TypeCommandToggleStar,
				Payload: bus.CommandToggleStarPayload{PhotoID: "b.jpg", Starred: true, OldStarred: false},
			},
		},
	}

	var payload AppState
	var gotSync bool
	srv.SetBroadcastHook(func(name string, data any) {
		if name != "SyncState" {
			return
		}
		state, ok := data.(AppState)
		if !ok {
			t.Fatalf("SyncState payload type = %T, want AppState", data)
		}
		// Capture ONLY the final authoritative state for this test.
		// (SyncFullState emits multiple SyncStates during chunking).
		if len(state.Photos) > 0 {
			payload = state
			gotSync = true
		}
	})

	srv.SyncFullState()

	if !gotSync {
		t.Fatal("SyncState authoritative event was not emitted")
	}

	// Mutate live app state after the broadcast and verify payload remains unchanged.
	srv.appStateMu.Lock()
	mutated := srv.appState.Photos["a.jpg"]
	mutated.IsStarred = true
	srv.appState.Photos["a.jpg"] = mutated
	srv.appState.VisibleOrder[0] = "mutated.jpg"
	srv.appState.History[0] = bus.Event{
		Type:    bus.TypeCommandTrashPhoto,
		Payload: bus.CommandTrashPhotoPayload{PhotoID: "a.jpg"},
	}
	srv.appStateMu.Unlock()

	if payload.Photos["a.jpg"].IsStarred {
		t.Fatal("SyncState payload Photos map was mutated after emit")
	}
	if payload.VisibleOrder[0] != "a.jpg" {
		t.Fatalf("SyncState payload VisibleOrder changed to %q", payload.VisibleOrder[0])
	}

	// Verify that History and InitialState are NOT sent to UI (optimized payload)
	if payload.History != nil {
		t.Error("UI payload should NOT carry the History log")
	}
	if payload.InitialState != nil {
		t.Error("UI payload should NOT carry the InitialState snapshot")
	}
	if payload.UndoLen != 1 {
		t.Errorf("Expected UndoLen 1, got %d", payload.UndoLen)
	}
}

func TestSyncFullStateSnapshot_BuildSyncSnapshotDeepCopy(t *testing.T) {
	initial := AppState{
		Photos: map[string]Photo{
			"x.jpg": {ID: "x.jpg", Label: 1},
		},
		VisibleOrder: []string{"x.jpg"},
		History: []bus.Event{
			{
				Type:    bus.TypeCommandLabelPhoto,
				Payload: bus.CommandLabelPhotoPayload{PhotoID: "x.jpg", Label: 1},
			},
		},
		InitialState: &AppState{
			Photos: map[string]Photo{
				"x.jpg": {ID: "x.jpg", Label: 0},
			},
			VisibleOrder: []string{"x.jpg"},
		},
	}

	snapshot := BuildSyncSnapshot(initial, true)

	// Verify optimization: History/InitialState should be nil in UI snapshot
	if snapshot.History != nil {
		t.Error("Snapshot History should be nil")
	}
	if snapshot.InitialState != nil {
		t.Error("Snapshot InitialState should be nil")
	}
	if snapshot.UndoLen != 1 {
		t.Errorf("Expected UndoLen 1, got %d", snapshot.UndoLen)
	}

	// Verify deep copy of Photos and VisibleOrder
	photo := initial.Photos["x.jpg"]
	photo.Label = 7
	initial.Photos["x.jpg"] = photo
	initial.VisibleOrder[0] = "changed.jpg"

	if snapshot.Photos["x.jpg"].Label != 1 {
		t.Fatalf("snapshot.Photos mutated, got label %d", snapshot.Photos["x.jpg"].Label)
	}
	if snapshot.VisibleOrder[0] != "x.jpg" {
		t.Fatalf("snapshot.VisibleOrder mutated, got %q", snapshot.VisibleOrder[0])
	}
}
