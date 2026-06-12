package review

import (
	"context"
	"fmt"
	stdjpeg "image/jpeg"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"quickcull/internal/domain"
	"quickcull/internal/persistence"
	"quickcull/internal/utils"
)

// HTTP cache-control header values.
const (
	// cacheControlImmutable is applied to content-addressed thumbnails which
	// never change for a given URL (hash includes path + mtime + size).
	cacheControlImmutable = "public, max-age=31536000, immutable"
	// cacheControlRevalidate is applied to full media files which may be
	// re-encoded or edited in place.
	cacheControlRevalidate = "no-cache, must-revalidate"
)

// thumbsDirName is the subdirectory under cacheDir that holds generated thumbnails.
const thumbsDirName = "thumbs"

// thumbsDirSegment is the path segment used to detect thumbnail paths in HTTP requests.
var thumbsDirSegment = string(filepath.Separator) + thumbsDirName + string(filepath.Separator)

// File extensions and placeholder filenames used by the purge logic.
const (
	// jpgExt is the only derived-cache extension we purge.
	jpgExt = ".jpg"
	// placeholderError is the sentinel thumbnail filename for decode failures.
	placeholderError = "error_placeholder.jpg"
)

// Export event names broadcast to the UI.
const (
	eventExportError     = "export:error"
	eventExportCancelled = "export:cancelled"
	eventExportProgress  = "export:progress"
	eventExportComplete  = "export:complete"
	eventFolderChanged   = "folder:changed"
)

// copySuffix is the string appended before the extension when a destination
// file already exists and an overwrite must be avoided.
const copySuffix = "_copy"

// URL path prefixes handled by ServeMedia.
const (
	mediaPathThumb = "/thumb/"
	mediaPathFull  = "/full/"
)

// staleThumbnailsCap is the initial capacity of the stale-thumbnail slice
// allocated during a purge walk to avoid repeated small reallocations.
const staleThumbnailsCap = 256

// dirPerms is the permission mode used when creating cache or export directories.
const dirPerms = 0700

// placeholderJPEGQuality is the JPEG quality used when encoding an inline
// error-placeholder image directly into the HTTP response.
const placeholderJPEGQuality = 50

