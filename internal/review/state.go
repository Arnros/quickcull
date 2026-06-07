package review

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"quickcull/internal/domain"
	"quickcull/internal/utils"
)

const (
	starsFile     = ".stars"
	rotationsFile = ".rotations"
	labelsFile    = ".labels"
	positionFile  = ".position"
)

// dirPerm is the permission mode used when creating directories managed by State.
const dirPerm = 0o700

// maxCollisionSuffix is the highest numeric suffix tried before falling back to
// a nanosecond timestamp when resolving filename collisions on restore.
const maxCollisionSuffix = 9999

// sortByDateWorkerNumerator and sortByDateWorkerDenominator define the fraction
// of logical CPUs used as workers during date-based sorting (3/4 of NumCPU).
const (
	sortByDateWorkerNumerator   = 3
	sortByDateWorkerDenominator = 4
)

// State manages the file list and provides filesystem-level operations like Trash.
// Metadata (stars, labels, rotations) is now handled by the Reducer and Persistence layers.
type State struct {
	mu           sync.RWMutex
	root         string
	cacheDir     string
	files        []string
	filesLower   []string
	pathToIndex  map[string]int             // relPath → index for O(1) lookup
	types        map[string]domain.FileType // relPath → type
	trashedCount int
}

// NewState creates a new state and loads legacy sidecar metadata for migration.
func NewState(root string, files []string) *State {
	s := &State{
		root:        root,
		cacheDir:    domain.GetCacheDir(root),
		files:       files,
		filesLower:  make([]string, len(files)),
		pathToIndex: make(map[string]int),
		types:       make(map[string]domain.FileType),
	}
	for i, f := range files {
		s.filesLower[i] = strings.ToLower(f)
	}
	s.rebuildIndexLocked()
	return s
}

// rebuildIndexLocked rebuilds the relPath→index map from the current files slice.
// Must be called with mu held for writing.
func (s *State) rebuildIndexLocked() {
	s.pathToIndex = make(map[string]int, len(s.files))
	for i, f := range s.files {
		s.pathToIndex[f] = i
	}
}

// AddFile adds a single file to the state during discovery.
func (s *State) AddFile(relPath string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.pathToIndex[relPath]; ok {
		return len(s.files)
	}

	idx := len(s.files)
	s.files = append(s.files, relPath)
	s.filesLower = append(s.filesLower, strings.ToLower(relPath))
	s.pathToIndex[relPath] = idx
	return len(s.files)
}

// SortFiles performs a final sort of the discovered files.
func (s *State) SortFiles() {
	s.mu.Lock()
	defer s.mu.Unlock()
	sort.Strings(s.files)
	s.filesLower = make([]string, len(s.files))
	for i, f := range s.files {
		s.filesLower[i] = strings.ToLower(f)
	}
	s.rebuildIndexLocked()
}

// ResolveIndex verifies that the file at index matches relPath.
// If relPath is empty it returns index unchanged; otherwise it searches for relPath.
func (s *State) ResolveIndex(index int, relPath string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if relPath == "" {
		return index
	}
	if index >= 0 && index < len(s.files) && s.files[index] == relPath {
		return index
	}
	return s.findIndexLocked(relPath)
}

// Len returns the number of non-trashed files currently tracked.
func (s *State) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.files)
}

// Get returns the relative path of the file at the given index.
func (s *State) Get(index int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 || index >= len(s.files) {
		return "", domain.ErrIndexOutOfBounds
	}
	return s.files[index], nil
}

// AbsPath returns the absolute filesystem path of the file at the given index.
func (s *State) AbsPath(index int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 || index >= len(s.files) {
		return "", domain.ErrIndexOutOfBounds
	}
	return filepath.Join(s.root, s.files[index]), nil
}

// ActiveAbsPaths returns absolute paths for files currently present in active state.
func (s *State) ActiveAbsPaths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.files))
	for _, rel := range s.files {
		out = append(out, filepath.Join(s.root, rel))
	}
	return out
}

// Root returns the root directory this State was created for.
func (s *State) Root() string { return s.root }

// CacheDir returns the cache directory path derived from Root.
func (s *State) CacheDir() string { return s.cacheDir }

