package review

import (
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"quickcull/internal/persistence"
)

func writeTestJPEG(t *testing.T, root, rel string) {
	t.Helper()

	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir for %q failed: %v", rel, err)
	}

	f, err := os.Create(abs)
	if err != nil {
		t.Fatalf("create jpeg %q failed: %v", rel, err)
	}
	defer f.Close()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg %q failed: %v", rel, err)
	}
}

func newLoadStatePersistenceTestServer(t *testing.T) (*Server, *persistence.MetadataStore) {
	t.Helper()

	store, err := persistence.NewMetadataStore(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("failed to create metadata store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	srv := NewServer()
	srv.persistence = store
	startupSnapshotOverride.Store(srv, true)
	t.Cleanup(func() {
		startupSnapshotOverride.Delete(srv)
	})
	return srv, store
}

func TestServerLoadStateLoadsHistoryFromPersistence(t *testing.T) {
	root := t.TempDir()
	writeTestJPEG(t, root, "a.jpg")

	srv, store := newLoadStatePersistenceTestServer(t)
	folderID := domain.GetFolderID(root)

	wantHistory := []bus.Event{
		{Type: bus.TypeCommandToggleStar, Payload: bus.CommandToggleStarPayload{PhotoID: "a.jpg", Starred: true, OldStarred: false}},
		{Type: bus.TypeCommandLabelPhoto, Payload: bus.CommandLabelPhotoPayload{PhotoID: "a.jpg", Label: 3, OldLabel: 0}},
	}
	historyData, err := json.Marshal(wantHistory)
	if err != nil {
		t.Fatalf("marshal history failed: %v", err)
	}
	if err := store.SaveHistory(folderID, historyData); err != nil {
		t.Fatalf("SaveHistory failed: %v", err)
	}

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if got := len(srv.appState.History); got != len(wantHistory) {
		t.Fatalf("history length mismatch: got %d, want %d", got, len(wantHistory))
	}
	if got := srv.appState.History[0].Payload.(bus.CommandToggleStarPayload).PhotoID; got != "a.jpg" {
		t.Fatalf("unexpected first history payload photo id: got %q, want %q", got, "a.jpg")
	}
	if got := srv.appState.History[1].Payload.(bus.CommandLabelPhotoPayload); got.Label != 3 || got.OldLabel != 0 {
		t.Fatalf("unexpected second history payload: got %+v", got)
	}
}

func TestServerLoadStateHydratesPersistedMetadataAndRecalculatesCountsIgnoringTrashed(t *testing.T) {
	root := t.TempDir()
	files := []string{"keep-star.jpg", "keep-label.jpg", "trashed-star-label.jpg"}
	for _, f := range files {
		writeTestJPEG(t, root, f)
	}

	srv, store := newLoadStatePersistenceTestServer(t)
	folderID := domain.GetFolderID(root)

	persisted := map[string]persistence.PhotoMetadata{
		"keep-star.jpg":          {IsStarred: true, Label: 0, Rotation: 90, IsTrashed: false},
		"keep-label.jpg":         {IsStarred: false, Label: 4, Rotation: 180, IsTrashed: false},
		"trashed-star-label.jpg": {IsStarred: true, Label: 7, Rotation: 270, IsTrashed: true},
	}
	if err := store.SaveFolderMetadata(folderID, persisted); err != nil {
		t.Fatalf("SaveFolderMetadata failed: %v", err)
	}

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	starPhoto := srv.appState.Photos["keep-star.jpg"]
	if !starPhoto.IsStarred || starPhoto.Rotation != 90 || starPhoto.Label != 0 || starPhoto.IsTrashed {
		t.Fatalf("unexpected hydrated metadata for keep-star.jpg: %+v", starPhoto)
	}

	labelPhoto := srv.appState.Photos["keep-label.jpg"]
	if labelPhoto.IsStarred || labelPhoto.Rotation != 180 || labelPhoto.Label != 4 || labelPhoto.IsTrashed {
		t.Fatalf("unexpected hydrated metadata for keep-label.jpg: %+v", labelPhoto)
	}

	trashedPhoto := srv.appState.Photos["trashed-star-label.jpg"]
	if !trashedPhoto.IsStarred || trashedPhoto.Rotation != 270 || trashedPhoto.Label != 7 || !trashedPhoto.IsTrashed {
		t.Fatalf("unexpected hydrated metadata for trashed-star-label.jpg: %+v", trashedPhoto)
	}

	if srv.appState.StarredCount != 1 {
		t.Fatalf("StarredCount mismatch: got %d, want 1", srv.appState.StarredCount)
	}
	if srv.appState.LabeledCount != 1 {
		t.Fatalf("LabeledCount mismatch: got %d, want 1", srv.appState.LabeledCount)
	}
}

func TestServerLoadStateIncludesScannedFilesMissingFromPersistenceAndKeepsVisibleOrderCoherent(t *testing.T) {
	root := t.TempDir()
	writeTestJPEG(t, root, "b.jpg")
	writeTestJPEG(t, root, "a.jpg")
	writeTestJPEG(t, root, "sub/c.jpg")

	srv, store := newLoadStatePersistenceTestServer(t)
	folderID := domain.GetFolderID(root)

	persisted := map[string]persistence.PhotoMetadata{
		"a.jpg":       {IsStarred: true, Label: 2, Rotation: 90, IsTrashed: false},
		"missing.jpg": {IsStarred: false, Label: 0, Rotation: 0, IsTrashed: true},
	}
	if err := store.SaveFolderMetadata(folderID, persisted); err != nil {
		t.Fatalf("SaveFolderMetadata failed: %v", err)
	}

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if _, ok := srv.appState.Photos["b.jpg"]; !ok {
		t.Fatal("scanned file b.jpg missing from Photos")
	}
	if _, ok := srv.appState.Photos["sub/c.jpg"]; !ok {
		t.Fatal("scanned file sub/c.jpg missing from Photos")
	}
	if _, ok := srv.appState.Photos["missing.jpg"]; !ok {
		t.Fatal("persisted file missing.jpg should still be present in Photos")
	}

	b := srv.appState.Photos["b.jpg"]
	if b.IsStarred || b.Label != 0 || b.Rotation != 0 || b.IsTrashed {
		t.Fatalf("scanned file b.jpg should have default hydrated values, got %+v", b)
	}

	wantOrder := []string{"a.jpg", "b.jpg", "sub/c.jpg"}
	if len(srv.appState.VisibleOrder) != len(wantOrder) {
		t.Fatalf("VisibleOrder length mismatch: got %d, want %d (%v)", len(srv.appState.VisibleOrder), len(wantOrder), srv.appState.VisibleOrder)
	}
	for i, want := range wantOrder {
		if got := srv.appState.VisibleOrder[i]; got != want {
			t.Fatalf("VisibleOrder[%d] mismatch: got %q, want %q; full=%v", i, got, want, srv.appState.VisibleOrder)
		}
	}
	for _, id := range srv.appState.VisibleOrder {
		if id == "missing.jpg" {
			t.Fatalf("VisibleOrder should only contain scanned files, got %v", srv.appState.VisibleOrder)
		}
	}
}

func TestServerLoadStateStreamingDiscovery(t *testing.T) {
	root := t.TempDir()
	writeTestJPEG(t, root, "photo1.jpg")
	writeTestJPEG(t, root, "photo2.jpg")
	writeTestJPEG(t, root, "photo3.jpg")

	srv, _ := newLoadStatePersistenceTestServer(t)

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	// Verify state population
	if srv.state.Len() != 3 {
		t.Fatalf("Expected 3 files in state, got %d", srv.state.Len())
	}

	if len(srv.appState.VisibleOrder) != 3 {
		t.Fatalf("Expected 3 files in VisibleOrder, got %d", len(srv.appState.VisibleOrder))
	}
}

func TestLoadState_UsesPersistedSnapshotForImmediateVisibleOrder(t *testing.T) {
	root := t.TempDir()
	writeTestJPEG(t, root, "a.jpg")
	writeTestJPEG(t, root, "b.jpg")

	srv, store := newLoadStatePersistenceTestServer(t)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	folderID := domain.GetFolderID(absRoot)
	sig := BuildFolderSignature(absRoot)
	if sig == "" {
		t.Fatal("expected non-empty folder signature")
	}
	wantFirstOrder := []string{"b.jpg", "a.jpg"}
	if err := store.SaveFolderSnapshot(folderID, persistence.FolderSnapshot{
		Version:      folderSnapshotVersion,
		RootPath:     absRoot,
		SavedAt:      time.Now().Unix(),
		Signature:    sig,
		VisibleOrder: wantFirstOrder,
	}); err != nil {
		t.Fatalf("SaveFolderSnapshot failed: %v", err)
	}

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

	var gotFirst AppState
	select {
	case gotFirst = <-firstSyncState:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first SyncState")
	}

	if !slices.Equal(gotFirst.VisibleOrder, wantFirstOrder) {
		t.Fatalf("first SyncState should hydrate snapshot order, got %v want %v", gotFirst.VisibleOrder, wantFirstOrder)
	}

	close(startScan)
	if err := <-loadErr; err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	wantFinal := []string{"a.jpg", "b.jpg"}
	if !slices.Equal(srv.appState.VisibleOrder, wantFinal) {
		t.Fatalf("final visible order mismatch, got %v want %v", srv.appState.VisibleOrder, wantFinal)
	}
}

func TestLoadState_FallsBackWhenSnapshotInvalid(t *testing.T) {
	root := t.TempDir()
	writeTestJPEG(t, root, "a.jpg")

	srv, store := newLoadStatePersistenceTestServer(t)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	folderID := domain.GetFolderID(absRoot)
	if err := store.SaveFolderSnapshot(folderID, persistence.FolderSnapshot{
		Version:      folderSnapshotVersion,
		RootPath:     absRoot,
		SavedAt:      time.Now().Unix(),
		Signature:    "invalid-signature",
		VisibleOrder: []string{"stale.jpg"},
	}); err != nil {
		t.Fatalf("SaveFolderSnapshot failed: %v", err)
	}

	startScan := make(chan struct{})
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		<-startScan
		filesChan <- "a.jpg"
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

	var gotFirst AppState
	select {
	case gotFirst = <-firstSyncState:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first SyncState")
	}

	if len(gotFirst.VisibleOrder) != 0 {
		t.Fatalf("expected fallback to empty bootstrap order, got %v", gotFirst.VisibleOrder)
	}

	close(startScan)
	if err := <-loadErr; err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
}

func TestLoadState_ReconcileUpdatesOrderWhenFilesystemChanged(t *testing.T) {
	root := t.TempDir()
	writeTestJPEG(t, root, "a.jpg")
	writeTestJPEG(t, root, "b.jpg")

	srv, store := newLoadStatePersistenceTestServer(t)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	folderID := domain.GetFolderID(absRoot)
	sig := BuildFolderSignature(absRoot)
	if sig == "" {
		t.Fatal("expected non-empty folder signature")
	}

	// Deliberately inject stale order while keeping a matching signature.
	if err := store.SaveFolderSnapshot(folderID, persistence.FolderSnapshot{
		Version:      folderSnapshotVersion,
		RootPath:     absRoot,
		SavedAt:      time.Now().Unix(),
		Signature:    sig,
		VisibleOrder: []string{"ghost.jpg", "b.jpg", "a.jpg"},
	}); err != nil {
		t.Fatalf("SaveFolderSnapshot failed: %v", err)
	}

	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		filesChan <- "b.jpg"
		filesChan <- "a.jpg"
		return nil
	})

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	wantOrder := []string{"a.jpg", "b.jpg"}
	if !slices.Equal(srv.appState.VisibleOrder, wantOrder) {
		t.Fatalf("final visible order mismatch: got %v want %v", srv.appState.VisibleOrder, wantOrder)
	}

	saved, found := store.GetFolderSnapshot(folderID)
	if !found {
		t.Fatal("expected snapshot to be saved after reconcile")
	}
	if !slices.Equal(saved.VisibleOrder, wantOrder) {
		t.Fatalf("saved snapshot should be reconciled order: got %v want %v", saved.VisibleOrder, wantOrder)
	}
}