// ServeFile serves a file with optional content-type.
func (s *Server) ServeFile(w http.ResponseWriter, r *http.Request, path string, contentType string) {
	slog.Debug("ServeFile: starting", "path", path, "range", r.Header.Get("Range"))
	f, err := os.Open(path) // #nosec G304 -- path is resolved and sanitized before ServeFile call.
	if err != nil {
		slog.Error("ServeFile: open failed", "path", path, "error", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		slog.Error("ServeFile: stat failed", "path", path, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	// Thumbnails are content-addressed on disk (hash includes path+mtime+size),
	// so they can be cached aggressively. Full media remains revalidated.
	if strings.Contains(path, thumbsDirSegment) {
		w.Header().Set("Cache-Control", cacheControlImmutable)
	} else {
		w.Header().Set("Cache-Control", cacheControlRevalidate)
	}
	
	slog.Debug("ServeFile: content serving", "path", path, "size", info.Size())
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
	slog.Debug("ServeFile: completed", "path", path)
}

// PurgeDerivedCacheFiles removes derived cache files (thumbs and conversions)
// that are not present in keepPaths. It never touches state files (cache.db, .stars...).
func (s *Server) PurgeDerivedCacheFiles(keepPaths map[string]struct{}) int {
	cacheDir := s.cacheDir
	if cacheDir == "" {
		return 0
	}

	removed := 0
	thumbDir := filepath.Join(cacheDir, thumbsDirName)
	staleThumbs := make([]string, 0, staleThumbnailsCap)

	// Purge thumbnail files.
	if err := filepath.WalkDir(thumbDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != jpgExt {
			return nil
		}
		base := filepath.Base(path)
		if base == placeholderError {
			return nil
		}
		if _, ok := keepPaths[path]; ok {
			return nil
		}
		staleThumbs = append(staleThumbs, path)
		return nil
	}); err != nil {
		slog.Warn("PurgeDerivedCacheFiles: walk error", "dir", thumbDir, "error", err)
	}
	for _, path := range staleThumbs {
		if err := os.Remove(path); err == nil {
			removed++
		}
	}

	// Purge converted files at cache root.
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return removed
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != jpgExt {
			continue
		}
		path := filepath.Join(cacheDir, e.Name())
		if _, ok := keepPaths[path]; ok {
			continue
		}
		if err := os.Remove(path); err == nil {
			removed++
		}
	}

	return removed
}

func (s *Server) getBurstInfo(index int, absPath string) *burstResult {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("getBurstInfo panic", "index", index, "path", absPath, "error", r)
		}
	}()

	if v, ok := s.burstCache.Load(index); ok {
		return v.(*burstResult)
	}

	// Capture a snapshot of the current state to avoid racing with LoadState
	// which replaces s.state entirely under stateMu.
	state := s.getState()
	if state == nil {
		return nil
	}

	exifInfo := s.cache.GetMetadata(absPath)
	if exifInfo == nil || exifInfo.Date == "" {
		s.burstCache.Store(index, (*burstResult)(nil))
		return nil
	}

	curDate := utils.ParseExifDate(exifInfo.Date)
	if curDate.IsZero() {
		s.burstCache.Store(index, (*burstResult)(nil))
		return nil
	}

	cfg := domain.GetConfig()
	burstLimit := time.Duration(cfg.BurstSeconds) * time.Second
	maxSearch := cfg.BurstMaxFiles

	burstCount := 1
	burstIndex := 1

	start := index - maxSearch
	if start < 0 {
		start = 0
	}
	for i := index - 1; i >= start; i-- {
		p, _ := state.AbsPath(i)
		if ex := s.cache.GetMetadataCached(p); ex != nil && ex.Date != "" {
			d := utils.ParseExifDate(ex.Date)
			if diff := curDate.Sub(d); !d.IsZero() && diff >= 0 && diff < burstLimit {
				burstCount++
				burstIndex++
				continue
			}
		}
		break
	}

	total := state.Len()
	end := index + maxSearch
	if end >= total {
		end = total - 1
	}
	for i := index + 1; i <= end; i++ {
		p, _ := state.AbsPath(i)
		if ex := s.cache.GetMetadataCached(p); ex != nil && ex.Date != "" {
			d := utils.ParseExifDate(ex.Date)
			if diff := d.Sub(curDate); !d.IsZero() && diff >= 0 && diff < burstLimit {
				burstCount++
				continue
			}
		}
		break
	}

	var br *burstResult
	if burstCount > 1 {
		br = &burstResult{count: burstCount, index: burstIndex}
	}
	s.burstCache.Store(index, br)
	return br
}

func (s *Server) invalidateBurstCache() {
	s.burstCache.Range(func(k, v any) bool {
		s.burstCache.Delete(k)
		return true
	})
}

