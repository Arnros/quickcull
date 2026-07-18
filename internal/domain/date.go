package domain

import "time"

// DateSource indicates the origin of the extracted date.
type DateSource int

const (
	SourceNone        DateSource = iota
	SourceEXIF                   // DateTimeOriginal, DateTimeDigitized, DateTime
	SourceXMP                    // XMP metadata
	SourceIPTC                   // IPTC DateCreated + TimeCreated
	SourceFilename               // Extracted from filename
	SourceFileModTime            // File modification time (fallback, less reliable)
)

// DateInfo encapsulates an extracted date with its source.
type DateInfo struct {
	Date   time.Time
	Source DateSource
}
