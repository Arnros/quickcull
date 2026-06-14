package persistence

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func newTestStore(t *testing.T) StateStore {
	t.Helper()
	store, err := NewMetadataStore(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("failed to create metadata store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestSaveLoadHistoryRoundTripAndMissingFolder(t *testing.T) {
	store := newTestStore(t)

	folderID := "folder-history"
	history := []byte(`["toggle-star","set-label"]`)
	if err := store.SaveHistory(folderID, history); err != nil {
		t.Fatalf("SaveHistory failed: %v", err)
	}

	got, err := store.LoadHistory(folderID)
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if !reflect.DeepEqual(got, history) {
		t.Fatalf("history mismatch: got %q, want %q", got, history)
	}

	missing, err := store.LoadHistory("missing-folder")
	if err != nil {
		t.Fatalf("LoadHistory for missing folder returned error: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil history for missing folder, got %q", missing)
	}
}

func TestSaveGetFolderInfoRoundTripAndMissing(t *testing.T) {
	store := newTestStore(t)

	folderID := "folder-info"
	want := FolderInfo{
		Path:          "/tmp/photos/session-a",
		SavedPosition: 42,
		LastScanned:   1712345678,
	}

	if err := store.SaveFolderInfo(folderID, want); err != nil {
		t.Fatalf("SaveFolderInfo failed: %v", err)
	}

	got, found := store.GetFolderInfo(folderID)
	if !found {
		t.Fatal("expected folder info to be found")
	}
	if got != want {
		t.Fatalf("folder info mismatch: got %+v, want %+v", got, want)
	}

	missing, found := store.GetFolderInfo("missing-folder")
	if found {
		t.Fatalf("expected missing folder info to not be found, got %+v", missing)
	}
	if missing != (FolderInfo{}) {
		t.Fatalf("expected zero-value folder info for missing folder, got %+v", missing)
	}
}

func TestClearMetadataScope(t *testing.T) {
	seed := map[string]PhotoMetadata{
		"a.jpg": {IsStarred: true, Label: 5, Rotation: 90, IsTrashed: false},
		"b.jpg": {IsStarred: false, Label: 2, Rotation: 180, IsTrashed: true},
		"c.jpg": {IsStarred: true, Label: 0, Rotation: 270, IsTrashed: false},
		"d.jpg": {IsStarred: false, Label: 0, Rotation: 0, IsTrashed: false},
	}

	tests := []struct {
		name  string
		scope string
		want  map[string]PhotoMetadata
	}{
		{
			name:  "stars",
			scope: "stars",
			want: map[string]PhotoMetadata{
				"a.jpg": {IsStarred: false, Label: 5, Rotation: 90, IsTrashed: false},
				"b.jpg": {IsStarred: false, Label: 2, Rotation: 180, IsTrashed: true},
				"c.jpg": {IsStarred: false, Label: 0, Rotation: 270, IsTrashed: false},
				"d.jpg": {IsStarred: false, Label: 0, Rotation: 0, IsTrashed: false},
			},
		},
		{
			name:  "labels",
			scope: "labels",
			want: map[string]PhotoMetadata{
				"a.jpg": {IsStarred: true, Label: 0, Rotation: 90, IsTrashed: false},
				"b.jpg": {IsStarred: false, Label: 0, Rotation: 180, IsTrashed: true},
				"c.jpg": {IsStarred: true, Label: 0, Rotation: 270, IsTrashed: false},
				"d.jpg": {IsStarred: false, Label: 0, Rotation: 0, IsTrashed: false},
			},
		},
		{
			name:  "all",
			scope: "all",
			want: map[string]PhotoMetadata{
				"a.jpg": {IsStarred: false, Label: 0, Rotation: 90, IsTrashed: false},
				"b.jpg": {IsStarred: false, Label: 0, Rotation: 180, IsTrashed: true},
				"c.jpg": {IsStarred: false, Label: 0, Rotation: 270, IsTrashed: false},
				"d.jpg": {IsStarred: false, Label: 0, Rotation: 0, IsTrashed: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			folderID := "folder-clear-" + tt.scope
			if err := store.SaveFolderMetadata(folderID, seed); err != nil {
				t.Fatalf("SaveFolderMetadata failed: %v", err)
			}

			if err := store.ClearMetadataScope(folderID, tt.scope); err != nil {
				t.Fatalf("ClearMetadataScope(%q) failed: %v", tt.scope, err)
			}

			got, err := store.LoadFolderMetadata(folderID)
			if err != nil {
				t.Fatalf("LoadFolderMetadata failed: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				keys := make([]string, 0, len(got))
				for k := range got {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				t.Fatalf("metadata mismatch for scope %q on keys %v: got %+v, want %+v", tt.scope, keys, got, tt.want)
			}
		})
	}
}

func TestSaveGetFolderSnapshotRoundTripAndMissing(t *testing.T) {
	store := newTestStore(t)

	folderID := "folder-snapshot"
	want := FolderSnapshot{
		Version:      1,
		RootPath:     "E:/media",
		SavedAt:      1712345678,
		Signature:    "sig-abc",
		VisibleOrder: []string{"a.jpg", "sub/b.jpg", "c.raw"},
	}
	if err := store.SaveFolderSnapshot(folderID, want); err != nil {
		t.Fatalf("SaveFolderSnapshot failed: %v", err)
	}

	got, found := store.GetFolderSnapshot(folderID)
	if !found {
		t.Fatal("expected snapshot to be found")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot mismatch: got %+v, want %+v", got, want)
	}

	missing, found := store.GetFolderSnapshot("missing-folder")
	if found {
		t.Fatalf("expected missing snapshot to be not found, got %+v", missing)
	}
	if !reflect.DeepEqual(missing, FolderSnapshot{}) {
		t.Fatalf("expected zero-value snapshot for missing folder, got %+v", missing)
	}
}