// ExportFilesPaths starts an asynchronous export process.
func (s *Server) ExportFilesPaths(paths []string, destDir string, move bool) error {
	s.exportMu.Lock()
	if s.exportCancel != nil {
		s.exportCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.exportCancel = cancel
	s.exportMu.Unlock()

	utils.SafeGo(func() {
		s.runExport(ctx, paths, destDir, move)
	})
	return nil
}

// CancelExport stops any running export.
func (s *Server) CancelExport() {
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	if s.exportCancel != nil {
		s.exportCancel()
		s.exportCancel = nil
	}
}

func (s *Server) runExport(ctx context.Context, paths []string, destDir string, move bool) {
	state := s.getState()
	if state == nil {
		return
	}

	defer func() {
		s.exportMu.Lock()
		s.exportCancel = nil
		s.exportMu.Unlock()
	}()

	if err := os.MkdirAll(destDir, dirPerms); err != nil {
		s.broadcast(eventExportError, map[string]any{"error": err.Error()})
		return
	}

	total := len(paths)
	movedSomething := false

	for i, relPath := range paths {
		select {
		case <-ctx.Done():
			s.broadcast(eventExportCancelled, nil)
			return
		default:
		}

		absSrc, ok := s.resolveExportSource(state, relPath)
		if !ok {
			continue
		}

		absDest := s.resolveExportDest(destDir, absSrc)

		if err := s.exportSingleFile(absSrc, absDest, move, &movedSomething); err != nil {
			slog.Error("Export operation failed", "src", absSrc, "dest", absDest, "move", move, "error", err)
			continue
		}
		if move {
			s.transferMovedMetadata(relPath, destDir, absDest)
		}

		// Emit progress.
		s.broadcast(eventExportProgress, map[string]any{
			"current": i + 1,
			"total":   total,
			"file":    filepath.Base(absSrc),
		})
	}

	// If we moved files, the local state is now invalid (files are gone).
	if movedSomething {
		s.broadcast(eventFolderChanged, nil)
	}

	s.broadcast(eventExportComplete, map[string]any{
		"total": total,
	})
}

func (s *Server) transferMovedMetadata(sourcePhotoID, destRoot, absDest string) {
	if s.persistence == nil {
		return
	}

	s.appStateMu.RLock()
	if s.appState == nil {
		s.appStateMu.RUnlock()
		return
	}

	sourceRoot := s.appState.Root
	photo, ok := s.appState.Photos[sourcePhotoID]
	s.appStateMu.RUnlock()
	if !ok {
		return
	}

	destPhotoID, err := filepath.Rel(destRoot, absDest)
	if err != nil {
		slog.Warn("Failed to compute destination metadata key for moved photo", "dest_root", destRoot, "dest", absDest, "error", err)
		return
	}

	meta := persistence.PhotoMetadata{
		IsStarred: photo.IsStarred,
		Label:     photo.Label,
		Rotation:  photo.Rotation,
		IsTrashed: false,
	}

	sourceFolderID := domain.GetFolderID(sourceRoot)
	destFolderID := domain.GetFolderID(destRoot)
	if err := s.persistence.SavePhotoMetadata(destFolderID, destPhotoID, meta); err != nil {
		slog.Warn("Failed to persist metadata for moved photo in destination folder", "folder", destFolderID, "photo", destPhotoID, "error", err)
		return
	}
	if err := s.persistence.RemovePhotoMetadata(sourceFolderID, sourcePhotoID); err != nil {
		slog.Warn("Failed to remove metadata for moved photo from source folder", "folder", sourceFolderID, "photo", sourcePhotoID, "error", err)
	}
}

// resolveExportSource returns the absolute source path for a relative path in
// the current state. Returns ("", false) if the file cannot be located.
func (s *Server) resolveExportSource(state *State, relPath string) (string, bool) {
	idx := state.FindIndex(relPath)
	if idx < 0 {
		return "", false
	}
	absPath, err := state.AbsPath(idx)
	if err != nil {
		return "", false
	}
	return absPath, true
}

// resolveExportDest computes the destination path for absSrc inside destDir,
// appending copySuffix before the extension when the target already exists.
func (s *Server) resolveExportDest(destDir, absSrc string) string {
	filename := filepath.Base(absSrc)
	absDest := filepath.Join(destDir, filename)
	if _, err := os.Stat(absDest); err == nil {
		ext := filepath.Ext(filename)
		base := strings.TrimSuffix(filename, ext)
		absDest = filepath.Join(destDir, fmt.Sprintf("%s%s%s", base, copySuffix, ext))
	}
	return absDest
}

// exportSingleFile copies or moves absSrc to absDest.
// It sets *movedSomething to true when a move succeeds.
func (s *Server) exportSingleFile(absSrc, absDest string, move bool, movedSomething *bool) error {
	if !move {
		return utils.CopyFile(absSrc, absDest)
	}

	// Try atomic rename first, fallback to copy+delete.
	if err := os.Rename(absSrc, absDest); err == nil {
		*movedSomething = true
		return nil
	}

	if err := utils.CopyFile(absSrc, absDest); err != nil {
		return fmt.Errorf("copy-then-delete move failed: %w", err)
	}

	// Only remove source if copy succeeded perfectly.
	if err := utils.RemoveFile(absSrc); err != nil {
		slog.Warn("Failed to remove source after copy during move", "file", absSrc, "error", err)
	} else {
		*movedSomething = true
	}
	return nil
}

// parseIndexFromPath strips the given URL prefix and parses the remaining
// segment as a decimal integer index.  Returns an error if the segment is
// not a valid integer (caller should respond with 400 Bad Request).
func parseIndexFromPath(path, prefix string) (int, error) {
	return strconv.Atoi(strings.TrimPrefix(path, prefix))
}

// ServeMedia handles requests for thumbnails and full media.
// It expects the prefix (e.g., /raw-media) to be already stripped from the request.
func (s *Server) ServeMedia(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	state := s.getState()
	if state == nil {
		http.NotFound(w, r)
		return
	}

	// Handle Thumbnails: /thumb/{index}
	if strings.HasPrefix(path, mediaPathThumb) {
		// Mark UI activity to pause background filler.
		s.lastUIActivity.Store(time.Now().UnixNano())

		index, err := parseIndexFromPath(path, mediaPathThumb)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		absPath, err := state.AbsPath(index)
		if err == nil {
			utils.LogNav("ServeMedia: thumbnail request", "index", index, "path", absPath)
			s.serveThumb(w, r, absPath)
			return
		}
	}

	// Handle Full Media: /full/{index}
	if strings.HasPrefix(path, mediaPathFull) {
		index, err := parseIndexFromPath(path, mediaPathFull)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		absPath, err := state.AbsPath(index)
		if err == nil {
			utils.LogNav("ServeMedia: full media request", "index", index, "path", absPath)
			s.serveFullMedia(w, r, absPath)
			return
		}
	}

	http.NotFound(w, r)
}

// serveThumb resolves and serves the thumbnail for the file at absPath,
// falling back to a placeholder image on any processing error.
func (s *Server) serveThumb(w http.ResponseWriter, r *http.Request, absPath string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("serveThumb panic", "path", absPath, "error", r)
		}
	}()

	processedPath, errProc := s.ResolveProcessedPath(absPath)
	if errProc != nil {
		// Graceful fallback: for transient metadata/conversion issues,
		// still try thumbnail generation from source file.
		slog.Warn("Thumbnail processed-path resolution failed; falling back to source", "source", absPath, "error", errProc)
		processedPath = absPath
	}

	// OPTIMIZATION: Check for existing thumb BEFORE locking to avoid congestion.
	// We calculate the path using the cached metadata fingerprint if available.
	if meta := s.cache.GetMetadata(processedPath); meta != nil && meta.Fingerprint != "" {
		thumbPath := ThumbCachePathWithFingerprint(processedPath, meta.Fingerprint, s.cacheDir)
		if _, err := os.Stat(thumbPath); err == nil {
			s.ServeFile(w, r, thumbPath, "image/jpeg")
			return
		}
	}

	// Fallback to full GetThumbnail (with locking) if not found above.
	thumbPath, errThumb := GetThumbnail(processedPath, s.cacheDir, s.cache.computeSem)
	if errThumb != nil {
		s.servePlaceholder(w)
		return
	}
	s.ServeFile(w, r, thumbPath, "image/jpeg")
}

