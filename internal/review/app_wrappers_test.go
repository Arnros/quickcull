package review

import (
	"context"
	"testing"
	"time"

	"quickcull/internal/persistence"
)

func TestAppCancelExportCancelsActiveExport(t *testing.T) {
	server := NewServer()
	exportCtx, cancel := context.WithCancel(context.Background())
	server.exportMu.Lock()
	server.exportCancel = cancel
	server.exportMu.Unlock()

	NewApp(server).CancelExport()

	select {
	case <-exportCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("App.CancelExport did not cancel the active export")
	}
	server.exportMu.Lock()
	defer server.exportMu.Unlock()
	if server.exportCancel != nil {
		t.Fatal("App.CancelExport left the export cancellation function active")
	}
}

func TestAppFlushPersistenceWaitsForQueuedWrite(t *testing.T) {
	server := NewServer()
	store := newNonBlockingPersistence()
	server.persistence = store
	writer := server.asyncPersistenceWriter()
	t.Cleanup(server.closePersistence)
	writer.enqueueSingle("folder", "photo.jpg", persistence.PhotoMetadata{Label: 3})

	NewApp(server).FlushPersistence()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.photoCalls != 1 {
		t.Fatalf("persisted photo calls = %d, want 1", store.photoCalls)
	}
	if store.lastPhotoMeta.Label != 3 {
		t.Fatalf("persisted label = %d, want 3", store.lastPhotoMeta.Label)
	}
}

func TestAppFlushPersistenceAllowsNilReceiver(t *testing.T) {
	var app *App
	app.FlushPersistence()
	(&App{}).FlushPersistence()
}
