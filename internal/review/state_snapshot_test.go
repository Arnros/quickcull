package review

import (
	"encoding/json"
	"strings"
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

	var payload AppStateDTO
	var gotSync bool
	srv.SetBroadcastHook(func(name string, data any) {
		if name != "SyncState" {
			return
		}
		state, ok := data.(AppStateDTO)
		if !ok {
			t.Fatalf("SyncState payload type = %T, want AppStateDTO", data)
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
	mutated := srv.appState.materializePhotos()["a.jpg"]
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

	// UI payload does not carry History or InitialState at all since AppStateDTO doesn't export them.
	// We just verify UndoLen.
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

	// Verify optimization: History/InitialState are excluded by DTO structure.
	if snapshot.UndoLen != 1 {
		t.Errorf("Expected UndoLen 1, got %d", snapshot.UndoLen)
	}

	// Verify deep copy of Photos and VisibleOrder
	photo := initial.materializePhotos()["x.jpg"]
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

func TestAppStateDTO_JSONSerializationContract(t *testing.T) {
	dto := AppStateDTO{
		VisibleOrder: []string{"img1.jpg"},
		TrashedCount: 5,
		UndoLen:      3,
		Photos: map[string]Photo{
			"img1.jpg": {ID: "img1.jpg", IsStarred: true},
		},
	}

	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Failed to marshal AppStateDTO: %v", err)
	}
	jsonStr := string(b)

	// Verify exact keys expected by Svelte frontend
	expectedKeys := []string{
		`"VisibleOrder"`,
		`"Photos"`,
		`"TrashedCount"`,
		`"undoLen"`, // explicitly lowercased via json tag
		`"IsStarred"`,
	}

	for _, key := range expectedKeys {
		if !strings.Contains(jsonStr, key) {
			t.Errorf("JSON serialization missing expected key %s. Got: %s", key, jsonStr)
		}
	}
}

func TestBuildSyncSnapshotSelective_FiltersToAffectedPhotos(t *testing.T) {
	state := AppState{
		Root:         "/tmp",
		Photos:       map[string]Photo{"a.jpg": {ID: "a.jpg", IsStarred: true}, "b.jpg": {ID: "b.jpg"}, "c.jpg": {ID: "c.jpg"}},
		VisibleOrder: []string{"a.jpg", "b.jpg", "c.jpg"},
	}
	dto := BuildSyncSnapshotSelective(state, true, []string{"a.jpg", "c.jpg"})

	if !dto.IsPartial {
		t.Error("expected IsPartial=true")
	}
	if len(dto.Photos) != 2 {
		t.Fatalf("expected 2 photos in selective snapshot, got %d", len(dto.Photos))
	}
	if _, ok := dto.Photos["b.jpg"]; ok {
		t.Error("BuildSyncSnapshotSelective must exclude unlisted photos")
	}
	if !dto.Photos["a.jpg"].IsStarred {
		t.Error("a.jpg metadata must be preserved in selective snapshot")
	}
	if len(dto.VisibleOrder) != 3 {
		t.Errorf("VisibleOrder must not be filtered, got %d", len(dto.VisibleOrder))
	}
}

func TestBroadcastAppStateSelective_EmitsSyncState(t *testing.T) {
	srv := NewServer()
	srv.appState = &AppState{
		Root:         "/tmp",
		Photos:       map[string]Photo{"a.jpg": {ID: "a.jpg", IsStarred: true}},
		VisibleOrder: []string{"a.jpg"},
	}

	var got AppStateDTO
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventSyncState {
			got = data.(AppStateDTO)
		}
	})

	srv.broadcastAppStateSelective(srv.appState, true, []string{"a.jpg"})

	if got.Root != "/tmp" {
		t.Errorf("expected root /tmp, got %q", got.Root)
	}
	if !got.IsPartial {
		t.Errorf("expected IsPartial=true")
	}
	if len(got.Photos) != 1 {
		t.Errorf("expected 1 photo, got %d", len(got.Photos))
	}
}
