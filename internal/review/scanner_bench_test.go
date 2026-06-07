package review

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkScanner10k simulate the scanning of 10,000 files to ensure zero-ingestion performance.
func BenchmarkScanner10k(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "quickcull-bench-10k")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create 10,000 dummy files
	for i := 0; i < 10000; i++ {
		fname := filepath.Join(tmpDir, fmt.Sprintf("img_%d.jpg", i))
		_ = os.WriteFile(fname, []byte("fake"), 0644)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filesChan := make(chan string, 1000)
		go func() {
			_ = ScanFiles(tmpDir, filesChan)
		}()
		count := 0
		for range filesChan {
			count++
		}
		if count != 10000 {
			b.Fatalf("Expected 10000 files, got %d", count)
		}
	}
}
