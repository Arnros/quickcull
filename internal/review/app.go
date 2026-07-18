package review

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"quickcull/internal/exif"
	"quickcull/internal/utils"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	server    *Server
	loading   atomic.Bool
	refreshMu sync.Mutex
}

var applyEXIFOrientation = ApplyEXIFOrientation
var resetExiftoolAvailabilityCache = exif.ResetExiftoolAvailabilityCache

// NewApp creates a new App struct
func NewApp(server *Server) *App {
	return &App{
		server: server,
	}
}

// FlushPersistence blocks until queued persistence writes have been flushed.
func (a *App) FlushPersistence() {
	if a == nil || a.server == nil {
		return
	}
	a.server.flushPersistence()
}

// Log receives a log message from the frontend and writes it to the backend log
func (a *App) Log(level string, message string, context map[string]any) {
	attrs := []any{}
	for k, v := range context {
		attrs = append(attrs, slog.Any(k, v))
	}

	switch strings.ToLower(level) {
	case "debug":
		slog.Debug(message, attrs...)
	case "info":
		slog.Info(message, attrs...)
	case "warn":
		slog.Warn(message, attrs...)
	case "error":
		slog.Error(message, attrs...)
	default:
		slog.Info(message, attrs...)
	}
}

// Startup is called when the app starts.
func (a *App) Startup(ctx context.Context) error {
	a.ctx = ctx
	a.server.ctx = ctx

	// Initialize persistence
	if err := a.server.InitPersistence(); err != nil {
		return errors.Join(domain.ErrPersistenceInit, err)
	}

	// [v2 Refactoring] Start the immutable Reducer engine
	a.server.StartEventEngine(ctx)

	// Log System Info for support
	slog.Info("Application Startup",
		slog.String("os", runtime.GOOS),
		slog.String("arch", runtime.GOARCH),
		slog.Int("cpus", runtime.NumCPU()),
		slog.String("version", domain.AppVersion),
	)
	return nil
}

// OpenFolder opens a folder and initializes the state
func (a *App) OpenFolder(path string) error {
	defer utils.HandlePanic()

	if path == "" {
		return domain.ErrPathRequired
	}
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	if !a.loading.CompareAndSwap(false, true) {
		slog.Warn("OpenFolder: Load already in progress, skipping request", "path", path)
		return nil
	}
	defer a.loading.Store(false)

	slog.Info("OpenFolder: Loading new folder", "path", path)

	err := a.server.LoadState(path)
	if err != nil {
		slog.Error("OpenFolder: Failed to load state", "path", path, "error", err)
		return err
	}
	if err := domain.AddToHistory(path); err != nil {
		slog.Warn("OpenFolder: Failed to add folder to history", "path", path, "error", err)
	}
	a.server.startBackgroundAnalysis(a.bgContext())
	a.emitStateUpdate()

	slog.Info("OpenFolder: Folder loaded successfully", "path", path)
	return nil
}

// bgContext returns the app context, falling back to Background if not yet initialised.
func (a *App) bgContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func resolveRequestIndex(state *State, index int, relPath string) (int, error) {
	resolved := state.ResolveIndex(index, relPath)
	if resolved == -1 {
		return -1, domain.ErrIndexOutOfBounds
	}
	return resolved, nil
}

