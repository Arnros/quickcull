package domain

import "testing"

func TestCodedErrorHelpers(t *testing.T) {
	err := NewCodedError("sample")
	if err.Error() != ErrorPrefix+"sample" {
		t.Fatalf("unexpected coded error string: %q", err.Error())
	}

	if ErrFolderNotFound.Error() != "QCERR:folder_not_found" {
		t.Fatalf("unexpected predefined coded error: %q", ErrFolderNotFound.Error())
	}
}

func TestDateSourceString(t *testing.T) {
	cases := []struct {
		s    DateSource
		want string
	}{
		{SourceNone, "None"},
		{SourceEXIF, "EXIF"},
		{SourceXMP, "XMP"},
		{SourceIPTC, "IPTC"},
		{SourceFilename, "Filename"},
		{SourceFileModTime, "FileModTime"},
		{DateSource(999), "None"},
	}

	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Fatalf("DateSource(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}
