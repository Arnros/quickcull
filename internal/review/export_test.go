package review

import (
	"errors"
	"os"
	"path/filepath"
	"quickcull/internal/bus"
	"quickcull/internal/persistence"
	"slices"
	"sync"
	"testing"
	"time"
)

type failingMovePersistence struct {
	persistence.StateStore
	saveErr     error
	removeCalls int
}

func (p *failingMovePersistence) SavePhotoMetadata(string, string, persistence.PhotoMetadata) error {
	return p.saveErr
}

func (p *failingMovePersistence) RemovePhotoMetadata(string, string) error {
	p.removeCalls++
	return nil
}

func waitForExport(t *testing.T, srv *Server) {
	t.Helper()
	for {
		srv.exportMu.Lock()
		active := srv.exportCancel != nil
		srv.exportMu.Unlock()
		if !active {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServerExportFiles(t *testing.T) {
	// 1. Setup source directory
	srcDir, err := os.MkdirTemp("", "quickcull-export-src")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	files := []string{"test1.jpg", "test2.jpg"}
	for _, f := range files {
		_ = os.WriteFile(filepath.Join(srcDir, f), []byte("dummy"), 0644)
	}

	// 2. Setup dest directory
	destDir, err := os.MkdirTemp("", "quickcull-export-dest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(destDir)

	// 3. Setup server
	srv := NewServer()
	if err := srv.LoadState(srcDir); err != nil {
		t.Fatal(err)
	}

	// 4. Export (Copy mode)
	paths := []string{"test1.jpg", "test2.jpg"}
	if err := srv.ExportFilesPaths(paths, destDir, false); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// WAIT for async export
	waitForExport(t, srv)

	// 5. Verify
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(destDir, f)); os.IsNotExist(err) {
			t.Errorf("exported file %s not found in dest dir", f)
		}
	}

	// 6. Export (Move mode)
	destDir2, _ := os.MkdirTemp("", "quickcull-export-dest2")
	defer os.RemoveAll(destDir2)

	if err := srv.ExportFilesPaths(paths, destDir2, true); err != nil {
		t.Fatalf("move export failed: %v", err)
	}

	// WAIT for async export
	waitForExport(t, srv)

	// Verify moved files are in new dest and gone from source
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(destDir2, f)); os.IsNotExist(err) {
			t.Errorf("moved file %s not found in dest dir", f)
		}
		if _, err := os.Stat(filepath.Join(srcDir, f)); !os.IsNotExist(err) {
			t.Errorf("moved file %s still exists in source dir", f)
		}
	}
}

func TestMoveExportPreservesMetadataInDestinationFolder(t *testing.T) {
	_, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	photoID := "b.jpg"
	mustApplyEvent(t, app.server, bus.Event{
		Type:    bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{PhotoID: photoID, Starred: true, OldStarred: false},
	})
	mustApplyEvent(t, app.server, bus.Event{
		Type:    bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: photoID, Label: 3, OldLabel: 0},
	})
	mustApplyEvent(t, app.server, bus.Event{
		Type:    bus.TypeCommandRotatePhoto,
		Payload: bus.CommandRotatePhotoPayload{PhotoID: photoID, Direction: "right"},
	})

	destDir, err := os.MkdirTemp("", "quickcull-export-meta-dest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(destDir)

	if err := app.server.ExportFilesPaths([]string{photoID}, destDir, true); err != nil {
		t.Fatalf("move export failed: %v", err)
	}

	waitForExport(t, app.server)

	srv2 := NewServer()
	srv2.persistence = app.server.persistence
	if err := srv2.LoadState(destDir); err != nil {
		t.Fatal(err)
	}

	got := srv2.appState.materializePhotos()[photoID]
	if !got.IsStarred {
		t.Error("expected moved photo to keep star metadata")
	}
	if got.Label != 3 {
		t.Errorf("expected moved photo to keep label 3, got %d", got.Label)
	}
	if got.Rotation != 90 {
		t.Errorf("expected moved photo to keep rotation 90, got %d", got.Rotation)
	}
}

