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
		{append([]byte("GIF87a"), make([]byte, 6)...), FileTypeGIF},
		{append([]byte("GIF89a"), make([]byte, 6)...), FileTypeGIF},
		{append([]byte("BM"), make([]byte, 10)...), FileTypeBMP},
		{[]byte{0x49, 0x49, 0x2A, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}, FileTypeTIFF},
		{[]byte{0x4D, 0x4D, 0x00, 0x2A, 0, 0, 0, 0, 0, 0, 0, 0}, FileTypeTIFF},
		{webp, FileTypeWebP},
		{[]byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, FileTypeHEIC},
		{[]byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'x'}, FileTypeHEIC},
		{[]byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'm', 'i', 'f', '1'}, FileTypeHEIC},
		{[]byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'f'}, FileTypeHEIC},
		{[]byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, FileTypeUnknown},
		{[]byte("short"), FileTypeUnknown},
		{make([]byte, magicHeaderSize), FileTypeUnknown},
	}

	for _, tc := range cases {
		if got := DetectFromMagicBytes(tc.hdr); got != tc.want {
			t.Fatalf("DetectFromMagicBytes() = %v, want %v", got, tc.want)
		}
	}
}

func TestDetectFromPath(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "photo.jpg")
	if err := os.WriteFile(p, []byte{0xFF, 0xD8, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if got := DetectFromPath(p); got != FileTypeJPEG {
		t.Fatalf("DetectFromPath() = %v, want %v", got, FileTypeJPEG)
	}

}