// Trash moves the file at index to the .trash/ subdirectory and removes it from
// the active file list.
func (s *State) Trash(index int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.files) {
		return len(s.files), domain.ErrIndexOutOfBounds
	}
	relPath := s.files[index]
	if err := s.trashFileLocked(relPath); err != nil {
		return len(s.files), err
	}
	s.stateRemove(relPath)
	return len(s.files), nil
}

// TrashMultiplePaths moves multiple files to .trash/ efficiently.
func (s *State) TrashMultiplePaths(paths []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, relPath := range paths {
		if s.findIndexLocked(relPath) < 0 {
			continue
		}
		if err := s.trashFileLocked(relPath); err != nil {
			slog.Warn("TrashMultiplePaths: failed to move file to trash", "path", relPath, "err", err)
			continue
		}
		s.stateRemove(relPath)
	}
	return len(s.files), nil
}

// trashFileLocked performs the physical move of relPath into .trash/.
// Must be called with mu held for writing.
func (s *State) trashFileLocked(relPath string) error {
	srcPath := filepath.Join(s.root, relPath)
	trashPath := filepath.Join(s.root, domain.DirTrash, relPath)
	if err := os.MkdirAll(filepath.Dir(trashPath), dirPerm); err != nil {
		slog.Warn("trashFileLocked: could not create trash directory", "path", filepath.Dir(trashPath), "err", err)
		return domain.ErrTrashDirCreate
	}
	if err := os.Rename(srcPath, trashPath); err != nil {
		if copyErr := utils.CopyFile(srcPath, trashPath); copyErr != nil {
			slog.Warn("trashFileLocked: copy fallback failed", "path", relPath, "err", copyErr)
			return domain.ErrTrashCopyFailed
		}
		if removeErr := utils.RemoveFile(srcPath); removeErr != nil {
			slog.Warn("trashFileLocked: could not remove source after copy", "path", srcPath, "err", removeErr)
		}
	}
	return nil
}

// findIndexLocked returns the index of relPath, or -1 if not present.
// Must be called with mu held (read or write).
func (s *State) findIndexLocked(relPath string) int {
	if idx, ok := s.pathToIndex[relPath]; ok {
		return idx
	}
	return -1
}

// FindIndex returns the index of relPath, or -1 if not found.
func (s *State) FindIndex(relPath string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.findIndexLocked(relPath)
}

// UpdateFiles replaces the entire file list and rebuilds internal indexes.
func (s *State) UpdateFiles(newFiles []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = newFiles
	s.filesLower = make([]string, len(newFiles))
	for i, f := range newFiles {
		s.filesLower[i] = strings.ToLower(f)
	}
	s.rebuildIndexLocked()
	s.types = make(map[string]domain.FileType)
	return len(newFiles)
}

// SortByDate reorders files by their EXIF capture date, falling back to mtime.
// Date resolution is parallelised across a fraction of available CPUs.
func (s *State) SortByDate(cache *MediaCache) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.files) <= 1 {
		return
	}
	type fileInfo struct {
		relPath      string
		relPathLower string
		date         time.Time
	}
	numWorkers := runtime.NumCPU() * sortByDateWorkerNumerator / sortByDateWorkerDenominator
	if numWorkers < 1 {
		numWorkers = 1
	}
	infos := make([]fileInfo, len(s.files))
	var wg sync.WaitGroup
	tasks := make(chan int, len(s.files))
	for i := range s.files {
		tasks <- i
	}
	close(tasks)
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		utils.SafeGo(func() {
			defer wg.Done()
			for i := range tasks {
				rel := s.files[i]
				abs := filepath.Join(s.root, rel)
				var date time.Time
				if meta := cache.GetMetadata(abs); meta != nil && meta.Date != "" {
					date = utils.ParseExifDate(meta.Date)
				}
				if date.IsZero() {
					if stat, err := os.Stat(abs); err == nil {
						date = stat.ModTime()
					}
				}
				infos[i] = fileInfo{
					relPath:      rel,
					relPathLower: strings.ToLower(rel),
					date:         date,
				}
			}
		})
	}
	wg.Wait()
	sort.SliceStable(infos, func(i, j int) bool {
		if !infos[i].date.Equal(infos[j].date) {
			return infos[i].date.Before(infos[j].date)
		}
		return infos[i].relPath < infos[j].relPath
	})
	for i, info := range infos {
		s.files[i] = info.relPath
		s.filesLower[i] = info.relPathLower
	}
	s.rebuildIndexLocked()
}

