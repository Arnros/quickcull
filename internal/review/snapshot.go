package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"quickcull/internal/persistence"
)

const folderSnapshotVersion = 1

const snapshotTopLevelSample = 64

// BuildFolderSignature computes a deterministic lightweight signature for a folder.
// The signature covers:
//   - The root directory's own mtime and top-level entry count.
//   - A sample of up to snapshotTopLevelSample top-level entries (name, size, mtime).
//   - The mtime of every top-level sub-directory, so that adding or removing files
//     inside sub-folders also invalidates the snapshot.
func BuildFolderSignature(root string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	info, err := os.Stat(absRoot)
	if err != nil || !info.IsDir() {
		return ""
	}
	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return ""
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "root=%s|mtime=%d|n=%d", absRoot, info.ModTime().UnixNano(), len(entries))

	limit := len(entries)
	if limit > snapshotTopLevelSample {
		limit = snapshotTopLevelSample
	}
	for i := 0; i < limit; i++ {
		e := entries[i]
		_, _ = fmt.Fprintf(h, "|name=%s|dir=%t", e.Name(), e.IsDir())
		if fi, err := e.Info(); err == nil {
			_, _ = fmt.Fprintf(h, "|size=%d|mtime=%d", fi.Size(), fi.ModTime().UnixNano())
		}
		// For sub-directories, include their own mtime so that adding or removing
		// files inside them also invalidates the snapshot signature.
		if e.IsDir() {
			subPath := filepath.Join(absRoot, e.Name())
			if subInfo, err := os.Stat(subPath); err == nil {
				_, _ = fmt.Fprintf(h, "|submtime=%d", subInfo.ModTime().UnixNano())
			}
		}
	}
	if len(entries) > snapshotTopLevelSample {
		var maxMtime int64
		for i := snapshotTopLevelSample; i < len(entries); i++ {
			if fi, err := entries[i].Info(); err == nil {
				if t := fi.ModTime().UnixNano(); t > maxMtime {
					maxMtime = t
				}
			}
		}
		_, _ = fmt.Fprintf(h, "|extra=%d|extramtime=%d", len(entries)-snapshotTopLevelSample, maxMtime)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// IsSnapshotUsable checks whether a persisted snapshot can be trusted for fast startup.
func IsSnapshotUsable(root string, snap persistence.FolderSnapshot, currentSig string) bool {
	if snap.Version != folderSnapshotVersion {
		return false
	}
	if !samePath(root, snap.RootPath) {
		return false
	}
	if snap.Signature == "" || currentSig == "" {
		return false
	}
	return snap.Signature == currentSig
}

func samePath(a, b string) bool {
	aa := filepath.Clean(a)
	bb := filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(aa, bb)
	}
	return aa == bb
}