// GetFile returns metadata for a specific index
func (a *App) GetFile(index int, skipAnalysis bool) (FileResponse, error) {
	defer utils.HandlePanic()
	utils.LogNav("GetFile: start", "index", index, "skipAnalysis", skipAnalysis)
	state, err := a.requireState()
	if err != nil {
		return FileResponse{}, err
	}

	relPath, err := state.Get(index)
	if err != nil {
		return FileResponse{}, err
	}

	a.server.PrioritizeRange(index, index-filmstripDefaultRadius, index+filmstripDefaultRadius+1)

	absPath, _ := state.AbsPath(index)
	format := domain.FromExtension(filepath.Ext(relPath)).String()

	result := FileResponse{
		Filename: filepath.ToSlash(relPath),
		Type:     "image",
		Format:   format,
		Index:    index,
		Total:    state.Len(),
		Folder:   filepath.ToSlash(filepath.Dir(relPath)),
		TxID:     a.server.NextTxID(),
	}

	// [v2 Refactoring] Overlay metadata from the immutable state
	a.server.appStateMu.RLock()
	if a.server.appState != nil {
		if p, ok := a.server.appState.Photos[relPath]; ok {
			result.Starred = p.IsStarred
			result.Label = p.Label
			result.Rotation = p.Rotation
		}
	}
	a.server.appStateMu.RUnlock()

	if !skipAnalysis {
		if br := a.server.getBurstInfo(index, absPath); br != nil {
			result.Burst = &BurstInfoResp{
				Count: br.count,
				Index: br.index,
			}
		}
	}

	if size, err := state.FileSize(index); err == nil {
		result.Size = size
	}

	if meta := a.server.cache.GetMetadata(absPath); meta != nil {
		if meta.Width > 0 && meta.Height > 0 {
			result.Width = meta.Width
			result.Height = meta.Height
		}
		result.Camera = meta.Camera
		result.ISO = meta.ISO
		result.Aperture = meta.Aperture
		result.Shutter = meta.Shutter
		result.Focal = meta.Focal
		result.Date = meta.Date
	}

	if index > 0 {
		prevPath, _ := state.AbsPath(index - 1)
		curPath, _ := state.AbsPath(index)
		if prevPath != "" && curPath != "" {
			if sim := a.server.cache.CompareSimilarity(prevPath, curPath); sim >= 0 {
				result.Similarity = sim
			}
		}
	}

	utils.LogNav("GetFile: complete", "index", index, "txID", result.TxID)
	return result, nil
}

// SetLabel sets a color label (1 to domain.MaxLabel) or 0 to clear, for one or multiple files
func (a *App) SetLabel(index int, path string, paths []string, label int) (any, error) {
	if label < 0 || label > domain.MaxLabel {
		return nil, domain.ErrInvalidCriteria
	}

	if len(paths) > 0 {
		return a.executeBatchAction(paths, bus.TypeCommandLabelPhoto, func(rel string, p Photo) any {
			if p.Label == label {
				return nil // already in desired state, skip
			}
			return bus.CommandLabelPhotoPayload{
				PhotoID:  rel,
				Label:    label,
				OldLabel: p.Label,
			}
		})
	}

	state, actualPath, _, err := a.verifyFile(index, path)
	if err != nil {
		return nil, err
	}

	p, _ := a.getPhoto(actualPath)
	oldLabel := p.Label

	return a.executePhotoActionVerified(state, bus.TypeCommandLabelPhoto, bus.CommandLabelPhotoPayload{
		PhotoID:  actualPath,
		Label:    label,
		OldLabel: oldLabel,
	})
}

// ToggleStar sets the star state of a file or set of files to the given value.
func (a *App) ToggleStar(index int, path string, paths []string, starred bool) (any, error) {
	if len(paths) > 0 {
		return a.executeBatchAction(paths, bus.TypeCommandToggleStar, func(rel string, p Photo) any {
			if p.IsStarred == starred {
				return nil // already in desired state, skip
			}
			return bus.CommandToggleStarPayload{PhotoID: rel, Starred: starred, OldStarred: p.IsStarred}
		})
	}

	state, actualPath, _, err := a.verifyFile(index, path)
	if err != nil {
		return nil, err
	}

	p, _ := a.getPhoto(actualPath)
	oldStarred := p.IsStarred

	return a.executePhotoActionVerified(state, bus.TypeCommandToggleStar, bus.CommandToggleStarPayload{
		PhotoID:    actualPath,
		Starred:    starred,
		OldStarred: oldStarred,
	})
}