func TestStaleFileReferenceReconciledDuringMove(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dest := t.TempDir()
	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}
	if err := os.Remove(filepath.Join(root, "b.jpg")); err != nil {
		t.Fatal(err)
	}

	var eventsMu sync.Mutex
	var events []string
	var snapshots []AppStateDTO
	srv.SetBroadcastHook(func(name string, data any) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, name)
		if name == eventSyncState {
			snapshots = append(snapshots, data.(AppStateDTO))
		}
	})

	if err := srv.ExportFilesPaths([]string{"a.jpg", "b.jpg"}, dest, true); err != nil {
		t.Fatal(err)
	}
	waitForExport(t, srv)
	res, err := app.Refresh(0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 {
		t.Fatalf("source total = %d", res.Total)
	}
	if _, err := os.Stat(filepath.Join(dest, "a.jpg")); err != nil {
		t.Fatalf("successful move missing from destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.jpg")); !os.IsNotExist(err) {
		t.Fatalf("successful move remains in source: %v", err)
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()
	folderChanged := slices.Index(events, eventFolderChanged)
	exportComplete := slices.Index(events, eventExportComplete)
	if folderChanged < 0 || exportComplete < 0 || folderChanged > exportComplete {
		t.Fatalf("unexpected export event order: %v", events)
	}
	if len(snapshots) != 1 || len(snapshots[0].Photos) != 0 {
		t.Fatalf("refresh did not clear source state: %+v", snapshots)
	}
}

func TestPartialMoveFailedFileRemainsAtSource(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dest := t.TempDir()
	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}

	// Apply metadata before the failed move to verify it is preserved.
	if _, _, err := srv.applyEvent(bus.Event{
		Type:    bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: "b.jpg", Label: 3},
	}); err != nil {
		t.Fatal(err)
	}

	// resolveExportDest renames b.jpg → b_copy.jpg when a collision exists.
	// Pre-create b.jpg as a regular file (triggers rename) and b_copy.jpg
	// as a directory so os.Rename fails with ENOTDIR.  The copy fallback
	// also fails, leaving b.jpg at the source.
	if err := os.WriteFile(filepath.Join(dest, "b.jpg"), []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dest, "b_copy.jpg"), 0o755); err != nil {
		t.Fatal(err)
	}

	var eventsMu sync.Mutex
	var events []string
	var snapshots []AppStateDTO
	var exportCompletePayload map[string]any
	srv.SetBroadcastHook(func(name string, data any) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, name)
		if name == eventSyncState {
			snapshots = append(snapshots, data.(AppStateDTO))
		}
		if name == eventExportComplete {
			exportCompletePayload = data.(map[string]any)
		}
	})

	if err := srv.ExportFilesPaths([]string{"a.jpg", "b.jpg"}, dest, true); err != nil {
		t.Fatal(err)
	}
	waitForExport(t, srv)

	// a.jpg moved successfully.
	if _, err := os.Stat(filepath.Join(dest, "a.jpg")); err != nil {
		t.Fatalf("successful move missing from destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.jpg")); !os.IsNotExist(err) {
		t.Fatal("successful move still present at source")
	}

	// b.jpg move failed: source preserved with original content.
	if _, err := os.Stat(filepath.Join(root, "b.jpg")); err != nil {
		t.Fatalf("failed move removed source file: %v", err)
	}
	bContent, _ := os.ReadFile(filepath.Join(root, "b.jpg"))
	if string(bContent) != "jpeg" {
		t.Fatal("source content modified, move may have partially succeeded")
	}

	res, err := app.Refresh(0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("source total = %d, want 1 (b.jpg preserved)", res.Total)
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()
	if len(snapshots) != 1 {
		t.Fatalf("expected one SyncState, got %d", len(snapshots))
	}
	got := snapshots[0]
	if !slices.Equal(got.VisibleOrder, []string{"b.jpg"}) {
		t.Fatalf("VisibleOrder = %v, want [b.jpg]", got.VisibleOrder)
	}
	if _, ok := got.Photos["b.jpg"]; !ok {
		t.Fatal("preserved file missing from authoritative state")
	}
	if p := got.Photos["b.jpg"]; p.Label != 3 {
		t.Fatalf("preserved photo lost its label: got %d, want 3", p.Label)
	}
	if got.LabeledCount != 1 {
		t.Fatalf("labeled count = %d, want 1", got.LabeledCount)
	}
	if _, ok := got.Photos["a.jpg"]; ok {
		t.Fatal("moved file still in authoritative state")
	}
	if exportCompletePayload["root"] != root {
		t.Fatalf("export root = %v, want %s", exportCompletePayload["root"], root)
	}
	if moved, ok := exportCompletePayload["movedPaths"].([]string); !ok || !slices.Equal(moved, []string{"a.jpg"}) {
		t.Fatalf("movedPaths = %#v, want [a.jpg]", exportCompletePayload["movedPaths"])
	}
}

