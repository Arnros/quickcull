package review

import (
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"quickcull/internal/bus"
)

// TestHTTPIntegration validates the exact handler configuration used in main.go:
// http.StripPrefix("/raw-media", http.HandlerFunc(srv.ServeMedia))
// This ensures that the frontend and backend agree on the routing contract.
func TestHTTPIntegration(t *testing.T) {
	// 1. Setup a dummy project structure
	tmpDir, err := os.MkdirTemp("", "quickcull-http-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	imgPath := filepath.Join(tmpDir, "test.jpg")
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	// Create a valid 1x1 JPEG
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if err := jpeg.Encode(f, img, nil); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// 2. Setup the server
	srv := NewServer()
	if err := srv.LoadState(tmpDir); err != nil {
		t.Fatal(err)
	}

	// 3. Setup the handler chain exactly as in main.go
	handler := http.StripPrefix("/raw-media", http.HandlerFunc(srv.ServeMedia))

	// 4. Test various integration scenarios
	tests := []struct {
		name         string
		url          string
		expectedCode int
	}{
		{"Valid Full Media", "/raw-media/full/0", http.StatusOK},
		{"Valid Thumbnail", "/raw-media/thumb/0", http.StatusOK},
		{"Wrong Prefix", "/media/full/0", http.StatusNotFound},
		{"Missing Index", "/raw-media/full/", http.StatusBadRequest},
		{"Invalid Index", "/raw-media/full/99", http.StatusNotFound},
		{"Internal Path Direct Access", "/full/0", http.StatusNotFound}, // Should fail because StripPrefix expects /raw-media
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("%s: expected code %d, got %d", tt.name, tt.expectedCode, w.Code)
			}
		})
	}
}

func TestThumbnailAfterUndoTrashDoesNotReturnPlaceholder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	tmpDir, err := os.MkdirTemp("", "quickcull-http-undo-thumb-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	imgPath := filepath.Join(tmpDir, "test.jpg")
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	if err := jpeg.Encode(f, img, nil); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	srv := NewServer()
	if err := srv.LoadState(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Reproduce production semantics explicitly:
	// physical trash + immutable history event, then Undo.
	st := srv.getState()
	if st == nil {
		t.Fatal("state not loaded")
	}
	idx := st.FindIndex("test.jpg")
	if idx < 0 {
		t.Fatal("test.jpg not found in state")
	}
	if _, err := st.Trash(idx); err != nil {
		t.Fatalf("physical trash failed: %v", err)
	}
	if _, _, err := srv.applyEvent(bus.Event{
		Type:    bus.TypeCommandTrashPhoto,
		Payload: bus.CommandTrashPhotoPayload{PhotoID: "test.jpg"},
	}); err != nil {
		t.Fatalf("event trash failed: %v", err)
	}

	app := NewApp(srv)
	if _, err := app.Undo(); err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "test.jpg")); err != nil {
		t.Fatalf("restored source missing after undo: %v", err)
	}

	handler := http.StripPrefix("/raw-media", http.HandlerFunc(srv.ServeMedia))
	req := httptest.NewRequest(http.MethodGet, "/raw-media/thumb/0", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}

	decoded, _, err := image.Decode(w.Body)
	if err != nil {
		t.Fatalf("response is not a decodable image: %v", err)
	}
	b := decoded.Bounds()
	// Error placeholder is a fixed 300x300 image.
	if b.Dx() == 300 && b.Dy() == 300 {
		t.Fatalf("expected real thumbnail after undo, got error placeholder size %dx%d", b.Dx(), b.Dy())
	}
}
