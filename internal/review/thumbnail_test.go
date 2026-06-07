package review

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"quickcull/internal/domain"
)

func TestSaveThumbnailDurableWriteSuccess(t *testing.T) {
	thumbDir := t.TempDir()
	thumbPath := filepath.Join(thumbDir, "thumb.jpg")

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	gotPath, err := saveThumbnail(img, thumbPath)
	if err != nil {
		t.Fatalf("saveThumbnail failed: %v", err)
	}
	if gotPath != thumbPath {
		t.Fatalf("expected %s, got %s", thumbPath, gotPath)
	}
	if _, err := os.Stat(thumbPath); err != nil {
		t.Fatalf("expected thumbnail to exist: %v", err)
	}
	if _, err := os.Stat(thumbPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected no temp thumbnail, got err=%v", err)
	}
	out, err := os.Open(thumbPath)
	if err != nil {
		t.Fatalf("open generated thumbnail failed: %v", err)
	}
	defer out.Close()
	if _, err := jpeg.Decode(out); err != nil {
		t.Fatalf("generated thumbnail is not valid JPEG: %v", err)
	}
}

func TestSaveThumbnailDurableWriteCleansTmp(t *testing.T) {
	thumbDir := t.TempDir()
	thumbPath := filepath.Join(thumbDir, "thumb.jpg")

	if err := os.Mkdir(thumbPath+".tmp", 0o755); err != nil {
		t.Fatalf("create tmp directory failed: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if _, err := saveThumbnail(img, thumbPath); err == nil {
		t.Fatalf("expected saveThumbnail to fail")
	}
	if _, err := os.Stat(thumbPath); !os.IsNotExist(err) {
		t.Fatalf("expected thumbnail to not exist, got err=%v", err)
	}
	if info, err := os.Stat(thumbPath + ".tmp"); err != nil {
		t.Fatalf("expected temp directory to remain, got err=%v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected temp path to be a directory")
	}
}

func TestSaveThumbnailOrphanTmpNotServedAsValid(t *testing.T) {
	srcDir := t.TempDir()
	cacheDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "source.jpg")

	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	f, err := os.Create(srcPath)
	if err != nil {
		t.Fatalf("create source image failed: %v", err)
	}
	if err := jpeg.Encode(f, src, &jpeg.Options{Quality: 90}); err != nil {
		_ = f.Close()
		t.Fatalf("encode source image failed: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close source image failed: %v", err)
	}

	thumbPath, err := expectedThumbPath(srcPath, cacheDir)
	if err != nil {
		t.Fatalf("compute expected thumb path failed: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(thumbPath), 0o700); err != nil {
		t.Fatalf("mkdir thumbs dir failed: %v", err)
	}
	orphanTmp := thumbPath + ".tmp"
	if err := os.WriteFile(orphanTmp, []byte("orphan tmp"), 0o600); err != nil {
		t.Fatalf("create orphan temp thumb failed: %v", err)
	}

	gotPath, err := GetThumbnail(srcPath, cacheDir, nil)
	if err != nil {
		t.Fatalf("GetThumbnail failed: %v", err)
	}
	if gotPath != thumbPath {
		t.Fatalf("expected %s, got %s", thumbPath, gotPath)
	}

	if _, err := os.Stat(orphanTmp); !os.IsNotExist(err) {
		t.Fatalf("expected orphan temp to be removed/replaced, got err=%v", err)
	}

	out, err := os.Open(gotPath)
	if err != nil {
		t.Fatalf("open generated thumbnail failed: %v", err)
	}
	defer out.Close()
	if _, err := jpeg.Decode(out); err != nil {
		t.Fatalf("generated thumbnail is not valid JPEG: %v", err)
	}
}

func expectedThumbPath(src string, cacheDir string) (string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	fingerprint := fmt.Sprintf("%d-%d", info.Size(), info.ModTime().Unix())
	return ThumbCachePathWithFingerprint(src, fingerprint, cacheDir), nil
}

// TestGetThumbnailFromConvertedUsesProcessedJPEG verifies that
// getThumbnailFromConverted decodes a pre-existing processed JPEG and saves a
// correctly-sized thumbnail, without touching the source file at all.
func TestGetThumbnailFromConvertedUsesProcessedJPEG(t *testing.T) {
	srcDir := t.TempDir()
	cacheDir := t.TempDir()

	// Create a fake RAW source file (content does not matter — it must not be read)
	srcPath := filepath.Join(srcDir, "shot.dng")
	if err := os.WriteFile(srcPath, []byte("not a real raw file"), 0o600); err != nil {
		t.Fatalf("write fake source: %v", err)
	}

	// Build the processed JPEG that ConvertRAW would have produced
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("stat source: %v", err)
	}
	fingerprint := fmt.Sprintf("%d-%d", srcInfo.Size(), srcInfo.ModTime().UnixNano())
	processedPath := ProcessedCachePathWithFingerprint(srcPath, fingerprint, cacheDir)
	if err := os.MkdirAll(filepath.Dir(processedPath), 0o700); err != nil {
		t.Fatalf("mkdir processed dir: %v", err)
	}

	// Write a valid 100×100 JPEG at the processed cache path
	processedImg := image.NewRGBA(image.Rect(0, 0, 100, 100))
	pf, err := os.Create(processedPath)
	if err != nil {
		t.Fatalf("create processed JPEG: %v", err)
	}
	if err := jpeg.Encode(pf, processedImg, &jpeg.Options{Quality: 90}); err != nil {
		_ = pf.Close()
		t.Fatalf("encode processed JPEG: %v", err)
	}
	if err := pf.Close(); err != nil {
		t.Fatalf("close processed JPEG: %v", err)
	}

	// Compute expected thumbnail path
	thumbPath := ThumbCachePathWithFingerprint(srcPath, fingerprint, cacheDir)
	if err := os.MkdirAll(filepath.Dir(thumbPath), 0o700); err != nil {
		t.Fatalf("mkdir thumb dir: %v", err)
	}

	// Call the function under test
	gotPath, err := getThumbnailFromConverted(srcPath, domain.FileTypeRAW, fingerprint, cacheDir, thumbPath, nil)
	if err != nil {
		t.Fatalf("getThumbnailFromConverted returned error: %v", err)
	}
	if gotPath != thumbPath {
		t.Fatalf("expected %s, got %s", thumbPath, gotPath)
	}

	// Validate it is a valid JPEG
	tf, err := os.Open(gotPath)
	if err != nil {
		t.Fatalf("open thumbnail: %v", err)
	}
	defer tf.Close()
	thumbImg, err := jpeg.Decode(tf)
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	b := thumbImg.Bounds()
	if b.Dx() > thumbMaxSize || b.Dy() > thumbMaxSize {
		t.Fatalf("thumbnail too large: %dx%d, max %d", b.Dx(), b.Dy(), thumbMaxSize)
	}
}

