package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetFilteredIndicesCharacterization(t *testing.T) {
	root := t.TempDir()
	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	sizes := []int{1, 2, 3}
	for i, name := range files {
		if err := os.WriteFile(filepath.Join(root, name), make([]byte, sizes[i]*megabyte), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	state := NewState(root, files)
	cache := NewMediaCache()
	cache.exifCache[filepath.Join(root, "a.jpg")] = &EXIFInfo{Camera: "Canon", ISO: "100", Date: "2023-01-10 12:00:00"}
	cache.exifCache[filepath.Join(root, "b.jpg")] = &EXIFInfo{Camera: "Canon", ISO: "800", Date: "2023-06-15 12:00:00"}
	cache.exifCache[filepath.Join(root, "c.jpg")] = &EXIFInfo{Camera: "Sony", ISO: "800", Date: "2024-01-01 12:00:00"}

	tests := []struct {
		name    string
		filters map[string]string
		want    []int
	}{
		{name: "empty", filters: map[string]string{}, want: nil},
		{name: "camera and ISO", filters: map[string]string{"camera": "Canon", "iso": "800"}, want: []int{1}},
		{name: "inclusive date range", filters: map[string]string{"dateFrom": "2023-01-10", "dateTo": "2023-06-15"}, want: []int{0, 1}},
		{name: "inclusive size range", filters: map[string]string{"sizeMin": "2", "sizeMax": "3"}, want: []int{1, 2}},
		{name: "combined metadata and size", filters: map[string]string{"camera": "Canon", "sizeMin": "1.5", "dateFrom": "2023-02-01"}, want: []int{1}},
		{name: "invalid sizes are ignored", filters: map[string]string{"sizeMin": "bad", "sizeMax": "-1"}, want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cache.GetFilteredIndices(state, tc.filters)
			if len(got) != len(tc.want) {
				t.Fatalf("indices = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("indices = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
