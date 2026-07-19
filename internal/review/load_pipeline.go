package review

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"quickcull/internal/persistence"
	"quickcull/internal/utils"
)

var (
	// startupSnapshotOverride is a test-only hook that allows forcing a snapshot
	// hit/miss regardless of actual filesystem state.
	startupSnapshotOverride sync.Map // map[*Server]bool
)

type loadSummary struct {
	Result           string
	Duration         time.Duration
	Discovered       int
	SnapshotHit      bool
	SnapshotMiss     string
	Reconciled       int
	MetadataRestored int
	UndoRestored     int
	SyncEmitted      bool
}

func logLoadSummary(v loadSummary) {
	utils.LogCore("LoadState: lifecycle summary",
		"result", v.Result,
		"duration_ms", v.Duration.Milliseconds(),
		"discovered", v.Discovered,
		"snapshot_hit", v.SnapshotHit,
		"snapshot_fallback_reason", v.SnapshotMiss,
		"reconcile_diff_count", v.Reconciled,
		"metadata_restored", v.MetadataRestored,
		"undo_restored", v.UndoRestored,
		"sync_emitted", v.SyncEmitted,
	)
}

func (s *Server) loadStateBootstrap(root string) (*loadPipeline, error) {
	startedAt := time.Now()
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("LoadState: failed to resolve absolute path: %w", err)
	}

	utils.LogCore("LoadState: Starting", "path", root)

	folderID := domain.GetFolderID(absRoot)
	cacheDir := filepath.Join(domain.GetAppCacheDir(), folderID)
	utils.LogCore("LoadState: Opening media cache", "dir", cacheDir)
	s.cacheDir = cacheDir
	s.cache.LoadCache(cacheDir)
	utils.LogCore("LoadState: Cache opened")

	s.stateMu.Lock()
	s.appStateMu.Lock()
	s.appState = &AppState{
		Root:     absRoot,
		CacheDir: cacheDir,
		Photos:   make(map[string]Photo),
	}

	metadataRestored := s.loadPersistedMetadataLocked(folderID)
	undoRestored := s.loadPersistedHistoryLocked(folderID)

	s.appStateMu.Unlock()
	s.stateMu.Unlock()

	// Perform discovery bootstrap: check if we can hydrate from a snapshot.
	newState := NewState(absRoot, []string{})
	snapshotUsed, snapshotMiss := s.hydrateStateFromSnapshot(folderID, absRoot, newState)

	if snapshotUsed {
		s.appStateMu.Lock()
		s.appState.VisibleOrder = newState.files
		for _, f := range newState.files {
			if _, ok := s.appState.Photos[f]; !ok {
				s.appState.Photos[f] = Photo{ID: f}
			}
		}
		s.appStateMu.Unlock()
	}

	// Standardize on SyncState (structural)
	s.SyncFullState()

	s.stateMu.Lock()
	s.state = newState
	s.stateMu.Unlock()

	return &loadPipeline{
		absRoot:          absRoot,
		folderID:         folderID,
		newState:         newState,
		startedAt:        startedAt,
		metadataRestored: metadataRestored,
		undoRestored:     undoRestored,
		snapshotUsed:     snapshotUsed,
		snapshotMiss:     snapshotMiss,
		syncEmitted:      true,
		scanned:          make(map[string]struct{}),
	}, nil
}

func (s *Server) loadPersistedHistoryLocked(folderID string) int {
	if s.persistence == nil {
		return 0
	}
	historyData, err := s.persistence.LoadHistory(folderID)
	if err != nil {
		slog.Debug("LoadState: no persisted history", "folder", folderID, "error", err)
		return 0
	}
	if len(historyData) == 0 {
		return 0
	}

	var history []bus.Event
	if err := json.Unmarshal(historyData, &history); err != nil {
		utils.LogWarn("LoadState: failed to decode history JSON, starting fresh",
			"folder", folderID, "error", err)
		return 0
	}

	if s.appState != nil {
		s.appState.History = history
		s.appState.UndoLen = len(history)
	}
	return len(history)
}

func (s *Server) loadPersistedMetadataLocked(folderID string) int {
	if s.persistence == nil {
		return 0
	}
	meta, err := s.persistence.LoadFolderMetadata(folderID)
	if err != nil {
		slog.Debug("LoadState: no persisted folder metadata", "folder", folderID, "error", err)
		return 0
	}
	for id, m := range meta {
		s.appState.Photos[id] = Photo{
			ID:        id,
			IsStarred: m.IsStarred,
			Rotation:  m.Rotation,
			Label:     m.Label,
			IsTrashed: m.IsTrashed,
		}
	}
	return len(meta)
}

func (s *Server) loadStateIngest(p *loadPipeline) (int, error) {
	utils.LogCore("LoadState: Starting discovery consumer")

	filesChan := make(chan string, filesChanBufSize)
	errCh := make(chan error, 1)
	go func() {
		errCh <- scanFiles(p.absRoot, filesChan)
	}()

	lastEmit := time.Now()
	count := 0

	for relPath := range filesChan {
		p.scanned[relPath] = struct{}{}
		count++

		s.recordDiscoveredFile(relPath)
		p.newState.AddFile(relPath)

		zeroIdx := count - 1
		priority := 0
		if zeroIdx < analysisStartupPriorityCount {
			priority = analysisStartupPriority
		}
		s.analysisQueue.Push(zeroIdx, priority)

		if time.Since(lastEmit) > ingestProgressInterval {
			s.emitIngestProgress(count)

			if count > 0 && count%sortSyncCadence == 0 {
				s.RefreshVisibleOrder()
			}

			lastEmit = time.Now()
		}
	}

	if err := <-errCh; err != nil {
		return count, err
	}

	return count, nil
}

