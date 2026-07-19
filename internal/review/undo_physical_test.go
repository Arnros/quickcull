package review

import (
	"os"
	"path/filepath"
	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"testing"
)

// TestUndoTrashRestoresOriginalPosition verifies that undoing a trash re-inserts the photo
// at its original index in the visible order, not at the end.
func TestUndoTrashRestoresOriginalPosition(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "quickcull-undo-pos-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create three files: a.jpg, b.jpg, c.jpg (alphabetical → indices 0,1,2)
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("fake"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	srv := NewServer()
	if err := srv.LoadState(tmpDir); err != nil {
		t.Fatal(err)
	}
	state := srv.getState()
	if state.Len() != 3 {
		t.Fatalf("Expected 3 files, got %d", state.Len())
	}

	// Trash b.jpg (index 1) physically, then record event with OriginalIndex=1
	if _, err := state.Trash(1); err != nil {
		t.Fatalf("Trash failed: %v", err)
	}
	if state.Len() != 2 {
		t.Fatalf("Expected 2 files after trash, got %d", state.Len())
	}

	trashEvent := bus.Event{
		Type: bus.TypeCommandTrashPhoto,
		Payload: bus.CommandTrashPhotoPayload{
			PhotoID:       "b.jpg",
			OriginalIndex: 1,
		},
	}
	if applied, _, err := srv.applyEvent(trashEvent); err != nil || !applied {
		t.Fatalf("Trash event failed: applied=%v err=%v", applied, err)
	}

	// Undo → b.jpg should be restored at index 1 (between a.jpg and c.jpg)
	undoEvent := bus.Event{Type: bus.TypeCommandUndo, Payload: bus.CommandUndoPayload{}}
	if applied, _, err := srv.applyEvent(undoEvent); err != nil || !applied {
		t.Fatalf("Undo failed: applied=%v err=%v", applied, err)
	}

	if state.Len() != 3 {
		t.Fatalf("Expected 3 files after undo, got %d", state.Len())
	}

	// Verify b.jpg is at index 1, not at end
	idx := state.FindIndex("b.jpg")
	if idx != 1 {
		t.Errorf("b.jpg should be at index 1 after undo, got %d", idx)
	}
	// Verify order is preserved: a.jpg=0, b.jpg=1, c.jpg=2
	if state.FindIndex("a.jpg") != 0 {
		t.Errorf("a.jpg should be at index 0, got %d", state.FindIndex("a.jpg"))
	}
	if state.FindIndex("c.jpg") != 2 {
		t.Errorf("c.jpg should be at index 2, got %d", state.FindIndex("c.jpg"))
	}
}

// TestUndoBatchTrashRestoresOriginalPositions verifies that undoing a batch-trash
// command re-inserts every photo at its original index, not at the end of the list.
func TestUndoBatchTrashRestoresOriginalPositions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "quickcull-undo-batch-pos-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create five files; alphabetical order → indices 0..4
	files := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("fake"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	srv := NewServer()
	if err := srv.LoadState(tmpDir); err != nil {
		t.Fatal(err)
	}
	state := srv.getState()
	if state.Len() != 5 {
		t.Fatalf("want 5 files, got %d", state.Len())
	}

	// Trash b.jpg (original index 1) and d.jpg (original index 3) as a batch.
	// Record indices BEFORE physical removal, matching what App.Trash does.
	bIdx := state.FindIndex("b.jpg") // 1
	dIdx := state.FindIndex("d.jpg") // 3
	if bIdx != 1 || dIdx != 3 {
		t.Fatalf("unexpected pre-trash indices: b=%d d=%d", bIdx, dIdx)
	}

	if _, err := state.TrashMultiplePaths([]string{"b.jpg", "d.jpg"}); err != nil {
		t.Fatalf("TrashMultiplePaths: %v", err)
	}
	if state.Len() != 3 {
		t.Fatalf("want 3 files after trash, got %d", state.Len())
	}

	batchEv := bus.Event{
		Type: bus.TypeCommandBatch,
		Payload: bus.CommandBatchPayload{Events: []bus.Event{
			{Type: bus.TypeCommandTrashPhoto, Payload: bus.CommandTrashPhotoPayload{
				PhotoID: "b.jpg", OldIsTrashed: false, OriginalIndex: bIdx,
			}},
			{Type: bus.TypeCommandTrashPhoto, Payload: bus.CommandTrashPhotoPayload{
				PhotoID: "d.jpg", OldIsTrashed: false, OriginalIndex: dIdx,
			}},
		}},
	}
	if applied, _, err := srv.applyEvent(batchEv); err != nil || !applied {
		t.Fatalf("batch trash event: applied=%v err=%v", applied, err)
	}

	// Undo → b.jpg and d.jpg must come back at their original positions.
	undoEv := bus.Event{Type: bus.TypeCommandUndo, Payload: bus.CommandUndoPayload{}}
	if applied, _, err := srv.applyEvent(undoEv); err != nil || !applied {
		t.Fatalf("undo: applied=%v err=%v", applied, err)
	}

	if state.Len() != 5 {
		t.Fatalf("want 5 files after undo, got %d", state.Len())
	}

	want := map[string]int{"a.jpg": 0, "b.jpg": 1, "c.jpg": 2, "d.jpg": 3, "e.jpg": 4}
	for name, wantIdx := range want {
		if gotIdx := state.FindIndex(name); gotIdx != wantIdx {
			t.Errorf("%s: want index %d, got %d", name, wantIdx, gotIdx)
		}
	}

	// Verify physical restoration: files must be back in the root, not in .trash/.
	trashRoot := filepath.Join(tmpDir, domain.DirTrash)
	for _, name := range []string{"b.jpg", "d.jpg"} {
		if _, err := os.Stat(filepath.Join(tmpDir, name)); os.IsNotExist(err) {
			t.Errorf("%s: file missing from root after undo", name)
		}
		if _, err := os.Stat(filepath.Join(trashRoot, name)); err == nil {
			t.Errorf("%s: file still in .trash after undo", name)
		}
	}
}

func TestUndoPhysicalTrashRestoration(t *testing.T) {
	// 1. Setup temporary directory with a test file
	tmpDir, err := os.MkdirTemp("", "quickcull-undo-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fileName := "test.jpg"
	filePath := filepath.Join(tmpDir, fileName)
	if err := os.WriteFile(filePath, []byte("fake-image-data"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Initialize server and load state
	srv := NewServer()
	// Disable background analysis for deterministic test
	if err := srv.LoadState(tmpDir); err != nil {
		t.Fatal(err)
	}

	state := srv.getState()
	if state.Len() != 1 {
		t.Fatalf("Expected 1 file, got %d", state.Len())
	}

	// 3. Perform TRASH action via App-like flow
	// We simulate what App.Trash does: physically move + publish event
	trashPath := filepath.Join(tmpDir, domain.DirTrash, fileName)
	
	// Use the actual state.Trash to move the file
	newTotal, err := state.Trash(0)
	if err != nil {
		t.Fatalf("Trash failed: %v", err)
	}
	if newTotal != 0 {
		t.Errorf("Expected 0 files after trash, got %d", newTotal)
	}

	// Verify file is in trash
	if _, err := os.Stat(trashPath); os.IsNotExist(err) {
		t.Error("File should be in .trash folder")
	}
	if _, err := os.Stat(filePath); err == nil {
		t.Error("File should NOT be in original location")
	}

	// Record the event in AppState manually as App.Trash would have done
	trashEvent := bus.Event{
		Type: bus.TypeCommandTrashPhoto,
		Payload: bus.CommandTrashPhotoPayload{
			PhotoID: fileName,
		},
	}
	// Use applyEvent for trash too, to ensure InitialState is correctly setup
	appliedTrash, _, err := srv.applyEvent(trashEvent)
	if err != nil {
		t.Fatalf("Trash event failed: %v", err)
	}
	if !appliedTrash {
		t.Fatal("Trash event not applied")
	}

	// 4. Perform UNDO via applyEvent (the core logic we fixed)
	undoEvent := bus.Event{
		Type:    bus.TypeCommandUndo,
		Payload: bus.CommandUndoPayload{},
	}
	
	applied, undoneEvent, err := srv.applyEvent(undoEvent)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !applied {
		t.Fatal("Undo was not applied")
	}
	if undoneEvent.Type != bus.TypeCommandTrashPhoto {
		t.Errorf("Expected undone event to be Trash, got %v", undoneEvent.Type)
	}

	// 5. VERIFY PHYSICAL RESTORATION
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("CRITICAL: File was NOT restored to original location after Undo")
	}
	if _, err := os.Stat(trashPath); err == nil {
		t.Error("File should NOT be in .trash folder after Undo")
	}

	// Verify state consistency
	if state.Len() != 1 {
		t.Errorf("Expected state to have 1 file after Undo, got %d", state.Len())
	}
}

// TestUndo_MixedBatchStructuralRestores verifies that undoing a mixed batch
// (trash + star + label) restores everything: physical files back to root,
// stars cleared, labels cleared. Uses app.Undo() (the public API) to verify
// UndoResponse correctness, not just srv.applyEvent().
func TestUndo_MixedBatchStructuralRestores(t *testing.T) {
	root, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	photoIDs := []string{"a.jpg", "b.jpg", "c.jpg"}

	// 1. Physically trash c.jpg before recording the batch event
	state := app.server.getState()
	cIdx := state.FindIndex(photoIDs[2])
	if cIdx < 0 {
		t.Fatalf("%s not found in state", photoIDs[2])
	}
	if _, err := state.TrashMultiplePaths([]string{photoIDs[2]}); err != nil {
		t.Fatalf("TrashMultiplePaths: %v", err)
	}

	// 2. Apply a mixed batch event (trash c.jpg, star a.jpg, label b.jpg=3)
	batchEv := bus.Event{
		Type: bus.TypeCommandBatch,
		Payload: bus.CommandBatchPayload{Events: []bus.Event{
			{Type: bus.TypeCommandTrashPhoto, Payload: bus.CommandTrashPhotoPayload{
				PhotoID: photoIDs[2], OriginalIndex: cIdx,
			}},
			{Type: bus.TypeCommandToggleStar, Payload: bus.CommandToggleStarPayload{
				PhotoID: photoIDs[0], Starred: true, OldStarred: false,
			}},
			{Type: bus.TypeCommandLabelPhoto, Payload: bus.CommandLabelPhotoPayload{
				PhotoID: photoIDs[1], Label: 3,
			}},
		}},
	}
	if applied, _, err := app.server.applyEvent(batchEv); err != nil || !applied {
		t.Fatalf("mixed batch event: applied=%v err=%v", applied, err)
	}

	// Sanity: c.jpg is gone from active state, a.jpg starred, b.jpg labeled
	if app.server.getState().FindIndex(photoIDs[2]) >= 0 {
		t.Fatal("c.jpg should be absent after batch trash")
	}
	if !app.server.appState.materializePhotos()[photoIDs[0]].IsStarred {
		t.Fatal("a.jpg should be starred")
	}
	if app.server.appState.materializePhotos()[photoIDs[1]].Label != 3 {
		t.Fatal("b.jpg should have label 3")
	}

	// 3. Undo via the public App API (not srv.applyEvent)
	resp, err := app.Undo()
	if err != nil {
		t.Fatalf("first Undo: %v", err)
	}

	// 4. Verify UndoResponse fields
	if resp.ActionType == "" {
		t.Error("UndoResponse.ActionType should not be empty")
	}
	if resp.Index < 0 {
		t.Error("UndoResponse.Index should be >= 0")
	}

	// 5. All three files must be present in active state
	if app.server.getState().Len() != 3 {
		t.Errorf("want 3 files after undo, got %d", app.server.getState().Len())
	}
	for _, name := range photoIDs {
		if idx := app.server.getState().FindIndex(name); idx < 0 {
			t.Errorf("%s missing from state after undo", name)
		}
	}

	// 6. Physical restoration: c.jpg must be back in root, not in .trash
	trashRoot := filepath.Join(root, domain.DirTrash)
	cPath := filepath.Join(root, photoIDs[2])
	if _, err := os.Stat(cPath); os.IsNotExist(err) {
		t.Errorf("%s: file missing from root after undo", photoIDs[2])
	}
	if _, err := os.Stat(filepath.Join(trashRoot, photoIDs[2])); err == nil {
		t.Errorf("%s: still in .trash after undo", photoIDs[2])
	}

	// 7. Metadata rolled back: star undone, label undone
	if app.server.appState.materializePhotos()[photoIDs[0]].IsStarred {
		t.Errorf("%s: star should be cleared after undo", photoIDs[0])
	}
	if app.server.appState.materializePhotos()[photoIDs[1]].Label != 0 {
		t.Errorf("%s: label should be 0 after undo, got %d", photoIDs[1], app.server.appState.materializePhotos()[photoIDs[1]].Label)
	}

	// 8. Second Undo must return ErrNothingToUndo (only one action in history)
	if _, err := app.Undo(); err != domain.ErrNothingToUndo {
		t.Errorf("second Undo: want ErrNothingToUndo, got %v", err)
	}
}
