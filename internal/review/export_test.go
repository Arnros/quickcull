package review

import (
	"quickcull/internal/bus"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	for {
		srv.exportMu.Lock()
		active := srv.exportCancel != nil
		srv.exportMu.Unlock()
		if !active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

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
	for {
		srv.exportMu.Lock()
		active := srv.exportCancel != nil
		srv.exportMu.Unlock()
		if !active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

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
	app.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{PhotoID: photoID, Starred: true, OldStarred: false},
	})
	app.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: photoID, Label: 3, OldLabel: 0},
	})
	app.server.applyEvent(bus.Event{
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

	for {
		app.server.exportMu.Lock()
		active := app.server.exportCancel != nil
		app.server.exportMu.Unlock()
		if !active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	srv2 := NewServer()
	srv2.persistence = app.server.persistence
	if err := srv2.LoadState(destDir); err != nil {
		t.Fatal(err)
	}

	got := srv2.appState.Photos[photoID]
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
