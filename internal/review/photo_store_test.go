package review

import "testing"

func TestPhotoStoreWithChangesPreservesParent(t *testing.T) {
	base := newPhotoStore(map[string]Photo{"a": {ID: "a"}, "b": {ID: "b"}})
	next := base.WithChanges(map[string]Photo{"a": {ID: "a", IsStarred: true}})
	before, _ := base.Get("a")
	after, _ := next.Get("a")
	if before.IsStarred || !after.IsStarred || next.Len() != 2 {
		t.Fatalf("parent mutated or child incorrect: before=%+v after=%+v", before, after)
	}
}

func TestPhotoStoreMaterializeReturnsDetachedMap(t *testing.T) {
	store := newPhotoStore(map[string]Photo{"a": {ID: "a"}})
	materialized := store.Materialize()
	materialized["a"] = Photo{ID: "a", IsStarred: true}
	got, _ := store.Get("a")
	if got.IsStarred {
		t.Fatal("materialized map mutated immutable store")
	}
}

func TestPhotoStoreCompactsAtMaximumDepth(t *testing.T) {
	store := newPhotoStore(map[string]Photo{"a": {ID: "a"}})
	for rotation := 1; rotation <= photoStoreMaxDepth; rotation++ {
		store = store.WithChanges(map[string]Photo{"a": {ID: "a", Rotation: rotation}})
	}

	if store.depth != 0 {
		t.Fatalf("depth after compaction = %d, want 0", store.depth)
	}
	got, ok := store.Get("a")
	if !ok || got.Rotation != photoStoreMaxDepth {
		t.Fatalf("newest value not retained: got=%+v ok=%v", got, ok)
	}
}

func TestPhotoStoreRangeReturnsEachIDOnceWithNewestValue(t *testing.T) {
	store := newPhotoStore(map[string]Photo{
		"a": {ID: "a"},
		"b": {ID: "b"},
	}).WithChanges(map[string]Photo{
		"a": {ID: "a", IsStarred: true},
	}).WithChanges(map[string]Photo{
		"a": {ID: "a", Rotation: 90},
	})

	visited := make(map[string]Photo)
	store.Range(func(id string, photo Photo) bool {
		if _, exists := visited[id]; exists {
			t.Fatalf("Range visited %q more than once", id)
		}
		visited[id] = photo
		return true
	})

	if len(visited) != 2 || visited["a"].Rotation != 90 {
		t.Fatalf("unexpected Range result: %+v", visited)
	}
}