func TestCancelledMoveEmitsFolderChangedAfterPartialSuccess(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dest := t.TempDir()
	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var events []string
	var cancelledPayload map[string]any
	cancelled := make(chan struct{})
	var cancelOnce sync.Once
	srv.SetBroadcastHook(func(name string, data any) {
		mu.Lock()
		events = append(events, name)
		mu.Unlock()
		if name == eventExportProgress && data.(map[string]any)["current"].(int) == 1 {
			srv.CancelExport()
		}
		if name == eventExportCancelled {
			cancelledPayload, _ = data.(map[string]any)
			cancelOnce.Do(func() { close(cancelled) })
		}
	})

	if err := srv.ExportFilesPaths([]string{"a.jpg", "b.jpg"}, dest, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for export cancellation")
	}

	mu.Lock()
	defer mu.Unlock()
	if slices.Index(events, eventFolderChanged) < 0 {
		t.Fatalf("partial cancelled move did not emit folder:changed: %v", events)
	}
	if _, err := os.Stat(filepath.Join(dest, "a.jpg")); err != nil {
		t.Fatalf("first file was not moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.jpg")); err != nil {
		t.Fatalf("unprocessed file did not remain at source: %v", err)
	}
	if cancelledPayload["root"] != root {
		t.Fatalf("cancelled export root = %v, want %s", cancelledPayload["root"], root)
	}
	if moved, ok := cancelledPayload["movedPaths"].([]string); !ok || !slices.Equal(moved, []string{"a.jpg"}) {
		t.Fatalf("cancelled movedPaths = %#v, want [a.jpg]", cancelledPayload["movedPaths"])
	}
}

func TestMoveMetadataSaveFailureKeepsSourceMetadata(t *testing.T) {
	root := t.TempDir()
	dest := t.TempDir()
	srv := NewServer()
	srv.appState = &AppState{
		Root: root,
		Photos: map[string]Photo{
			"a.jpg": {ID: "a.jpg", IsStarred: true, Label: 4},
		},
		VisibleOrder: []string{"a.jpg"},
	}
	store := &failingMovePersistence{saveErr: errors.New("destination persistence unavailable")}
	srv.persistence = store

	srv.transferMovedMetadata("a.jpg", dest, filepath.Join(dest, "a.jpg"))

	if store.removeCalls != 0 {
		t.Fatalf("source metadata removed after destination save failure: %d calls", store.removeCalls)
	}
	if p := srv.appState.materializePhotos()["a.jpg"]; !p.IsStarred || p.Label != 4 {
		t.Fatalf("in-memory source metadata changed: %+v", p)
	}
}

