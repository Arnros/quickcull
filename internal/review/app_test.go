package review

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"quickcull/internal/persistence"
)

func newAppWithState(t *testing.T, files []string) (*App, *Server, string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	for _, f := range files {
		writeTinyJPEG(t, filepath.Join(root, f))
	}
	sort.Strings(files)

	server := NewServer()
	server.state = NewState(root, files)
	server.cacheDir = server.state.CacheDir()
	server.cache.LoadCache(server.cacheDir)

	// [v2 Refactoring] Initialize the immutable appState for tests
	initialAppState := AppState{
		Root:         root,
		Photos:       make(map[string]Photo, len(files)),
		VisibleOrder: make([]string, len(files)),
	}
	for i, f := range files {
		initialAppState.VisibleOrder[i] = f
		initialAppState.Photos[f] = Photo{
			ID: f,
		}
	}
	server.appState = &initialAppState

	t.Cleanup(func() { server.cache.Close() })

	app := NewApp(server)
	ctx, cancel := context.WithCancel(context.Background())
	app.ctx = ctx
	t.Cleanup(cancel)
	server.StartEventEngine(app.ctx)
	return app, server, root
}

func TestAppOpenFolderValidation(t *testing.T) {
	app := NewApp(NewServer())
	if err := app.OpenFolder(""); err != domain.ErrPathRequired {
		t.Fatalf("expected ErrPathRequired, got %v", err)
	}

	empty := t.TempDir()
	app.ctx = context.Background()
	if err := app.OpenFolder(empty); err != domain.ErrNoMediaFiles {
		t.Fatalf("expected ErrNoMediaFiles, got %v", err)
	}
}

func TestAppCoreActionsAndQueries(t *testing.T) {
	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	app, _, _ := newAppWithState(t, files)

	// Test basic stats
	stats, err := app.GetStats()
	if err != nil || stats.Total != 3 {
		t.Fatalf("GetStats failed: %v", err)
	}

	if _, err := app.GetFile(99, true); err != domain.ErrIndexOutOfBounds {
		t.Fatalf("expected out-of-bounds error, got %v", err)
	}

	// Toggle Star
	if _, err := app.ToggleStar(0, "a.jpg", nil, true); err != nil {
		t.Fatalf("ToggleStar failed: %v", err)
	}
	var starred FilteredIndicesResponse
	waitForCondition(t, "expected index 0 to be starred", func() bool {
		starred, _ = app.GetStarredIndices()
		return slices.Equal(starred.Indices, []int{0})
	})
	if !slices.Equal(starred.Indices, []int{0}) {
		t.Fatalf("Expected index 0 to be starred, got %v", starred.Indices)
	}

	// Set Label
	if _, err := app.SetLabel(1, "b.jpg", nil, 3); err != nil {
		t.Fatalf("SetLabel failed: %v", err)
	}
	var labeled FilteredIndicesResponse
	waitForCondition(t, "expected index 1 to have label 3", func() bool {
		labeled, _ = app.GetLabelIndices(3)
		return slices.Equal(labeled.Indices, []int{1})
	})
	if !slices.Equal(labeled.Indices, []int{1}) {
		t.Fatalf("Expected index 1 to have label 3, got %v", labeled.Indices)
	}

	// Rotate
	if _, err := app.Rotate(2, "c.jpg", "right"); err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}
	var f FileResponse
	waitForCondition(t, "expected rotation 90 after rotate right", func() bool {
		f, _ = app.GetFile(2, false)
		return f.Rotation == 90
	})
	if f.Rotation != 90 {
		t.Fatalf("Expected rotation 90, got %d", f.Rotation)
	}

	// Trash
	trashRes, err := app.Trash(0, "a.jpg", nil)
	if err != nil {
		t.Fatalf("Trash failed: %v", err)
	}
	if trashRes.Total != 2 {
		t.Fatalf("unexpected trash result total: %d", trashRes.Total)
	}
}

func TestAppBatchTrashRobustness(t *testing.T) {
	// Mixed paths simulating subdirectories and Windows-style entries (after normalization)
	files := []string{
		"2024/01/a.jpg",
		"2024/01/b.jpg",
		"2024/02/c.jpg",
		"root.jpg",
	}
	app, _, _ := newAppWithState(t, files)

	// Trash multiple files
	toTrash := []string{"2024/01/a.jpg", "2024/02/c.jpg"}
	res, err := app.Trash(0, "", toTrash)
	if err != nil {
		t.Fatalf("Batch Trash failed: %v", err)
	}

	if res.Total != 2 {
		t.Errorf("Expected 2 files remaining, got %d", res.Total)
	}

	// Verify the correct files remain in the correct order
	remaining0, _ := app.GetFile(0, true)
	if remaining0.Filename != "2024/01/b.jpg" {
		t.Errorf("Expected 2024/01/b.jpg at index 0, got %s", remaining0.Filename)
	}

	remaining1, _ := app.GetFile(1, true)
	if remaining1.Filename != "root.jpg" {
		t.Errorf("Expected root.jpg at index 1, got %s", remaining1.Filename)
	}
}

