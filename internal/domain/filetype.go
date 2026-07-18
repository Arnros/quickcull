package domain

import (
	"bytes"
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

type magicDetector struct {
	fileType FileType
	matches  func([]byte) bool
}

var magicDetectors = []magicDetector{
	{FileTypeJPEG, func(h []byte) bool { return bytes.Equal(h[:3], []byte{0xFF, 0xD8, 0xFF}) }},
	{FileTypePNG, func(h []byte) bool { return bytes.Equal(h[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) }},
	{FileTypeGIF, func(h []byte) bool {
		return bytes.Equal(h[:6], []byte("GIF87a")) || bytes.Equal(h[:6], []byte("GIF89a"))
	}},
	{FileTypeBMP, func(h []byte) bool { return bytes.Equal(h[:2], []byte("BM")) }},
	{FileTypeTIFF, func(h []byte) bool {
		return bytes.Equal(h[:4], []byte{0x49, 0x49, 0x2A, 0x00}) || bytes.Equal(h[:4], []byte{0x4D, 0x4D, 0x00, 0x2A})
	}},
	{FileTypeWebP, func(h []byte) bool { return bytes.Equal(h[:4], []byte("RIFF")) && bytes.Equal(h[8:12], []byte("WEBP")) }},
	{FileTypeHEIC, isHEICHeader},
}

// DetectFromMagicBytes detects the file type using its magic bytes header
func DetectFromMagicBytes(header []byte) FileType {
	if len(header) < magicHeaderSize {
		return FileTypeUnknown
	}
	for _, detector := range magicDetectors {
		if detector.matches(header) {
			return detector.fileType
		}
	}
	return FileTypeUnknown
}

func isHEICHeader(header []byte) bool {
	if !bytes.Equal(header[4:8], []byte("ftyp")) {
		return false
	}
	switch string(header[8:12]) {
	case "heic", "heix", "mif1", "heif":
		return true
	default:
		return false
	}
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