func TestExportSelectionLabelZeroReportsAllNonZeroLabels(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"red.jpg", "blue.jpg", "none.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dest := t.TempDir()
	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	for id, label := range map[string]int{"red.jpg": 1, "blue.jpg": 2} {
		if _, _, err := srv.applyEvent(bus.Event{
			Type:    bus.TypeCommandLabelPhoto,
			Payload: bus.CommandLabelPhotoPayload{PhotoID: id, Label: label},
		}); err != nil {
			t.Fatal(err)
		}
	}

	complete := make(chan map[string]any, 1)
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventExportComplete {
			complete <- data.(map[string]any)
		}
	})
	app := &App{server: srv}
	if err := app.ExportSelection("label", 0, dest, true); err != nil {
		t.Fatal(err)
	}

	select {
	case payload := <-complete:
		moved, ok := payload["movedPaths"].([]string)
		if !ok {
			t.Fatalf("movedPaths type = %T", payload["movedPaths"])
		}
		slices.Sort(moved)
		if !slices.Equal(moved, []string{"blue.jpg", "red.jpg"}) {
			t.Fatalf("movedPaths = %v", moved)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for export completion")
	}
	if _, err := os.Stat(filepath.Join(root, "none.jpg")); err != nil {
		t.Fatalf("unlabelled photo should remain: %v", err)
	}
}

func TestExportSelectionStarredReportsOnlyStarredPaths(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"star.jpg", "plain.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := srv.applyEvent(bus.Event{
		Type:    bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{PhotoID: "star.jpg", Starred: true},
	}); err != nil {
		t.Fatal(err)
	}
	complete := make(chan map[string]any, 1)
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventExportComplete {
			complete <- data.(map[string]any)
		}
	})
	if err := (&App{server: srv}).ExportSelection("starred", 0, t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-complete:
		if moved, ok := payload["movedPaths"].([]string); !ok || !slices.Equal(moved, []string{"star.jpg"}) {
			t.Fatalf("movedPaths = %#v, want [star.jpg]", payload["movedPaths"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for starred export")
	}
}

func TestMoveReportsRelativePathsForSameBasenameInSubfolders(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"one/same.jpg", "two/same.jpg"} {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	complete := make(chan map[string]any, 1)
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventExportComplete {
			complete <- data.(map[string]any)
		}
	})
	paths := []string{"one/same.jpg", "two/same.jpg"}
	if err := srv.ExportFilesPaths(paths, t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-complete:
		moved := payload["movedPaths"].([]string)
		if !slices.Equal(moved, paths) {
			t.Fatalf("movedPaths = %v, want %v", moved, paths)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for nested export")
	}
}

func TestSecondExportCancelsFirstWithoutMixingMovedPaths(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	cancelled := make(chan map[string]any, 1)
	complete := make(chan map[string]any, 1)
	var startSecond sync.Once
	srv.SetBroadcastHook(func(name string, data any) {
		if name == eventExportProgress && data.(map[string]any)["file"] == "a.jpg" {
			startSecond.Do(func() {
				if err := srv.ExportFilesPaths([]string{"c.jpg"}, dest, true); err != nil {
					t.Errorf("second export: %v", err)
				}
			})
		}
		if name == eventExportCancelled {
			cancelled <- data.(map[string]any)
		}
		if name == eventExportComplete {
			complete <- data.(map[string]any)
		}
	})
	if err := srv.ExportFilesPaths([]string{"a.jpg", "b.jpg"}, dest, true); err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-cancelled:
		if moved := payload["movedPaths"].([]string); !slices.Equal(moved, []string{"a.jpg"}) {
			t.Fatalf("cancelled movedPaths = %v", moved)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first export cancellation")
	}
	select {
	case payload := <-complete:
		if moved := payload["movedPaths"].([]string); !slices.Equal(moved, []string{"c.jpg"}) {
			t.Fatalf("second movedPaths = %v", moved)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second export completion")
	}
}

func TestMoveDoesNotReportPathWhenSourceRemovalFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure cannot be reproduced as root")
	}
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "photo.jpg")
	if err := os.WriteFile(source, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sourceDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sourceDir, 0o755) })
	destination := filepath.Join(t.TempDir(), "photo.jpg")
	moved := false

	err := (&Server{}).exportSingleFile(source, destination, true, &moved)

	if err != nil {
		t.Fatalf("copy fallback should succeed even when source removal fails: %v", err)
	}
	if moved {
		t.Fatal("path reported moved although source removal failed")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source should remain: %v", err)
	}
}
