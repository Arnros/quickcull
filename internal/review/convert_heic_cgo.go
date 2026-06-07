//go:build cgo

package review

import (
	"bufio"
	"fmt"
	"os"
	"quickcull/internal/utils"

	"github.com/jdeng/goheif"
)

// HeicSupported returns true if the binary was built with CGo support.
func HeicSupported() bool {
	return true
}

// ConvertHEIC converts a HEIC file to JPEG and stores it in the cache.
func ConvertHEIC(src, fingerprint, cacheDir string, orientation int) (string, error) {
	return convertOrchestrator(src, fingerprint, cacheDir, func(src string, outPath string) error {
		f, err := os.Open(src) // #nosec G304 -- source path comes from the indexed media root.
		if err != nil {
			return fmt.Errorf("could not open HEIC file: %w", err)
		}
		defer f.Close()

		bufIn := utils.DefaultBufferPool.Get()
		defer utils.DefaultBufferPool.Put(bufIn)
		br := bufio.NewReaderSize(f, len(bufIn))

		img, err := goheif.Decode(br)
		if err != nil {
			return fmt.Errorf("could not decode HEIC file: %w", err)
		}

		if orientation > 1 {
			img = utils.OrientImage(img, orientation)
		}

		return saveToJPEG(img, outPath)
	})
}
