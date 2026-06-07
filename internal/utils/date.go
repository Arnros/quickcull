package utils

import "time"

// ParseExifDate attempts to parse a date string from EXIF metadata.
// It supports the standard EXIF format, common alternatives, and ISO 8601
// variants with timezone offsets emitted by modern cameras (iPhone, Sony, Fuji).
func ParseExifDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006:01:02 15:04:05",           // Standard EXIF
		"2006-01-02 15:04:05",           // Alternative flat
		"2006:01:02 15:04:05Z07:00",     // EXIF + timezone offset
		"2006-01-02T15:04:05Z07:00",     // ISO 8601 with offset
		"2006-01-02T15:04:05",           // ISO 8601 local
		"2006-01-02T15:04:05Z",          // ISO 8601 UTC
		"2006-01-02T15:04:05.000Z07:00", // ISO 8601 + milliseconds + offset
		"2006-01-02T15:04:05.000Z",      // ISO 8601 + milliseconds UTC
		"2006-01-02T15:04:05.000",       // ISO 8601 + milliseconds local
		"2006:01:02 15:04:05.000",       // EXIF + milliseconds
	} {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t
		}
	}
	return time.Time{}
}
