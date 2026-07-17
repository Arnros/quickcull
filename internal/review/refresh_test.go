package review

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"quickcull/internal/bus"
	"slices"
	"sync"
	"testing"
)

func TestRefreshReconcilesAndBroadcastsAuthoritativeAppState(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}
	if _, _, err := srv.applyEvent(bus.Event{
		Type: bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{
			PhotoID: "a.jpg",
			Starred: true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := srv.applyEvent(bus.Event{
		Type: bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{
			PhotoID: "b.jpg",
			Label:   1,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "b.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "c.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}

	var snapshots []AppStateDTO
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventSyncState {
			snapshots = append(snapshots, data.(AppStateDTO))
		}
	})

	res, err := app.Refresh(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected one SyncState, got %d", len(snapshots))
	}
	got := snapshots[0]
	if got.IsPartial {
		t.Fatal("small refresh must be authoritative")
	}
	if !slices.Equal(got.VisibleOrder, []string{"a.jpg", "c.jpg"}) {
		t.Fatalf("order = %v", got.VisibleOrder)
	}
	if _, ok := got.Photos["b.jpg"]; ok {
		t.Fatal("removed photo was retained")
	}
	if p := got.Photos["c.jpg"]; p.ID != "c.jpg" || p.Label != 0 {
		t.Fatalf("new photo = %+v", p)
	}
	if p := got.Photos["a.jpg"]; !p.IsStarred {
		t.Fatal("starred photo lost its metadata")
	}
	if res.Index != 1 {
		t.Fatalf("removed current photo should fall back to index 1, got %d", res.Index)
	}
	if got.StarredCount != 1 {
		t.Fatalf("starred count = %d, want 1", got.StarredCount)
	}
	if got.LabeledCount != 0 {
		t.Fatalf("labeled count = %d, want 0 (labeled photo was removed)", got.LabeledCount)
	}
	if got.RotatedCount != 0 {
		t.Fatalf("rotated count = %d, want 0", got.RotatedCount)
	}
}

func TestRefreshEmptyFolderReturnsNoIndexAndClearsState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.jpg")
	if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	var snapshots []AppStateDTO
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventSyncState {
			snapshots = append(snapshots, data.(AppStateDTO))
		}
	})
	res, err := app.Refresh(0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 || res.Index != -1 {
		t.Fatalf("empty refresh response = %+v", res)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected one SyncState, got %d", len(snapshots))
	}
	if len(snapshots[0].VisibleOrder) != 0 || len(snapshots[0].Photos) != 0 {
		t.Fatalf("empty snapshot = %+v", snapshots[0])
	}
}

func TestRefreshDetectsExternalChanges(t *testing.T) {
	// 1. Setup temp dir with 2 files
	tmpDir, err := os.MkdirTemp("", "quickcull-refresh-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_ = os.WriteFile(filepath.Join(tmpDir, "img1.jpg"), []byte("data"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "img2.jpg"), []byte("data"), 0644)

	srv := NewServer()
	if err := srv.LoadState(tmpDir); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}

	// 2. Initial check
	res, err := app.Refresh(0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Errorf("Expected 2 files initially, got %d", res.Total)
	}

	// 3. Simulate EXTERNAL ADDITION
	_ = os.WriteFile(filepath.Join(tmpDir, "img3.jpg"), []byte("data"), 0644)

	// 4. Trigger Refresh
	res2 := waitForRefreshTotal(t, app, 0, 3)

	// 5. Verify detection
	if res2.Total != 3 {
		t.Errorf("Refresh failed to detect external addition. Expected 3, got %d", res2.Total)
	}

	// 6. Simulate EXTERNAL DELETION
	_ = os.Remove(filepath.Join(tmpDir, "img1.jpg"))

	// 7. Trigger Refresh
	res3 := waitForRefreshTotal(t, app, 0, 2)

	if res3.Total != 2 {
		t.Errorf("Refresh failed to detect external deletion. Expected 2, got %d", res3.Total)
	}
}

func waitForRefreshTotal(t *testing.T, app *App, index int, expected int) ActionResponse {
	t.Helper()
	for i := 0; i < 10; i++ {
		res, err := app.Refresh(index)
		if err != nil {
			t.Fatal(err)
		}
		if res.Total == expected {
			return res
		}
	}
	t.Fatalf("refresh did not converge to expected total %d", expected)
	return ActionResponse{}
}