// serveFullMedia serves the full-resolution media for the file at absPath.
// Web-native formats (JPEG, PNG, WebP) are streamed directly; everything else
// is run through ResolveProcessedPath first.
func (s *Server) serveFullMedia(w http.ResponseWriter, r *http.Request, absPath string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("serveFullMedia panic", "path", absPath, "error", r)
		}
	}()

	ext := strings.ToLower(filepath.Ext(absPath))
	ft := domain.FromExtension(ext)

	// FAST PATH: Browser GPU decoding for web-native formats.
	if ft == domain.FileTypeJPEG || ft == domain.FileTypePNG || ft == domain.FileTypeWebP {
		var mimeType string
		switch ft {
		case domain.FileTypeJPEG:
			mimeType = "image/jpeg"
		case domain.FileTypePNG:
			mimeType = "image/png"
		case domain.FileTypeWebP:
			mimeType = "image/webp"
		}
		s.ServeFile(w, r, absPath, mimeType)
		return
	}

	// SLOW PATH: RAW/HEIC/Video conversion. All processed images are JPEGs.
	processedPath, errProc := s.ResolveProcessedPath(absPath)
	if errProc != nil {
		slog.Error("Failed to resolve processed path", "source", absPath, "error", errProc)
		s.servePlaceholder(w)
		return
	}
	s.ServeFile(w, r, processedPath, "image/jpeg")
}

func (s *Server) servePlaceholder(w http.ResponseWriter) {
	img := utils.GenerateErrorPlaceholder()
	w.Header().Set("Content-Type", "image/jpeg")
	if err := stdjpeg.Encode(w, img, &stdjpeg.Options{Quality: placeholderJPEGQuality}); err != nil {
		slog.Warn("Failed to encode placeholder image", "error", err)
	}
}
