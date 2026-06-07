package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshDetectsExternalChanges(t *testing.T) {
	// 1. Setup temp dir with 2 files
	tmpDir, err := os.MkdirTemp("", "quickcull-refresh-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_ = os.WriteFile(filepath.Join(tmpDir, "img1.jpg"), []byte("data"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "img2.jpg"), []byte("data"), 0644)

	srv := NewServer()
	if err := srv.LoadState(tmpDir); err != nil {
		t.Fatal(err)
	}
	app := &App{server: srv}

	// 2. Initial check
	res, err := app.Refresh(0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Errorf("Expected 2 files initially, got %d", res.Total)
	}

	// 3. Simulate EXTERNAL ADDITION
	_ = os.WriteFile(filepath.Join(tmpDir, "img3.jpg"), []byte("data"), 0644)

	// 4. Trigger Refresh
	res2 := waitForRefreshTotal(t, app, 0, 3)

	// 5. Verify detection
	if res2.Total != 3 {
		t.Errorf("Refresh failed to detect external addition. Expected 3, got %d", res2.Total)
	}

	// 6. Simulate EXTERNAL DELETION
	_ = os.Remove(filepath.Join(tmpDir, "img1.jpg"))

	// 7. Trigger Refresh
	res3 := waitForRefreshTotal(t, app, 0, 2)

	if res3.Total != 2 {
		t.Errorf("Refresh failed to detect external deletion. Expected 2, got %d", res3.Total)
	}
}

func waitForRefreshTotal(t *testing.T, app *App, index int, expected int) ActionResponse {
	t.Helper()
	for i := 0; i < 10; i++ {
		res, err := app.Refresh(index)
		if err != nil {
			t.Fatal(err)
		}
		if res.Total == expected {
			return res
		}
	}
	t.Fatalf("refresh did not converge to expected total %d", expected)
	return ActionResponse{}
}
