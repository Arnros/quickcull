package review

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"quickcull/internal/domain"
	internalexif "quickcull/internal/exif"
	"quickcull/internal/utils"

	"github.com/corona10/goimagehash"
	"go.etcd.io/bbolt"
	"golang.org/x/sync/singleflight"
)

// PersistentCache defines the operations for caching media metadata.
type PersistentCache interface {
	GetHash(path string) (uint64, bool)
	PutHash(path string, hash uint64) error
	DeleteHash(path string) error
	GetMetadata(path string) (*EXIFInfo, bool)
	PutMetadata(path string, info *EXIFInfo) error
	DeleteMetadata(path string) error
	IterateHashes(fn func(path string, hash uint64) bool)
	IterateMetadata(fn func(path string, info *EXIFInfo) bool)
	Close() error
}

// boltCache implements PersistentCache using BoltDB.
type boltCache struct {
	db *bbolt.DB
}

func (b *boltCache) GetHash(path string) (uint64, bool) {
	var h uint64
	found := false
	_ = b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(phashBucket))
		if bucket == nil {
			return nil
		}
		v := bucket.Get([]byte(path))
		if len(v) == hashByteSize {
			h = binary.LittleEndian.Uint64(v)
			found = true
		}
		return nil
	})
	return h, found
}

func (b *boltCache) PutHash(path string, hash uint64) error {
	return b.db.Batch(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(phashBucket))
		buf := make([]byte, hashByteSize)
		binary.LittleEndian.PutUint64(buf, hash)
		return bucket.Put([]byte(path), buf)
	})
}

func (b *boltCache) DeleteHash(path string) error {
	return b.db.Batch(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(phashBucket))
		if bucket == nil {
			return nil
		}
		return bucket.Delete([]byte(path))
	})
}

// decodeMetadataBytes tries gob first (current format), then JSON (migration fallback).
func decodeMetadataBytes(v []byte) *EXIFInfo {
	var info *EXIFInfo
	if gob.NewDecoder(bytes.NewReader(v)).Decode(&info) == nil {
		return info
	}
	if json.Unmarshal(v, &info) == nil {
		return info
	}
	return nil
}

func (b *boltCache) GetMetadata(path string) (*EXIFInfo, bool) {
	var (
		info  *EXIFInfo
		found bool
	)
	_ = b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(metadataBucket))
		if bucket == nil {
			return nil
		}
		if v := bucket.Get([]byte(path)); v != nil {
			info = decodeMetadataBytes(v)
			if info != nil {
				if info.Fingerprint == CalculateFingerprint(path) {
					found = true
				} else {
					info = nil
				}
			}
		}
		return nil
	})
	return info, found
}

func (b *boltCache) PutMetadata(path string, info *EXIFInfo) error {
	return b.db.Batch(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(metadataBucket))
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		if err := enc.Encode(info); err != nil {
			return err
		}
		return bucket.Put([]byte(path), buf.Bytes())
	})
}

func (b *boltCache) DeleteMetadata(path string) error {
	return b.db.Batch(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(metadataBucket))
		if bucket == nil {
			return nil
		}
		return bucket.Delete([]byte(path))
	})
}

func (b *boltCache) IterateMetadata(fn func(path string, info *EXIFInfo) bool) {
	_ = b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(metadataBucket))
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(k, v []byte) error {
			if info := decodeMetadataBytes(v); info != nil {
				if !fn(string(k), info) {
					return errStopIteration
				}
			}
			return nil
		})
	})
}

func (b *boltCache) IterateHashes(fn func(path string, hash uint64) bool) {
	_ = b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(phashBucket))
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(k, v []byte) error {
			if len(v) != hashByteSize {
				return nil
			}
			h := binary.LittleEndian.Uint64(v)
			if !fn(string(k), h) {
				return errStopIteration
			}
			return nil
		})
	})
}

func (b *boltCache) Close() error {
	return b.db.Close()
}

type bkNode struct {
	index    int
	hash     uint64
	children map[int]*bkNode
}