// SortByName sorts files lexicographically by relative path.
func (s *State) SortByName() {
	s.mu.Lock()
	defer s.mu.Unlock()
	sort.Strings(s.files)
	for i, f := range s.files {
		s.filesLower[i] = strings.ToLower(f)
	}
	s.rebuildIndexLocked()
}

// FolderInfo describes a contiguous run of files that share a parent directory.
type FolderInfo struct {
	Path       string `json:"path"`
	Count      int    `json:"count"`
	StartIndex int    `json:"startIndex"`
}

// Folders returns a slice of FolderInfo grouping consecutive files by directory.
func (s *State) Folders() []FolderInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.files) == 0 {
		return nil
	}
	var folders []FolderInfo

	// Initialize with the first file's directory.
	currentDir := filepath.ToSlash(filepath.Dir(s.files[0]))
	startIdx := 0
	count := 0

	for i, f := range s.files {
		dir := filepath.ToSlash(filepath.Dir(f))
		if dir != currentDir {
			folders = append(folders, FolderInfo{Path: currentDir, Count: count, StartIndex: startIdx})
			currentDir = dir
			startIdx = i
			count = 1
		} else {
			count++
		}
	}
	// Append last group.
	folders = append(folders, FolderInfo{Path: currentDir, Count: count, StartIndex: startIdx})
	return folders
}

// ListTrash returns the relative paths of all files currently in .trash/.
func (s *State) ListTrash() ([]string, error) {
	s.mu.RLock()
	root := s.root
	s.mu.RUnlock()
	trashRoot := filepath.Join(root, domain.DirTrash)
	if _, err := os.Stat(trashRoot); err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var items []string
	_ = filepath.WalkDir(trashRoot, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if rel, err := filepath.Rel(trashRoot, path); err == nil {
				items = append(items, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	sort.Strings(items)
	return items, nil
}

// RestoreFromTrash moves the given relative paths back from .trash/ to the root,
// adds them to the active file list, and returns the paths that were successfully restored.
func (s *State) RestoreFromTrash(relPaths []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	restored := make([]string, 0, len(relPaths))
	trashRoot := filepath.Join(s.root, domain.DirTrash)
	for _, rel := range relPaths {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == "" || strings.HasPrefix(rel, "../") {
			continue
		}
		src := filepath.Join(trashRoot, filepath.FromSlash(rel))
		dst := filepath.Join(s.root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
			slog.Warn("RestoreFromTrash: could not create destination directory", "path", filepath.Dir(dst), "err", err)
		}
		if _, err := os.Stat(dst); err == nil {
			dst = resolveCollision(dst)
		}
		if err := moveFile(src, dst); err == nil {
			relDst, _ := filepath.Rel(s.root, dst)
			relDst = filepath.ToSlash(relDst)
			restored = append(restored, relDst)
		}
	}
	for _, rel := range restored {
		s.stateInsert(rel, len(s.files))
	}
	return restored, nil
}

// RestoreFromTrashAt moves a single file from .trash/ back to its original location,
// inserting it at insertAt in the active file list. Used by the undo system to preserve
// the original sort position of the restored photo.
func (s *State) RestoreFromTrashAt(relPath string, insertAt int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == "." || relPath == "" || strings.HasPrefix(relPath, "../") {
		return domain.ErrInvalidPath
	}

	trashRoot := filepath.Join(s.root, domain.DirTrash)
	src := filepath.Join(trashRoot, filepath.FromSlash(relPath))
	dst := filepath.Join(s.root, filepath.FromSlash(relPath))

	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		slog.Warn("RestoreFromTrashAt: could not create destination directory", "path", filepath.Dir(dst), "err", err)
	}
	if _, err := os.Stat(dst); err == nil {
		dst = resolveCollision(dst)
	}
	if err := moveFile(src, dst); err != nil {
		return err
	}

	relDst, _ := filepath.Rel(s.root, dst)
	relDst = filepath.ToSlash(relDst)
	s.stateInsert(relDst, insertAt)
	return nil
}

// TrashedCount returns the number of files that have been trashed in this session.
func (s *State) TrashedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trashedCount
}

// FileSize returns the size in bytes of the file at the given index.
func (s *State) FileSize(index int) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 || index >= len(s.files) {
		return 0, domain.ErrIndexOutOfBounds
	}
	absPath := filepath.Join(s.root, s.files[index])
	info, err := os.Stat(absPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// InitialTotal returns the total number of files at startup (active + trashed).
func (s *State) InitialTotal() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.files) + s.trashedCount
}