// Trash moves files to trash
func (a *App) Trash(index int, path string, paths []string) (ActionResponse, error) {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()

	slog.Debug("Trash action received", "index", index, "path", path, "paths_count", len(paths))
	state, err := a.requireState()
	if err != nil {
		return ActionResponse{}, err
	}

	var targetIndex int
	var removedAbsPaths []string

	// Multi-select by explicit paths is safer against stale indices.
	if len(paths) > 0 {
		slog.Debug("Trashing multiple files", "paths", paths)
		targetIndex = index
		// Collect absolute paths and create events
		var events []bus.Event
		for _, rel := range paths {
			idx := state.FindIndex(rel)
			if idx != -1 {
				if abs, err := state.AbsPath(idx); err == nil {
					removedAbsPaths = append(removedAbsPaths, abs)
				}
			}
			events = append(events, bus.Event{
				Type: bus.TypeCommandTrashPhoto,
				Payload: bus.CommandTrashPhotoPayload{
					PhotoID:       rel,
					OriginalIndex: idx,
				},
			})
		}

		// Update physical state BEFORE publishing so the EventEngine
		// sees a consistent state when it calls syncPhysicalState.
		newTotal, err := state.TrashMultiplePaths(paths)
		if err != nil {
			return ActionResponse{}, err
		}

		// Apply trash events synchronously
		if err := a.applyTrashEvents(events); err != nil {
			return ActionResponse{}, err
		}
		return a.finalizeAction(state, newTotal, targetIndex, removedAbsPaths), nil
	}

	// Single file trash
	state, actualPath, resolvedIndex, err := a.verifyFile(index, path)
	if err != nil {
		return ActionResponse{}, err
	}

	slog.Debug("Trashing single file", "index", resolvedIndex, "path", actualPath)
	targetIndex = resolvedIndex
	if abs, err := state.AbsPath(resolvedIndex); err == nil {
		removedAbsPaths = append(removedAbsPaths, abs)
	}

	newTotal, err := state.Trash(resolvedIndex)
	if err != nil {
		return ActionResponse{}, err
	}

	// Apply single trash event synchronously
	_, _, err = a.server.applyEvent(bus.Event{
		Type: bus.TypeCommandTrashPhoto,
		Payload: bus.CommandTrashPhotoPayload{
			PhotoID:       actualPath,
			OriginalIndex: resolvedIndex,
		},
	})
	if err != nil {
		return ActionResponse{}, err
	}

	return a.finalizeAction(state, newTotal, targetIndex, removedAbsPaths), nil
}

// applyTrashEvents applies the appropriate trash event(s) synchronously based on count.
// Uses a single event for one file, or a batch event for multiple files.
func (a *App) applyTrashEvents(events []bus.Event) error {
	var err error
	switch len(events) {
	case 1:
		_, _, err = a.server.applyEvent(events[0])
	default:
		_, _, err = a.server.applyEvent(bus.Event{
			Type:    bus.TypeCommandBatch,
			Payload: bus.CommandBatchPayload{Events: events},
		})
	}
	return err
}

// GetConfig returns the app configuration
func (a *App) GetConfig() (domain.Config, error) {
	return domain.GetConfig(), nil
}

// UpdateConfig updates the app configuration
func (a *App) UpdateConfig(cfg domain.Config) error {
	err := domain.UpdateConfig(cfg)
	if err == nil {
		resetExiftoolAvailabilityCache()
	}
	return err
}

// GetFolders returns current state folders
func (a *App) GetFolders() ([]FolderInfo, error) {
	state, err := a.requireState()
	if err != nil {
		return []FolderInfo{}, nil
	}
	folders := state.Folders()
	if folders == nil {
		return []FolderInfo{}, nil
	}
	return folders, nil
}

// GetStarredIndices returns the indices of starred files
func (a *App) GetStarredIndices() (FilteredIndicesResponse, error) {
	a.server.appStateMu.RLock()
	defer a.server.appStateMu.RUnlock()

	if a.server.appState == nil {
		return FilteredIndicesResponse{Indices: []int{}}, nil
	}

	indices := []int{}
	for i, id := range a.server.appState.VisibleOrder {
		photo, ok := a.server.appState.Photos[id]
		if ok && photo.IsStarred {
			indices = append(indices, i)
		}
	}
	return FilteredIndicesResponse{Indices: indices}, nil
}

// GetLabelIndices returns indices for a specific color label (1 to domain.MaxLabel) or any labeled (0)
func (a *App) GetLabelIndices(label int) (FilteredIndicesResponse, error) {
	a.server.appStateMu.RLock()
	defer a.server.appStateMu.RUnlock()

	if a.server.appState == nil {
		return FilteredIndicesResponse{Indices: []int{}}, nil
	}

	indices := []int{}
	for i, id := range a.server.appState.VisibleOrder {
		photo, ok := a.server.appState.Photos[id]
		if !ok {
			continue
		}
		if label == 0 {
			if photo.Label != 0 {
				indices = append(indices, i)
			}
		} else if photo.Label == label {
			indices = append(indices, i)
		}
	}
	return FilteredIndicesResponse{Indices: indices}, nil
}