// MediaCache manages pHash and EXIF metadata persistence.
type MediaCache struct {
	mu          sync.RWMutex
	hashCache   map[string]*goimagehash.ImageHash
	exifCache   map[string]*EXIFInfo
	persistence PersistentCache
	hashGroup   singleflight.Group
	exifGroup   singleflight.Group
	cacheDir    string
	computeSem  chan struct{}

	bkRoot *bkNode
}

const (
	phashBucket    = "phash"
	metadataBucket = "metadata"
	metaBucket     = "meta"
	exiftoolSigKey = "exiftool_signature"
)

const (
	// pHashBits is the bit-width of a DCT perceptual hash (64-bit fingerprint).
	pHashBits = 64
	// hashByteSize is pHashBits/8 — the little-endian uint64 on-disk encoding size.
	hashByteSize = 8

	cacheDBName       = "cache.db"
	dbFilePerm        = 0600
	dbOpenTimeout     = 1 * time.Second
	dbRecreateTimeout = 5 * time.Second
	dropPathTimeout   = 2 * time.Second

	// megabyte is used for file-size filter conversions (MB → bytes).
	megabyte = 1024 * 1024
	// dateCompareLength is the "YYYY-MM-DD" prefix length used for date range comparisons.
	dateCompareLength = 10
	// bkCandidatesInitCap is the initial capacity of the BK-tree candidate slice.
	bkCandidatesInitCap = 16
)

var errStopIteration = errors.New("stop iteration")

// NewMediaCache creates a new cache manager.
func NewMediaCache() *MediaCache {
	numCompute := runtime.NumCPU() * computeCPUNumerator / computeCPUDenominator
	if numCompute < 1 {
		numCompute = 1
	}
	return &MediaCache{
		hashCache:  make(map[string]*goimagehash.ImageHash),
		exifCache:  make(map[string]*EXIFInfo),
		computeSem: make(chan struct{}, numCompute),
	}
}

// LoadCache opens the BoltDB cache.
func (c *MediaCache) LoadCache(cacheDir string) {
	c.mu.Lock()
	c.cacheDir = cacheDir
	// CLEAR IN-MEMORY CACHES to prevent accumulation across folder changes
	c.hashCache = make(map[string]*goimagehash.ImageHash)
	c.exifCache = make(map[string]*EXIFInfo)
	c.bkRoot = nil
	defer c.mu.Unlock()

	if c.persistence != nil {
		if err := c.persistence.Close(); err != nil {
			slog.Warn("Failed to close persistence cache", "error", err)
		}
	}

	_ = os.MkdirAll(cacheDir, 0o700)
	dbPath := filepath.Join(cacheDir, cacheDBName)
	db, err := bbolt.Open(dbPath, dbFilePerm, &bbolt.Options{
		Timeout: dbOpenTimeout,
		NoSync:  true, // Performance optimization for cache DB
	})
	if err != nil {
		slog.Warn("Failed to open cache DB, attempting to recreate", "error", err)
		_ = os.Remove(dbPath)
		db, err = bbolt.Open(dbPath, dbFilePerm, &bbolt.Options{
			Timeout: dbRecreateTimeout,
			NoSync:  true,
		})
		if err != nil {
			slog.Error("Failed to recreate cache DB", "error", err)
			return
		}
	}

	currentSig := internalexif.ExiftoolSignature()

	// Ensure buckets exist + invalidate metadata when exiftool signature changes.
	_ = db.Update(func(tx *bbolt.Tx) error {
		_, _ = tx.CreateBucketIfNotExists([]byte(phashBucket))
		_, _ = tx.CreateBucketIfNotExists([]byte(metadataBucket))
		meta, _ := tx.CreateBucketIfNotExists([]byte(metaBucket))
		if meta == nil {
			return nil
		}

		prevSig := string(meta.Get([]byte(exiftoolSigKey)))
		// Migrate legacy caches (no signature) and invalidate on signature changes.
		if currentSig != "" && prevSig != currentSig {
			if tx.Bucket([]byte(metadataBucket)) != nil {
				if err := tx.DeleteBucket([]byte(metadataBucket)); err != nil {
					return err
				}
				if _, err := tx.CreateBucket([]byte(metadataBucket)); err != nil {
					return err
				}
			}
		}
		if currentSig != "" {
			_ = meta.Put([]byte(exiftoolSigKey), []byte(currentSig))
		}
		return nil
	})

	c.persistence = &boltCache{db: db}
}

