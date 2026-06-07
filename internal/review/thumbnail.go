package review

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"quickcull/internal/domain"
	"quickcull/internal/exif"
	"quickcull/internal/utils"

	"github.com/disintegration/imaging"
)

const (
	thumbCacheVersion = "v7"
	thumbMaxSize      = 240
)

// ThumbCachePathForSource returns the deterministic thumbnail cache path for a source file.
func ThumbCachePathForSource(src, cacheDir string) (string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	fingerprint := fmt.Sprintf("%d-%d", info.Size(), info.ModTime().Unix())
	return ThumbCachePathWithFingerprint(src, fingerprint, cacheDir), nil
}

// ThumbCachePathWithFingerprint returns the thumbnail path using a pre-calculated fingerprint.
func ThumbCachePathWithFingerprint(src, fingerprint, cacheDir string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s", src, fingerprint, thumbCacheVersion)))
	return filepath.Join(cacheDir, "thumbs", fmt.Sprintf("%x.jpg", h[:16]))
}

// GetThumbnail generates or retrieves a thumbnail for an image.
// It prioritizes embedded thumbnails for speed.
func GetThumbnail(src string, cacheDir string, computeSem chan struct{}) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("GetThumbnail panic", "path", src, "error", r, "stack", string(debug.Stack()))
		}
	}()

	ext := strings.ToLower(filepath.Ext(src))
	ft := domain.FromExtension(ext)

	thumbDir := filepath.Join(cacheDir, "thumbs")
	if err := os.MkdirAll(thumbDir, 0700); err != nil {
		return "", err
	}

	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	fingerprint := fmt.Sprintf("%d-%d", info.Size(), info.ModTime().Unix())
	thumbPath := ThumbCachePathWithFingerprint(src, fingerprint, cacheDir)

	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
	}

	mu := getFileLock(src)
	mu.Lock()
	defer mu.Unlock()

	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
	}

	// Non-native formats (RAW, HEIC, TIFF): delegate to the conversion pipeline.
	if ft.IsRaw() || ft.IsHEIC() || ft.IsTIFF() {
		return getThumbnailFromConverted(src, ft, fingerprint, cacheDir, thumbPath, computeSem)
	}

	// Native formats (JPEG, PNG, WebP, …): decode directly.
	if computeSem != nil {
		computeSem <- struct{}{}
		defer func() { <-computeSem }()
	}

	orientation := GetOrientation(src)
	f, err := os.Open(src) // #nosec G304 -- source path comes from scanned media list.
	if err != nil {
		return "", err
	}
	defer f.Close()

	bufIn := utils.DefaultBufferPool.Get()
	defer utils.DefaultBufferPool.Put(bufIn)
	br := bufio.NewReaderSize(f, len(bufIn))

	img, _, err := image.Decode(br)
	if err != nil {
		if !recordAnalysisIssue(analysisIssueThumbnail, src, err) {
			slog.Debug("GetThumbnail: failed to decode image, using error placeholder", "source", src, "error", err)
		}
		return getPlaceholderPath(cacheDir)
	}

	if orientation > 1 {
		img = utils.OrientImage(img, orientation)
	}

	return saveThumbnail(img, thumbPath)
}

// getThumbnailFromConverted generates a thumbnail for non-native formats (RAW, HEIC, TIFF)
// by delegating to the same conversion pipeline used by the full viewer.
// It calls ConvertRAW / ConvertHEIC / ConvertTIFF, decodes the resulting JPEG,
// resizes it, and saves to thumbPath.
// On any error it logs a warning and returns an error placeholder.
func getThumbnailFromConverted(
	src string,
	ft domain.FileType,
	fingerprint, cacheDir, thumbPath string,
	computeSem chan struct{},
) (string, error) {
	var processedPath string
	var err error

	// Pre-check: if the processed JPEG is already in cache, use it directly
	// without re-running the conversion pipeline.
	cachedProcessedPath := ProcessedCachePathWithFingerprint(src, fingerprint, cacheDir)
	if isUsableProcessedJPEG(cachedProcessedPath) {
		processedPath = cachedProcessedPath
	}

	if processedPath == "" {
		switch {
		case ft.IsRaw():
			processedPath, err = ConvertRAW(src, fingerprint, cacheDir)
		case ft.IsHEIC():
			orientation := GetOrientation(src)
			processedPath, err = ConvertHEIC(src, fingerprint, cacheDir, orientation)
			if err != nil && exif.IsExiftoolAvailable() {
				slog.Debug("getThumbnailFromConverted: HEIC native failed, trying exiftool", "source", src, "error", err)
				processedPath, err = ConvertRAW(src, fingerprint, cacheDir)
			}
		case ft.IsTIFF():
			orientation := GetOrientation(src)
			processedPath, err = ConvertTIFF(src, fingerprint, cacheDir, orientation)
		default:
			err = fmt.Errorf("unsupported format for conversion: %s", filepath.Ext(src))
		}
	}

	if err != nil {
		if !recordAnalysisIssue(analysisIssueThumbnail, src, err) {
			slog.Debug("getThumbnailFromConverted: conversion failed, using placeholder", "source", src, "error", err)
		}
		return getPlaceholderPath(cacheDir)
	}

	f, err := os.Open(processedPath) // #nosec G304 -- processedPath is generated under cache dir.
	if err != nil {
		if !recordAnalysisIssue(analysisIssueThumbnail, processedPath, err) {
			slog.Debug("getThumbnailFromConverted: failed to open converted file", "path", processedPath, "error", err)
		}
		return getPlaceholderPath(cacheDir)
	}
	defer f.Close()

	if computeSem != nil {
		computeSem <- struct{}{}
		defer func() { <-computeSem }()
	}

	img, _, err := image.Decode(f)
	if err != nil {
		if !recordAnalysisIssue(analysisIssueThumbnail, processedPath, err) {
			slog.Debug("getThumbnailFromConverted: failed to decode converted JPEG", "path", processedPath, "error", err)
		}
		return getPlaceholderPath(cacheDir)
	}

	return saveThumbnail(img, thumbPath)
}

func saveThumbnail(img image.Image, thumbPath string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("saveThumbnail panic", "path", thumbPath, "error", r)
		}
	}()

	// Apply orientation if not already handled by fast path or needed
	// (Actually, imaging.Fit handles the orientation better if we pass the original)

	// Resize for grid/filmstrip usage.
	thumb := imaging.Fit(img, thumbMaxSize, thumbMaxSize, imaging.Box)

	tmpPath := thumbPath + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600) // #nosec G304 -- temp output path is generated by this function.
	if err != nil {
		return "", err
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = utils.RemoveFile(tmpPath)
		}
	}()

	bw := bufio.NewWriter(out)
	if err := jpeg.Encode(bw, thumb, &jpeg.Options{Quality: 80}); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := bw.Flush(); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, thumbPath); err != nil {
		return "", err
	}
	cleanup = false
	utils.SyncParentDirBestEffort(thumbPath)

	return thumbPath, nil
}

func getPlaceholderPath(cacheDir string) (string, error) {
	thumbDir := filepath.Join(cacheDir, "thumbs")
	_ = os.MkdirAll(thumbDir, 0700)

	name := "error_placeholder.jpg"
	placeholderPath := filepath.Join(thumbDir, name)

	if _, err := os.Stat(placeholderPath); err == nil {
		return placeholderPath, nil
	}

	img := utils.GenerateErrorPlaceholder()

	if err := saveToJPEG(img, placeholderPath); err != nil {
		return "", err
	}
	return placeholderPath, nil
}