// GetFilters returns unique metadata values for filtering
func (a *App) GetFilters() (FilterValuesResponse, error) {
	defer utils.HandlePanic()
	state := a.server.getState()
	res := FilterValuesResponse{Cameras: []string{}, ISOs: []string{}}
	if state == nil {
		return res, nil
	}

	cameras, isos := a.server.cache.GetFilterValues()
	res.Cameras = cameras
	res.ISOs = isos

	return res, nil
}

// GetFilteredIndices returns a list of indices matching the criteria
func (a *App) GetFilteredIndices(filters map[string]string) (FilteredIndicesResponse, error) {
	state := a.server.getState()
	if state == nil {
		return FilteredIndicesResponse{Indices: []int{}}, nil
	}

	indices := a.server.cache.GetFilteredIndices(state, filters)
	if indices == nil {
		indices = []int{}
	}

	return FilteredIndicesResponse{Indices: indices}, nil
}

// RevealInExplorer opens the system file explorer at the file's location
func (a *App) RevealInExplorer(index int) error {
	state := a.server.getState()
	if state == nil {
		return domain.ErrFolderNotFound
	}
	absPath, err := state.AbsPath(index)
	if err != nil {
		return err
	}

	return utils.RevealFileInExplorer(absPath)
}

// Refresh rescans the current folder
func (a *App) Refresh(currentIndex int) (ActionResponse, error) {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()

	state := a.server.getState()
	if state == nil {
		return ActionResponse{}, domain.ErrFolderNotFound
	}

	newFiles, err := scanRefreshFiles(state.Root())
	if err != nil {
		return ActionResponse{}, newScanError(scanOpRefresh, state.Root(), err)
	}

	if currentState := a.server.getState(); currentState != state {
		return a.staleRefreshResponse(currentState, currentIndex), nil
	}

	var curRel string
	if rel, err := state.Get(currentIndex); err == nil {
		curRel = rel
	}

	newTotal := state.UpdateFiles(newFiles)
	a.server.invalidateBurstCache()
	a.purgeRefreshCaches(state)
	a.server.ReconcileScannedFiles(newFiles)

	newIndex := resolveRefreshIndex(state, curRel, currentIndex, newTotal)
	a.server.startBackgroundAnalysis(a.bgContext())

	return ActionResponse{Stats: a.snapshotStats(), Total: newTotal, Index: newIndex}, nil
}

