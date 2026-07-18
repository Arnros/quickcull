package review

import (
	"path/filepath"
	"testing"

	"github.com/corona10/goimagehash"
)

func TestGetDuplicateGroupsRebuildsTreeAfterStateShrinks(t *testing.T) {
	root := t.TempDir()
	cache := NewMediaCache()
	hash := goimagehash.NewImageHash(42, goimagehash.PHash)

	firstFiles := []string{"old-a.jpg", "old-b.jpg", "old-c.jpg"}
	for _, name := range firstFiles {
		cache.hashCache[filepath.Join(root, name)] = hash
	}
	if groups := cache.GetDuplicateGroups(NewState(root, firstFiles), 100); len(groups) != 1 || len(groups[0]) != 3 {
		t.Fatalf("initial duplicate groups = %v, want [[0 1 2]]", groups)
	}

	secondFiles := []string{"new-a.jpg", "new-b.jpg"}
	for _, name := range secondFiles {
		cache.hashCache[filepath.Join(root, name)] = hash
	}
	groups := cache.GetDuplicateGroups(NewState(root, secondFiles), 100)
	if len(groups) != 1 || len(groups[0]) != 2 || groups[0][0] != 0 || groups[0][1] != 1 {
		t.Fatalf("duplicate groups after shrink = %v, want [[0 1]]", groups)
	}
}
