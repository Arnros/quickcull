package domain

import "time"

// DateSource indicates the origin of the extracted date.
type DateSource int

const (
	SourceNone DateSource = iota
	SourceEXIF            // DateTimeOriginal, DateTimeDigitized, DateTime
	SourceXMP             // XMP metadata
	SourceIPTC            // IPTC DateCreated + TimeCreated
	SourceFilename        // Extracted from filename
	SourceFileModTime     // File modification time (fallback, less reliable)
)

// String returns the textual representation of the source.
func (s DateSource) String() string {
	switch s {
	case SourceEXIF:
		return "EXIF"
	case SourceXMP:
		return "XMP"
	case SourceIPTC:
		return "IPTC"
	case SourceFilename:
		return "Filename"
	case SourceFileModTime:
		return "FileModTime"
	default:
		return "None"
	}
}

// DateInfo encapsulates an extracted date with its source.
type DateInfo struct {
	Date   time.Time
	Source DateSource
}
