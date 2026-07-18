package review

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

func TestScanFiles(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "quickcull-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a file structure
	filesToCreate := []string{
		"photo1.jpg",
		"photo2.png",
		"README.txt",         // unsupported
		".hidden.jpg",        // hidden
		".trash/deleted.jpg", // in .trash
		"sub/photo3.jpg",
		"sub/.hidden/photo4.jpg",    // in hidden folder
		".duplicates/duplicate.jpg", // in excluded folder
	}

	for _, f := range filesToCreate {
		path := filepath.Join(tempDir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	tests := []struct {
		name     string
		root     string
		expected []string
	}{
		{
			name: "standard scan",
			root: tempDir,
			expected: []string{
				"photo1.jpg",
				"photo2.png",
				"sub/photo3.jpg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filesChan := make(chan string, 100)
			go func() {
				_ = ScanFiles(tt.root, filesChan)
			}()
			var got []string
			for f := range filesChan {
				got = append(got, f)
			}
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ScanFiles() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestScanFilesMissingRoot(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")
	filesChan := make(chan string, 1)
	if err := ScanFiles(missingRoot, filesChan); err == nil {
		t.Fatal("expected ScanFiles to fail on missing root")
	}
}

func TestScanFilesLargeTree(t *testing.T) {
	root := t.TempDir()
	const dirCount = 200

	for i := 0; i < dirCount; i++ {
		dir := filepath.Join(root, "batch", "d"+strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", dir, err)
		}
		fp := filepath.Join(dir, "img.jpg")
		if err := os.WriteFile(fp, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s failed: %v", fp, err)
		}
	}

	filesChan := make(chan string, dirCount)
	go func() {
		_ = ScanFiles(root, filesChan)
	}()
	var got []string
	for f := range filesChan {
		got = append(got, f)
	}

	if len(got) != dirCount {
		t.Fatalf("expected %d files, got %d", dirCount, len(got))
	}
}

func TestScanFilesRootIsFile(t *testing.T) {
	root := t.TempDir()
	rootFile := filepath.Join(root, "not-a-directory.jpg")
	if err := os.WriteFile(rootFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write root file failed: %v", err)
	}

	filesChan := make(chan string, 1)
	err := ScanFiles(rootFile, filesChan)
	if err == nil {
		t.Fatal("expected ScanFiles to fail when root path is a file")
	}

	for f := range filesChan {
		t.Fatalf("did not expect discovered files on root scan error, got %q", f)
	}
}