// GetType returns the detected FileType for the file at the given index,
// caching the result for subsequent calls.
func (s *State) GetType(index int) domain.FileType {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.files) {
		return domain.FileTypeUnknown
	}
	relPath := s.files[index]
	if t, ok := s.types[relPath]; ok {
		return t
	}
	absPath := filepath.Join(s.root, relPath)
	t := domain.DetectFromPath(absPath)
	s.types[relPath] = t
	return t
}

// stateRemove removes relPath from the active file list and increments trashedCount.
// Must be called with mu held for writing.
func (s *State) stateRemove(relPath string) {
	idx, ok := s.pathToIndex[relPath]
	if !ok {
		return
	}

	copy(s.files[idx:], s.files[idx+1:])
	s.files = s.files[:len(s.files)-1]

	copy(s.filesLower[idx:], s.filesLower[idx+1:])
	s.filesLower = s.filesLower[:len(s.filesLower)-1]

	delete(s.types, relPath)
	s.trashedCount++
	s.rebuildIndexLocked()
}

// StateRemove is the exported wrapper around stateRemove.
func (s *State) StateRemove(relPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateRemove(relPath)
}

// stateInsert inserts relPath at index in the active file list and decrements trashedCount.
// Must be called with mu held for writing.
func (s *State) stateInsert(relPath string, index int) {
	if index < 0 {
		index = 0
	}
	if index > len(s.files) {
		index = len(s.files)
	}
	s.files = append(s.files, "")
	copy(s.files[index+1:], s.files[index:])
	s.files[index] = relPath

	s.filesLower = append(s.filesLower, "")
	copy(s.filesLower[index+1:], s.filesLower[index:])
	s.filesLower[index] = strings.ToLower(relPath)

	delete(s.types, relPath)
	if s.trashedCount > 0 {
		s.trashedCount--
	}
	s.rebuildIndexLocked()
}

// StateInsert is the exported wrapper around stateInsert.
func (s *State) StateInsert(relPath string, index int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateInsert(relPath, index)
}

// LoadPosition reads the persisted cursor position from the cache directory.
// Returns 0 if no position file exists or the value is out of bounds.
func (s *State) LoadPosition() int {
	path := filepath.Join(s.cacheDir, positionFile)
	data, err := os.ReadFile(path) // #nosec G304 -- position sidecar path is under state cache directory.
	if err != nil {
		return 0
	}
	val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	s.mu.RLock()
	defer s.mu.RUnlock()
	if val < 0 {
		return 0
	}
	if val >= len(s.files) {
		if len(s.files) > 0 {
			return len(s.files) - 1
		}
		return 0
	}
	return val
}

// moveFile moves src to dst, falling back to a copy-then-delete when os.Rename fails
// (e.g. across filesystem boundaries).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return copyAndDelete(src, dst)
	}
	return nil
}

// copyAndDelete copies src to dst byte-for-byte and then removes src.
func copyAndDelete(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- source path is managed by state operations.
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		slog.Warn("copyAndDelete: could not create destination directory", "path", filepath.Dir(dst), "err", err)
	}
	out, err := os.Create(dst) // #nosec G304 -- destination path is managed by state operations.
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return utils.RemoveFile(src)
}

// resolveCollision appends an incrementing numeric suffix to path until a free
// name is found, falling back to a nanosecond timestamp after maxCollisionSuffix attempts.
func resolveCollision(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	for i := 1; i <= maxCollisionSuffix; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
}
