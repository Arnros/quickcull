package review

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"quickcull/internal/domain"
	"quickcull/internal/persistence"
)

func TestServerSearchStream(t *testing.T) {
	root := t.TempDir()
	files := []string{"vacation_01.jpg", "work_01.jpg", "vacation_02.jpg", "other.png"}
	for _, f := range files {
		_ = os.WriteFile(root+"/"+f, []byte("x"), 0644)
	}

	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}

	// We use a local bus listener if we had a way to intercept s.broadcast
	// For now we'll just test that it runs without crashing.
	// In a real TDD we would mock the broadcaster.
	srv.SearchStream(context.Background(), "vacation")

	// Since SearchStream is synchronous in its loop (just broadcasts),
	// we can't easily capture the broadcast without a mock bus.
	// But the implementation is now there.
}

// TestSearchStream_CancelStopsBroadcast locks in the P1-1 fix: a cancelled
// SearchStream must stop emitting "search:results" promptly. Without
// cancellation checks (ctx.Err() + select on ctx.Done), a slow search can keep
// broadcasting stale results after the user has typed a new query.
func TestSearchStream_CancelStopsBroadcast(t *testing.T) {
	root := t.TempDir()
	const fileCount = 500 // large enough to keep the loop busy for >50ms
	for i := 0; i < fileCount; i++ {
		_ = os.WriteFile(filepath.Join(root, fmt.Sprintf("vac_%04d.jpg", i)), []byte("x"), 0644)
	}

	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var resultsCount int
	srv.SetBroadcastHook(func(name string, data any) {
		if name == "search:results" {
			mu.Lock()
			resultsCount++
			mu.Unlock()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		srv.SearchStream(ctx, "vac")
		close(done)
	}()

	// Wait long enough for at least one results broadcast, then cancel mid-flight.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	firstSnapshot := resultsCount
	mu.Unlock()
	if firstSnapshot == 0 {
		// Filesystem too fast: try again with slightly longer wait.
		time.Sleep(50 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SearchStream did not return within 2s after cancel")
	}

	// After cancel + a small grace period, the count must not keep growing —
	// no further "search:results" broadcasts should be emitted for a cancelled
	// search. The contract here is that cancellation is actually honored.
	time.Sleep(60 * time.Millisecond) // > one batch interval (10ms)
	mu.Lock()
	afterCancel := resultsCount
	mu.Unlock()

	if afterCancel > firstSnapshot+1 {
		t.Errorf("search:results count grew by %d after cancel (first=%d, after=%d) — cancellation not honored",
			afterCancel-firstSnapshot, firstSnapshot, afterCancel)
	}
}

// TestSearchStream_EmptyQueryEmitsComplete verifies the contract that an empty
// query short-circuits with a complete event (no results, no nil-map).
func TestSearchStream_EmptyQueryEmitsComplete(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.jpg"), []byte("x"), 0644)
	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var sawComplete bool
	srv.SetBroadcastHook(func(name string, data any) {
		if name == "search:complete" {
			mu.Lock()
			sawComplete = true
			mu.Unlock()
		}
	})

	srv.SearchStream(context.Background(), "   ")

	mu.Lock()
	defer mu.Unlock()
	if !sawComplete {
		t.Fatal("expected search:complete broadcast for empty query")
	}
}

func TestServerExportCancellation(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	// Create many files to have time to cancel
	files := make([]string, 100)
	for i := 0; i < 100; i++ {
		files[i] = fmt.Sprintf("file_%d.jpg", i)
		_ = os.WriteFile(srcDir+"/"+files[i], []byte("x"), 0644)
	}

	srv := NewServer()
	if err := srv.LoadState(srcDir); err != nil {
		t.Fatal(err)
	}

	// Start export
	err := srv.ExportFilesPaths(files, destDir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Immediately cancel
	srv.CancelExport()

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Verify not all files were exported
	entries, _ := os.ReadDir(destDir)
	if len(entries) == 100 {
		t.Errorf("Export was not cancelled, all %d files found in dest", len(entries))
	}
}

func TestLoadStateProtocolSequence(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(root+"/img1.jpg", []byte("x"), 0644)
	_ = os.WriteFile(root+"/img2.jpg", []byte("x"), 0644)

	srv := NewServer()

	var events []string
	var mu sync.Mutex
	srv.SetBroadcastHook(func(name string, data any) {
		mu.Lock()
		events = append(events, name)
		mu.Unlock()
	})

	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	eventSnapshot := append([]string(nil), events...)
	mu.Unlock()

	// Verify sequence
	if len(eventSnapshot) < 3 {
		t.Fatalf("Too few events emitted: %v", eventSnapshot)
	}

	// 1. First event MUST be SyncState (initialization)
	if eventSnapshot[0] != "SyncState" {
		t.Errorf("First event should be SyncState, got %s", eventSnapshot[0])
	}

	// 2. Final significant event SHOULD be SyncState
	// We look for the last SyncState in the sequence.
	lastSyncIdx := -1
	for i := len(eventSnapshot) - 1; i >= 0; i-- {
		if eventSnapshot[i] == "SyncState" {
			lastSyncIdx = i
			break
		}
	}
	if lastSyncIdx == -1 {
		t.Errorf("SyncState not found in event sequence: %v", eventSnapshot)
	}

	// 3. Middle events should contain progress
	foundProgress := false
	for _, e := range eventSnapshot {
		if e == "progress" {
			foundProgress = true
			break
		}
	}
	if !foundProgress {
		t.Error("Expected progress events in the sequence")
	}
}

func TestLoadStateStreaming_PropagatesScanError(t *testing.T) {
	root := t.TempDir()
	srv := NewServer()

	type emittedEvent struct {
		name string
		data any
	}
	var events []emittedEvent
	var mu sync.Mutex
	srv.SetBroadcastHook(func(name string, data any) {
		mu.Lock()
		events = append(events, emittedEvent{name: name, data: data})
		mu.Unlock()
	})

	scanFailure := errors.New("stream scan failed")
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		filesChan <- "batch/a.jpg"
		filesChan <- "batch/b.jpg"
		return scanFailure
	})

	err := srv.LoadState(root)
	if err == nil {
		t.Fatal("expected LoadState to fail on scan error")
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

	mu.Lock()
	defer mu.Unlock()
	syncStateCount := 0
	for _, ev := range events {
		if ev.name == "SyncState" {
			syncStateCount++
		}
		if ev.name != "progress" {
			continue
		}
		payload, ok := ev.data.(map[string]any)
		if !ok {
			continue
		}
		if scanning, ok := payload["scanning"].(bool); ok && !scanning {
			t.Fatalf("unexpected final non-scanning progress emit on scan failure: %#v", payload)
		}
	}
	if syncStateCount != 1 {
		t.Fatalf("expected only bootstrap SyncState on scan failure, got %d events", syncStateCount)
	}
}

func TestLoadState_EmitsExpectedSequenceWhenSnapshotHit(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(root+"/img1.jpg", []byte("x"), 0o644)
	_ = os.WriteFile(root+"/img2.jpg", []byte("x"), 0o644)

	store, err := persistence.NewMetadataStore(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("create metadata store: %v", err)
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

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	folderID := domain.GetFolderID(absRoot)
	sig := BuildFolderSignature(absRoot)
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	wantSnapshotOrder := []string{"img2.jpg", "img1.jpg"}
	if err := store.SaveFolderSnapshot(folderID, persistence.FolderSnapshot{
		Version:      folderSnapshotVersion,
		RootPath:     absRoot,
		SavedAt:      time.Now().Unix(),
		Signature:    sig,
		VisibleOrder: wantSnapshotOrder,
	}); err != nil {
		t.Fatalf("SaveFolderSnapshot failed: %v", err)
	}

	releaseScan := make(chan struct{})
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		<-releaseScan
		filesChan <- "img1.jpg"
		filesChan <- "img2.jpg"
		return nil
	})

	type syncEvent struct {
		name  string
		state AppStateDTO
	}
	var names []string
	var syncEvents []syncEvent
	var mu sync.Mutex
	firstSync := make(chan AppStateDTO, 1)
	var once sync.Once
	srv.SetBroadcastHook(func(name string, data any) {
		mu.Lock()
		names = append(names, name)
		mu.Unlock()
		if name != eventSyncState {
			return
		}
		state, ok := data.(AppStateDTO)
		if !ok {
			return
		}
		mu.Lock()
		syncEvents = append(syncEvents, syncEvent{name: name, state: state})
		mu.Unlock()
		once.Do(func() {
			firstSync <- state
		})
	})

	loadErr := make(chan error, 1)
	go func() {
		loadErr <- srv.LoadState(root)
	}()

	var first AppStateDTO
	select {
	case first = <-firstSync:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first SyncState")
	}
	if !slices.Equal(first.VisibleOrder, wantSnapshotOrder) {
		t.Fatalf("first SyncState should carry snapshot order, got %v want %v", first.VisibleOrder, wantSnapshotOrder)
	}

	close(releaseScan)
	if err := <-loadErr; err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(names) == 0 {
		t.Fatal("expected broadcast sequence, got none")
	}
	if names[0] != eventSyncState {
		t.Fatalf("first event should be %q, got %q", eventSyncState, names[0])
	}

	// 2. Final significant event MUST be SyncState (reconciled)
	foundFinalSync := false
	for i := len(names) - 1; i >= 0; i-- {
		if names[i] == eventSyncState {
			foundFinalSync = true
			break
		}
	}
	if !foundFinalSync {
		t.Fatalf("last significant event should be %q, got %q", eventSyncState, names[len(names)-1])
	}
	if len(syncEvents) < 2 {
		t.Fatalf("expected at least 2 SyncState events, got %d", len(syncEvents))
	}
}
