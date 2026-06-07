package review

import (
	"quickcull/internal/domain"
)

// AppStats contains global application and folder metrics.
type AppStats struct {
	Total          int    `json:"total"`
	InitialTotal   int    `json:"initialTotal"`
	TrashedCount   int    `json:"trashedCount"`
	StarredCount   int    `json:"starredCount"`
	RotatedCount   int    `json:"rotatedCount"`
	LabeledCount   int    `json:"labeledCount"`
	UndoLen        int    `json:"undoLen"`
	SavedPosition  int    `json:"savedPosition"`
	MaxLabel       int    `json:"maxLabel"`
	HeicSupported  bool   `json:"heicSupported"`
	Version        string `json:"version"`
	IoWorkers      int    `json:"ioWorkers"`
	HashDeferred   bool   `json:"hashDeferred"`
	CacheMetaGC    int    `json:"cacheMetaGc"`
	CacheHashGC    int    `json:"cacheHashGc"`
	CacheDerivedGC int    `json:"cacheDerivedGc"`
}

// snapshotStats assembles a point-in-time snapshot of application metrics.
func (s *Server) snapshotStats(state *State) AppStats {
	stats := AppStats{
		HeicSupported: HeicSupported(),
		Version:       domain.AppVersion,
		MaxLabel:      domain.MaxLabel,
	}

	// 1. Authoritative Domain State
	s.loadDomainStats(&stats)

	// 2. Physical State (Legacy & Physical Order)
	if state != nil {
		s.loadLegacyStats(state, &stats)
	}

	// 3. Telemetry & Performance
	s.loadTelemetryStats(&stats)

	return stats
}

func (s *Server) loadDomainStats(stats *AppStats) {
	s.appStateMu.RLock()
	defer s.appStateMu.RUnlock()

	if s.appState == nil {
		return
	}

	stats.StarredCount = s.appState.StarredCount
	stats.LabeledCount = s.appState.LabeledCount
	stats.RotatedCount = s.appState.RotatedCount
	stats.TrashedCount = s.appState.TrashedCount
	stats.UndoLen = s.appState.UndoLen
}

func (s *Server) loadLegacyStats(state *State, stats *AppStats) {
	stats.Total = state.Len()
	stats.InitialTotal = state.InitialTotal()

	// Fallback for trashed count if AppState is not yet active
	if stats.TrashedCount == 0 && state.TrashedCount() > 0 {
		stats.TrashedCount = state.TrashedCount()
	}

	// Persistence / Position
	stats.SavedPosition = s.GetSavedPosition(state.Root())
	if stats.SavedPosition <= 0 {
		stats.SavedPosition = state.LoadPosition()
	}
}

func (s *Server) loadTelemetryStats(stats *AppStats) {
	// Follow lock ordering: appStateMu -> perfMu
	w, hd, cm, ch, cd := s.perfSnapshot()
	stats.IoWorkers = w
	stats.HashDeferred = hd
	stats.CacheMetaGC = cm
	stats.CacheHashGC = ch
	stats.CacheDerivedGC = cd
}

// ActionResponse is a common response for actions that update stats and state.
type ActionResponse struct {
	Stats AppStats `json:"stats"`
	Ok    bool     `json:"ok,omitempty"`
	Index int      `json:"index,omitempty"`
	Total int      `json:"total,omitempty"`
}

// FileResponse represents the metadata of a single file.
type FileResponse struct {
	Filename   string         `json:"filename"`
	Type       string         `json:"type"`
	Format     string         `json:"format"`
	Index      int            `json:"index"`
	Total      int            `json:"total"`
	Folder     string         `json:"folder"`
	Starred    bool           `json:"starred"`
	Rotation   int            `json:"rotation"`
	Label      int            `json:"label"`
	Size       int64          `json:"size,omitempty"`
	Width      int            `json:"width,omitempty"`
	Height     int            `json:"height,omitempty"`
	Camera     string         `json:"camera,omitempty"`
	ISO        string         `json:"iso,omitempty"`
	Aperture   string         `json:"aperture,omitempty"`
	Shutter    string         `json:"shutter,omitempty"`
	Focal      string         `json:"focal,omitempty"`
	Date       string         `json:"date,omitempty"`
	Similarity float64        `json:"similarity,omitempty"`
	Burst      *BurstInfoResp `json:"burst,omitempty"`
	TxID       uint64         `json:"txID"`
}

type BurstInfoResp struct {
	Count int `json:"count"`
	Index int `json:"index"`
}

// LabelResponse is returned after setting a label.
type LabelResponse struct {
	Stats AppStats `json:"stats"`
	Label int      `json:"label"`
}

// StarResponse is returned after toggling a star.
type StarResponse struct {
	Stats   AppStats `json:"stats"`
	Starred bool     `json:"starred"`
}

// UndoResponse describes the state after an undo action.
type UndoResponse struct {
	Stats      AppStats `json:"stats"`
	Index      int      `json:"index"`
	Total      int      `json:"total"`
	ActionType string   `json:"actionType"`
}

// TrashListResponse lists items in trash.
type TrashListResponse struct {
	Items []string `json:"items"`
}

// RestoreResponse describes the state after restoring files.
type RestoreResponse struct {
	Stats    AppStats `json:"stats"`
	Restored []string `json:"restored"`
	Index    int      `json:"index"`
	Total    int      `json:"total"`
}

// SysCheckResponse contains system health information.
type SysCheckResponse struct {
	Exiftool     bool                `json:"exiftool"`
	OS           string              `json:"os"`
	Arch         string              `json:"arch"`
	Capabilities RuntimeCapabilities `json:"capabilities"`
}

// RuntimeCapabilities describes effective runtime features by media operation.
type RuntimeCapabilities struct {
	RawPreview  bool `json:"rawPreview"`
	RawMetadata bool `json:"rawMetadata"`
	HeicDecode  bool `json:"heicDecode"`
	ExifWrite   bool `json:"exifWrite"`
}

// AnalysisProgressResponse contains progress data.
type AnalysisProgressResponse struct {
	Current int `json:"current"`
	Total   int `json:"total"`
}

// ProgressCounts is a compact progress payload shared across events.
type ProgressCounts struct {
	Current int `json:"current"`
	Total   int `json:"total"`
}

// StateUpdatePayload is the top-level UI refresh event payload.
type StateUpdatePayload struct {
	Stats      AppStats       `json:"stats"`
	Analysis   ProgressCounts `json:"analysis"`
	Thumbnails ProgressCounts `json:"thumbnails"`
}

// FilterValuesResponse contains metadata values for filtering.
type FilterValuesResponse struct {
	Cameras []string `json:"cameras"`
	ISOs    []string `json:"isos"`
}

// FilteredIndicesResponse contains the result of a filter.
type FilteredIndicesResponse struct {
	Indices []int `json:"indices"`
}

// PathResponse is used for dialog results.
type PathResponse struct {
	Path string `json:"path"`
}

// BrowseResponse represents a directory listing.
type BrowseResponse struct {
	Path    string   `json:"path"`
	Parent  string   `json:"parent"`
	Sep     string   `json:"sep"`
	Entries []string `json:"entries"`
}

// BookmarkResponse represents user bookmarks.
type BookmarkResponse struct {
	Bookmarks []Bookmark `json:"bookmarks"`
	Home      string     `json:"home"`
	Sep       string     `json:"sep"`
}

type Bookmark struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	Icon  string `json:"icon"`
}

// RotationResponse contains the current rotation.
type RotationResponse struct {
	Rotation int `json:"rotation"`
}