func TestUndoWithoutHistoryReturnsNothingToUndo(t *testing.T) {
	app, _, _ := newAppWithState(t, []string{"a.jpg"})

	_, err := app.Undo()
	if err != domain.ErrNothingToUndo {
		t.Fatalf("expected ErrNothingToUndo, got %v", err)
	}
}

func TestAppResolveIndexUsesPathOverStaleIndex(t *testing.T) {
	files := []string{"a.jpg", "b.jpg"}
	app, _, _ := newAppWithState(t, files)

	// Scenario: UI thinks b.jpg is at index 0 (stale), but it's at index 1.
	// ToggleStar should find b.jpg via path and star it.
	if _, err := app.ToggleStar(0, "b.jpg", nil, true); err != nil {
		t.Fatalf("ToggleStar failed: %v", err)
	}
	var res FilteredIndicesResponse
	waitForCondition(t, "expected b.jpg to be starred at index 1", func() bool {
		res, _ = app.GetStarredIndices()
		return slices.Equal(res.Indices, []int{1})
	})
	if !slices.Equal(res.Indices, []int{1}) {
		t.Errorf("expected b.jpg (index 1) to be starred via path-based resolution, got %v", res.Indices)
	}
}

func TestAppHTTPMetadataServing(t *testing.T) {
	app, _, root := newAppWithState(t, []string{"test.jpg"})
	absPath := filepath.Join(root, "test.jpg")

	// Generate thumbnail file manually to test serving
	cacheDir := domain.GetCacheDir(root)
	thumbPath, _ := ThumbCachePathForSource(absPath, cacheDir)
	_ = os.MkdirAll(filepath.Dir(thumbPath), 0755)
	_ = os.WriteFile(thumbPath, []byte("fake-thumb"), 0644)

	req := httptest.NewRequest("GET", "/thumb/0", nil)
	w := httptest.NewRecorder()
	app.server.ServeMedia(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected HTTP 200, got %d", w.Code)
	}
}

func TestAppTrashRobustness(t *testing.T) {
	app, _, root := newAppWithState(t, []string{"test.jpg"})
	absPath := filepath.Join(root, "test.jpg")

	// Make file read-only to test potential failures (though Trash usually succeeds if parent is writable)
	_ = os.Chmod(absPath, 0400)

	// Chmod root to 0500 to prevent deletion
	_ = os.Chmod(root, 0500)
	defer os.Chmod(root, 0755)

	_, trashErr := app.Trash(0, "test.jpg", nil)
	// We expect failure or success depending on OS, but app should not panic
	_ = trashErr

	stats, err := app.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.Total != 1 {
		t.Fatalf("expected file to remain after trash failure, total=%d", stats.Total)
	}
}

func TestBuildRuntimeCapabilities(t *testing.T) {
	capsWithoutExiftool := buildRuntimeCapabilities(false)
	if capsWithoutExiftool.RawPreview {
		t.Fatal("expected raw preview disabled without exiftool")
	}
	if capsWithoutExiftool.RawMetadata {
		t.Fatal("expected raw metadata disabled without exiftool")
	}
	if capsWithoutExiftool.ExifWrite {
		t.Fatal("expected EXIF write disabled without exiftool")
	}
	if capsWithoutExiftool.HeicDecode != HeicSupported() {
		t.Fatalf("expected heic decode=%v without exiftool, got %v", HeicSupported(), capsWithoutExiftool.HeicDecode)
	}

	capsWithExiftool := buildRuntimeCapabilities(true)
	if !capsWithExiftool.RawPreview || !capsWithExiftool.RawMetadata || !capsWithExiftool.ExifWrite {
		t.Fatal("expected raw preview/metadata and exif write enabled with exiftool")
	}
	if !capsWithExiftool.HeicDecode {
		t.Fatal("expected heic decode enabled when exiftool is available")
	}
}

func TestSysCheckIncludesCapabilities(t *testing.T) {
	app := NewApp(NewServer())
	res, err := app.SysCheck()
	if err != nil {
		t.Fatalf("SysCheck failed: %v", err)
	}
	expected := buildRuntimeCapabilities(res.Exiftool)
	if res.Capabilities != expected {
		t.Fatalf("unexpected capabilities: got %+v want %+v", res.Capabilities, expected)
	}
}

