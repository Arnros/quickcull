package review

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"quickcull/internal/domain"
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
