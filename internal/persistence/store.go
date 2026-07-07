package persistence

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"go.etcd.io/bbolt"
)

// StateStore is the interface for persistence operations.
type StateStore interface {
	Close() error
	ClearMetadataScope(folderID, scope string) error
	LoadFolderMetadata(folderID string) (map[string]PhotoMetadata, error)
	SavePhotoMetadata(folderID, photoID string, meta PhotoMetadata) error
	RemovePhotoMetadata(folderID, photoID string) error
	SaveFolderMetadata(folderID string, metadata map[string]PhotoMetadata) error
	SaveHistory(folderID string, history []byte) error
	LoadHistory(folderID string) ([]byte, error)
	GetFolderInfo(folderID string) (FolderInfo, bool)
	SaveFolderInfo(folderID string, info FolderInfo) error
	GetFolderSnapshot(folderID string) (FolderSnapshot, bool)
	SaveFolderSnapshot(folderID string, snap FolderSnapshot) error
}

// MetadataStore handles centralized storage using BoltDB
type MetadataStore struct {
	db *bbolt.DB
}

const (
	bucketInfo     = "_info"
	dbOpenTimeout  = 2 * time.Second
	snapshotPrefix = "snapshot:"
)

// NewMetadataStore initializes the database at the given path.
func NewMetadataStore(dbPath string) (StateStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create metadata directory: %w", err)
	}

	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: dbOpenTimeout})
	if err != nil {
		return nil, fmt.Errorf("open metadata database: %w", err)
	}

	// Initialize system bucket
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketInfo))
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init metadata system bucket: %w", err)
	}

	return &MetadataStore{db: db}, nil
}

// Close closes the database.
func (s *MetadataStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ClearMetadataScope removes specific metadata fields (stars or labels) for all photos in a folder.
func (s *MetadataStore) ClearMetadataScope(folderID, scope string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(folderID))
		if b == nil {
			return nil
		}

		return b.ForEach(func(k, v []byte) error {
			var meta PhotoMetadata
			if err := json.Unmarshal(v, &meta); err != nil {
				return nil // Skip corrupted
			}

			changed := false
			if scope == "stars" || scope == "all" {
				if meta.IsStarred {
					meta.IsStarred = false
					changed = true
				}
			}
			if scope == "labels" || scope == "all" {
				if meta.Label != 0 {
					meta.Label = 0
					changed = true
				}
			}

			if changed {
				data, err := json.Marshal(meta)
				if err != nil {
					return fmt.Errorf("marshal metadata for %s: %w", string(k), err)
				}
				if err := b.Put(k, data); err != nil {
					return fmt.Errorf("clear scope %q for %s: %w", scope, string(k), err)
				}
			}
			return nil
		})
	})
}

// LoadFolderMetadata loads all metadata for a given folder identified by its unique hash.
func (s *MetadataStore) LoadFolderMetadata(folderID string) (map[string]PhotoMetadata, error) {
	metadata := make(map[string]PhotoMetadata)
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(folderID))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			key := string(k)
			if key == "_history" || key == "_snapshot" {
				return nil
			}
			var meta PhotoMetadata
			if err := json.Unmarshal(v, &meta); err != nil {
				slog.Warn("persistence: skipping unmarshable metadata", "folder", folderID, "key", key, "error", err)
				return nil
			}
			metadata[key] = meta
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("load folder metadata %s: %w", folderID, err)
	}
	return metadata, nil
}

// SavePhotoMetadata saves metadata for a single photo in a folder.
func (s *MetadataStore) SavePhotoMetadata(folderID, photoID string, meta PhotoMetadata) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(folderID))
		if err != nil {
			return fmt.Errorf("create bucket for %s: %w", folderID, err)
		}
		data, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("marshal metadata for %s/%s: %w", folderID, photoID, err)
		}
		if err := b.Put([]byte(photoID), data); err != nil {
			return fmt.Errorf("save metadata for %s/%s: %w", folderID, photoID, err)
		}
		return nil
	})
}

// RemovePhotoMetadata deletes the persisted metadata entry for a single photo.
func (s *MetadataStore) RemovePhotoMetadata(folderID, photoID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(folderID))
		if b == nil {
			return nil
		}
		if err := b.Delete([]byte(photoID)); err != nil {
			return fmt.Errorf("remove metadata for %s/%s: %w", folderID, photoID, err)
		}
		return nil
	})
}