func TestAppReadAPIsWithoutLoadedState(t *testing.T) {
	app := NewApp(NewServer())

	folders, err := app.GetFolders()
	if err != nil {
		t.Fatalf("GetFolders failed: %v", err)
	}
	if len(folders) != 0 {
		t.Fatalf("expected empty folders, got %v", folders)
	}

	filters, err := app.GetFilters()
	if err != nil {
		t.Fatalf("GetFilters failed: %v", err)
	}
	if len(filters.Cameras) != 0 || len(filters.ISOs) != 0 {
		t.Fatalf("expected empty filters, got %+v", filters)
	}

	indices, err := app.GetFilteredIndices(map[string]string{"camera": "x"})
	if err != nil {
		t.Fatalf("GetFilteredIndices failed: %v", err)
	}
	if len(indices.Indices) != 0 {
		t.Fatalf("expected empty indices, got %v", indices.Indices)
	}

	progress, err := app.GetAnalysisProgress()
	if err != nil {
		t.Fatalf("GetAnalysisProgress failed: %v", err)
	}
	if progress.Current != 0 || progress.Total != 0 {
		t.Fatalf("expected zero progress, got %+v", progress)
	}
}

func TestAppSetSortOrderValidation(t *testing.T) {
	t.Run("no state", func(t *testing.T) {
		app := NewApp(NewServer())
		if err := app.SetSortOrder("name"); err != domain.ErrFolderNotFound {
			t.Fatalf("expected ErrFolderNotFound, got %v", err)
		}
	})

	t.Run("invalid order", func(t *testing.T) {
		app, _, _ := newAppWithState(t, []string{"a.jpg", "b.jpg"})
		if err := app.SetSortOrder("invalid"); err != domain.ErrInvalidCriteria {
			t.Fatalf("expected ErrInvalidCriteria, got %v", err)
		}
	})
}

func TestAppExportSelectionValidation(t *testing.T) {
	t.Run("no state", func(t *testing.T) {
		app := NewApp(NewServer())
		if err := app.ExportSelection("starred", 0, t.TempDir(), false); err != domain.ErrFolderNotFound {
			t.Fatalf("expected ErrFolderNotFound, got %v", err)
		}
	})

	t.Run("no match returns nil", func(t *testing.T) {
		app, _, _ := newAppWithState(t, []string{"a.jpg", "b.jpg"})
		if err := app.ExportSelection("starred", 0, t.TempDir(), false); err != nil {
			t.Fatalf("expected nil when no files match, got %v", err)
		}
	})
}

func TestAppBrowseFiltersHiddenAndSorts(t *testing.T) {
	root := t.TempDir()
	mkdir := func(name string) {
		t.Helper()
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
	}
	mkdir("z-last")
	mkdir("a-first")
	mkdir(".hidden")
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	app := NewApp(NewServer())
	res, err := app.Browse(root)
	if err != nil {
		t.Fatalf("Browse failed: %v", err)
	}
	if res.Path != filepath.Clean(root) {
		t.Fatalf("unexpected path: %s", res.Path)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("expected two visible directories, got %v", res.Entries)
	}
	if !slices.Equal(res.Entries, []string{"a-first", "z-last"}) {
		t.Fatalf("unexpected sorted entries: %v", res.Entries)
	}
}

func writeTinyJPEG(t *testing.T, path string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00\xff\xdb\x00\x43\x00\x08\x06\x06\x07\x06\x05\x08\x07\x07\x07\x09\x09\x08\x0a\x0c\x14\x08\x08\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\xff\xc0\x00\x0b\x08\x00\x01\x00\x01\x01\x01\x11\x00\xff\xc4\x00\x14\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xc4\x00\x14\x10\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xda\x00\x08\x01\x01\x00\x00\x3f\x00\x37\xff\xd9")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write tiny jpeg: %v", err)
	}
}

func TestBatchStarForcesStateOnMixedSelection(t *testing.T) {
	// a.jpg already starred, b.jpg not starred, c.jpg not starred.
	// Batch ToggleStar(starred=true) must star b and c without unstarring a.
	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	app, server, _ := newAppWithState(t, files)

	// Pre-star a.jpg
	if _, err := app.ToggleStar(0, "a.jpg", nil, true); err != nil {
		t.Fatalf("pre-star a.jpg: %v", err)
	}
	waitForCondition(t, "a.jpg should be starred", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		return server.appState != nil && server.appState.Photos["a.jpg"].IsStarred
	})

	// Batch force-star all three (a already starred, b and c not).
	if _, err := app.ToggleStar(0, "", []string{"a.jpg", "b.jpg", "c.jpg"}, true); err != nil {
		t.Fatalf("batch ToggleStar: %v", err)
	}
	waitForCondition(t, "all three should be starred", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		if server.appState == nil {
			return false
		}
		return server.appState.Photos["a.jpg"].IsStarred &&
			server.appState.Photos["b.jpg"].IsStarred &&
			server.appState.Photos["c.jpg"].IsStarred
	})

	server.appStateMu.RLock()
	defer server.appStateMu.RUnlock()
	for _, f := range files {
		if !server.appState.Photos[f].IsStarred {
			t.Errorf("%s should be starred after force-star batch", f)
		}
	}
	if server.appState.StarredCount != 3 {
		t.Errorf("StarredCount = %d, want 3", server.appState.StarredCount)
	}
}

