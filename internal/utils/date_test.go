package utils

import (
	"testing"
	"time"
)

func TestParseExifDate(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantZero  bool
		wantYear  int
		wantMonth time.Month
		wantDay   int
		wantHour  int
	}{
		// Format 1: Standard EXIF "2006:01:02 15:04:05"
		{
			name:      "standard_exif",
			input:     "2023:06:15 14:30:00",
			wantZero:  false,
			wantYear:  2023,
			wantMonth: time.June,
			wantDay:   15,
			wantHour:  14,
		},
		// Format 2: Alternative flat "2006-01-02 15:04:05"
		{
			name:      "alternative_flat",
			input:     "2021-12-25 08:00:00",
			wantZero:  false,
			wantYear:  2021,
			wantMonth: time.December,
			wantDay:   25,
			wantHour:  8,
		},
		// Format 3: EXIF + timezone offset "2006:01:02 15:04:05Z07:00"
		{
			name:      "exif_with_timezone",
			input:     "2022:03:10 09:15:00+00:00",
			wantZero:  false,
			wantYear:  2022,
			wantMonth: time.March,
			wantDay:   10,
			wantHour:  9,
		},
		// Format 4: ISO 8601 with offset "2006-01-02T15:04:05Z07:00"
		{
			name:      "iso8601_with_offset",
			input:     "2020-07-04T12:00:00+00:00",
			wantZero:  false,
			wantYear:  2020,
			wantMonth: time.July,
			wantDay:   4,
			wantHour:  12,
		},
		// Format 5: ISO 8601 local "2006-01-02T15:04:05"
		{
			name:      "iso8601_local",
			input:     "2019-01-01T00:00:00",
			wantZero:  false,
			wantYear:  2019,
			wantMonth: time.January,
			wantDay:   1,
			wantHour:  0,
		},
		// Format 6: ISO 8601 UTC "2006-01-02T15:04:05Z"
		{
			name:      "iso8601_utc",
			input:     "2024-11-30T23:59:59Z",
			wantZero:  false,
			wantYear:  2024,
			wantMonth: time.November,
			wantDay:   30,
			wantHour:  23,
		},
		// Timezone positive (+02:00): verify year/month/day
		{
			name:      "positive_timezone",
			input:     "2023:08:20 16:00:00+02:00",
			wantZero:  false,
			wantYear:  2023,
			wantMonth: time.August,
			wantDay:   20,
			wantHour:  16,
		},
		// Timezone negative (-05:00): verify year/month/day
		{
			name:      "negative_timezone",
			input:     "2023:08:20 10:00:00-05:00",
			wantZero:  false,
			wantYear:  2023,
			wantMonth: time.August,
			wantDay:   20,
			wantHour:  10,
		},
		// Empty string → zero time
		{
			name:     "empty_string",
			input:    "",
			wantZero: true,
		},
		// Invalid format
		{
			name:     "invalid_format",
			input:    "not-a-date",
			wantZero: true,
		},
		// Partial format (YYYY:MM:DD without time)
		{
			name:     "partial_format",
			input:    "2006:01:02",
			wantZero: true,
		},
		// ISO 8601 + milliseconds + offset (iPhone, modern Android)
		{
			name:      "iso8601_millis_offset",
			input:     "2024-05-18T10:30:45.123+02:00",
			wantZero:  false,
			wantYear:  2024,
			wantMonth: time.May,
			wantDay:   18,
			wantHour:  10,
		},
		// ISO 8601 + milliseconds UTC
		{
			name:      "iso8601_millis_utc",
			input:     "2024-05-18T10:30:45.123Z",
			wantZero:  false,
			wantYear:  2024,
			wantMonth: time.May,
			wantDay:   18,
			wantHour:  10,
		},
		// ISO 8601 + milliseconds local
		{
			name:      "iso8601_millis_local",
			input:     "2024-05-18T10:30:45.123",
			wantZero:  false,
			wantYear:  2024,
			wantMonth: time.May,
			wantDay:   18,
			wantHour:  10,
		},
		// EXIF + milliseconds (Sony, Nikon)
		{
			name:      "exif_millis",
			input:     "2024:05:18 10:30:45.123",
			wantZero:  false,
			wantYear:  2024,
			wantMonth: time.May,
			wantDay:   18,
			wantHour:  10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseExifDate(tt.input)
			if tt.wantZero {
				if !got.IsZero() {
					t.Errorf("ParseExifDate(%q) = %v, want zero time", tt.input, got)
				}
				return
			}
			if got.IsZero() {
				t.Errorf("ParseExifDate(%q) returned zero time, expected non-zero", tt.input)
				return
			}
			if got.Year() != tt.wantYear {
				t.Errorf("ParseExifDate(%q).Year() = %d, want %d", tt.input, got.Year(), tt.wantYear)
			}
			if got.Month() != tt.wantMonth {
				t.Errorf("ParseExifDate(%q).Month() = %v, want %v", tt.input, got.Month(), tt.wantMonth)
			}
			if got.Day() != tt.wantDay {
				t.Errorf("ParseExifDate(%q).Day() = %d, want %d", tt.input, got.Day(), tt.wantDay)
			}
			if got.Hour() != tt.wantHour {
				t.Errorf("ParseExifDate(%q).Hour() = %d, want %d", tt.input, got.Hour(), tt.wantHour)
			}
		})
	}
}