// SaveCache is a no-op.
func (c *MediaCache) SaveCache(cacheDir string) {}

// Close closes the database.
func (c *MediaCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.persistence != nil {
		if err := c.persistence.Close(); err != nil {
			slog.Warn("Failed to close persistence cache", "error", err)
		}
		c.persistence = nil
	}
}

// CompareSimilarity returns the % similarity between two files.
func (c *MediaCache) CompareSimilarity(path1, path2 string) float64 {
	h1 := c.GetHash(path1)
	h2 := c.GetHash(path2)

	if h1 == nil || h2 == nil {
		return -1
	}

	dist, err := h1.Distance(h2)
	if err != nil {
		return -1
	}
	// Clamp to the valid pHash range [0, 64] for defensive safety.
	if dist < 0 {
		dist = 0
	} else if dist > pHashBits {
		dist = pHashBits
	}

	return (1.0 - float64(dist)/float64(pHashBits)) * 100.0
}

// GetHashCached returns the pHash only if it's already in memory or persistence.
// It NEVER triggers a new calculation.
func (c *MediaCache) GetHashCached(path string) *goimagehash.ImageHash {
	c.mu.RLock()
	if h, ok := c.hashCache[path]; ok {
		c.mu.RUnlock()
		return h
	}
	p := c.persistence
	c.mu.RUnlock()

	if p != nil {
		if val, ok := p.GetHash(path); ok {
			h := goimagehash.NewImageHash(val, goimagehash.PHash)
			c.mu.Lock()
			c.hashCache[path] = h
			c.mu.Unlock()
			return h
		}
	}
	return nil
}

// GetMetadataCached returns metadata only if it's already in memory or persistence.
func (c *MediaCache) GetMetadataCached(path string) *EXIFInfo {
	c.mu.RLock()
	if info, ok := c.exifCache[path]; ok {
		c.mu.RUnlock()
		return info
	}
	p := c.persistence
	c.mu.RUnlock()

	if p != nil {
		if info, ok := p.GetMetadata(path); ok {
			c.mu.Lock()
			c.exifCache[path] = info
			c.mu.Unlock()
			return info
		}
	}
	return nil
}

// GetHash returns the pHash of a file, with caching.
func (c *MediaCache) GetHash(path string) *goimagehash.ImageHash {
	// 1. In-memory check
	c.mu.RLock()
	if h, ok := c.hashCache[path]; ok {
		c.mu.RUnlock()
		return h
	}
	p := c.persistence
	c.mu.RUnlock()

	// 2. Persistence check
	if p != nil {
		if val, ok := p.GetHash(path); ok {
			h := goimagehash.NewImageHash(val, goimagehash.PHash)
			c.mu.Lock()
			c.hashCache[path] = h
			c.mu.Unlock()
			return h
		}
	}

	// 3. Classify
	ext := strings.ToLower(filepath.Ext(path))
	if !domain.FromExtension(ext).SupportsPHash() {
		return nil
	}

	// 4. Calculate — singleflight.Do ensures that concurrent requests for the
	// same path share a single computation instead of launching N goroutines.
	val, err, _ := c.hashGroup.Do(path, func() (interface{}, error) {
		c.mu.RLock()
		cacheDir := c.cacheDir
		computeSem := c.computeSem
		c.mu.RUnlock()

		targetPath := path
		if cacheDir != "" {
			// TRY THUMBNAIL FIRST for pHash (massive speedup)
			if thumb, err := GetThumbnail(path, cacheDir, computeSem); err == nil && thumb != "" {
				targetPath = thumb
			}
		}

		f, err := os.Open(targetPath) // #nosec G304 -- path comes from the indexed media root or internal cache.
		if err != nil {
			return nil, err
		}
		defer f.Close()

		// If we are NOT using a thumbnail, this is a full-size decode.
		// We MUST use the semaphore here. (GetThumbnail already uses it if needed).
		if targetPath == path && computeSem != nil {
			computeSem <- struct{}{}
			defer func() { <-computeSem }()
		}

		img, _, err := image.Decode(f)
		if err != nil {
			return nil, err
		}

		hash, err := goimagehash.PerceptionHash(img)
		if err != nil {
			return nil, err
		}

		c.mu.Lock()
		c.hashCache[path] = hash
		p := c.persistence
		c.mu.Unlock()

		if p != nil {
			if err := p.PutHash(path, hash.GetHash()); err != nil {
				slog.Warn("Failed to persist pHash to cache", "path", path, "error", err)
			}
		}

		return hash, nil
	})

	if err != nil {
		if !recordAnalysisIssue(analysisIssueHash, path, err) {
			slog.Debug("GetHash: pHash computation failed", "path", path, "error", err)
		}
		return nil
	}
	return val.(*goimagehash.ImageHash)
}