func TestBatchLabelForcesStateOnMixedSelection(t *testing.T) {
	// b.jpg already has label 2, a.jpg and c.jpg have no label.
	// Batch SetLabel(2) must label a and c without re-emitting for b.
	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	app, server, _ := newAppWithState(t, files)

	if _, err := app.SetLabel(1, "b.jpg", nil, 2); err != nil {
		t.Fatalf("pre-label b.jpg: %v", err)
	}
	waitForCondition(t, "b.jpg should have label 2", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		return server.appState != nil && server.appState.Photos["b.jpg"].Label == 2
	})

	if _, err := app.SetLabel(0, "", []string{"a.jpg", "b.jpg", "c.jpg"}, 2); err != nil {
		t.Fatalf("batch SetLabel: %v", err)
	}
	waitForCondition(t, "all three should have label 2", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		if server.appState == nil {
			return false
		}
		return server.appState.Photos["a.jpg"].Label == 2 &&
			server.appState.Photos["b.jpg"].Label == 2 &&
			server.appState.Photos["c.jpg"].Label == 2
	})

	server.appStateMu.RLock()
	defer server.appStateMu.RUnlock()
	for _, f := range files {
		if server.appState.Photos[f].Label != 2 {
			t.Errorf("%s label = %d, want 2", f, server.appState.Photos[f].Label)
		}
	}
	if server.appState.LabeledCount != 3 {
		t.Errorf("LabeledCount = %d, want 3", server.appState.LabeledCount)
	}
}

func TestBatchUndoRevertsAllPhotosAtOnce(t *testing.T) {
	// Scenario: star 3 photos as a batch, then undo once → all 3 lose their star.
	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	app, server, _ := newAppWithState(t, files)

	// Batch star all three.
	if _, err := app.ToggleStar(0, "", []string{"a.jpg", "b.jpg", "c.jpg"}, true); err != nil {
		t.Fatalf("batch ToggleStar: %v", err)
	}
	waitForCondition(t, "all three should be starred after batch", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		if server.appState == nil {
			return false
		}
		return server.appState.Photos["a.jpg"].IsStarred &&
			server.appState.Photos["b.jpg"].IsStarred &&
			server.appState.Photos["c.jpg"].IsStarred
	})

	// Verify history has exactly 1 entry (the batch), not 3.
	waitForCondition(t, "history should have exactly 1 batch entry", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		return server.appState != nil && len(server.appState.History) == 1
	})

	// Undo once.
	if _, err := app.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	waitForCondition(t, "all three should be unstarred after single undo", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		if server.appState == nil {
			return false
		}
		return !server.appState.Photos["a.jpg"].IsStarred &&
			!server.appState.Photos["b.jpg"].IsStarred &&
			!server.appState.Photos["c.jpg"].IsStarred
	})

	server.appStateMu.RLock()
	defer server.appStateMu.RUnlock()
	for _, f := range files {
		if server.appState.Photos[f].IsStarred {
			t.Errorf("%s should not be starred after undo", f)
		}
	}
	if server.appState.StarredCount != 0 {
		t.Errorf("StarredCount = %d, want 0", server.appState.StarredCount)
	}
}

func TestBatchActionsNormalizeWindowsPaths(t *testing.T) {
	files := []string{"a.jpg", "sub/b.jpg"}
	app, server, _ := newAppWithState(t, files)

	// Simulate frontend payload on Windows where one selected path uses backslashes.
	paths := []string{"a.jpg", "sub\\b.jpg"}

	if _, err := app.ToggleStar(0, "", paths, true); err != nil {
		t.Fatalf("batch ToggleStar with windows paths: %v", err)
	}
	waitForCondition(t, "both files should be starred", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		if server.appState == nil {
			return false
		}
		return server.appState.Photos["a.jpg"].IsStarred &&
			server.appState.Photos["sub/b.jpg"].IsStarred
	})

	if _, err := app.SetLabel(0, "", paths, 3); err != nil {
		t.Fatalf("batch SetLabel with windows paths: %v", err)
	}
	waitForCondition(t, "both files should have label 3", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		if server.appState == nil {
			return false
		}
		return server.appState.Photos["a.jpg"].Label == 3 &&
			server.appState.Photos["sub/b.jpg"].Label == 3
	})
}