func scanRefreshFiles(root string) ([]string, error) {
	filesChan := make(chan string, 100)
	scanErrCh := make(chan error, 1)
	utils.SafeGo(func() { scanErrCh <- scanFiles(root, filesChan) })

	var files []string
	for file := range filesChan {
		files = append(files, file)
	}
	if err := <-scanErrCh; err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func (a *App) staleRefreshResponse(state *State, currentIndex int) ActionResponse {
	total := 0
	index := -1
	if state != nil {
		total = state.Len()
		if total > 0 {
			index = min(max(currentIndex, 0), total-1)
		}
	}
	return ActionResponse{Stats: a.snapshotStats(), Total: total, Index: index}
}

func (a *App) purgeRefreshCaches(state *State) {
	activePaths := state.ActiveAbsPaths()
	validSet := make(map[string]struct{}, len(activePaths))
	derivedKeep := make(map[string]struct{}, len(activePaths)*2+2)
	for _, p := range activePaths {
		validSet[p] = struct{}{}
		if thumbPath, err := ThumbCachePathForSource(p, a.server.cacheDir); err == nil {
			derivedKeep[thumbPath] = struct{}{}
		}
		if meta := a.server.cache.GetMetadata(p); meta != nil && meta.Fingerprint != "" {
			processedPath := ProcessedCachePathWithFingerprint(p, meta.Fingerprint, a.server.cacheDir)
			derivedKeep[processedPath] = struct{}{}
		}
	}
	metaRemoved, hashRemoved := a.server.cache.PurgeMissing(validSet)
	derivedRemoved := a.server.PurgeDerivedCacheFiles(derivedKeep)
	a.server.recordCacheGC(metaRemoved, hashRemoved, derivedRemoved)
	if metaRemoved > 0 || hashRemoved > 0 {
		slog.Info("Cache GC completed", "metadata_removed", metaRemoved, "hash_removed", hashRemoved)
	}
	if derivedRemoved > 0 {
		slog.Info("Derived cache GC completed", "files_removed", derivedRemoved)
	}
}

func resolveRefreshIndex(state *State, currentPath string, currentIndex, total int) int {
	newIndex := -1
	if currentPath != "" {
		if idx := state.FindIndex(currentPath); idx != -1 {
			newIndex = idx
		}
	}
	if newIndex == -1 && total > 0 {
		newIndex = min(currentIndex, total-1)
		if newIndex < 0 {
			newIndex = 0
		}
	}
	return newIndex
}

// Rotate rotates an image
func (a *App) Rotate(index int, path string, direction string) (ActionResponse, error) {
	if direction != "left" && direction != "right" {
		return ActionResponse{}, domain.ErrInvalidRotationDir
	}

	state, actualPath, _, err := a.verifyFile(index, path)
	if err != nil {
		return ActionResponse{}, err
	}

	return a.executePhotoActionVerified(state, bus.TypeCommandRotatePhoto, bus.CommandRotatePhotoPayload{
		PhotoID:   actualPath,
		Direction: direction,
	})
}

// RotateReset resets rotation
func (a *App) RotateReset(index int, path string) (ActionResponse, error) {
	state, actualPath, _, err := a.verifyFile(index, path)
	if err != nil {
		return ActionResponse{}, err
	}

	return a.executePhotoActionVerified(state, bus.TypeCommandRotatePhoto, bus.CommandRotatePhotoPayload{
		PhotoID:   actualPath,
		Direction: "reset",
	})
}

// ApplyRotation applies visual rotation to EXIF
func (a *App) ApplyRotation(index int, path string) error {
	state, actualPath, resolvedIndex, err := a.verifyFile(index, path)
	if err != nil {
		return err
	}

	absPath := filepath.Join(state.Root(), actualPath)
	rotation := func() int {
		a.server.appStateMu.RLock()
		defer a.server.appStateMu.RUnlock()
		if a.server.appState != nil {
			if p, ok := a.server.appState.Photos[actualPath]; ok {
				return p.Rotation
			}
		}
		return 0
	}()

	if rotation == 0 {
		return nil
	}
	ft := state.GetType(resolvedIndex)

	if !ft.SupportsEXIFWrite() {
		return domain.ErrExifWriteUnsupported
	}

	err = applyEXIFOrientation(a.bgContext(), absPath, rotation)
	if err == nil {
		// Invalidate cache since the file has changed physically
		a.server.cache.DropPath(absPath)
		a.dropDerivedCacheForSource(absPath)

		// [v2 Refactoring] Reset visual rotation in immutable state too
		if _, _, err := a.server.applyEvent(bus.Event{
			Type: bus.TypeCommandRotatePhoto,
			Payload: bus.CommandRotatePhotoPayload{
				PhotoID:   actualPath,
				Direction: "reset",
			},
		}); err != nil {
			slog.Warn("ApplyRotation: failed to reset visual rotation", "path", actualPath, "error", err)
		}
	}
	return err
}

// ReanalyzeMetadata forces metadata refresh for the current file and returns updated file payload.
func (a *App) ReanalyzeMetadata(index int) (FileResponse, error) {
	state, err := a.requireState()
	if err != nil {
		return FileResponse{}, err
	}
	absPath, err := state.AbsPath(index)
	if err != nil {
		return FileResponse{}, err
	}
	a.server.cache.RefreshMetadata(absPath)
	return a.GetFile(index, true)
}

// Undo reverts last action
func (a *App) Undo() (UndoResponse, error) {
	state := a.server.getState()
	if state == nil {
		return UndoResponse{}, domain.ErrFolderNotFound
	}

	// [v2 Refactoring] Use Event Bus for Undo synchronously to return the response
	ev := bus.Event{
		Type:    bus.TypeCommandUndo,
		Payload: bus.CommandUndoPayload{},
	}

	ok, undoneEvent, err := a.server.applyEvent(ev)
	if err != nil {
		return UndoResponse{}, err
	}
	if !ok {
		return UndoResponse{}, domain.ErrNothingToUndo
	}

	actionType, photoID, index := undoResponseTarget(undoneEvent)
	if photoID != "" && index < 0 {
		index = state.FindIndex(photoID)
	}
	if actionType == "" {
		return UndoResponse{}, domain.ErrNothingToUndo
	}

	return UndoResponse{
		Stats:      a.snapshotStats(),
		ActionType: actionType,
		Index:      index,
	}, nil
}

func undoResponseTarget(event bus.Event) (actionType, photoID string, index int) {
	index = -1
	photoID = photoIDFromPayload(event.Payload)
	switch payload := event.Payload.(type) {
	case bus.CommandTrashPhotoPayload:
		return "trash", payload.PhotoID, payload.OriginalIndex
	case bus.CommandToggleStarPayload:
		return "star", payload.PhotoID, index
	case bus.CommandLabelPhotoPayload:
		return "label", payload.PhotoID, index
	case bus.CommandRotatePhotoPayload:
		return "rotate", payload.PhotoID, index
	case bus.CommandBatchPayload:
		if len(payload.Events) == 0 {
			return "", "", 0
		}
		return undoActionType(payload.Events[0].Payload), "", 0
	default:
		return "", photoID, index
	}
}

func undoActionType(payload any) string {
	switch payload.(type) {
	case bus.CommandToggleStarPayload:
		return "star"
	case bus.CommandLabelPhotoPayload:
		return "label"
	case bus.CommandTrashPhotoPayload:
		return "trash"
	case bus.CommandRotatePhotoPayload:
		return "rotate"
	default:
		return ""
	}
}

// GetHistory returns folder history
func (a *App) GetHistory() ([]string, error) {
	h, err := domain.GetHistory()
	if h == nil {
		return []string{}, nil
	}
	return h, err
}

// GetDuplicates returns groups of image indices that are visually similar.
func (a *App) GetDuplicates(threshold float64) ([][]int, error) {
	defer utils.HandlePanic()
	state := a.server.getState()
	if state == nil {
		return nil, domain.ErrFolderNotFound
	}
	if threshold <= 0 {
		threshold = dupSimilarityThreshold
	}
	groups := a.server.cache.GetDuplicateGroups(state, threshold)
	if groups == nil {
		groups = [][]int{}
	}
	return groups, nil
}

// RemoveHistory removes a path from history
func (a *App) RemoveHistory(path string) error {
	if err := domain.RemoveFromHistory(path); err != nil {
		slog.Warn("Failed to remove folder from history", "path", path, "error", err)
	}
	return nil
}

// BrowseDialog opens a native folder picker
func (a *App) BrowseDialog() (PathResponse, error) {
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select Photos Folder",
	})
	if err != nil {
		return PathResponse{}, err
	}
	return PathResponse{Path: path}, nil
}