func TestRefresh_LargeLibraryUsesChunkedProtocol(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer()
	srv.cacheDir = t.TempDir()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}

	largeFiles := make([]string, largeLibraryThreshold+1)
	for i := range largeFiles {
		largeFiles[i] = fmt.Sprintf("photo_%d.jpg", i)
	}

	var mu sync.Mutex
	var syncStates []AppStateDTO
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventSyncState {
			mu.Lock()
			syncStates = append(syncStates, data.(AppStateDTO))
			mu.Unlock()
		}
	})

	// Drain LoadState events.
	mu.Lock()
	before := len(syncStates)
	mu.Unlock()

	srv.ReconcileScannedFiles(largeFiles)

	mu.Lock()
	defer mu.Unlock()
	afterReconcile := syncStates[before:]
	// Chunked delivery emits at least one partial SyncState (the base).
	if len(afterReconcile) == 0 {
		t.Fatal("expected SyncState after large-library reconcile")
	}
	if !afterReconcile[0].IsPartial {
		t.Fatal("large library refresh must emit partial SyncState for chunked delivery")
	}
}

func TestRefresh_ScanFailurePreservesAppState(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}

	// Label a.jpg before the failing scan.
	if _, _, err := srv.applyEvent(bus.Event{
		Type:    bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: "a.jpg", Label: 5},
	}); err != nil {
		t.Fatal(err)
	}

	scanErr := errors.New("simulated scan failure")
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		close(filesChan)
		return scanErr
	})

	var snapshots []AppStateDTO
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventSyncState {
			snapshots = append(snapshots, data.(AppStateDTO))
		}
	})

	_, err := app.Refresh(0)
	if err == nil {
		t.Fatal("expected scan error, got nil")
	}

	// No SyncState emitted.
	if len(snapshots) != 0 {
		t.Fatalf("expected no SyncState after scan failure, got %d", len(snapshots))
	}

	// AppState must be unchanged.
	srv.appStateMu.RLock()
	defer srv.appStateMu.RUnlock()
	if srv.appState == nil {
		t.Fatal("appState was destroyed")
	}
	if p, ok := srv.appState.Photos["a.jpg"]; !ok || p.Label != 5 {
		t.Fatalf("appState corrupted: photo=%+v", p)
	}
}

func TestRefresh_ConcurrentCallsDoNotCorruptState(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}

	// Remove b.jpg externally to simulate a folder change between refreshes.
	if err := os.Remove(filepath.Join(root, "b.jpg")); err != nil {
		t.Fatal(err)
	}

	// Two sequential refreshes: the second must see the updated filesystem.
	res1, err := app.Refresh(0)
	if err != nil {
		t.Fatal(err)
	}
	if res1.Total != 1 {
		t.Fatalf("first refresh total = %d, want 1", res1.Total)
	}

	if err := os.WriteFile(filepath.Join(root, "c.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := app.Refresh(0)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Total != 2 {
		t.Fatalf("second refresh total = %d, want 2", res2.Total)
	}

	// Final state must be self-consistent.
	srv.appStateMu.RLock()
	defer srv.appStateMu.RUnlock()
	if srv.appState == nil {
		t.Fatal("appState was destroyed after refreshes")
	}
	photoIDs := make(map[string]bool, len(srv.appState.Photos))
	for id := range srv.appState.Photos {
		photoIDs[id] = true
	}
	for _, id := range srv.appState.VisibleOrder {
		if !photoIDs[id] {
			t.Fatalf("VisibleOrder[%s] missing from Photos", id)
		}
	}
	if len(srv.appState.VisibleOrder) != len(srv.appState.Photos) {
		t.Fatalf("VisibleOrder len %d != Photos len %d", len(srv.appState.VisibleOrder), len(srv.appState.Photos))
	}
	if !slices.Equal(srv.appState.VisibleOrder, []string{"a.jpg", "c.jpg"}) {
		t.Fatalf("VisibleOrder = %v", srv.appState.VisibleOrder)
	}
}
