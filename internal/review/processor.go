package review

import (
	"context"
	"path/filepath"
	"strings"

	"quickcull/internal/domain"
)

// ProcessorContext holds dependencies needed by processors.
type ProcessorContext struct {
	Cache              *MediaCache
	CacheDir           string
	ComputeSem         chan struct{}
	SkipBackgroundHash bool
	IsThumbnailEager   bool
	SkipHeavyThumbnail bool // true for bulk-tier items: suppresses HEIC/RAW thumbnail generation
}

// MediaProcessor defines a plugin for processing specific media types in the background.
type MediaProcessor interface {
	// Supports returns true if this processor can handle the given extension (lowercase, e.g. ".jpg").
	Supports(ext string) bool

	// Process executes the background analysis for the file.
	Process(ctx context.Context, path string, pctx *ProcessorContext) error
}

// --- ImageProcessor ---

type ImageProcessor struct{}

func (p *ImageProcessor) Supports(ext string) bool {
	ft := domain.FromExtension(ext)
	return ft.IsPhoto() && ft != domain.FileTypeRAW
}

func (p *ImageProcessor) Process(ctx context.Context, path string, pctx *ProcessorContext) error {
	ft := domain.DetectFromPath(path)

	// 1. I/O-intensive: EXIF extraction (cached in BoltDB)
	_ = pctx.Cache.GetMetadata(path)

	// 2. Compute-intensive: Perceptual Hash
	if ft.SupportsPHash() && !pctx.SkipBackgroundHash {
		pctx.Cache.GetHash(path)
	}

	// 3. Thumbnails
	if pctx.IsThumbnailEager && !shouldSkipEagerHEICThumbnail(ft, pctx) && !pctx.SkipHeavyThumbnail {
		_, _ = GetThumbnail(path, pctx.CacheDir, pctx.ComputeSem)
	}

	return nil
}

func shouldSkipEagerHEICThumbnail(ft domain.FileType, pctx *ProcessorContext) bool {
	// For very large libraries we defer expensive work (SkipBackgroundHash=true).
	// On !cgo builds, HEIC conversion falls back to exiftool and can dominate startup.
	return ft == domain.FileTypeHEIC && pctx != nil && pctx.SkipBackgroundHash && !HeicSupported()
}

// --- RAWProcessor ---

type RAWProcessor struct{}

func (p *RAWProcessor) Supports(ext string) bool {
	return domain.FromExtension(ext) == domain.FileTypeRAW
}

func (p *RAWProcessor) Process(ctx context.Context, path string, pctx *ProcessorContext) error {
	// 1. EXIF extraction (often uses exiftool, so it's slower)
	_ = pctx.Cache.GetMetadata(path)

	// 2. Thumbnails (uses preview images embedded in RAW)
	if pctx.IsThumbnailEager && !pctx.SkipHeavyThumbnail {
		_, _ = GetThumbnail(path, pctx.CacheDir, pctx.ComputeSem)
	}

	// Note: We don't do PHash on RAWs currently as they need to be decoded first
	return nil
}

// DefaultProcessors returns the standard set of processors.
func DefaultProcessors() []MediaProcessor {
	return []MediaProcessor{
		&ImageProcessor{},
		&RAWProcessor{},
	}
}

// GetProcessorFor returns the first processor capable of handling the file, or nil.
func GetProcessorFor(path string, processors []MediaProcessor) MediaProcessor {
	ext := strings.ToLower(filepath.Ext(path))
	for _, p := range processors {
		if p.Supports(ext) {
			return p
		}
	}
	return nil
}