// GetMetadata returns EXIF info, with caching.
func (c *MediaCache) GetMetadata(path string) *EXIFInfo {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("MediaCache.GetMetadata panic", "path", path, "error", r)
		}
	}()

	// 1. In-memory
	c.mu.RLock()
	if info, ok := c.exifCache[path]; ok {
		c.mu.RUnlock()
		return info
	}
	p := c.persistence
	c.mu.RUnlock()

	// 2. Persistence check
	if p != nil {
		if info, ok := p.GetMetadata(path); ok {
			// Legacy cache migration: old key meant "no exiftool".
			// If exiftool is now available, re-extract once instead of returning stale value.
			if info.Camera == rawNoExiftoolCameraKey && internalexif.IsExiftoolAvailable() {
				// Continue to extraction path below.
			} else
			// Keep cached metadata when it already carries useful filter fields.
			// Re-extract only for truly empty records.
			if (info.Width != 0 && info.Height != 0) || info.Camera != "" || info.ISO != "" || info.Date != "" {
				c.mu.Lock()
				c.exifCache[path] = info
				c.mu.Unlock()
				return info
			}
		}
	}

	// 3. Extract — singleflight.Do prevents redundant extractions when
	// multiple goroutines request metadata for the same path at the same time.
	val, err, _ := c.exifGroup.Do(path, func() (interface{}, error) {
		info := ExtractEXIFInfo(path)
		if info == nil {
			return nil, nil
		}

		c.mu.Lock()
		c.exifCache[path] = info
		p := c.persistence
		c.mu.Unlock()

		if p != nil {
			if err := p.PutMetadata(path, info); err != nil {
				slog.Warn("Failed to persist metadata to cache", "path", path, "error", err)
			}
		}

		return info, nil
	})

	if err != nil || val == nil {
		return nil
	}
	return val.(*EXIFInfo)
}

// RefreshMetadata clears cache entries and forces metadata re-extraction.
func (c *MediaCache) RefreshMetadata(path string) *EXIFInfo {
	c.mu.Lock()
	delete(c.exifCache, path)
	p := c.persistence
	c.mu.Unlock()

	if p != nil {
		_ = p.DeleteMetadata(path)
	}
	return c.GetMetadata(path)
}

// DropPath removes all cached entries associated with a source path.
func (c *MediaCache) DropPath(path string) {
	c.mu.Lock()
	delete(c.exifCache, path)
	delete(c.hashCache, path)
	p := c.persistence
	c.mu.Unlock()

	if p != nil {
		// Use a timeout for physical deletion to avoid deadlocks
		done := make(chan struct{}, 1)
		utils.SafeGo(func() {
			_ = p.DeleteMetadata(path)
			_ = p.DeleteHash(path)
			done <- struct{}{}
		})

		select {
		case <-done:
		case <-time.After(dropPathTimeout):
			slog.Warn("DropPath DB operation timed out, skipping physical delete", "path", path)
		}
	}
}