func TestBatchMetadataActionsEmitStateSyncForUI(t *testing.T) {
	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	app, server, _ := newAppWithState(t, files)

	var (
		mu        sync.Mutex
		syncState int
		syncDelta int
	)
	server.SetBroadcastHook(func(name string, _ any) {
		mu.Lock()
		defer mu.Unlock()
		switch name {
		case "SyncState":
			syncState++
		case "SyncDelta":
			syncDelta++
		}
	})

	if _, err := app.ToggleStar(0, "", []string{"a.jpg", "b.jpg"}, true); err != nil {
		t.Fatalf("batch ToggleStar: %v", err)
	}
	waitForCondition(t, "batch star should trigger a UI state sync event", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return syncState > 0 || syncDelta > 0
	})

	if _, err := app.SetLabel(0, "", []string{"a.jpg", "b.jpg"}, 4); err != nil {
		t.Fatalf("batch SetLabel: %v", err)
	}
	waitForCondition(t, "batch label should trigger a UI state sync event", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return syncState > 1 || syncDelta > 1
	})

	mu.Lock()
	defer mu.Unlock()
	if syncState == 0 && syncDelta == 0 {
		t.Fatalf("expected SyncState or SyncDelta for batch metadata actions")
	}
}

func TestResetMetadataEmitsSyncState(t *testing.T) {
	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	app, server, _ := newAppWithState(t, files)

	// Star two photos first.
	if _, err := app.ToggleStar(0, "", []string{"a.jpg", "b.jpg"}, true); err != nil {
		t.Fatalf("ToggleStar: %v", err)
	}
	waitForCondition(t, "star should trigger sync event", func() bool {
		server.appStateMu.RLock()
		defer server.appStateMu.RUnlock()
		return server.appState != nil && server.appState.StarredCount == 2
	})

	var (
		mu        sync.Mutex
		syncState int
	)
	server.SetBroadcastHook(func(name string, _ any) {
		mu.Lock()
		defer mu.Unlock()
		if name == "SyncState" {
			syncState++
		}
	})

	if err := app.ResetStars(); err != nil {
		t.Fatalf("ResetStars: %v", err)
	}
	waitForCondition(t, "ResetMetadata must emit SyncState so photo tiles refresh", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return syncState > 0
	})
}

func waitForCondition(t *testing.T, message string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(message)
}

// TestResetAppCache_BusyWhenLoadingInProgress locks in the P2-7 fix: if
// OpenFolder holds the `loading` CAS, ResetAppCache must refuse to race by
// returning domain.ErrLoadInProgress rather than nil-checking state in the
// middle of a load.
func TestResetAppCache_BusyWhenLoadingInProgress(t *testing.T) {
	app := NewApp(NewServer())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	app.ctx = ctx

	// Simulate OpenFolder in flight.
	if !app.loading.CompareAndSwap(false, true) {
		t.Fatal("expected initial CAS(false,true) to succeed")
	}

	err := app.ResetAppCache()
	if err != domain.ErrLoadInProgress {
		t.Fatalf("expected ErrLoadInProgress, got %v", err)
	}

	// Release the load; ResetAppCache must now succeed (no state to clean, but
	// coordination behavior itself is the contract being tested).
	app.loading.Store(false)

	if err := app.ResetAppCache(); err != nil {
		t.Fatalf("expected nil error after load released, got %v", err)
	}
	if app.server.state != nil {
		t.Errorf("state should be nil after ResetAppCache")
	}
}

// TestResetAppCache_PreservesConfigJSON ensures the user's config.json survives
// a cache-wide wipe — it is the one entry explicitly spared by name.
func TestResetAppCache_PreservesConfigJSON(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Dir(cacheDir))

	domain.GetAppCacheDir() // ensure dir

	cfgPath := filepath.Join(domain.GetAppCacheDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"windowWidth":1234}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(domain.GetAppCacheDir(), "stale.db"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(NewServer())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	app.ctx = ctx

	if err := app.ResetAppCache(); err != nil {
		t.Fatalf("ResetAppCache: %v", err)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config.json should be preserved, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(domain.GetAppCacheDir(), "stale.db")); err == nil {
		t.Errorf("stale.db should have been removed")
	}
}

// TestResetAppCache_WakesIdleAnalysisWorkers ensures the coordination path that
// was the bug P0-2: when background analysis is mid-flight (and idle on the
// queue), ResetAppCache+Wait must return within a bounded deadline instead of
// hanging on cond.Wait.
func TestResetAppCache_WakesIdleAnalysisWorkers(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	app, server, _ := newAppWithState(t, []string{"a.jpg", "b.jpg", "c.jpg"})

	// Start analysis with workers sitting on an empty queue: this is the exact
	// condition that hung ResetAppCache before the WakeAndCancel fix.
	ctx, cancel := context.WithCancel(app.ctx)
	t.Cleanup(cancel)
	server.startBackgroundAnalysis(ctx)

	// Give workers time to enter cond.Wait on the empty queue.
	time.Sleep(150 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- app.ResetAppCache() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ResetAppCache returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ResetAppCache hung > 2s — analysis workers were not woken")
	}
}

