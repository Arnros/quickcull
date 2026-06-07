//go:build cgo

package review

import (
	"fmt"
	"os"
	"strings"

	"github.com/jdeng/goheif"
	"github.com/rwcarlsen/goexif/exif"
)

// extractEXIFInfoFromHEIC extracts EXIF metadata from a HEIC file using goheif.
func extractEXIFInfoFromHEIC(absPath string, info *EXIFInfo) {
	f, err := os.Open(absPath) // #nosec G304 -- path comes from the indexed media root.
	if err != nil {
		return
	}
	defer f.Close()

	exifData, err := goheif.ExtractExif(f)
	if err != nil {
		return
	}

	x, err := exif.Decode(strings.NewReader(string(exifData)))
	if err != nil {
		return
	}

	// Model
	if tag, err := x.Get(exif.Model); err == nil {
		if s, err := tag.StringVal(); err == nil {
			info.Camera = strings.TrimSpace(s)
		}
	}

	// ISO
	if tag, err := x.Get(exif.ISOSpeedRatings); err == nil {
		info.ISO = tag.String()
	}

	// Aperture
	if tag, err := x.Get(exif.FNumber); err == nil {
		if rat, err := tag.Rat(0); err == nil {
			val, _ := rat.Float64()
			info.Aperture = fmt.Sprintf("f/%.1f", val)
		}
	}

	// Exposure Time
	if tag, err := x.Get(exif.ExposureTime); err == nil {
		if rat, err := tag.Rat(0); err == nil {
			n, d := rat.Num().Int64(), rat.Denom().Int64()
			info.Shutter = formatShutter(float64(n), float64(d))
		}
	}

	// Focal Length
	if tag, err := x.Get(exif.FocalLength); err == nil {
		if rat, err := tag.Rat(0); err == nil {
			val, _ := rat.Float64()
			info.Focal = fmt.Sprintf("%.0f mm", val)
		}
	}

	// Date
	if tag, err := x.Get(exif.DateTimeOriginal); err == nil {
		if s, err := tag.StringVal(); err == nil {
			info.Date = s
		}
	}
}

// extractHEICOrientation returns the EXIF orientation of a HEIC file using goheif.
func extractHEICOrientation(absPath string) int {
	f, err := os.Open(absPath) // #nosec G304 -- path comes from the indexed media root.
	if err != nil {
		return 1
	}
	defer f.Close()

	exifData, err := goheif.ExtractExif(f)
	if err != nil {
		return 1
	}

	x, err := exif.Decode(strings.NewReader(string(exifData)))
	if err != nil {
		return 1
	}

	if tag, err := x.Get(exif.Orientation); err == nil {
		if v, err := tag.Int(0); err == nil {
			return v
		}
	}

	return 1
}