// ExiftoolDialog opens a native file picker for exiftool binary
func (a *App) ExiftoolDialog() (PathResponse, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select Exiftool Binary",
	})
	if err != nil {
		return PathResponse{}, err
	}
	return PathResponse{Path: path}, nil
}

// Browse returns directory contents
func (a *App) Browse(reqPath string) (BrowseResponse, error) {
	if reqPath == "" {
		reqPath, _ = os.UserHomeDir()
	}
	reqPath = filepath.Clean(reqPath)

	info, err := os.Stat(reqPath)
	if err != nil || !info.IsDir() {
		return BrowseResponse{}, domain.ErrFolderNotFound
	}

	entries, err := os.ReadDir(reqPath)
	if err != nil {
		return BrowseResponse{}, err
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	if dirs == nil {
		dirs = []string{}
	}
	sort.Strings(dirs)

	parent := filepath.Dir(reqPath)
	if parent == reqPath {
		parent = ""
	}

	return BrowseResponse{
		Path:    reqPath,
		Parent:  parent,
		Sep:     string(filepath.Separator),
		Entries: dirs,
	}, nil
}

// GetBookmarks returns OS bookmarks
func (a *App) GetBookmarks() (BookmarkResponse, error) {
	home, _ := os.UserHomeDir()
	sep := string(filepath.Separator)

	standard := []struct{ icon, label, folder string }{
		{"🏠", "Home", ""},
		{"🖥", "Desktop", "Desktop"},
		{"🖼", "Pictures", "Pictures"},
		{"⬇", "Downloads", "Downloads"},
	}

	bookmarks := []Bookmark{}
	for _, s := range standard {
		p := filepath.Join(home, s.folder)
		if _, err := os.Stat(p); err == nil {
			bookmarks = append(bookmarks, Bookmark{s.label, p, s.icon})
		}
	}

	return BookmarkResponse{
		Bookmarks: bookmarks,
		Home:      home,
		Sep:       sep,
	}, nil
}

// ResetStars clears all stars in the current folder
func (a *App) ResetStars() error {
	_, _, err := a.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandResetMetadata,
		Payload: bus.CommandResetMetadataPayload{Scope: "stars"},
	})
	return err
}

