package review

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"quickcull/internal/persistence"
)

// burstResult contains burst info for a file
type burstResult struct {
	count int
	index int
}

// Server is the core logic provider.
//
// Lock ordering (always acquire in this order to prevent deadlocks):
//
//	stateMu → appStateMu → progressMu → perfMu → exportMu → duplicateGroupsMu
//
// Never acquire a higher-ranked lock while already holding a lower-ranked one.
type Server struct {
	stateMu       sync.RWMutex
	state         *State
	cacheDir      string
	cache         *MediaCache
	burstCache    sync.Map // int → *burstResult (nil if no burst)
	ctx           context.Context
	analysisSched *analysisScheduler
	analysisQueue *AnalysisQueue
	searchCancel  context.CancelFunc
	searchMu      sync.Mutex
	progressMu    sync.RWMutex
	progressCur   int
	progressTotal int
	thumbCur      int
	thumbTotal    int

	// v2 Event Sourcing State
	appStateMu sync.RWMutex
	appState   *AppState

	txID           atomic.Uint64
	perfMu         sync.RWMutex
	lastIOWorkers  int
	hashDeferred   bool
	cacheMetaGC    int
	cacheHashGC    int
	cacheDerivedGC int

	Bus *bus.Bus

	persistMu      sync.Mutex
	persistAsync   *asyncMetadataWriter
	persistence    persistence.StateStore
	lastUIActivity atomic.Int64 // UnixNano timestamp of last thumbnail request

	exportCancel context.CancelFunc
	exportMu     sync.Mutex

	duplicateGroupsMu sync.Mutex
	duplicateGroups   [][]int
	// duplicateCheckRunning coalesces expensive BK-tree rebuilds triggered by
	// concurrent analysis workers.
	duplicateCheckRunning atomic.Bool
	lastDupEmit       atomic.Int64 // UnixNano, throttles "duplicates:found" broadcasts

	navPromotionTotal  atomic.Uint64
	lastViewReadyMs    atomic.Int64
	navPromotedIndices sync.Map // index(int) -> unix nanos (int64)
	scheduleSlot       atomic.Uint64
	viewReadyMu        sync.Mutex
	viewReadySamples   [viewReadySampleWindow]int64
	viewReadyCount     int
	viewReadyWrite     int
	schedulerModeMu    sync.Mutex
	schedulerModeName  schedulerModeType
	schedulerModeSince int64
	schedulerActiveMs  int64
	schedulerIdleMs    int64
	lastNavAtNs        int64
	navVelocityEWMA    float64

	savedPositionCacheMu sync.RWMutex
	savedPositionCache   map[string]int

	onBroadcastMu sync.RWMutex
	onBroadcast   func(name string, data any)
}

// NextTxID returns an incremented transaction ID.
func (s *Server) NextTxID() uint64 {
	return s.txID.Add(1)
}

// InitPersistence initializes the metadata database.
func (s *Server) InitPersistence() error {
	dbPath := filepath.Join(domain.GetAppCacheDir(), "metadata.db")
	var store persistence.StateStore
	var err error
	if dbPath != "" {
		store, err = persistence.NewMetadataStore(dbPath)
	}
	if err != nil {
		return err
	}
	s.persistence = store
	slog.Info("Persistence initialized", "path", dbPath)
	return nil
}

// NewServer creates a new server instance.
func NewServer() *Server {
	q := NewAnalysisQueue()
	return &Server{
		cache:              NewMediaCache(),
		analysisSched:      newAnalysisScheduler(q.WakeAndCancel, q.Reset),
		analysisQueue:      q,
		Bus:                bus.New(),
		savedPositionCache: make(map[string]int),
	}
}

func (s *Server) SetBroadcastHook(fn func(name string, data any)) {
	s.onBroadcastMu.Lock()
	s.onBroadcast = fn
	s.onBroadcastMu.Unlock()
}

// LoadState initializes the state for a given path.
func (s *Server) LoadState(root string) error {
	// Cache and state resources are replaced during bootstrap. Ensure workers
	// from the previous lifecycle have stopped reading them first.
	s.analysisSched.Cancel()
	s.analysisSched.Wait()
	s.analysisSched.BeginLoadLifecycle()

	pipeline, err := s.loadStateBootstrap(root)
	if err != nil {
		return err
	}

	finalCount, err := s.loadStateIngest(pipeline)
	if err != nil {
		return &ScanError{
			Operation: scanOpLoadState,
			Root:      root,
			Err:       err,
		}
	}

	if err := s.loadStateFinalize(pipeline, finalCount); err != nil {
		return err
	}

	if finalCount == 0 {
		return domain.ErrNoMediaFiles
	}

	// Discovery pushes analysis tasks using pre-sort indices.
	// Final sort changes index->path mapping, so clear any burst entries
	// that may have been populated before order stabilization.
	s.invalidateBurstCache()
	return nil
}

