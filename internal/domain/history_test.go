package domain

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryCRUDAndLimit(t *testing.T) {
	t.Setenv("QUICKCULL_TEST_CACHE_DIR", t.TempDir())

	var paths []string
	for i := 0; i < 12; i++ {
		p := filepath.Join(t.TempDir(), fmt.Sprintf("folder-%02d", i))
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("failed to create folder: %v", err)
		}
		paths = append(paths, p)
		if err := AddToHistory(p); err != nil {
			t.Fatalf("AddToHistory failed: %v", err)
		}
	}

	history, err := GetHistory()
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(history) != maxHistory {
		t.Fatalf("expected history size %d, got %d", maxHistory, len(history))
	}

	latestAbs, _ := filepath.Abs(paths[11])
	if history[0] != latestAbs {
		t.Fatalf("expected latest history entry %q, got %q", latestAbs, history[0])
	}

	oldestKeptAbs, _ := filepath.Abs(paths[2])
	if history[len(history)-1] != oldestKeptAbs {
		t.Fatalf("expected oldest kept history entry %q, got %q", oldestKeptAbs, history[len(history)-1])
	}

	// Re-adding an existing entry should move it to the top and keep size stable.
	readdedAbs, _ := filepath.Abs(paths[5])
	if err := AddToHistory(paths[5]); err != nil {
		t.Fatalf("AddToHistory (re-add) failed: %v", err)
	}
	history, err = GetHistory()
	if err != nil {
		t.Fatalf("GetHistory after re-add failed: %v", err)
	}
	if len(history) != maxHistory {
		t.Fatalf("expected history size %d after re-add, got %d", maxHistory, len(history))
	}
	if history[0] != readdedAbs {
		t.Fatalf("expected re-added path %q to be first, got %q", readdedAbs, history[0])
	}

	if err := RemoveFromHistory(paths[5]); err != nil {
		t.Fatalf("RemoveFromHistory failed: %v", err)
	}
	history, err = GetHistory()
	if err != nil {
		t.Fatalf("GetHistory after remove failed: %v", err)
	}
	for _, h := range history {
		if h == readdedAbs {
			t.Fatalf("expected removed path %q to be absent", readdedAbs)
		}
	}
}

func TestGetHistoryMissingFile(t *testing.T) {
	t.Setenv("QUICKCULL_TEST_CACHE_DIR", t.TempDir())

	history, err := GetHistory()
	if history != nil {
		t.Fatalf("expected nil history for missing file, got %v", history)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}