// ResetLabels clears all labels in the current folder
func (a *App) ResetLabels() error {
	_, _, err := a.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandResetMetadata,
		Payload: bus.CommandResetMetadataPayload{Scope: "labels"},
	})
	return err
}

// ResetAppCache clears the whole application cache directory but preserves the config file.
// Coordinated with OpenFolder via the `loading` CAS to avoid concurrent teardown.
func (a *App) ResetAppCache() error {
	slog.Info("ResetAppCache: clearing cache directory")
	if !a.loading.CompareAndSwap(false, true) {
		// A folder load is in progress: refuse to race to avoid tearing down
		// state/cache while a brand-new analysis is starting.
		return domain.ErrLoadInProgress
	}
	defer a.loading.Store(false)

	// Cancel analysis AND wake idle queue waiters so Wait() returns promptly.
	a.server.analysisSched.Cancel()
	a.server.analysisSched.Wait()

	a.server.closePersistence()
	a.server.cache.Close()

	a.server.stateMu.Lock()
	a.server.state = nil
	a.server.cacheDir = ""
	a.server.invalidateBurstCache()
	a.server.InvalidateSavedPositionCache()
	a.server.stateMu.Unlock()

	appCacheDir := domain.GetAppCacheDir()
	entries, err := os.ReadDir(appCacheDir)
	if err != nil {
		// If dir doesn't exist, nothing to do
		a.server.cache = NewMediaCache()
		return nil
	}

	var errs []error
	for _, entry := range entries {
		name := entry.Name()
		if name == "config.json" {
			continue // Preserve user preferences
		}
		path := filepath.Join(appCacheDir, name)
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("ResetAppCache: failed to remove path", "path", path, "error", err)
			errs = append(errs, err)
		}
	}

	a.server.cache = NewMediaCache()
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ListTrash lists relative paths currently in .trash/
func (a *App) ListTrash() (TrashListResponse, error) {
	state := a.server.getState()
	if state == nil {
		return TrashListResponse{Items: []string{}}, nil
	}
	items, err := state.ListTrash()
	if err != nil {
		return TrashListResponse{}, err
	}
	if items == nil {
		items = []string{}
	}
	return TrashListResponse{Items: items}, nil
}

// RestoreFromTrash moves files from .trash back into the folder.
func (a *App) RestoreFromTrash(relPaths []string) (RestoreResponse, error) {
	state := a.server.getState()
	if state == nil {
		return RestoreResponse{}, domain.ErrFolderNotFound
	}
	restored, err := state.RestoreFromTrash(relPaths)
	if err != nil {
		return RestoreResponse{}, err
	}
	a.server.invalidateBurstCache()
	a.server.RefreshVisibleOrder()

	idx := 0
	if len(restored) > 0 {
		idx = state.FindIndex(restored[0])
		if idx < 0 {
			idx = 0
		}
	}
	out := RestoreResponse{
		Stats:    a.snapshotStats(),
		Restored: restored,
		Total:    state.Len(),
		Index:    idx,
	}
	a.emitStateUpdate()
	return out, nil
}

// SysCheck checks for external tool availability
func (a *App) SysCheck() (SysCheckResponse, error) {
	exifPath := domain.ExiftoolPath()

	// Check if path exists directly first (for absolute paths)
	_, errStat := os.Stat(exifPath)
	found := errStat == nil

	// If not found as absolute, check in PATH
	if !found {
		_, errLook := exec.LookPath(exifPath)
		found = errLook == nil
	}

	return SysCheckResponse{
		Exiftool:     found,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Capabilities: buildRuntimeCapabilities(found),
	}, nil
}

