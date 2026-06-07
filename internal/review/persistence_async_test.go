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
