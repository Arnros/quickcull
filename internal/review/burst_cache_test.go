package review

import (
	"path/filepath"
	"testing"

	"quickcull/internal/domain"
)

func TestBurstCache_RemainsCoherentAfterSort(t *testing.T) {
	root := t.TempDir()
	writeTinyJPEG(t, filepath.Join(root, "m.jpg"))
	writeTinyJPEG(t, filepath.Join(root, "n.jpg"))
	writeTinyJPEG(t, filepath.Join(root, "a.jpg"))

	srv := NewServer()
	pauseScan := make(chan struct{})
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		filesChan <- "m.jpg"
		filesChan <- "n.jpg"
		<-pauseScan
		filesChan <- "a.jpg"
		return nil
	})

	loadErrCh := make(chan error, 1)
	go func() {
		loadErrCh <- srv.LoadState(root)
	}()

	waitForCondition(t, "expected first discovered files to be ingested", func() bool {
		state := srv.getState()
		return state != nil && state.Len() >= 2
	})

	srv.burstCache.Store(0, &burstResult{count: 99, index: 99})
	close(pauseScan)

	if err := <-loadErrCh; err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	state := srv.getState()
	if state == nil {
		t.Fatal("expected state after LoadState")
	}
	absPath, err := state.AbsPath(0)
	if err != nil {
		t.Fatalf("AbsPath(0) failed: %v", err)
	}
	if got := filepath.Base(absPath); got != "a.jpg" {
		t.Fatalf("expected sorted index 0 to be a.jpg, got %s", got)
	}

	if br := srv.getBurstInfo(0, absPath); br != nil {
		t.Fatalf("expected no stale burst cache at sorted index 0, got %+v", *br)
	}
}

func TestGetBurstInfoDetectsRealBurst(t *testing.T) {
	root := t.TempDir()
	writeTinyJPEG(t, filepath.Join(root, "a.jpg"))
	writeTinyJPEG(t, filepath.Join(root, "b.jpg"))
	writeTinyJPEG(t, filepath.Join(root, "c.jpg"))

	srv := NewServer()
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		filesChan <- "a.jpg"
		filesChan <- "b.jpg"
		filesChan <- "c.jpg"
		return nil
	})

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	state := srv.getState()
	if state == nil {
		t.Fatal("expected state after LoadState")
	}
	if state.Len() != 3 {
		t.Fatalf("expected 3 files, got %d", state.Len())
	}

	// Configure burst window.
	cfg := domain.GetConfig()
	cfg.BurstSeconds = 5
	if err := domain.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	// Resolve absolute paths after sort (a < b < c alphabetically).
	absPath0, _ := state.AbsPath(0) // a.jpg
	absPath1, _ := state.AbsPath(1) // b.jpg
	absPath2, _ := state.AbsPath(2) // c.jpg

	// Inject metadata directly into the in-memory cache.
	srv.cache.mu.Lock()
	srv.cache.exifCache[absPath0] = &EXIFInfo{Date: "2023:01:15 10:00:00"}
	srv.cache.exifCache[absPath1] = &EXIFInfo{Date: "2023:01:15 10:00:01"}
	srv.cache.exifCache[absPath2] = &EXIFInfo{Date: "2023:01:15 10:00:02"}
	srv.cache.mu.Unlock()

	// b.jpg is in the middle: 1 photo before (a.jpg) + 1 photo after (c.jpg).
	br := srv.getBurstInfo(1, absPath1)
	if br == nil {
		t.Fatal("expected burst result for middle photo, got nil")
	}
	if br.count != 3 {
		t.Errorf("expected burst count 3, got %d", br.count)
	}
	if br.index != 2 {
		t.Errorf("expected burst index 2, got %d", br.index)
	}
}

func TestGetBurstInfoIgnoresNegativeDateDiff(t *testing.T) {
	root := t.TempDir()
	writeTinyJPEG(t, filepath.Join(root, "a.jpg"))
	writeTinyJPEG(t, filepath.Join(root, "b.jpg"))

	srv := NewServer()
	setScanFilesForTest(t, func(_ string, filesChan chan<- string) error {
		defer close(filesChan)
		filesChan <- "a.jpg"
		filesChan <- "b.jpg"
		return nil
	})

	if err := srv.LoadState(root); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	state := srv.getState()
	if state == nil {
		t.Fatal("expected state after LoadState")
	}
	if state.Len() != 2 {
		t.Fatalf("expected 2 files, got %d", state.Len())
	}

	// After sort: index 0 = a.jpg (2017), index 1 = b.jpg (2015).
	absPath0, _ := state.AbsPath(0) // a.jpg
	absPath1, _ := state.AbsPath(1) // b.jpg

	// Inject metadata: a.jpg is newer than b.jpg → diff is negative when
	// looking forward from index 0, and negative when looking backward from index 1.
	srv.cache.mu.Lock()
	srv.cache.exifCache[absPath0] = &EXIFInfo{Date: "2017:12:16 12:29:31"}
	srv.cache.exifCache[absPath1] = &EXIFInfo{Date: "2015:10:15 16:43:09"}
	srv.cache.mu.Unlock()

	if br := srv.getBurstInfo(0, absPath0); br != nil {
		t.Errorf("expected nil burst for index 0 (no valid forward neighbour), got %+v", *br)
	}
	if br := srv.getBurstInfo(1, absPath1); br != nil {
		t.Errorf("expected nil burst for index 1 (negative diff with previous), got %+v", *br)
	}
}
