package review

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"quickcull/internal/persistence"
)

func setScanFilesForTest(t *testing.T, fn scanFilesFunc) {
	t.Helper()
	prev := scanFiles
	scanFiles = fn
	t.Cleanup(func() {
		scanFiles = prev
	})
}

func TestLoadState_ReturnsScanError(t *testing.T) {
	root := t.TempDir()
	srv := NewServer()

	scanFailure := errors.New("scan failed")
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		return scanFailure
	})

	err := srv.LoadState(root)
	if err == nil {
		t.Fatal("expected scan error, got nil")
	}
	var scanErr *ScanError
	if !errors.As(err, &scanErr) {
		t.Fatalf("expected ScanError, got %T (%v)", err, err)
	}
	if scanErr.Operation != scanOpLoadState {
		t.Fatalf("expected operation %q, got %q", scanOpLoadState, scanErr.Operation)
	}
	if !errors.Is(err, scanFailure) {
		t.Fatalf("expected wrapped scan failure, got %v", err)
	}
}

func TestRefresh_PropagatesScanError(t *testing.T) {
	app, _, _ := newAppWithState(t, []string{"a.jpg"})

	scanFailure := errors.New("refresh scan failed")
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		return scanFailure
	})

	_, err := app.Refresh(0)
	if err == nil {
		t.Fatal("expected scan error, got nil")
	}
	var scanErr *ScanError
	if !errors.As(err, &scanErr) {
		t.Fatalf("expected ScanError, got %T (%v)", err, err)
	}
	if scanErr.Operation != scanOpRefresh {
		t.Fatalf("expected operation %q, got %q", scanOpRefresh, scanErr.Operation)
	}
	if !errors.Is(err, scanFailure) {
		t.Fatalf("expected wrapped scan failure, got %v", err)
	}
}

func TestLoadPipeline_SortsAndFinalizesState(t *testing.T) {
	root := t.TempDir()
	writeTinyJPEG(t, root+"/b.jpg")
	writeTinyJPEG(t, root+"/a.jpg")

	srv := NewServer()
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		filesChan <- "b.jpg"
		filesChan <- "a.jpg"
		return nil
	})

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if got := srv.state.Len(); got != 2 {
		t.Fatalf("expected 2 files, got %d", got)
	}
	if got := srv.appState.VisibleOrder; len(got) != 2 || got[0] != "a.jpg" || got[1] != "b.jpg" {
		t.Fatalf("unexpected VisibleOrder: %v", got)
	}
}