func (s *Server) getState() *State {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

// SavePosition saves the current navigation index for the current folder.
func (s *Server) SavePosition(index int) {
	s.appStateMu.RLock()
	if s.persistence == nil || s.appState == nil {
		s.appStateMu.RUnlock()
		return
	}
	root := s.appState.Root
	total := len(s.appState.VisibleOrder)
	s.appStateMu.RUnlock()

	folderID := domain.GetFolderID(root)

	// Update memory cache
	s.savedPositionCacheMu.Lock()
	s.savedPositionCache[folderID] = index
	s.savedPositionCacheMu.Unlock()

	info, _ := s.persistence.GetFolderInfo(folderID)
	info.Path = root
	info.SavedPosition = index
	info.LastScanned = time.Now().Unix()
	if err := s.persistence.SaveFolderInfo(folderID, info); err != nil {
		slog.Warn("SavePosition: failed to persist folder info", "folder", folderID, "error", err)
	}

	// INTELLIGENT PREFETCHING:
	// Prioritize analysis for photos near the current position.
	prefetchRange := 10
	backRange := 5

	for i := 1; i <= prefetchRange; i++ {
		target := index + i
		if target < total {
			s.analysisQueue.Push(target, analysisStartupPriority)
		}
	}
	for i := 1; i <= backRange; i++ {
		target := index - i
		if target >= 0 {
			s.analysisQueue.Push(target, analysisStartupPriority)
		}
	}
}

// GetSavedPosition returns the last saved position for a given folder.
func (s *Server) GetSavedPosition(root string) int {
	folderID := domain.GetFolderID(root)

	// 1. Check Memory Cache (Read Lock)
	s.savedPositionCacheMu.RLock()
	pos, ok := s.savedPositionCache[folderID]
	s.savedPositionCacheMu.RUnlock()
	if ok {
		return pos
	}

	// 2. Fallback to Persistence
	if s.persistence == nil {
		return 0
	}
	info, found := s.persistence.GetFolderInfo(folderID)
	if !found {
		return 0
	}

	// Update cache (Write Lock)
	s.savedPositionCacheMu.Lock()
	s.savedPositionCache[folderID] = info.SavedPosition
	s.savedPositionCacheMu.Unlock()

	return info.SavedPosition
}

// InvalidateSavedPositionCache clears the in-memory folder position cache.
func (s *Server) InvalidateSavedPositionCache() {
	s.savedPositionCacheMu.Lock()
	s.savedPositionCache = make(map[string]int)
	s.savedPositionCacheMu.Unlock()
}

// PrioritizeIndices pushes a batch of indices to the front of the analysis queue.
func (s *Server) PrioritizeIndices(indices []int) {
	state := s.getState()
	if state == nil {
		return
	}
	total := state.Len()
	for _, idx := range indices {
		if idx < 0 || idx >= total {
			continue
		}
		s.analysisQueue.Push(idx, analysisStartupPriority)
	}
}

// SearchStream performs a case-insensitive filename search and streams results in batches.
func (s *Server) SearchStream(ctx context.Context, query string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("SearchStream panic", "query", query, "error", r)
		}
	}()

	state := s.getState()
	if state == nil {
		return
	}

	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		s.broadcast("search:complete", map[string]any{"indices": []int{}})
		return
	}

	indices := []int{}
	batchSize := 50

	s.stateMu.RLock()
	filesLower := state.filesLower
	s.stateMu.RUnlock()

	for i, f := range filesLower {
		if ctx.Err() != nil {
			return
		}
		if strings.Contains(f, query) {
			indices = append(indices, i)
		}

		if len(indices) >= batchSize {
			s.broadcast("search:results", map[string]any{
				"indices": indices,
				"query":   query,
				"append":  true,
			})
			indices = []int{}
			// Small sleep to avoid saturating the bridge if there are too many results
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}

	// Final batch
	s.broadcast("search:complete", map[string]any{
		"indices": indices,
		"query":   query,
	})
}