// PurgeMissing removes cache entries for files that are no longer part of the active set.
// It returns the number of metadata and hash entries removed.
func (c *MediaCache) PurgeMissing(validPaths map[string]struct{}) (int, int) {
	c.mu.RLock()
	p := c.persistence
	c.mu.RUnlock()
	if p == nil {
		return 0, 0
	}

	var staleMeta []string
	p.IterateMetadata(func(path string, info *EXIFInfo) bool {
		if _, ok := validPaths[path]; !ok {
			staleMeta = append(staleMeta, path)
		}
		return true
	})

	var staleHash []string
	p.IterateHashes(func(path string, hash uint64) bool {
		if _, ok := validPaths[path]; !ok {
			staleHash = append(staleHash, path)
		}
		return true
	})

	seen := make(map[string]struct{}, len(staleMeta)+len(staleHash))
	for _, path := range staleMeta {
		seen[path] = struct{}{}
	}
	for _, path := range staleHash {
		seen[path] = struct{}{}
	}
	for path := range seen {
		c.DropPath(path)
	}

	return len(staleMeta), len(staleHash)
}

// GetFilterValues returns unique camera and ISO values from the cache.
func (c *MediaCache) GetFilterValues() (cameras, isos []string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("GetFilterValues panic", "error", r)
		}
	}()

	c.mu.RLock()
	p := c.persistence
	c.mu.RUnlock()

	if p == nil {
		return nil, nil
	}

	camMap := make(map[string]bool)
	isoMap := make(map[string]bool)

	p.IterateMetadata(func(path string, info *EXIFInfo) bool {
		if info.Camera != "" {
			camMap[info.Camera] = true
		}
		if info.ISO != "" {
			isoMap[info.ISO] = true
		}
		return true
	})

	for cam := range camMap {
		cameras = append(cameras, cam)
	}
	for iso := range isoMap {
		isos = append(isos, iso)
	}
	sort.Strings(cameras)
	sort.Strings(isos)
	return
}

// GetFilteredIndices returns indices matching the provided filters.
func (c *MediaCache) GetFilteredIndices(state *State, filters map[string]string) []int {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("GetFilteredIndices panic", "error", r)
		}
	}()

	targetCamera := filters["camera"]
	targetISO := filters["iso"]
	targetDateFrom := strings.TrimSpace(filters["dateFrom"])
	targetDateTo := strings.TrimSpace(filters["dateTo"])
	minBytes, hasMin := parseSizeFilterBytes(filters["sizeMin"])
	maxBytes, hasMax := parseSizeFilterBytes(filters["sizeMax"])

	if targetCamera == "" && targetISO == "" && targetDateFrom == "" && targetDateTo == "" && !hasMin && !hasMax {
		return nil
	}

	var indices []int
	total := state.Len()
	for idx := 0; idx < total; idx++ {
		absPath, err := state.AbsPath(idx)
		if err != nil {
			continue
		}

		if hasMin || hasMax {
			st, err := os.Stat(absPath)
			if err != nil {
				continue
			}
			size := st.Size()
			if hasMin && size < minBytes {
				continue
			}
			if hasMax && size > maxBytes {
				continue
			}
		}

		if targetCamera != "" || targetISO != "" || targetDateFrom != "" || targetDateTo != "" {
			info := c.GetMetadataCached(absPath)
			if info == nil {
				continue
			}

			if targetCamera != "" && info.Camera != targetCamera {
				continue
			}
			if targetISO != "" && info.ISO != targetISO {
				continue
			}
			if targetDateFrom != "" || targetDateTo != "" {
				fileDate := strings.TrimSpace(info.Date)
				if len(fileDate) >= dateCompareLength {
					fileDate = fileDate[:dateCompareLength]
				}
				if fileDate == "" {
					continue
				}
				if targetDateFrom != "" && fileDate < targetDateFrom {
					continue
				}
				if targetDateTo != "" && fileDate > targetDateTo {
					continue
				}
			}
		}

		indices = append(indices, idx)
	}

	sort.Ints(indices)
	return indices
}

