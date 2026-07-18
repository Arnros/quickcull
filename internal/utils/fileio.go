package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// RemoveFile attempts to delete a file robustly.
// On Windows/WSL it handles the Read-Only flag and retries on locked files (NTFS).
// On other platforms a single attempt is made.
func RemoveFile(path string) error {
	if runtime.GOOS != "windows" {
		return os.Remove(path)
	}

	var lastErr error
	for i := 0; i < 10; i++ {
		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		lastErr = err

		// Try to remove the Read-Only flag; a Windows process could put it back.
		_ = os.Chmod(path, 0666) // #nosec G302 -- required on Windows to clear read-only bit before deletion.

		// Light exponential backoff for NTFS file-lock release.
		time.Sleep(time.Duration((i+1)*100) * time.Millisecond)
	}

	return fmt.Errorf("could not delete %s after 10 attempts: %w", path, lastErr)
}

// AtomicWriteFileDurable writes data atomically and adds durability guarantees.
// Sequence: write temp file -> fsync temp file -> rename -> best-effort parent dir fsync.
func AtomicWriteFileDurable(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"

	// Recover from stale temp directories (e.g. previous crashed/failed write)
	// that would otherwise cause "open ...tmp: is a directory".
	if info, err := os.Lstat(tmpPath); err == nil && info.IsDir() {
		if err := os.RemoveAll(tmpPath); err != nil {
			return err
		}
	}

	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) // #nosec G304 -- caller controls target path within app workflow.
	if err != nil {
		return err
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpPath)
		}
	}()

	if _, err := out.Write(data); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
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

	SyncParentDirBestEffort(path)
	return nil
}

// CopyFile copies a file from src to dst.
func CopyFile(src, dst string) error {
	s, err := os.Open(src) // #nosec G304 -- source path is resolved by internal state.
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst) // #nosec G304 -- destination path is resolved by internal state.
	if err != nil {
		return err
	}
	defer d.Close()

	if _, err := io.Copy(d, s); err != nil {
		return err
	}
	return d.Sync()
}

// SyncParentDirBestEffort fsyncs the parent directory metadata and ignores failures.
func SyncParentDirBestEffort(path string) {
	dir := filepath.Dir(path)
	d, err := os.Open(dir) // #nosec G304 -- parent directory derived from internal path.
	if err != nil {
		return
	}
	defer d.Close()
	_ = d.Sync()
}

// LimitString truncates a string to n characters and ensures it's valid UTF-8.
func LimitString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Truncate to n bytes
	res := s[:n]
	// Ensure we don't break a multi-byte character
	for i := len(res); i > 0 && res[i-1]&0xc0 == 0x80; i-- {
		res = s[:i-1]
	}
	return res
}