func TestRestoreFromTrashViaApp(t *testing.T) {
	root, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	state := app.server.getState()
	if _, err := state.Trash(0); err != nil {
		t.Fatal(err)
	}
	trashPath := filepath.Join(root, domain.DirTrash, "a.jpg")
	if _, err := os.Stat(trashPath); os.IsNotExist(err) {
		t.Fatal("file should be in .trash after trash")
	}
	app.server.invalidateBurstCache()
	app.server.RefreshVisibleOrder()

	res, err := app.RestoreFromTrash([]string{"a.jpg"})
	if err != nil {
		t.Fatalf("RestoreFromTrash: %v", err)
	}
	if res.Total != 3 {
		t.Errorf("total = %d, want 3", res.Total)
	}
	if !slices.Equal(res.Restored, []string{"a.jpg"}) {
		t.Errorf("restored = %v, want [a.jpg]", res.Restored)
	}
	if res.Index < 0 {
		t.Errorf("index = %d, want non-negative", res.Index)
	}
	if _, err := os.Stat(filepath.Join(root, "a.jpg")); err != nil {
		t.Fatalf("file missing from root after restore: %v", err)
	}
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Fatalf("trash path should be absent after restore, stat error: %v", err)
	}
}

func TestRestoreFromTrashNoState(t *testing.T) {
	app := NewApp(NewServer())
	_, err := app.RestoreFromTrash([]string{"x.jpg"})
	if err != domain.ErrFolderNotFound {
		t.Fatalf("want ErrFolderNotFound, got %v", err)
	}
}

func TestResetLabelsViaApp(t *testing.T) {
	_, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	if _, _, err := app.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: "a.jpg", Label: 3},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: "b.jpg", Label: 5},
	}); err != nil {
		t.Fatal(err)
	}
	app.server.appStateMu.RLock()
	beforeCount := app.server.appState.LabeledCount
	app.server.appStateMu.RUnlock()
	if beforeCount != 2 {
		t.Fatalf("expected 2 labeled photos before reset, got %d", beforeCount)
	}

	if err := app.ResetLabels(); err != nil {
		t.Fatalf("ResetLabels: %v", err)
	}

	app.server.appStateMu.RLock()
	defer app.server.appStateMu.RUnlock()
	if app.server.appState.LabeledCount != 0 {
		t.Errorf("labeled count = %d, want 0", app.server.appState.LabeledCount)
	}
	for _, id := range []string{"a.jpg", "b.jpg"} {
		if p := app.server.appState.Photos[id]; p.Label != 0 {
			t.Errorf("%s label = %d, want 0", id, p.Label)
		}
	}
}

func TestRotateResetViaApp(t *testing.T) {
	app, _, _ := newAppWithState(t, []string{"a.jpg", "b.jpg"})
	if _, err := app.Rotate(0, "a.jpg", "right"); err != nil {
		t.Fatal(err)
	}
	waitForCondition(t, "rotation 90", func() bool {
		f, _ := app.GetFile(0, false)
		return f.Rotation == 90
	})

	res, err := app.RotateReset(0, "a.jpg")
	if err != nil {
		t.Fatalf("RotateReset: %v", err)
	}
	if !res.Ok {
		t.Fatal("RotateReset should return Ok=true")
	}
	f, _ := app.GetFile(0, false)
	if f.Rotation != 0 {
		t.Errorf("rotation = %d after reset, want 0", f.Rotation)
	}
}

func TestRotateResetInvalidIndex(t *testing.T) {
	app, _, _ := newAppWithState(t, []string{"a.jpg"})
	_, err := app.RotateReset(0, "missing.jpg")
	if err != domain.ErrIndexOutOfBounds {
		t.Fatalf("want ErrIndexOutOfBounds for non-existent path, got %v", err)
	}
}

func TestApplyRotationZeroRotationReturnsNil(t *testing.T) {
	app, _, _ := newAppWithState(t, []string{"a.jpg"})
	if err := app.ApplyRotation(0, "a.jpg"); err != nil {
		t.Fatalf("ApplyRotation with zero rotation: %v", err)
	}
}