func (s *Server) recordDiscoveredFile(relPath string) {
	s.appStateMu.Lock()
	s.appState.VisibleOrder = append(s.appState.VisibleOrder, relPath)
	if _, ok := s.appState.Photos[relPath]; !ok {
		s.appState.Photos[relPath] = Photo{ID: relPath}
	}
	s.appStateMu.Unlock()
}

func (s *Server) emitIngestProgress(count int) {
	s.progressMu.Lock()
	s.progressTotal = count
	s.progressMu.Unlock()

	current, _, _, _ := s.analysisProgress()
	s.broadcast("progress", map[string]any{
		"current":         current,
		"total":           count,
		"scanning":        true,
		"discovery_count": count,
		"ready_count":     current,
	})
}

func (s *Server) loadStateFinalize(p *loadPipeline, finalCount int) error {
	p.newState.SortFiles()
	p.reconcileDif = s.reconcileState(p.newState, p.scanned)

	s.appStateMu.Lock()
	if s.appState == nil {
		s.appStateMu.Unlock()
		return domain.ErrFolderNotFound
	}

	s.appState.VisibleOrder = p.newState.files
	s.appState.TrashedCount = p.newState.trashedCount
	s.appState.RecalculateCounts()
	s.appState.photos = newPhotoStore(s.appState.Photos)
	s.appState.Photos = nil
	s.appStateMu.Unlock()

	s.progressMu.Lock()
	s.progressTotal = finalCount
	s.progressMu.Unlock()

	utils.LogCore("LoadState: Discovery complete", "count", finalCount)
	utils.LogCore("LoadState: snapshot metrics",
		"snapshot_hit", p.snapshotUsed,
		"snapshot_fallback_reason", p.snapshotMiss,
		"reconcile_diff_count", p.reconcileDif,
	)
	current, _, _, _ := s.analysisProgress()
	s.broadcast("progress", map[string]any{
		"current":         current,
		"total":           finalCount,
		"scanning":        false,
		"discovery_count": finalCount,
		"ready_count":     current,
	})

	if p.reconcileDif > 0 || !p.snapshotUsed {
		utils.LogCore("LoadState: Changes detected, refreshing UI state", "diff", p.reconcileDif)
		s.SyncFullState()
		p.syncEmitted = true
	} else {
		utils.LogCore("LoadState: No changes detected, skipping redundant UI sync")
	}

	s.stateMu.Lock()
	s.state = p.newState
	s.stateMu.Unlock()

	s.saveFolderSnapshot(p)
	logLoadSummary(loadSummary{
		Result:           "completed",
		Duration:         time.Since(p.startedAt),
		Discovered:       finalCount,
		SnapshotHit:      p.snapshotUsed,
		SnapshotMiss:     p.snapshotMiss,
		Reconciled:       p.reconcileDif,
		MetadataRestored: p.metadataRestored,
		UndoRestored:     p.undoRestored,
		SyncEmitted:      p.syncEmitted,
	})

	return nil
}

func (s *Server) reconcileState(newState *State, scanned map[string]struct{}) int {
	s.appStateMu.RLock()
	defer s.appStateMu.RUnlock()

	diff := 0
	// 1. Identify "ghost" photos that were in AppState/Snapshot but are NOT on disk (scanned)
	var toRemove []string
	for id := range s.appState.Photos {
		if _, ok := scanned[id]; !ok {
			toRemove = append(toRemove, id)
			diff++
		}
	}

	// 2. Remove them from the physical state index
	if diff > 0 {
		for _, id := range toRemove {
			newState.StateRemove(id)
		}
	}

	return diff
}

func (s *Server) hydrateStateFromSnapshot(folderID, absRoot string, newState *State) (bool, string) {
	if v, ok := startupSnapshotOverride.Load(s); ok {
		if !v.(bool) {
			return false, "test_override"
		}
	}

	if s.persistence == nil {
		return false, "no_persistence"
	}
	snap, ok := s.persistence.GetFolderSnapshot(folderID)
	if !ok {
		return false, "not_found"
	}

	sig := BuildFolderSignature(absRoot)
	if !IsSnapshotUsable(absRoot, snap, sig) {
		if snap.Signature != sig {
			return false, "signature_mismatch"
		}
		return false, "invalid"
	}

	newState.UpdateFiles(snap.VisibleOrder)
	return true, ""
}

func (s *Server) saveFolderSnapshot(p *loadPipeline) {
	if s.persistence == nil {
		return
	}
	sig := BuildFolderSignature(p.absRoot)
	if sig == "" {
		slog.Debug("LoadState: skip snapshot save, signature unavailable", "root", p.absRoot)
		return
	}

	snap := persistence.FolderSnapshot{
		Version:      folderSnapshotVersion,
		RootPath:     p.absRoot,
		SavedAt:      time.Now().Unix(),
		Signature:    sig,
		VisibleOrder: p.newState.files,
	}
	if err := s.persistence.SaveFolderSnapshot(p.folderID, snap); err != nil {
		utils.LogWarn("LoadState: failed to save startup snapshot", "folder", p.folderID, "error", err)
	}
}

type loadPipeline struct {
	absRoot          string
	folderID         string
	newState         *State
	scanned          map[string]struct{}
	startedAt        time.Time
	metadataRestored int
	undoRestored     int
	syncEmitted      bool
	snapshotUsed     bool
	snapshotMiss     string
	reconcileDif     int
}
