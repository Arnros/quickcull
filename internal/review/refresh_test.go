package review

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"quickcull/internal/bus"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
	var photoChunks int
	chunkPhotoIDs := make(map[string]struct{}, len(largeFiles))
	var chunkTotals []int
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventSyncState {
			mu.Lock()
			syncStates = append(syncStates, data.(AppStateDTO))
			mu.Unlock()
		}
		if name == "sync:state:photos" {
			mu.Lock()
			photoChunks++
			payload := data.(map[string]any)
			for id := range payload["photos"].(map[string]Photo) {
				chunkPhotoIDs[id] = struct{}{}
			}
			chunkTotals = append(chunkTotals, payload["total"].(int))
			mu.Unlock()
		}
	})

	// Drain LoadState events.
	mu.Lock()
	before := len(syncStates)
	beforeChunks := photoChunks
	mu.Unlock()

	srv.ReconcileScannedFiles(largeFiles)

	mu.Lock()
	defer mu.Unlock()
	afterReconcile := syncStates[before:]
	reconcileChunks := photoChunks - beforeChunks
	// Chunked delivery emits at least one partial SyncState (the base).
	if len(afterReconcile) == 0 {
		t.Fatal("expected SyncState after large-library reconcile")
	}
	if !afterReconcile[0].IsPartial {
		t.Fatal("large library refresh must emit partial SyncState for chunked delivery")
	}
	if reconcileChunks < 2 {
		t.Fatalf("expected at least 2 sync:state:photos chunks (5001 photos / 5000 chunk size), got %d", reconcileChunks)
	}
	if len(chunkPhotoIDs) != len(largeFiles) {
		t.Fatalf("chunked photo count = %d, want %d", len(chunkPhotoIDs), len(largeFiles))
	}
	if _, stale := chunkPhotoIDs["a.jpg"]; stale {
		t.Fatal("chunked delivery retained photo absent from reconciled order")
	}
	for _, total := range chunkTotals {
		if total != len(largeFiles) {
			t.Fatalf("chunk total = %d, want %d", total, len(largeFiles))
		}
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
	if p, ok := srv.appState.materializePhotos()["a.jpg"]; !ok || p.Label != 5 {
		t.Fatalf("appState corrupted: photo=%+v", p)
	}
}

func TestRefresh_SequentialCallsDoNotCorruptState(t *testing.T) {
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
	photoIDs := make(map[string]bool, srv.appState.photoCount())
	srv.appState.rangePhotos(func(id string, _ Photo) bool {
		photoIDs[id] = true
		return true
	})
	for _, id := range srv.appState.VisibleOrder {
		if !photoIDs[id] {
			t.Fatalf("VisibleOrder[%s] missing from Photos", id)
		}
	}
	if len(srv.appState.VisibleOrder) != srv.appState.photoCount() {
		t.Fatalf("VisibleOrder len %d != Photos len %d", len(srv.appState.VisibleOrder), srv.appState.photoCount())
	}
	if !slices.Equal(srv.appState.VisibleOrder, []string{"a.jpg", "c.jpg"}) {
		t.Fatalf("VisibleOrder = %v", srv.appState.VisibleOrder)
	}
}

func TestRefreshSerializesConcurrentScans(t *testing.T) {
	root := t.TempDir()
	srv := NewServer()
	srv.state = NewState(root, []string{"a.jpg"})
	srv.appState = &AppState{Root: root, Photos: map[string]Photo{"a.jpg": {ID: "a.jpg"}}, VisibleOrder: []string{"a.jpg"}}
	app := &App{server: srv}

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var active atomic.Int32
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		active.Add(1)
		entered <- struct{}{}
		<-release
		active.Add(-1)
		filesChan <- "a.jpg"
		close(filesChan)
		return nil
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = app.Refresh(0) }()
	<-entered
	go func() { defer wg.Done(); _, _ = app.Refresh(0) }()

	select {
	case <-entered:
		close(release)
		wg.Wait()
		t.Fatalf("refresh scans overlapped; active=%d", active.Load())
	case <-time.After(100 * time.Millisecond):
		close(release)
		wg.Wait()
	}
}

func TestRefreshDiscardsScanWhenFolderChanges(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	srv := NewServer()
	oldState := NewState(oldRoot, []string{"old.jpg"})
	srv.state = oldState
	srv.appState = &AppState{Root: oldRoot, Photos: map[string]Photo{"old.jpg": {ID: "old.jpg"}}, VisibleOrder: []string{"old.jpg"}}
	app := &App{server: srv}

	entered := make(chan struct{})
	release := make(chan struct{})
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		close(entered)
		<-release
		filesChan <- "stale.jpg"
		close(filesChan)
		return nil
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = app.Refresh(0)
	}()
	<-entered

	newState := NewState(newRoot, []string{"new.jpg"})
	srv.stateMu.Lock()
	srv.state = newState
	srv.stateMu.Unlock()
	srv.appStateMu.Lock()
	srv.appState = &AppState{Root: newRoot, Photos: map[string]Photo{"new.jpg": {ID: "new.jpg"}}, VisibleOrder: []string{"new.jpg"}}
	srv.appStateMu.Unlock()
	close(release)
	<-done

	srv.appStateMu.RLock()
	defer srv.appStateMu.RUnlock()
	if srv.appState.Root != newRoot || !slices.Equal(srv.appState.VisibleOrder, []string{"new.jpg"}) {
		t.Fatalf("stale scan overwrote new folder state: %+v", srv.appState)
	}
}

func TestOpenFolderWaitsForRefreshTransaction(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(oldRoot, "a.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newRoot, "b.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer()
	if err := srv.LoadState(oldRoot); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}

	refreshEntered := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var scanCalls atomic.Int32
	setScanFilesForTest(t, func(root string, filesChan chan<- string) error {
		if scanCalls.Add(1) == 1 {
			close(refreshEntered)
			<-releaseRefresh
			filesChan <- "a.jpg"
		} else {
			filesChan <- "b.jpg"
		}
		close(filesChan)
		return nil
	})

	refreshDone := make(chan struct{})
	go func() {
		defer close(refreshDone)
		_, _ = app.Refresh(0)
	}()
	<-refreshEntered

	openDone := make(chan error, 1)
	go func() { openDone <- app.OpenFolder(newRoot) }()

	select {
	case err := <-openDone:
		close(releaseRefresh)
		<-refreshDone
		t.Fatalf("OpenFolder completed during refresh transaction: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseRefresh)
	<-refreshDone
	if err := <-openDone; err != nil {
		t.Fatal(err)
	}
	if got := srv.getState().Root(); got != newRoot {
		t.Fatalf("final root = %q, want %q", got, newRoot)
	}
}