// SaveFolderMetadata performs a batch save of all metadata for a folder.
func (s *MetadataStore) SaveFolderMetadata(folderID string, metadata map[string]PhotoMetadata) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(folderID))
		if err != nil {
			return fmt.Errorf("create bucket for %s: %w", folderID, err)
		}
		for id, meta := range metadata {
			data, err := json.Marshal(meta)
			if err != nil {
				return fmt.Errorf("marshal metadata for %s: %w", id, err)
			}
			if err := b.Put([]byte(id), data); err != nil {
				return fmt.Errorf("save metadata for %s/%s: %w", folderID, id, err)
			}
		}
		return nil
	})
}

// SaveHistory persists the undo history for a folder.
func (s *MetadataStore) SaveHistory(folderID string, history []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(folderID))
		if err != nil {
			return fmt.Errorf("create bucket for %s: %w", folderID, err)
		}
		if err := b.Put([]byte("_history"), history); err != nil {
			return fmt.Errorf("save history for %s: %w", folderID, err)
		}
		return nil
	})
}

// LoadHistory retrieves the persisted undo history for a folder.
func (s *MetadataStore) LoadHistory(folderID string) ([]byte, error) {
	var history []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(folderID))
		if b == nil {
			return nil
		}
		v := b.Get([]byte("_history"))
		if v != nil {
			history = make([]byte, len(v))
			copy(history, v)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load history for %s: %w", folderID, err)
	}
	return history, nil
}

// GetFolderInfo retrieves the metadata about the folder itself (path, last scan).
func (s *MetadataStore) GetFolderInfo(folderID string) (FolderInfo, bool) {
	var info FolderInfo
	found := false
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketInfo))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(folderID))
		if v != nil {
			if err := json.Unmarshal(v, &info); err == nil {
				found = true
			}
		}
		return nil
	})
	return info, found
}

// SaveFolderInfo persists folder-level information.
func (s *MetadataStore) SaveFolderInfo(folderID string, info FolderInfo) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketInfo))
		data, err := json.Marshal(info)
		if err != nil {
			return fmt.Errorf("marshal folder info for %s: %w", folderID, err)
		}
		if err := b.Put([]byte(folderID), data); err != nil {
			return fmt.Errorf("save folder info for %s: %w", folderID, err)
		}
		return nil
	})
}

// SaveFolderSnapshot persists folder-level startup snapshot information.
func (s *MetadataStore) SaveFolderSnapshot(folderID string, snap FolderSnapshot) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketInfo))
		data, err := json.Marshal(snap)
		if err != nil {
			return fmt.Errorf("marshal folder snapshot for %s: %w", folderID, err)
		}
		if err := b.Put([]byte(snapshotPrefix+folderID), data); err != nil {
			return fmt.Errorf("save folder snapshot for %s: %w", folderID, err)
		}
		return nil
	})
}

// GetFolderSnapshot retrieves the startup snapshot for a folder.
func (s *MetadataStore) GetFolderSnapshot(folderID string) (FolderSnapshot, bool) {
	var snap FolderSnapshot
	found := false
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketInfo))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(snapshotPrefix + folderID))
		if v == nil {
			return nil
		}
		if err := json.Unmarshal(v, &snap); err == nil {
			found = true
		}
		return nil
	})
	return snap, found
}

// PhotoMetadata represents the persistent state of a photo.
type PhotoMetadata struct {
	IsStarred bool `json:"isStarred"`
	Label     int  `json:"label"`
	Rotation  int  `json:"rotation"`
	IsTrashed bool `json:"isTrashed"`
}

// FolderInfo represents persistent metadata about a folder.
type FolderInfo struct {
	Path          string `json:"path"`
	SavedPosition int    `json:"savedPosition"`
	LastScanned   int64  `json:"lastScanned"`
}

// FolderSnapshot stores a fast-reopen snapshot for a folder.
type FolderSnapshot struct {
	Version      int      `json:"version"`
	RootPath     string   `json:"rootPath"`
	SavedAt      int64    `json:"savedAt"`
	Signature    string   `json:"signature"`
	VisibleOrder []string `json:"visibleOrder"`
}
