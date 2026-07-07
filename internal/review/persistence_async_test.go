package review

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"quickcull/internal/bus"
	"quickcull/internal/persistence"
)

type blockingPersistence struct {
	mu            sync.Mutex
	release       chan struct{}
	photoCalls    int
	historyCalls  int
	lastPhotoMeta persistence.PhotoMetadata
	lastHistory   []byte
}

func newBlockingPersistence() *blockingPersistence {
	return &blockingPersistence{release: make(chan struct{})}
}

func (p *blockingPersistence) Close() error                                    { return nil }
func (p *blockingPersistence) ClearMetadataScope(folderID, scope string) error { return nil }
func (p *blockingPersistence) LoadFolderMetadata(folderID string) (map[string]persistence.PhotoMetadata, error) {
	return map[string]persistence.PhotoMetadata{}, nil
}
func (p *blockingPersistence) LoadHistory(folderID string) ([]byte, error) { return nil, nil }
func (p *blockingPersistence) GetFolderInfo(folderID string) (persistence.FolderInfo, bool) {
	return persistence.FolderInfo{}, false
}
func (p *blockingPersistence) SaveFolderInfo(folderID string, info persistence.FolderInfo) error {
	return nil
}
func (p *blockingPersistence) SaveFolderSnapshot(folderID string, snap persistence.FolderSnapshot) error {
	return nil
}
func (p *blockingPersistence) GetFolderSnapshot(folderID string) (persistence.FolderSnapshot, bool) {
	return persistence.FolderSnapshot{}, false
}
func (p *blockingPersistence) SaveFolderMetadata(folderID string, metadata map[string]persistence.PhotoMetadata) error {
	<-p.release
	return nil
}
func (p *blockingPersistence) SavePhotoMetadata(folderID, photoID string, meta persistence.PhotoMetadata) error {
	p.mu.Lock()
	p.photoCalls++
	p.lastPhotoMeta = meta
	p.mu.Unlock()
	<-p.release
	return nil
}
func (p *blockingPersistence) RemovePhotoMetadata(folderID, photoID string) error { return nil }
func (p *blockingPersistence) SaveHistory(folderID string, history []byte) error {
	p.mu.Lock()
	p.historyCalls++
	p.lastHistory = append([]byte(nil), history...)
	p.mu.Unlock()
	<-p.release
	return nil
}

func TestApplyEventDoesNotBlockOnPersistence(t *testing.T) {
	srv := NewServer()
	store := newBlockingPersistence()
	srv.persistence = store
	srv.appState = &AppState{
		Root: "/tmp/library",
		Photos: map[string]Photo{
			"a.jpg": {ID: "a.jpg"},
		},
		VisibleOrder: []string{"a.jpg"},
	}

	done := make(chan struct{})
	go func() {
		_, _, _ = srv.applyEvent(bus.Event{
			Type: bus.TypeCommandToggleStar,
			Payload: bus.CommandToggleStarPayload{
				PhotoID:    "a.jpg",
				Starred:    true,
				OldStarred: false,
			},
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("applyEvent blocked on slow persistence")
	}

	close(store.release)
	srv.flushPersistence()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.photoCalls == 0 {
		t.Fatal("expected photo metadata persistence to be enqueued")
	}
	if store.historyCalls == 0 {
		t.Fatal("expected history persistence to be enqueued")
	}
	if !store.lastPhotoMeta.IsStarred {
		t.Fatalf("expected persisted photo metadata to be starred, got %+v", store.lastPhotoMeta)
	}

	var history []bus.Event
	if err := json.Unmarshal(store.lastHistory, &history); err != nil {
		t.Fatalf("unmarshal persisted history: %v", err)
	}
	if len(history) != 1 || history[0].Type != bus.TypeCommandToggleStar {
		t.Fatalf("unexpected persisted history: %+v", history)
	}
}

// TestAsyncMetadataWriter_CloseFlushesPending verifies that close() flushes any
// pending writes before exiting (the P1 code path: "stopTimer(); w.flush();
// close(doneCh)"). Without this guarantee, the app would lose unflushed
// metadata at shutdown.
func TestAsyncMetadataWriter_CloseFlushesPending(t *testing.T) {
	store := newNonBlockingPersistence()
	w := newAsyncMetadataWriter(store)

	const photoCount = 10
	for i := 0; i < photoCount; i++ {
		w.enqueueSingle("folder1", "photo"+itoa(i), persistence.PhotoMetadata{IsStarred: true})
	}
	w.enqueueHistory("folder1", []byte("h1"))

	// close() must flush all pending before returning.
	w.close()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.photoCalls != photoCount {
		t.Errorf("photoCalls = %d, want %d (pending must be flushed at close)", store.photoCalls, photoCount)
	}
	if store.historyCalls != 1 {
		t.Errorf("historyCalls = %d, want 1", store.historyCalls)
	}
	if len(store.lastHistory) != 2 {
		t.Errorf("lastHistory length = %d, want 2 (\"h1\")", len(store.lastHistory))
	}
}

// TestAsyncMetadataWriter_FlushAndWaitSerializes ensures flushAndWait actually
// blocks until the loop has completed a flush, providing the synchronization
// API used by ResetAppCache and OnBeforeClose.
func TestAsyncMetadataWriter_FlushAndWaitSerializes(t *testing.T) {
	store := newNonBlockingPersistence()
	w := newAsyncMetadataWriter(store)

	w.enqueueSingle("f", "p", persistence.PhotoMetadata{IsStarred: true})

	done := make(chan struct{})
	go func() {
		w.flushAndWait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("flushAndWait did not return within 2s")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.photoCalls == 0 {
		t.Fatal("expected at least one photo call after flushAndWait")
	}
}

// TestAsyncMetadataWriter_CloseWithoutPendingIsFast ensures close() with an
// empty queue returns promptly (no deadlock waiting on an unstarted timer).
func TestAsyncMetadataWriter_CloseWithoutPendingIsFast(t *testing.T) {
	store := newNonBlockingPersistence()
	w := newAsyncMetadataWriter(store)

	done := make(chan struct{})
	go func() {
		w.close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("close() with no pending writes took > 1s")
	}
}

// newNonBlockingPersistence returns a blockingPersistence variant where Save*
// calls never block, so tests can flush deterministically without releasing.
func newNonBlockingPersistence() *blockingPersistence {
	p := newBlockingPersistence()
	p.release = make(chan struct{}, 1000)
	for i := 0; i < 1000; i++ {
		p.release <- struct{}{}
	}
	return p
}

// itoa is a tiny strconv-free helper to avoid pulling in strconv just for test
// formatting.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
