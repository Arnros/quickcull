package review

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/bep/imagemeta"
	"github.com/rwcarlsen/goexif/exif"

	"quickcull/internal/domain"
	internalexif "quickcull/internal/exif"
	"quickcull/internal/utils"

	_ "golang.org/x/image/webp"
)

// EXIFInfo contains metadata information for display.
type EXIFInfo struct {
	Fingerprint string `json:"fingerprint,omitempty"`
	Camera      string `json:"camera,omitempty"`
	ISO         string `json:"iso,omitempty"`
	Aperture    string `json:"aperture,omitempty"`
	Shutter     string `json:"shutter,omitempty"`
	Focal       string `json:"focal,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Date        string `json:"date,omitempty"`
}

// CalculateFingerprint returns a combination of file size and mod time.
func CalculateFingerprint(absPath string) string {
	info, err := os.Stat(absPath)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
}

const (
	rawNoExiftoolCameraKey       = "__raw_no_exiftool__"
	rawMetadataUnavailableCamKey = "__raw_metadata_unavailable__"
)

// ExtractEXIFInfo extracts useful metadata from a file.
func ExtractEXIFInfo(absPath string) *EXIFInfo {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("ExtractEXIFInfo panic", "path", absPath, "error", r, "stack", string(debug.Stack()))
		}
	}()

	info := &EXIFInfo{
		Fingerprint: CalculateFingerprint(absPath),
	}

	ext := strings.ToLower(filepath.Ext(absPath))
	ft := domain.FromExtension(ext)

	switch ft {
	case domain.FileTypeHEIC:
		extractEXIFInfoFromHEIC(absPath, info)
	case domain.FileTypeJPEG, domain.FileTypePNG, domain.FileTypeTIFF, domain.FileTypeWebP:
		extractEXIFInfoWithImagemeta(absPath, ft, info)
	case domain.FileTypeRAW:
		extractEXIFWithExiftool(absPath, info)
	}

	// FALLBACK 1: If Camera is still empty, try Exiftool if available
	if info.Camera == "" && internalexif.IsExiftoolAvailable() {
		extractEXIFWithExiftool(absPath, info)
	}

	// FALLBACK 2: If width/height are still zero, use standard image.DecodeConfig (Very fast)
	if info.Width == 0 || info.Height == 0 {
		if f, err := os.Open(absPath); err == nil { // #nosec G304 -- path comes from the indexed media root.
			if cfg, _, err := image.DecodeConfig(f); err == nil {
				info.Width = cfg.Width
				info.Height = cfg.Height
			}
			_ = f.Close()
		}
	}

	// FALLBACK 3: If Camera is still empty for RAW,
	// use stable keys so UI can localize the exact situation.
	if info.Camera == "" && ft == domain.FileTypeRAW {
		if internalexif.IsExiftoolAvailable() {
			info.Camera = rawMetadataUnavailableCamKey
		} else {
			info.Camera = rawNoExiftoolCameraKey
		}
	}

	return info
}

// extractEXIFWithExiftool extracts metadata using the external Exiftool binary.
func extractEXIFWithExiftool(absPath string, info *EXIFInfo) {
	if !internalexif.IsExiftoolAvailable() {
		return
	}

	m, err := internalexif.ExtractMetadata(absPath)
	if err != nil {
		if !recordAnalysisIssue(analysisIssueEXIF, absPath, err) {
			slog.Debug("ExtractEXIFWithExiftool: exiftool extraction failed", "path", absPath, "error", err)
		}
		return
	}

	if m.Model != "" {
		info.Camera = utils.LimitString(m.Model, 128)
	}
	if m.ISO != "" {
		info.ISO = utils.LimitString(m.ISO, 32)
	}
	if m.Aperture != "" {
		info.Aperture = utils.LimitString(m.Aperture, 32)
	}
	if m.Shutter != "" {
		info.Shutter = utils.LimitString(m.Shutter, 32)
	}
	if m.Focal != "" {
		info.Focal = utils.LimitString(m.Focal, 32)
	}
	if m.Width > 0 {
		info.Width = m.Width
	}
	if m.Height > 0 {
		info.Height = m.Height
	}
	if m.Date != "" {
		info.Date = normalizeDate(utils.LimitString(m.Date, 64))
	}
}

// extractEXIFInfoWithImagemeta extracts EXIF using bep/imagemeta.
func extractEXIFInfoWithImagemeta(absPath string, ft domain.FileType, info *EXIFInfo) {
	f, err := os.Open(absPath) // #nosec G304 -- path comes from the indexed media root.
	if err != nil {
		return
	}
	defer f.Close()

	var format imagemeta.ImageFormat
	switch ft {
	case domain.FileTypePNG:
		format = imagemeta.PNG
	case domain.FileTypeTIFF:
		format = imagemeta.TIFF
	case domain.FileTypeWebP:
		format = imagemeta.WebP
	default:
		format = imagemeta.JPEG
	}

	_, _ = imagemeta.Decode(imagemeta.Options{
		R:           f,
		ImageFormat: format,
		HandleTag: func(ti imagemeta.TagInfo) error {
			handleImagemetaTag(info, ti)
			return nil
		},
	})
}

type imagemetaTagHandler func(*EXIFInfo, imagemeta.TagInfo) bool

var imagemetaTagHandlers = []imagemetaTagHandler{
	handleImagemetaIdentityTag,
	handleImagemetaExposureTag,
	handleImagemetaDimensionTag,
	handleImagemetaDateTag,
}

func handleImagemetaTag(info *EXIFInfo, tag imagemeta.TagInfo) {
	for _, handler := range imagemetaTagHandlers {
		if handler(info, tag) {
			return
		}
	}
}

func handleImagemetaIdentityTag(info *EXIFInfo, tag imagemeta.TagInfo) bool {
	switch tag.Tag {
	case "Model":
		if value, ok := tag.Value.(string); ok {
			info.Camera = utils.LimitString(strings.TrimSpace(value), 128)
		}
	case "ISOSpeedRatings":
		info.ISO = utils.LimitString(fmt.Sprintf("%v", tag.Value), 32)
	default:
		return false
	}
	return true
}

func handleImagemetaExposureTag(info *EXIFInfo, tag imagemeta.TagInfo) bool {
	ratio, ok := tag.Value.(imagemeta.Rat[uint32])
	if !ok || ratio.Den() == 0 {
		return tag.Tag == "FNumber" || tag.Tag == "ExposureTime" || tag.Tag == "FocalLength"
	}
	switch tag.Tag {
	case "FNumber":
		info.Aperture = fmt.Sprintf("f/%.1f", float64(ratio.Num())/float64(ratio.Den()))
	case "ExposureTime":
		info.Shutter = formatShutter(float64(ratio.Num()), float64(ratio.Den()))
	case "FocalLength":
		info.Focal = fmt.Sprintf("%.0f mm", float64(ratio.Num())/float64(ratio.Den()))
	default:
		return false
	}
	return true
}

func handleImagemetaDimensionTag(info *EXIFInfo, tag imagemeta.TagInfo) bool {
	value, ok := toInt(tag.Value)
	switch tag.Tag {
	case "PixelXDimension":
		if ok {
			info.Width = value
		}
	case "PixelYDimension":
		if ok {
			info.Height = value
		}
	case "ImageWidth":
		if ok && info.Width == 0 {
			info.Width = value
		}
	case "ImageLength":
		if ok && info.Height == 0 {
			info.Height = value
		}
	default:
		return false
	}
	return true
}

func handleImagemetaDateTag(info *EXIFInfo, tag imagemeta.TagInfo) bool {
	if tag.Tag != "DateTimeOriginal" && tag.Tag != "DateTime" {
		return false
	}
	if value, ok := tag.Value.(string); ok && info.Date == "" {
		info.Date = normalizeDate(utils.LimitString(value, 64))
	}
	return true
}

// normalizeDate converts EXIF date format "2024:11:15 14:30:00" to "2024-11-15 14:30:00".
func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 10 {
		return s
	}
	// If the first 10 chars contain ':', replace them with '-'
	// But only if it looks like YYYY:MM:DD (not already YYYY-MM-DD)
	datePart := s[:10]
	if strings.Count(datePart, ":") == 2 {
		return strings.Replace(datePart, ":", "-", 2) + s[10:]
	}
	return s
}

// formatShutter formats exposure time (numerator/denominator).
func formatShutter(n, d float64) string {
	val := n / d
	if val >= 1 {
		return fmt.Sprintf("%.1f s", val)
	}
	// Display as fraction 1/X
	ratio := d / n
	return fmt.Sprintf("1/%d s", int(math.Round(ratio)))
}

// toInt converts an EXIF value to int.
func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case uint16:
		return int(val), true
	case uint32:
		return int(val), true
	case float64:
		return int(val), true
	}
	return 0, false
}

// GetOrientation extracts the EXIF Orientation tag from a file.
// Returns 1 (standard) if absent or error.
func GetOrientation(absPath string) (orientation int) {
	orientation = 1
	defer func() {
		if r := recover(); r != nil {
			slog.Error("GetOrientation panic", "path", absPath, "error", r)
		}
	}()
	ft := domain.DetectFromPath(absPath)

	switch ft {
	case domain.FileTypeHEIC:
		orientation = extractHEICOrientation(absPath)
	case domain.FileTypeJPEG, domain.FileTypeTIFF, domain.FileTypeRAW:
		f, err := os.Open(absPath) // #nosec G304 -- path comes from the indexed media root.
		if err == nil {
			defer f.Close()
			if x, err := exif.Decode(f); err == nil {
				if tag, err := x.Get(exif.Orientation); err == nil {
					if v, err := tag.Int(0); err == nil {
						if v < 1 || v > 8 {
							slog.Debug("GetOrientation: invalid EXIF orientation, normalizing to 1", "path", absPath, "orientation", v)
							orientation = 1
						} else {
							orientation = v
						}
						slog.Debug("GetOrientation: orientation detected", "path", absPath, "orientation", orientation)
					}
				}
			} else {
				// Missing/invalid EXIF is common on exported/transcoded JPEGs.
				// Keep this as debug noise, not warning.
				slog.Debug("GetOrientation: EXIF decode failed", "path", absPath, "error", err)
			}
		}
	case domain.FileTypePNG, domain.FileTypeWebP:
		f, err := os.Open(absPath) // #nosec G304 -- path comes from the indexed media root.
		if err == nil {
			defer f.Close()
			imgFormat := imagemeta.PNG
			if ft == domain.FileTypeWebP {
				imgFormat = imagemeta.WebP
			}
			_, _ = imagemeta.Decode(imagemeta.Options{
				R:           f,
				ImageFormat: imgFormat,
				HandleTag: func(ti imagemeta.TagInfo) error {
					if ti.Tag == "Orientation" {
						if v, ok := toInt(ti.Value); ok {
							orientation = v
						}
					}
					return nil
				},
			})
		}
	}

	return orientation
}
