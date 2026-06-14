package review

import (
	"path/filepath"
	"sort"
	"testing"

	"quickcull/internal/domain"
	"quickcull/internal/persistence"
)

func newServerWithPersistenceForPositionTests(t *testing.T, root string, files []string) (*Server, persistence.StateStore) {
	t.Helper()

	store, err := persistence.NewMetadataStore(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("failed to create metadata store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	srv := NewServer()
	srv.persistence = store
	srv.appState = &AppState{
		Root:         root,
		Photos:       make(map[string]Photo, len(files)),
		VisibleOrder: append([]string(nil), files...),
	}
	for _, f := range files {
		srv.appState.Photos[f] = Photo{ID: f}
	}

	return srv, store
}

func expectedPrefetchIndices(index, total int) []int {
	indices := make([]int, 0, 15)
	set := make(map[int]struct{}, 15)
	for i := 1; i <= 10; i++ {
		target := index + i
		if target < total {
			set[target] = struct{}{}
		}
	}
	for i := 1; i <= 5; i++ {
		target := index - i
		if target >= 0 {
			set[target] = struct{}{}
		}
	}
	for idx := range set {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	return indices
}

func drainQueueIndices(q *AnalysisQueue) []int {
	q.Close()
	out := make([]int, 0)
	for {
		idx, _, ok := q.Pop()
		if !ok {
			break
		}
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

func TestServerSaveAndGetSavedPositionWithPersistence(t *testing.T) {
	root := t.TempDir()
	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	srv, store := newServerWithPersistenceForPositionTests(t, root, files)

	const wantPos = 2
	srv.SavePosition(wantPos)

	gotPos := srv.GetSavedPosition(root)
	if gotPos != wantPos {
		t.Fatalf("saved position mismatch: got %d, want %d", gotPos, wantPos)
	}

	folderID := domain.GetFolderID(root)
	info, found := store.GetFolderInfo(folderID)
	if !found {
		t.Fatal("expected folder info to be persisted")
	}
	if info.Path != root {
		t.Fatalf("saved folder path mismatch: got %q, want %q", info.Path, root)
	}
	if info.SavedPosition != wantPos {
		t.Fatalf("saved position in folder info mismatch: got %d, want %d", info.SavedPosition, wantPos)
	}
	if info.LastScanned <= 0 {
		t.Fatalf("expected LastScanned > 0, got %d", info.LastScanned)
	}
}

func TestServerSavePositionQueuesExpectedIndices(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"00.jpg", "01.jpg", "02.jpg", "03.jpg", "04.jpg",
		"05.jpg", "06.jpg", "07.jpg", "08.jpg", "09.jpg",
		"10.jpg", "11.jpg", "12.jpg", "13.jpg", "14.jpg",
		"15.jpg", "16.jpg", "17.jpg", "18.jpg", "19.jpg",
	}

	tests := []struct {
		name  string
		index int
	}{
		{name: "middle index", index: 4},
		{name: "near end", index: 18},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newServerWithPersistenceForPositionTests(t, root, files)

			srv.SavePosition(tt.index)

			got := drainQueueIndices(srv.analysisQueue)
			want := expectedPrefetchIndices(tt.index, len(files))
			if len(got) != len(want) {
				t.Fatalf("queue size mismatch: got %d (%v), want %d (%v)", len(got), got, len(want), want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("queued indices mismatch: got %v, want %v", got, want)
				}
			}
		})
	}
}