func buildRuntimeCapabilities(exiftoolFound bool) RuntimeCapabilities {
	heicNative := HeicSupported()
	return RuntimeCapabilities{
		RawPreview:  exiftoolFound,
		RawMetadata: exiftoolFound,
		HeicDecode:  heicNative || exiftoolFound,
		ExifWrite:   exiftoolFound,
	}
}

// ExportFiles copies or moves the specified paths to a destination folder.
func (a *App) ExportFiles(paths []string, destDir string, move bool) error {
	if len(paths) == 0 {
		return nil
	}
	return a.server.ExportFilesPaths(paths, destDir, move)
}

// ExportSelection exports all files matching the criteria (starred or label).
func (a *App) ExportSelection(criteria string, label int, destDir string, move bool) error {
	a.server.appStateMu.RLock()
	defer a.server.appStateMu.RUnlock()

	if a.server.appState == nil {
		return domain.ErrFolderNotFound
	}

	var paths []string
	for _, id := range a.server.appState.VisibleOrder {
		p := a.server.appState.Photos[id]
		match := false
		switch criteria {
		case "starred":
			match = p.IsStarred
		case "label":
			if label == 0 {
				match = p.Label != 0
			} else {
				match = p.Label == label
			}
		}

		if match {
			paths = append(paths, id)
		}
	}

	if len(paths) == 0 {
		return nil
	}

	return a.server.ExportFilesPaths(paths, destDir, move)
}

// SetSortOrder changes the file list order ("name" or "date").
func (a *App) SetSortOrder(order string) error {
	state := a.server.getState()
	if state == nil {
		return domain.ErrFolderNotFound
	}

	switch order {
	case "date":
		state.SortByDate(a.server.cache)
	case "name":
		state.SortByName()
	default:
		return domain.ErrInvalidCriteria
	}

	a.server.invalidateBurstCache()
	a.finalizeStructuralChange()
	return nil
}

// SavePosition saves the current index
func (a *App) SavePosition(index int) {
	a.server.SavePosition(index)
}

// Quit closes the app
func (a *App) Quit() {
	a.server.analysisSched.Cancel()
	utils.StopFlusher()
	exif.Cleanup()
	wailsruntime.Quit(a.ctx)
}

// SearchStream triggers a streaming search based on a query.
// Cancels any previous search to avoid stale results and goroutine leaks.
func (a *App) SearchStream(query string) {
	state := a.server.getState()
	if state == nil {
		return
	}

	a.server.searchMu.Lock()
	if a.server.searchCancel != nil {
		a.server.searchCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.server.searchCancel = cancel
	a.server.searchMu.Unlock()

	utils.SafeGo(func() {
		defer cancel()
		a.server.SearchStream(ctx, query)
	})
}

// PrioritizeIndices tells the backend which indices are currently visible.
func (a *App) PrioritizeIndices(indices []int) {
	a.server.PrioritizeIndices(indices)
}

// CancelExport stops any running export.
func (a *App) CancelExport() {
	a.server.CancelExport()
}

// OpenConfigFolder opens the system file explorer at the application configuration/cache directory.
func (a *App) OpenConfigFolder() error {
	return utils.OpenFolderInExplorer(domain.GetAppCacheDir())
}

// ExportLogs reads the application log, anonymizes it, and prompts the user to save it.
func (a *App) ExportLogs() error {
	logPath := domain.GetLogPath()
	data, err := os.ReadFile(logPath) // #nosec G304 -- application log path is controlled by app config.
	if err != nil {
		return err
	}

	sensitive := []string{}
	if state := a.server.getState(); state != nil {
		sensitive = append(sensitive, state.Root())
	}

	anonymized := utils.AnonymizeLogContent(string(data), sensitive...)

	savePath, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Export Logs",
		DefaultFilename: "quickcull_logs.txt",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Text Files (*.txt)", Pattern: "*.txt"},
		},
	})
	if err != nil || savePath == "" {
		return err
	}

	cleanPath, err := validatedExportSavePath(savePath)
	if err != nil {
		return err
	}
	// #nosec G703 -- path is selected by the local user through the native save dialog.
	return os.WriteFile(cleanPath, []byte(anonymized), 0o600)
}

func validatedExportSavePath(path string) (string, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", domain.ErrExportFailed
	}
	info, err := os.Stat(filepath.Dir(clean))
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", domain.ErrExportFailed
	}
	return clean, nil
}
