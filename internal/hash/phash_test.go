package hash

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDistanceToSimilarityBounds(t *testing.T) {
	if got := DistanceToSimilarity(0); got != 100.0 {
		t.Fatalf("expected 100, got %v", got)
	}
	if got := DistanceToSimilarity(64); got != 0.0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := DistanceToSimilarity(128); got != 0.0 {
		t.Fatalf("expected clamped 0, got %v", got)
	}
}

func TestSimilarityToDistanceBounds(t *testing.T) {
	if got := SimilarityToDistance(100); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := SimilarityToDistance(0); got != 64 {
		t.Fatalf("expected 64, got %d", got)
	}
	if got := SimilarityToDistance(-10); got != 64 {
		t.Fatalf("expected clamped 64, got %d", got)
	}
}

func TestComputePerceptualHash(t *testing.T) {
	t.Run("invalid path", func(t *testing.T) {
		if _, err := ComputePerceptualHash("/definitely/missing/image.png"); err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("valid png", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "img.png")
		if err := writeSolidPNG(path); err != nil {
			t.Fatalf("failed to write png: %v", err)
		}
		h, err := ComputePerceptualHash(path)
		if err != nil {
			t.Fatalf("unexpected hash error: %v", err)
		}
		if h == nil {
			t.Fatal("expected non-nil hash")
		}
	})
}

func writeSolidPNG(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 120, G: 80, B: 40, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
