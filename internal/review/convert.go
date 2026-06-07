package review

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"quickcull/internal/domain"
	"quickcull/internal/exif"
	"quickcull/internal/utils"

	"golang.org/x/image/tiff"
)

// convertSem limits to 1 simultaneous heavy conversion to prevent memory spikes.
var convertSem = make(chan struct{}, 1)

var (
	isExiftoolAvailable = exif.IsExiftoolAvailable
	extractThumbnail    = exif.ExtractThumbnail
)

const processedCacheVersion = "v5"

// conversionFunc is a helper type for specific file type decoders.
type conversionFunc func(src string, outPath string) error

// convertOrchestrator handles the common logic for any file conversion:
// cache check, locking, semaphore management, and directory creation.
func convertOrchestrator(src, fingerprint, cacheDir string, convertFn conversionFunc) (string, error) {
	cachePath := ProcessedCachePathWithFingerprint(src, fingerprint, cacheDir)

	// 1. First check without lock (Optimistic)
	if isUsableProcessedJPEG(cachePath) {
		return cachePath, nil
	}

	// 2. Lock this specific file
	mu := getFileLock(src)
	mu.Lock()
	defer mu.Unlock()

	// 3. Re-check after acquiring lock
	if isUsableProcessedJPEG(cachePath) {
		return cachePath, nil
	}

	// 4. Limit concurrency for heavy tasks
	convertSem <- struct{}{}
	defer func() { <-convertSem }()

	if err := os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
		return "", fmt.Errorf("could not create cache folder: %w", err)
	}

	// 5. Execute the actual conversion
	if err := convertFn(src, cachePath); err != nil {
		return "", err
	}

	return cachePath, nil
}

// ConvertTIFF converts a TIFF file to JPEG and stores it in cache.
func ConvertTIFF(src, fingerprint, cacheDir string, orientation int) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("ConvertTIFF panic", "path", src, "error", r)
		}
	}()

	return convertOrchestrator(src, fingerprint, cacheDir, func(src string, outPath string) error {
		f, err := os.Open(src) // #nosec G304 -- source path comes from the indexed media root.
		if err != nil {
			return fmt.Errorf("could not open TIFF file: %w", err)
		}
		defer f.Close()

		bufIn := utils.DefaultBufferPool.Get()
		defer utils.DefaultBufferPool.Put(bufIn)
		br := bufio.NewReaderSize(f, len(bufIn))

		img, err := tiff.Decode(br)
		if err != nil {
			return fmt.Errorf("could not decode TIFF file: %w", err)
		}

		if orientation > 1 {
			img = utils.OrientImage(img, orientation)
		}

		return saveToJPEG(img, outPath)
	})
}

// ConvertRAW attempts to extract the JPEG preview from a RAW file using exiftool.
func ConvertRAW(src, fingerprint, cacheDir string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("ConvertRAW panic", "path", src, "error", r)
		}
	}()

	return convertOrchestrator(src, fingerprint, cacheDir, func(src string, outPath string) error {
		if !isExiftoolAvailable() {
			return fmt.Errorf("exiftool not found")
		}

		if err := extractThumbnail(src, outPath); err != nil {
			return err
		}

		// Ensure extractor produced a non-empty preview file.
		if info, err := os.Stat(outPath); err != nil || info.Size() == 0 {
			return fmt.Errorf("extracted preview missing or empty")
		}
		// Some RAW files expose non-JPEG embedded previews (for example TIFF).
		// Normalize to JPEG so browser/full-media and thumbnail flows are stable.
		if err := normalizePreviewToJPEG(outPath); err != nil {
			return err
		}
		return nil
	})
}

// saveToJPEG saves an image atomically to path using a temp→rename pattern.
func saveToJPEG(img image.Image, path string) error {
	tmpPath := path + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600) // #nosec G304 -- output path is generated under cache dir.
	if err != nil {
		return err
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = utils.RemoveFile(tmpPath)
		}
	}()

	bufOut := utils.DefaultBufferPool.Get()
	defer utils.DefaultBufferPool.Put(bufOut)
	bw := bufio.NewWriterSize(out, len(bufOut))

	if err := jpeg.Encode(bw, img, &jpeg.Options{Quality: 92}); err != nil {
		_ = out.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func isUsableProcessedJPEG(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return false
	}

	f, err := os.Open(path) // #nosec G304 -- path is generated under cache dir.
	if err != nil {
		return false
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, 4096)
	if _, err := jpeg.DecodeConfig(br); err != nil {
		return false
	}
	return true
}

func normalizePreviewToJPEG(path string) error {
	f, err := os.Open(path) // #nosec G304 -- path is generated under cache dir.
	if err != nil {
		return err
	}
	defer f.Close()

	// Fast path: already a valid JPEG.
	if _, err := jpeg.DecodeConfig(f); err == nil {
		return nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("unsupported preview format: %w", err)
	}

	if err := saveToJPEG(img, path); err != nil {
		return err
	}
	utils.SyncParentDirBestEffort(path)
	return nil
}

// ResolveProcessedPath determines the best path to serve for a given source.
// It handles RAW, TIFF, and HEIC by converting them if necessary.
func (s *Server) ResolveProcessedPath(absPath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(absPath))
	ft := domain.FromExtension(ext)

	// Fast path: formats that don't need conversion should always fall back
	// to source path, even if metadata extraction temporarily fails.
	if !ft.IsRaw() && !ft.IsHEIC() && !ft.IsTIFF() {
		return absPath, nil
	}

	// Conversion path: needs a stable fingerprint for cache key.
	meta := s.cache.GetMetadata(absPath)
	fingerprint := ""
	if meta != nil {
		fingerprint = meta.Fingerprint
	}
	// Legacy cache compatibility: if metadata is missing or does not carry
	// a fingerprint yet, compute one from file path + modtime + size.
	if fingerprint == "" {
		fingerprint = CalculateFingerprint(absPath)
	}
	if fingerprint == "" {
		return "", fmt.Errorf("unable to resolve fingerprint for %s", absPath)
	}

	if ft.IsRaw() {
		return ConvertRAW(absPath, fingerprint, s.cacheDir)
	}
	if ft.IsHEIC() {
		orientation := GetOrientation(absPath)
		// Try native CGo conversion first
		path, err := ConvertHEIC(absPath, fingerprint, s.cacheDir, orientation)
		if err == nil {
			return path, nil
		}
		// Fallback to exiftool if native failed or not supported
		if exif.IsExiftoolAvailable() {
			slog.Debug("HEIC native conversion failed, trying exiftool", "path", absPath, "error", err)
			return ConvertRAW(absPath, fingerprint, s.cacheDir)
		}
		return "", err // Return original error if exiftool is also unavailable
	}
	if ft.IsTIFF() {
		orientation := GetOrientation(absPath)
		return ConvertTIFF(absPath, fingerprint, s.cacheDir, orientation)
	}

	return absPath, nil
}

// ProcessedCachePathWithFingerprint returns the cache path for a source file using a fingerprint.
func ProcessedCachePathWithFingerprint(src, fingerprint, cacheDir string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s", src, fingerprint, processedCacheVersion)))
	return filepath.Join(cacheDir, fmt.Sprintf("%x.jpg", h[:16]))
}
