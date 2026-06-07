package review

import (
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

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

	firstSyncState := make(chan AppState, 1)
	var once sync.Once
	srv.SetBroadcastHook(func(name string, data any) {
		if name != eventSyncState {
			return
		}
		state, ok := data.(AppState)
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