// TestGetThumbnailFromConvertedFallsBackToPlaceholderOnMissingCache verifies that
// getThumbnailFromConverted gracefully falls back to a placeholder when the source
// file does not exist on disk (causing ConvertRAW to fail) and no processed cache
// is available.
func TestGetThumbnailFromConvertedFallsBackToPlaceholderOnMissingCache(t *testing.T) {
	cacheDir := t.TempDir()

	// A fictitious DNG path that does not exist on disk
	srcPath := "/nonexistent/path/shot.dng"
	fingerprint := "fake-fingerprint"

	thumbPath := ThumbCachePathWithFingerprint(srcPath, fingerprint, cacheDir)

	// Call the function under test — ConvertRAW will fail because the file doesn't exist
	gotPath, err := getThumbnailFromConverted(srcPath, domain.FileTypeRAW, fingerprint, cacheDir, thumbPath, nil)
	if err != nil {
		t.Fatalf("getThumbnailFromConverted returned unexpected error: %v", err)
	}

	// The returned path must exist on disk
	if _, statErr := os.Stat(gotPath); statErr != nil {
		t.Fatalf("expected returned path to exist on disk, got stat error: %v", statErr)
	}

	// The file must be a valid JPEG (the placeholder)
	f, err := os.Open(gotPath)
	if err != nil {
		t.Fatalf("open returned path failed: %v", err)
	}
	defer f.Close()
	if _, err := jpeg.Decode(f); err != nil {
		t.Fatalf("returned file is not a valid JPEG: %v", err)
	}
}

func TestGetThumbnailFromConverted_IgnoresCorruptedProcessedCache(t *testing.T) {
	srcDir := t.TempDir()
	cacheDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "shot.dng")
	if err := os.WriteFile(srcPath, []byte("not a real raw file"), 0o600); err != nil {
		t.Fatalf("write fake source: %v", err)
	}

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("stat source: %v", err)
	}
	fingerprint := fmt.Sprintf("%d-%d", srcInfo.Size(), srcInfo.ModTime().UnixNano())
	processedPath := ProcessedCachePathWithFingerprint(srcPath, fingerprint, cacheDir)
	if err := os.MkdirAll(filepath.Dir(processedPath), 0o700); err != nil {
		t.Fatalf("mkdir processed dir: %v", err)
	}
	// Seed a corrupted processed cache file that should be ignored.
	if err := os.WriteFile(processedPath, []byte("broken"), 0o600); err != nil {
		t.Fatalf("write corrupted processed file: %v", err)
	}

	preview := []byte{
		0xFF, 0xD8, 0xFF, 0xD9,
	}
	var previewBuf bytes.Buffer
	previewImg := image.NewRGBA(image.Rect(0, 0, 2, 2))
	previewImg.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := jpeg.Encode(&previewBuf, previewImg, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode preview jpeg: %v", err)
	}
	preview = previewBuf.Bytes()
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

	thumbPath := ThumbCachePathWithFingerprint(srcPath, fingerprint, cacheDir)
	if err := os.MkdirAll(filepath.Dir(thumbPath), 0o700); err != nil {
		t.Fatalf("mkdir thumb dir: %v", err)
	}
	gotPath, err := getThumbnailFromConverted(srcPath, domain.FileTypeRAW, fingerprint, cacheDir, thumbPath, nil)
	if err != nil {
		t.Fatalf("getThumbnailFromConverted returned error: %v", err)
	}
	if filepath.Base(gotPath) == "error_placeholder.jpg" {
		t.Fatalf("expected rebuilt thumbnail, got placeholder at %s", gotPath)
	}
}
