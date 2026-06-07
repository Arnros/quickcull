package review

import (
	"log/slog"
	"sync"
	"time"

	"quickcull/internal/persistence"
	"quickcull/internal/utils"
)

const persistenceDebounce = 75 * time.Millisecond

type asyncMetadataWriter struct {
	store metadataPersistence

	mu             sync.Mutex
	pendingSingle  map[string]map[string]persistence.PhotoMetadata
	pendingFull    map[string]map[string]persistence.PhotoMetadata
	pendingHistory map[string][]byte

	signalCh chan struct{}
	flushCh  chan chan struct{}
	closeCh  chan struct{}
	doneCh   chan struct{}
}

func newAsyncMetadataWriter(store metadataPersistence) *asyncMetadataWriter {
	w := &asyncMetadataWriter{
		store:          store,
		pendingSingle:  make(map[string]map[string]persistence.PhotoMetadata),
		pendingFull:    make(map[string]map[string]persistence.PhotoMetadata),
		pendingHistory: make(map[string][]byte),
		signalCh:       make(chan struct{}, 1),
		flushCh:        make(chan chan struct{}),
		closeCh:        make(chan struct{}),
		doneCh:         make(chan struct{}),
	}
	utils.SafeGo(func() {
		w.loop()
	})
	return w
}

func (w *asyncMetadataWriter) enqueueSingle(folderID, photoID string, meta persistence.PhotoMetadata) {
	w.mu.Lock()
	if full, ok := w.pendingFull[folderID]; ok {
		full[photoID] = meta
	} else {
		if w.pendingSingle[folderID] == nil {
			w.pendingSingle[folderID] = make(map[string]persistence.PhotoMetadata)
		}
		w.pendingSingle[folderID][photoID] = meta
	}
	w.mu.Unlock()
	w.signal()
}

func (w *asyncMetadataWriter) enqueueFullFolder(folderID string, metadata map[string]persistence.PhotoMetadata) {
	w.mu.Lock()
	w.pendingFull[folderID] = clonePhotoMetadataMap(metadata)
	delete(w.pendingSingle, folderID)
	w.mu.Unlock()
	w.signal()
}

func (w *asyncMetadataWriter) enqueueHistory(folderID string, history []byte) {
	w.mu.Lock()
	w.pendingHistory[folderID] = append([]byte(nil), history...)
	w.mu.Unlock()
	w.signal()
}

func (w *asyncMetadataWriter) signal() {
	select {
	case w.signalCh <- struct{}{}:
	default:
	}
}

func (w *asyncMetadataWriter) flush() {
	w.mu.Lock()
	pendingSingle := w.pendingSingle
	pendingFull := w.pendingFull
	pendingHistory := w.pendingHistory
	w.pendingSingle = make(map[string]map[string]persistence.PhotoMetadata)
	w.pendingFull = make(map[string]map[string]persistence.PhotoMetadata)
	w.pendingHistory = make(map[string][]byte)
	w.mu.Unlock()

	var failedSingle = make(map[string]map[string]persistence.PhotoMetadata)
	var failedFull = make(map[string]map[string]persistence.PhotoMetadata)
	var failedHistory = make(map[string][]byte)

	for folderID, metadata := range pendingFull {
		if err := w.store.SaveFolderMetadata(folderID, metadata); err != nil {
			slog.Error("persistence_async: failed to save folder metadata", "folder", folderID, "error", err)
			failedFull[folderID] = metadata
		}
	}
	for folderID, photos := range pendingSingle {
		for photoID, meta := range photos {
			if err := w.store.SavePhotoMetadata(folderID, photoID, meta); err != nil {
				slog.Error("persistence_async: failed to save photo metadata", "folder", folderID, "photo", photoID, "error", err)
				if _, ok := failedSingle[folderID]; !ok {
					failedSingle[folderID] = make(map[string]persistence.PhotoMetadata)
				}
				failedSingle[folderID][photoID] = meta
			}
		}
	}
	for folderID, history := range pendingHistory {
		if err := w.store.SaveHistory(folderID, history); err != nil {
			slog.Error("persistence_async: failed to save history", "folder", folderID, "error", err)
			failedHistory[folderID] = history
		}
	}

	// Requeue failed writes if they haven't been superseded
	if len(failedSingle) > 0 || len(failedFull) > 0 || len(failedHistory) > 0 {
		w.mu.Lock()
		for folderID, metadata := range failedFull {
			if _, exists := w.pendingFull[folderID]; !exists {
				w.pendingFull[folderID] = metadata
			}
		}
		for folderID, photos := range failedSingle {
			if _, exists := w.pendingSingle[folderID]; !exists {
				w.pendingSingle[folderID] = make(map[string]persistence.PhotoMetadata)
			}
			for photoID, meta := range photos {
				if _, exists := w.pendingSingle[folderID][photoID]; !exists {
					w.pendingSingle[folderID][photoID] = meta
				}
			}
		}
		for folderID, history := range failedHistory {
			if _, exists := w.pendingHistory[folderID]; !exists {
				w.pendingHistory[folderID] = history
			}
		}
		w.mu.Unlock()
	}
}

func (w *asyncMetadataWriter) loop() {
	var (
		timer   *time.Timer
		timerCh <-chan time.Time
	)

	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerCh = nil
	}

	for {
		select {
		case <-w.signalCh:
			if timer == nil {
				timer = time.NewTimer(persistenceDebounce)
				timerCh = timer.C
				continue
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(persistenceDebounce)
		case <-timerCh:
			stopTimer()
			w.flush()
		case ack := <-w.flushCh:
			stopTimer()
			w.flush()
			close(ack)
		case <-w.closeCh:
			stopTimer()
			w.flush()
			close(w.doneCh)
			return
		}
	}
}

func (w *asyncMetadataWriter) flushAndWait() {
	ack := make(chan struct{})
	select {
	case w.flushCh <- ack:
		<-ack
	case <-w.closeCh:
		// Loop already stopped; any pending data was flushed in close().
	}
}

func (w *asyncMetadataWriter) close() {
	close(w.closeCh)
	<-w.doneCh
}

func clonePhotoMetadataMap(src map[string]persistence.PhotoMetadata) map[string]persistence.PhotoMetadata {
	dst := make(map[string]persistence.PhotoMetadata, len(src))
	for id, meta := range src {
		dst[id] = meta
	}
	return dst
}

func (s *Server) asyncPersistenceWriter() *asyncMetadataWriter {
	if s.persistence == nil {
		return nil
	}

	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	if s.persistAsync != nil && s.persistAsync.store == s.persistence {
		return s.persistAsync
	}
	if s.persistAsync != nil {
		s.persistAsync.close()
	}
	s.persistAsync = newAsyncMetadataWriter(s.persistence)
	return s.persistAsync
}

func (s *Server) flushPersistence() {
	s.persistMu.Lock()
	writer := s.persistAsync
	s.persistMu.Unlock()
	if writer != nil {
		writer.flushAndWait()
	}
}

func (s *Server) closePersistence() {
	s.persistMu.Lock()
	writer := s.persistAsync
	s.persistAsync = nil
	store := s.persistence
	s.persistence = nil
	s.persistMu.Unlock()

	if writer != nil {
		writer.close()
	}
	if store != nil {
		_ = store.Close()
	}
}