func TestLoadState_SnapshotDisabled_UsesLegacyFullScanFlow(t *testing.T) {
	root := t.TempDir()
	writeTinyJPEG(t, root+"/a.jpg")
	writeTinyJPEG(t, root+"/b.jpg")

	store, err := persistence.NewMetadataStore(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("failed to create metadata store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	srv := NewServer()
	srv.persistence = store

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	folderID := domain.GetFolderID(absRoot)
	sig := BuildFolderSignature(absRoot)
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	if err := store.SaveFolderSnapshot(folderID, persistence.FolderSnapshot{
		Version:      folderSnapshotVersion,
		RootPath:     absRoot,
		SavedAt:      time.Now().Unix(),
		Signature:    sig,
		VisibleOrder: []string{"b.jpg", "a.jpg"},
	}); err != nil {
		t.Fatalf("SaveFolderSnapshot failed: %v", err)
	}

	startupSnapshotOverride.Store(srv, false)
	t.Cleanup(func() {
		startupSnapshotOverride.Delete(srv)
	})

	startScan := make(chan struct{})
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		<-startScan
		filesChan <- "a.jpg"
		filesChan <- "b.jpg"
		return nil
	})

	firstSyncState := make(chan AppStateDTO, 1)
	var once sync.Once
	srv.SetBroadcastHook(func(name string, data any) {
		if name != eventSyncState {
			return
		}
		state, ok := data.(AppStateDTO)
		if !ok {
			return
		}
		once.Do(func() {
			firstSyncState <- state
		})
	})

	loadErr := make(chan error, 1)
	go func() {
		loadErr <- srv.LoadState(root)
	}()

	select {
	case state := <-firstSyncState:
		if len(state.VisibleOrder) != 0 {
			t.Fatalf("expected empty first SyncState when snapshot is disabled, got %v", state.VisibleOrder)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first SyncState")
	}

	close(startScan)
	if err := <-loadErr; err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	wantFinal := []string{"a.jpg", "b.jpg"}
	if !slices.Equal(srv.appState.VisibleOrder, wantFinal) {
		t.Fatalf("unexpected final order: got %v want %v", srv.appState.VisibleOrder, wantFinal)
	}
}

func TestReconcileScannedFiles_PreservesMetadataForSurvivingPhotos(t *testing.T) {
	root := t.TempDir()
	writeTinyJPEG(t, root+"/a.jpg")
	writeTinyJPEG(t, root+"/b.jpg")
	writeTinyJPEG(t, root+"/c.jpg")

	srv := NewServer()
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		filesChan <- "a.jpg"
		filesChan <- "b.jpg"
		filesChan <- "c.jpg"
		return nil
	})

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	// Apply metadata to a.jpg and b.jpg
	if _, _, err := srv.applyEvent(bus.Event{
		Type: bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{PhotoID: "a.jpg", Starred: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := srv.applyEvent(bus.Event{
		Type: bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: "b.jpg", Label: 2},
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate refresh: b.jpg removed, d.jpg added
	srv.ReconcileScannedFiles([]string{"a.jpg", "c.jpg", "d.jpg"})

	// a.jpg must keep its star
	p, ok := srv.appState.photo("a.jpg")
	if !ok || !p.IsStarred {
		t.Error("a.jpg must preserve IsStarred after reconcile")
	}
	// b.jpg is gone
	if _, ok := srv.appState.photo("b.jpg"); ok {
		t.Error("b.jpg must be removed after reconcile")
	}
	// c.jpg must still exist with default metadata
	if _, ok := srv.appState.photo("c.jpg"); !ok {
		t.Error("c.jpg must survive reconcile")
	}
	// d.jpg is new, must exist with empty metadata
	p, ok = srv.appState.photo("d.jpg")
	if !ok {
		t.Fatal("d.jpg must be added after reconcile")
	}
	if p.IsStarred || p.Label != 0 {
		t.Error("d.jpg must start with empty metadata")
	}
}

func TestLoadState_RestoresPersistedUndoOnBootstrap(t *testing.T) {
	store, err := persistence.NewMetadataStore(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("failed to create metadata store: %v", err)
	}
	defer store.Close()

	root := t.TempDir()
	writeTinyJPEG(t, root+"/a.jpg")
	absRoot, _ := filepath.Abs(root)
	folderID := domain.GetFolderID(absRoot)

	// Pre-seed history
	seedHistory := []bus.Event{
		{Type: bus.TypeCommandToggleStar, Payload: bus.CommandToggleStarPayload{PhotoID: "a.jpg", Starred: true, OldStarred: false}},
	}
	historyData, _ := json.Marshal(seedHistory)
	if err := store.SaveHistory(folderID, historyData); err != nil {
		t.Fatalf("SaveHistory failed: %v", err)
	}

	// Pre-seed metadata
	if err := store.SaveFolderMetadata(folderID, map[string]persistence.PhotoMetadata{
		"a.jpg": {IsStarred: true},
	}); err != nil {
		t.Fatalf("SaveFolderMetadata failed: %v", err)
	}

	srv := NewServer()
	srv.persistence = store

	startupSnapshotOverride.Store(srv, false)
	t.Cleanup(func() { startupSnapshotOverride.Delete(srv) })

	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		filesChan <- "a.jpg"
		return nil
	})

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	// Undo must be available
	srv.appStateMu.RLock()
	undoLen := srv.appState.UndoLen
	rootPath := srv.appState.Root
	srv.appStateMu.RUnlock()

	if undoLen != 1 {
		t.Errorf("Expected 1 undo slot, got %d", undoLen)
	}
	if rootPath == "" {
		t.Fatal("root must be set")
	}

	// Star must be restored from persisted metadata
	p, ok := srv.appState.photo("a.jpg")
	if !ok || !p.IsStarred {
		t.Error("a.jpg must be starred from persisted metadata")
	}
}
