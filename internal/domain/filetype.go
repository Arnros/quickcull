package domain

import (
	"os"
	"strings"
)

// FileType represents the actual media type of a file
type FileType int

const (
	FileTypeUnknown FileType = iota
	// Photos
	FileTypeJPEG
	FileTypeHEIC
	FileTypePNG
	FileTypeTIFF
	FileTypeWebP
	FileTypeGIF
	FileTypeBMP
	FileTypeRAW
)

// String returns the textual representation of the file type
func (ft FileType) String() string {
	switch ft {
	case FileTypeJPEG:
		return "JPEG"
	case FileTypeHEIC:
		return "HEIC"
	case FileTypePNG:
		return "PNG"
	case FileTypeTIFF:
		return "TIFF"
	case FileTypeWebP:
		return "WebP"
	case FileTypeGIF:
		return "GIF"
	case FileTypeBMP:
		return "BMP"
	case FileTypeRAW:
		return "RAW"
	default:
		return "Unknown"
	}
}

// IsPhoto returns true if the type is a photo
func (ft FileType) IsPhoto() bool {
	switch ft {
	case FileTypeJPEG, FileTypeHEIC, FileTypePNG, FileTypeTIFF,
		FileTypeWebP, FileTypeGIF, FileTypeBMP, FileTypeRAW:
		return true
	default:
		return false
	}
}

// IsRaw returns true if the type is a RAW file
func (ft FileType) IsRaw() bool {
	return ft == FileTypeRAW
}

// IsHEIC returns true if the type is a HEIC file
func (ft FileType) IsHEIC() bool {
	return ft == FileTypeHEIC
}

// IsTIFF returns true if the type is a TIFF file
func (ft FileType) IsTIFF() bool {
	return ft == FileTypeTIFF
}

// SupportsPHash returns true if the type supports perceptual hashing
func (ft FileType) SupportsPHash() bool {
	switch ft {
	case FileTypeJPEG, FileTypePNG, FileTypeGIF:
		return true
	default:
		return false
	}
}

// SupportsEXIFWrite returns true if the type supports EXIF metadata writing
func (ft FileType) SupportsEXIFWrite() bool {
	switch ft {
	case FileTypeJPEG, FileTypeHEIC, FileTypePNG:
		return true
	default:
		return false
	}
}

// Extension returns the canonical extension for this type
func (ft FileType) Extension() string {
	switch ft {
	case FileTypeJPEG:
		return ".jpg"
	case FileTypeHEIC:
		return ".heic"
	case FileTypePNG:
		return ".png"
	case FileTypeTIFF:
		return ".tiff"
	case FileTypeWebP:
		return ".webp"
	case FileTypeGIF:
		return ".gif"
	case FileTypeBMP:
		return ".bmp"
	case FileTypeRAW:
		return ".raw"
	default:
		return ""
	}
}

// magicHeaderSize is the number of bytes needed to identify any supported file type.
// 12 bytes covers RIFF/WEBP (needs bytes 8–11) and ISO base media (ftyp brand at 8–11).
const magicHeaderSize = 12

// DetectFromMagicBytes detects the file type using its magic bytes header
func DetectFromMagicBytes(header []byte) FileType {
	if len(header) < magicHeaderSize {
		return FileTypeUnknown
	}

	// JPEG: FFD8 FF
	if header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF {
		return FileTypeJPEG
	}

	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if header[0] == 0x89 && header[1] == 0x50 && header[2] == 0x4E && header[3] == 0x47 &&
		header[4] == 0x0D && header[5] == 0x0A && header[6] == 0x1A && header[7] == 0x0A {
		return FileTypePNG
	}

	// GIF: GIF87a or GIF89a
	if header[0] == 'G' && header[1] == 'I' && header[2] == 'F' && header[3] == '8' &&
		(header[4] == '7' || header[4] == '9') && header[5] == 'a' {
		return FileTypeGIF
	}

	// BMP: BM
	if header[0] == 'B' && header[1] == 'M' {
		return FileTypeBMP
	}

	// TIFF: II (little-endian) or MM (big-endian)
	if (header[0] == 0x49 && header[1] == 0x49 && header[2] == 0x2A && header[3] == 0x00) ||
		(header[0] == 0x4D && header[1] == 0x4D && header[2] == 0x00 && header[3] == 0x2A) {
		return FileTypeTIFF
	}

	// WebP: RIFF....WEBP
	if header[0] == 'R' && header[1] == 'I' && header[2] == 'F' && header[3] == 'F' &&
		header[8] == 'W' && header[9] == 'E' && header[10] == 'B' && header[11] == 'P' {
		return FileTypeWebP
	}

	// HEIC/HEIF: ftyp box
	if string(header[4:8]) == "ftyp" {
		brand := string(header[8:12])
		switch brand {
		case "heic", "heix", "mif1", "heif":
			return FileTypeHEIC
		}
	}

	return FileTypeUnknown
}

// DetectFromPath detects the file type by reading its header from disk
func DetectFromPath(path string) FileType {
	f, err := os.Open(path) // #nosec G304 -- path comes from the indexed media root.
	if err != nil {
		return FileTypeUnknown
	}
	defer f.Close()

	header := make([]byte, magicHeaderSize)
	n, err := f.Read(header)
	if err != nil || n < magicHeaderSize {
		return FileTypeUnknown
	}

	return DetectFromMagicBytes(header)
}

// FromExtension returns the FileType based on file extension
func FromExtension(ext string) FileType {
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg", ".jpeg":
		return FileTypeJPEG
	case ".heic", ".heif":
		return FileTypeHEIC
	case ".png":
		return FileTypePNG
	case ".tiff", ".tif":
		return FileTypeTIFF
	case ".webp":
		return FileTypeWebP
	case ".gif":
		return FileTypeGIF
	case ".bmp":
		return FileTypeBMP
	case ".raw", ".cr2", ".cr3", ".nef", ".arw", ".dng", ".orf", ".rw2", ".raf", ".pef", ".srw":
		return FileTypeRAW
	default:
		return FileTypeUnknown
	}
}

// MediaFile encapsulates a file path and its detected type
type MediaFile struct {
	Path string
	Type FileType
}

// NewMediaFile creates a new MediaFile and detects its type
func NewMediaFile(path string) MediaFile {
	return MediaFile{
		Path: path,
		Type: DetectFromPath(path),
	}
}

// PhotoExtensions contains all supported photo extensions
var PhotoExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".bmp": true, ".tiff": true, ".tif": true, ".webp": true,
	".heic": true, ".heif": true,
	".raw": true, ".cr2": true, ".cr3": true, ".nef": true,
	".arw": true, ".dng": true, ".orf": true, ".rw2": true,
	".raf": true, ".pef": true, ".srw": true,
}

// IsPhotoExtension checks if an extension is a supported photo
func IsPhotoExtension(ext string) bool {
	return PhotoExtensions[strings.ToLower(ext)]
}

// IsSupportedExtension checks if an extension is supported (Only photos now)
func IsSupportedExtension(ext string) bool {
	return IsPhotoExtension(ext)
}