func parseSizeFilterBytes(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return int64(n * megabyte), true
}

// thresholdToMaxDist converts a similarity percentage to a maximum Hamming distance.
func thresholdToMaxDist(threshold float64) int {
	d := int(math.Floor((100.0 - threshold) * float64(pHashBits) / 100.0))
	if d < 0 {
		return 0
	}
	if d > pHashBits {
		return pHashBits
	}
	return d
}

// unionFind is a simple path-compressed union-find structure for grouping similar indices.
type unionFind struct {
	parent []int
}

func newUnionFind(n int) unionFind {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return unionFind{parent: p}
}

func (uf *unionFind) find(x int) int {
	if uf.parent[x] != x {
		uf.parent[x] = uf.find(uf.parent[x])
	}
	return uf.parent[x]
}

func (uf *unionFind) union(a, b int) {
	ra, rb := uf.find(a), uf.find(b)
	if ra != rb {
		uf.parent[ra] = rb
	}
}

// bkSearch performs a recursive BK-tree range search.
func bkSearch(node *bkNode, hash uint64, radius int, out *[]int) {
	if node == nil {
		return
	}
	dist := bits.OnesCount64(hash ^ node.hash)
	if dist <= radius {
		*out = append(*out, node.index)
	}
	minD, maxD := dist-radius, dist+radius
	for treeDist, child := range node.children {
		if treeDist >= minD && treeDist <= maxD {
			bkSearch(child, hash, radius, out)
		}
	}
}

// GetDuplicateGroups returns groups of indices that represent similar photos.
func (c *MediaCache) GetDuplicateGroups(state *State, threshold float64) [][]int {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("GetDuplicateGroups panic", "error", r)
		}
	}()

	start := time.Now()
	total := state.Len()
	if total == 0 {
		return nil
	}
	maxDist := thresholdToMaxDist(threshold)

	// 1. Snapshot hashes under short RLock
	hashes := make([]uint64, total)
	valid := make([]bool, total)
	for i := 0; i < total; i++ {
		path, err := state.AbsPath(i)
		if err == nil {
			if h := c.GetHashCached(path); h != nil {
				hashes[i] = h.GetHash()
				valid[i] = true
			}
		}
	}

	// 2. Rebuild/update BK-tree under lock (inline insert avoids recursion depth issues on 24k+ photos)
	c.mu.Lock()
	for i := 0; i < total; i++ {
		if !valid[i] {
			continue
		}
		if c.bkRoot == nil {
			c.bkRoot = &bkNode{index: i, hash: hashes[i], children: make(map[int]*bkNode)}
			continue
		}
		node := c.bkRoot
		for {
			dist := bits.OnesCount64(hashes[i] ^ node.hash)
			if dist == 0 && i == node.index {
				break
			}
			if child, ok := node.children[dist]; ok {
				node = child
				continue
			}
			node.children[dist] = &bkNode{index: i, hash: hashes[i], children: make(map[int]*bkNode)}
			break
		}
	}
	root := c.bkRoot
	c.mu.Unlock()

	if root == nil {
		return nil
	}

	// 3. Similarity search using union-find
	uf := newUnionFind(total)
	for i := 0; i < total; i++ {
		if !valid[i] {
			continue
		}
		candidates := make([]int, 0, bkCandidatesInitCap)
		bkSearch(root, hashes[i], maxDist, &candidates)
		for _, j := range candidates {
			if j <= i || !valid[j] {
				continue
			}
			uf.union(i, j)
		}
	}

	groupMap := make(map[int][]int)
	for i := 0; i < total; i++ {
		if !valid[i] {
			continue
		}
		groupMap[uf.find(i)] = append(groupMap[uf.find(i)], i)
	}

	var groups [][]int
	for _, g := range groupMap {
		if len(g) > 1 {
			sort.Ints(g)
			groups = append(groups, g)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	slog.Info("GetDuplicateGroups: completed", "total", total, "groups", len(groups), "duration_ms", time.Since(start).Milliseconds())
	return groups
}
