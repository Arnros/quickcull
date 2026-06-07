package domain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromExtensionAndHelpers(t *testing.T) {
	if got := FromExtension(".JPG"); got != FileTypeJPEG {
		t.Fatalf("expected JPEG, got %v", got)
	}
	if got := FromExtension(".cr3"); got != FileTypeRAW {
		t.Fatalf("expected RAW, got %v", got)
	}
	if got := FromExtension(".unknown"); got != FileTypeUnknown {
		t.Fatalf("expected Unknown, got %v", got)
	}

	if !IsPhotoExtension(".HeIc") {
		t.Fatalf("expected .HeIc to be a photo extension")
	}
	if IsSupportedExtension(".txt") {
		t.Fatalf("expected .txt to be unsupported")
	}
	if !FileTypeJPEG.SupportsPHash() || FileTypeRAW.SupportsPHash() {
		t.Fatalf("unexpected perceptual hash support result")
	}
}

func TestDetectFromMagicBytes(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	webp := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}

	cases := []struct {
		hdr  []byte
		want FileType
	}{
		{jpeg, FileTypeJPEG},
		{png, FileTypePNG},
		{webp, FileTypeWebP},
		{[]byte("short"), FileTypeUnknown},
	}

	for _, tc := range cases {
		if got := DetectFromMagicBytes(tc.hdr); got != tc.want {
			t.Fatalf("DetectFromMagicBytes() = %v, want %v", got, tc.want)
		}
	}
}

func TestDetectFromPathAndNewMediaFile(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "photo.jpg")
	if err := os.WriteFile(p, []byte{0xFF, 0xD8, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if got := DetectFromPath(p); got != FileTypeJPEG {
		t.Fatalf("DetectFromPath() = %v, want %v", got, FileTypeJPEG)
	}

	mf := NewMediaFile(p)
	if mf.Path != p || mf.Type != FileTypeJPEG {
		t.Fatalf("unexpected MediaFile: %+v", mf)
	}
}
