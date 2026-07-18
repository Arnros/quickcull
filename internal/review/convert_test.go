package review

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/tiff"
)

func TestConvertRAW_AcceptsSmallValidPreviewFromExiftool(t *testing.T) {
	tmp := t.TempDir()
	srcPath := filepath.Join(tmp, "photo.dng")
	cacheDir := filepath.Join(tmp, "cache")

	preview := validJPEGBytes(t)
	if err := os.WriteFile(srcPath, []byte("raw"), 0o600); err != nil {
		t.Fatalf("write raw source: %v", err)
	}

	prevAvailable := isExiftoolAvailable
	prevExtract := extractThumbnail
	t.Cleanup(func() {
		isExiftoolAvailable = prevAvailable
		extractThumbnail = prevExtract
	})
	isExiftoolAvailable = func() bool { return true }
	extractThumbnail = func(_ string, dst string) error {
		return os.WriteFile(dst, preview, 0o600)
	}

	fp := CalculateFingerprint(srcPath)
	if fp == "" {
		t.Fatal("empty fingerprint")
	}

	got, err := ConvertRAW(srcPath, fp, cacheDir)
	if err != nil {
		t.Fatalf("ConvertRAW failed with a valid small preview: %v", err)
	}
	if got == "" {
		t.Fatal("ConvertRAW returned empty path")
	}
	if _, statErr := os.Stat(got); statErr != nil {
		t.Fatalf("converted file missing: %v", statErr)
	}
}

func TestResolveProcessedPath_FallbackFingerprintWhenMetadataMissingFingerprint(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "photos")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	srcPath := filepath.Join(root, "legacy.dng")
	if err := os.WriteFile(srcPath, []byte("raw"), 0o600); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	srv := NewServer()
	srv.cacheDir = filepath.Join(tmp, "cache")
	srv.cache = NewMediaCache()

	// Simulate legacy cache entry: metadata exists but fingerprint is empty.
	srv.cache.mu.Lock()
	srv.cache.exifCache[srcPath] = &EXIFInfo{
		Fingerprint: "",
		Camera:      "Legacy Camera",
	}
	srv.cache.mu.Unlock()

	prevAvailable := isExiftoolAvailable
	prevExtract := extractThumbnail
	t.Cleanup(func() {
		isExiftoolAvailable = prevAvailable
		extractThumbnail = prevExtract
	})
	isExiftoolAvailable = func() bool { return true }
	extractThumbnail = func(_ string, dst string) error {
		return os.WriteFile(dst, validJPEGBytes(t), 0o600)
	}

	got, err := srv.ResolveProcessedPath(srcPath)
	if err != nil {
		t.Fatalf("ResolveProcessedPath should fallback to computed fingerprint, got error: %v", err)
	}
	if got == "" {
		t.Fatal("ResolveProcessedPath returned empty path")
	}
}

func TestConvertRAW_RebuildsCorruptedCachedPreview(t *testing.T) {
	tmp := t.TempDir()
	srcPath := filepath.Join(tmp, "photo.dng")
	cacheDir := filepath.Join(tmp, "cache")
	if err := os.WriteFile(srcPath, []byte("raw"), 0o600); err != nil {
		t.Fatalf("write raw source: %v", err)
	}

	fingerprint := CalculateFingerprint(srcPath)
	if fingerprint == "" {
		t.Fatal("empty fingerprint")
	}
	cachePath := ProcessedCachePathWithFingerprint(srcPath, fingerprint, cacheDir)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}

	// Seed a stale corrupted processed file (>1000 bytes so old logic accepted it).
	bad := bytes.Repeat([]byte("x"), 2000)
	if err := os.WriteFile(cachePath, bad, 0o600); err != nil {
		t.Fatalf("write corrupted cache file: %v", err)
	}

	preview := validJPEGBytes(t)
	prevAvailable := isExiftoolAvailable
	prevExtract := extractThumbnail
	t.Cleanup(func() {
		isExiftoolAvailable = prevAvailable
		extractThumbnail = prevExtract
	})
	isExiftoolAvailable = func() bool { return true }
	extractThumbnail = func(_ string, dst string) error {
		return os.WriteFile(dst, preview, 0o600)
	}

	got, err := ConvertRAW(srcPath, fingerprint, cacheDir)
	if err != nil {
		t.Fatalf("ConvertRAW failed: %v", err)
	}
	if got != cachePath {
		t.Fatalf("unexpected cache path: got=%s want=%s", got, cachePath)
	}
	content, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if bytes.Equal(content, bad) {
		t.Fatal("expected corrupted cache file to be replaced")
	}
}

func TestSaveToJPEG_Atomic_NoTempOnSuccess(t *testing.T) {
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "out.jpg")

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))

	if err := saveToJPEG(img, dst); err != nil {
		t.Fatalf("saveToJPEG: %v", err)
	}

	// Target file must exist and be a valid JPEG.
	f, err := os.Open(dst)
	if err != nil {
		t.Fatalf("open result: %v", err)
	}
	defer f.Close()
	if _, err := jpeg.Decode(f); err != nil {
		t.Fatalf("result is not a valid JPEG: %v", err)
	}

	// .tmp must be cleaned up.
	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Fatal(".tmp file should not exist after successful saveToJPEG")
	}
}

func validJPEGBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 1, color.RGBA{G: 255, A: 255})
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode valid jpeg: %v", err)
	}
	return buf.Bytes()
}

func TestConvertRAW_TranscodesNonJPEGPreviewToJPEG(t *testing.T) {
	tmp := t.TempDir()
	srcPath := filepath.Join(tmp, "photo.dng")
	cacheDir := filepath.Join(tmp, "cache")
	if err := os.WriteFile(srcPath, []byte("raw"), 0o600); err != nil {
		t.Fatalf("write raw source: %v", err)
	}

	fp := CalculateFingerprint(srcPath)
	if fp == "" {
		t.Fatal("empty fingerprint")
	}

	// Build a tiny TIFF preview payload to simulate RAW files exposing a non-JPEG preview tag.
	var tiffPayload bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if err := tiff.Encode(&tiffPayload, img, nil); err != nil {
		t.Fatalf("encode tiff payload: %v", err)
	}

	prevAvailable := isExiftoolAvailable
	prevExtract := extractThumbnail
	t.Cleanup(func() {
		isExiftoolAvailable = prevAvailable
		extractThumbnail = prevExtract
	})
	isExiftoolAvailable = func() bool { return true }
	extractThumbnail = func(_ string, dst string) error {
		return os.WriteFile(dst, tiffPayload.Bytes(), 0o600)
	}

	got, err := ConvertRAW(srcPath, fp, cacheDir)
	if err != nil {
		t.Fatalf("ConvertRAW should transcode non-JPEG preview, got: %v", err)
	}
	f, err := os.Open(got)
	if err != nil {
		t.Fatalf("open converted jpeg: %v", err)
	}
	defer f.Close()
	if _, err := jpeg.Decode(f); err != nil {
		t.Fatalf("converted output is not a valid JPEG: %v", err)
	}
}
