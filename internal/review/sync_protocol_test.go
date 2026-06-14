package review

import (
	"fmt"
	"sync"
	"testing"
)

func TestSyncLargeLibraryProtocol(t *testing.T) {
	srv := NewServer()
	
	// Create a state with 5001 photos to trigger "large library" mode
	photoCount := 5001
	files := make([]string, photoCount)
	photos := make(map[string]Photo, photoCount)
	for i := 0; i < photoCount; i++ {
		name := fmt.Sprintf("photo_%d.jpg", i)
		files[i] = name
		photos[name] = Photo{ID: name}
	}
	
	appState := &AppState{
		Root:         "/tmp",
		VisibleOrder: files,
		Photos:       photos,
	}
	
	srv.appStateMu.Lock()
	srv.appState = appState
	srv.appStateMu.Unlock()

	// Ensure srv.getState() returns non-nil for RefreshVisibleOrder to work
	srv.state = NewState("/tmp", files)

	var mu sync.Mutex
	var events []string
	var finalPhotos map[string]Photo
	var syncCount int

	srv.SetBroadcastHook(func(name string, data any) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, name)
		if name == eventSyncState {
			syncCount++
			if state, ok := data.(AppStateDTO); ok {
				finalPhotos = state.Photos
			}
		}
	})

	// Trigger sync
	srv.SyncFullState()

	// Wait for async events if any (though SyncFullState is synchronous in its broadcast calls)
	
	mu.Lock()
	defer mu.Unlock()

	// 1. Verify sequence start
	if len(events) < 2 {
		t.Fatalf("Expected at least 2 events, got %d: %v", len(events), events)
	}
	
	// 2. Verify large library suppression
	// The final SyncState should have an empty Photos map
	if syncCount < 2 {
		t.Fatalf("Expected at least 2 SyncState events (bootstrap + final), got %d", syncCount)
	}
	
	if len(finalPhotos) != 0 {
		t.Errorf("Expected final SyncState to have 0 photos for large library, got %d", len(finalPhotos))
	}
	
	// 3. Verify chunks were sent
	foundChunks := false
	for _, e := range events {
		if e == "sync:state:photos" {
			foundChunks = true
			break
		}
	}
	if !foundChunks {
		t.Errorf("Expected sync:state:photos chunks to be sent")
	}

	// 4. Verify structural sync at the end
	last := events[len(events)-1]
	if last != eventSyncState {
		t.Errorf("Expected final structural event to be SyncState, got %s", last)
	}
}

func TestSyncSmallLibraryProtocol(t *testing.T) {
	srv := NewServer()
	
	// Small library
	photoCount := 10
	files := make([]string, photoCount)
	photos := make(map[string]Photo, photoCount)
	for i := 0; i < photoCount; i++ {
		name := fmt.Sprintf("photo_%d.jpg", i)
		files[i] = name
		photos[name] = Photo{ID: name}
	}
	
	appState := &AppState{
		Root:         "/tmp",
		VisibleOrder: files,
		Photos:       photos,
	}
	
	srv.appStateMu.Lock()
	srv.appState = appState
	srv.appStateMu.Unlock()

	// Ensure srv.getState() returns non-nil for RefreshVisibleOrder to work
	srv.state = NewState("/tmp", files)

	var mu sync.Mutex
	var finalPhotos map[string]Photo

	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventSyncState {
			if state, ok := data.(AppStateDTO); ok {
				mu.Lock()
				finalPhotos = state.Photos
				mu.Unlock()
			}
		}
	})

	srv.SyncFullState()

	mu.Lock()
	defer mu.Unlock()
	if len(finalPhotos) != photoCount {
		t.Errorf("Expected final SyncState to have %d photos for small library, got %d", photoCount, len(finalPhotos))
	}
}

func TestSyncEmptyFolderProtocol(t *testing.T) {
	srv := NewServer()
	
	// Empty state
	appState := &AppState{
		Root:         "/tmp/empty",
		VisibleOrder: []string{},
		Photos:       make(map[string]Photo),
	}
	
	srv.appStateMu.Lock()
	srv.appState = appState
	srv.appStateMu.Unlock()

	var mu sync.Mutex
	var events []string
	var syncCount int

	srv.SetBroadcastHook(func(name string, data any) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, name)
		if name == eventSyncState {
			syncCount++
		}
	})

	srv.SyncFullState()

	mu.Lock()
	defer mu.Unlock()

	// 1. Verify sequence: only one SyncState should be emitted for empty folders
	// to avoid confusing the UI or tests.
	if syncCount != 1 {
		t.Errorf("Expected exactly 1 SyncState for empty folder, got %d", syncCount)
	}

	for _, e := range events {
		if e == "sync:state:photos" || e == "sync:state:base" {
			t.Errorf("Did not expect chunked events for empty folder, got %s", e)
		}
	}
}

func TestSyncStructuralUpdateProtocol(t *testing.T) {
	srv := NewServer()
	
	// Normal library
	photoCount := 10
	files := make([]string, photoCount)
	photos := make(map[string]Photo, photoCount)
	for i := 0; i < photoCount; i++ {
		name := fmt.Sprintf("photo_%d.jpg", i)
		files[i] = name
		photos[name] = Photo{ID: name}
	}
	
	appState := &AppState{
		Root:         "/tmp",
		VisibleOrder: files,
		Photos:       photos,
	}
	
	srv.appStateMu.Lock()
	srv.appState = appState
	srv.appStateMu.Unlock()

	var mu sync.Mutex
	var isPartial bool
	var photoMap map[string]Photo

	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventSyncState {
			if state, ok := data.(AppStateDTO); ok {
				mu.Lock()
				isPartial = state.IsPartial
				photoMap = state.Photos
				mu.Unlock()
			}
		}
	})

	// Trigger a structural update (simulating what happens in applyEvent for isStructuralChange)
	snapshot := BuildSyncSnapshot(*appState, false)
	snapshot.IsPartial = true
	srv.broadcast(eventSyncState, snapshot)

	mu.Lock()
	defer mu.Unlock()
	
	if !isPartial {
		t.Errorf("Expected IsPartial to be true for structural update (photos omitted)")
	}
	if len(photoMap) != 0 {
		t.Errorf("Expected 0 photos in structural update, got %d", len(photoMap))
	}
}