func TestApplyRotationUnsupportedFormat(t *testing.T) {
	root := t.TempDir()
	name := "test.gif"
	abs := filepath.Join(root, name)
	if err := os.WriteFile(abs, []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x00\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer()
	srv.state = NewState(root, []string{name})
	srv.cacheDir = srv.state.CacheDir()
	srv.cache.LoadCache(srv.cacheDir)
	srv.appState = &AppState{
		Root:         root,
		Photos:       map[string]Photo{name: {ID: name}},
		VisibleOrder: []string{name},
	}
	t.Cleanup(func() { srv.cache.Close() })

	app := &App{server: srv}
	ctx, cancel := context.WithCancel(context.Background())
	app.ctx = ctx
	t.Cleanup(cancel)
	srv.StartEventEngine(app.ctx)

	if _, err := app.Rotate(0, name, "right"); err != nil {
		t.Fatal(err)
	}
	waitForCondition(t, "rotation set", func() bool {
		srv.appStateMu.RLock()
		defer srv.appStateMu.RUnlock()
		return srv.appState != nil && srv.appState.Photos[name].Rotation != 0
	})

	if err := app.ApplyRotation(0, name); err != domain.ErrExifWriteUnsupported {
		t.Fatalf("want ErrExifWriteUnsupported, got %v", err)
	}
}

func TestReanalyzeMetadataViaApp(t *testing.T) {
	app, _, _ := newAppWithState(t, []string{"a.jpg", "b.jpg"})
	res, err := app.ReanalyzeMetadata(0)
	if err != nil {
		t.Fatalf("ReanalyzeMetadata: %v", err)
	}
	if res.Filename != "a.jpg" {
		t.Fatalf("filename = %q, want a.jpg", res.Filename)
	}
	if res.Index != 0 {
		t.Fatalf("index = %d, want 0", res.Index)
	}
}

func TestReanalyzeMetadataNoState(t *testing.T) {
	app := NewApp(NewServer())
	_, err := app.ReanalyzeMetadata(0)
	if err != domain.ErrFolderNotFound {
		t.Fatalf("want ErrFolderNotFound, got %v", err)
	}
}

func TestReanalyzeMetadataOutOfBounds(t *testing.T) {
	app, _, _ := newAppWithState(t, []string{"a.jpg"})
	_, err := app.ReanalyzeMetadata(99)
	if err != domain.ErrIndexOutOfBounds {
		t.Fatalf("want ErrIndexOutOfBounds, got %v", err)
	}
}

func TestGetAppStateReturnsSnapshot(t *testing.T) {
	app, server, _ := newAppWithState(t, []string{"a.jpg", "b.jpg"})
	state, err := app.GetAppState()
	if err != nil {
		t.Fatalf("GetAppState: %v", err)
	}
	if len(state.VisibleOrder) != 2 {
		t.Fatalf("visible order len = %d, want 2", len(state.VisibleOrder))
	}

	state.Root = "hacked"
	state.VisibleOrder[0] = "hacked.jpg"
	photo := state.Photos["a.jpg"]
	photo.Label = 5
	state.Photos["a.jpg"] = photo
	server.appStateMu.RLock()
	serverRoot := server.appState.Root
	serverFirst := server.appState.VisibleOrder[0]
	serverLabel := server.appState.Photos["a.jpg"].Label
	server.appStateMu.RUnlock()
	if serverRoot == "hacked" {
		t.Fatal("GetAppState returned mutable reference to server state")
	}
	if serverFirst == "hacked.jpg" || serverLabel == 5 {
		t.Fatalf("GetAppState leaked nested state: order=%q label=%d", serverFirst, serverLabel)
	}
	if len(state.Photos) != 2 {
		t.Fatalf("photos map len = %d, want 2", len(state.Photos))
	}
}

func TestGetAppStateNoFolder(t *testing.T) {
	app := NewApp(NewServer())
	state, err := app.GetAppState()
	if err != nil {
		t.Fatalf("GetAppState with no state: %v", err)
	}
	if len(state.VisibleOrder) != 0 || len(state.Photos) != 0 {
		t.Errorf("expected empty state, got %+v", state)
	}
}

func TestListTrashViaApp(t *testing.T) {
	_, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	res, err := app.ListTrash()
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if res.Items == nil {
		t.Fatal("ListTrash must return non-nil Items on empty trash")
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected empty trash, got %v", res.Items)
	}

	if _, err := app.Trash(1, "b.jpg", nil); err != nil {
		t.Fatal(err)
	}
	waitForCondition(t, "b.jpg trashed", func() bool {
		res, _ := app.ListTrash()
		return len(res.Items) == 1
	})

	res2, _ := app.ListTrash()
	if !slices.Equal(res2.Items, []string{"b.jpg"}) {
		t.Fatalf("trash items = %v", res2.Items)
	}
}

func TestListTrashNoState(t *testing.T) {
	app := NewApp(NewServer())
	res, err := app.ListTrash()
	if err != nil {
		t.Fatalf("ListTrash no state: %v", err)
	}
	if res.Items == nil || len(res.Items) != 0 {
		t.Fatalf("expected empty non-nil items, got %v", res.Items)
	}
}

func TestExportFilesEmptyList(t *testing.T) {
	app := NewApp(NewServer())
	if err := app.ExportFiles(nil, t.TempDir(), false); err != nil {
		t.Fatalf("ExportFiles with nil: %v", err)
	}
	if err := app.ExportFiles([]string{}, t.TempDir(), true); err != nil {
		t.Fatalf("ExportFiles with empty slice: %v", err)
	}
}

func TestExportFilesViaApp(t *testing.T) {
	_, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	dest := t.TempDir()
	if err := app.ExportFiles([]string{"a.jpg"}, dest, false); err != nil {
		t.Fatalf("ExportFiles: %v", err)
	}
	waitForExport(t, app.server)
	if _, err := os.Stat(filepath.Join(dest, "a.jpg")); err != nil {
		t.Fatalf("exported file missing from dest: %v", err)
	}
}

func TestSavePositionViaApp(t *testing.T) {
	root := t.TempDir()
	srv := NewServer()
	srv.persistence, _ = persistence.NewMetadataStore(filepath.Join(t.TempDir(), "pos.db"))
	t.Cleanup(func() { srv.persistence.Close() })
	srv.appState = &AppState{
		Root:         root,
		VisibleOrder: []string{"a.jpg", "b.jpg", "c.jpg"},
		Photos:       map[string]Photo{"a.jpg": {ID: "a.jpg"}, "b.jpg": {ID: "b.jpg"}, "c.jpg": {ID: "c.jpg"}},
	}
	app := &App{server: srv}
	app.SavePosition(2)
	if got := srv.GetSavedPosition(root); got != 2 {
		t.Fatalf("saved position = %d, want 2", got)
	}
}

func TestGetHistoryAndRemoveHistoryViaApp(t *testing.T) {
	t.Setenv("QUICKCULL_TEST_CACHE_DIR", t.TempDir())
	app := NewApp(NewServer())
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	if err := domain.AddToHistory(first); err != nil {
		t.Fatal(err)
	}
	if err := domain.AddToHistory(second); err != nil {
		t.Fatal(err)
	}
	history, err := app.GetHistory()
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	firstAbs, _ := filepath.Abs(first)
	secondAbs, _ := filepath.Abs(second)
	if !slices.Equal(history, []string{secondAbs, firstAbs}) {
		t.Fatalf("history = %v", history)
	}
	if err := app.RemoveHistory(first); err != nil {
		t.Fatal(err)
	}
	history, err = app.GetHistory()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(history, []string{secondAbs}) {
		t.Fatalf("history after remove = %v", history)
	}
}

func TestSearchStreamNewSearchCancelsOldViaApp(t *testing.T) {
	root := t.TempDir()
	const fileCount = 200
	for i := 0; i < fileCount; i++ {
		_ = os.WriteFile(filepath.Join(root, fmt.Sprintf("vac_%04d.jpg", i)), []byte("x"), 0o644)
	}

	srv := NewServer()
	if err := srv.LoadState(root); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}
	app.ctx = context.Background()

	type searchEvent struct {
		name  string
		query string
	}
	var mu sync.Mutex
	var events []searchEvent
	firstResult := make(chan struct{}, 1)
	secondComplete := make(chan map[string]any, 1)
	srv.SetBroadcastHook(func(name string, data any) {
		if name != "search:results" && name != "search:complete" {
			return
		}
		payload := data.(map[string]any)
		query, _ := payload["query"].(string)
		mu.Lock()
		events = append(events, searchEvent{name: name, query: query})
		mu.Unlock()
		if name == "search:results" && query == "vac" {
			select {
			case firstResult <- struct{}{}:
			default:
			}
		}
		if name == "search:complete" && query == "vac_0001" {
			select {
			case secondComplete <- payload:
			default:
			}
		}
	})

	app.SearchStream("vac")
	select {
	case <-firstResult:
	case <-time.After(2 * time.Second):
		t.Fatal("first search produced no streamed results")
	}

	app.SearchStream("vac_0001")
	mu.Lock()
	boundary := len(events)
	mu.Unlock()
	var payload map[string]any
	select {
	case payload = <-secondComplete:
	case <-time.After(2 * time.Second):
		t.Fatal("second search did not complete")
	}
	wantIndex := srv.getState().FindIndex("vac_0001.jpg")
	if indices, ok := payload["indices"].([]int); !ok || !slices.Equal(indices, []int{wantIndex}) {
		t.Fatalf("second search indices = %#v, want [%d]", payload["indices"], wantIndex)
	}
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	for _, event := range events[boundary:] {
		if event.query == "vac" {
			t.Fatalf("cancelled search emitted %s after replacement started", event.name)
		}
	}
}

func TestPrioritizeIndicesValidAndOutOfBounds(t *testing.T) {
	app, server, _ := newAppWithState(t, []string{"a.jpg", "b.jpg", "c.jpg"})
	server.analysisQueue.Reset()

	app.PrioritizeIndices([]int{-1, 0, 2, 9999})
	app.PrioritizeIndices(nil)
	app.PrioritizeIndices([]int{})

	interactive, warm, bulk := server.analysisQueue.DepthByTier()
	if got := interactive + warm + bulk; got != 2 {
		t.Fatalf("queued item count = %d, want 2 valid indices", got)
	}
	if server.analysisQueue.HasUrgentTask() {
		t.Fatal("prioritize should not create urgent tasks")
	}
	got := make([]int, 0, 2)
	for range 2 {
		index, _, ok := server.analysisQueue.Pop()
		if !ok {
			t.Fatal("expected queued priority item")
		}
		got = append(got, index)
	}
	slices.Sort(got)
	if !slices.Equal(got, []int{0, 2}) {
		t.Fatalf("queued indices = %v, want [0 2]", got)
	}
}
