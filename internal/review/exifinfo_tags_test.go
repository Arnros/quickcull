package review

import (
	"testing"

	"github.com/bep/imagemeta"
)

func TestHandleImagemetaTagCharacterization(t *testing.T) {
	aperture, err := imagemeta.NewRat[uint32](28, 10)
	if err != nil {
		t.Fatalf("aperture rat: %v", err)
	}
	shutter, err := imagemeta.NewRat[uint32](1, 250)
	if err != nil {
		t.Fatalf("shutter rat: %v", err)
	}
	focal, err := imagemeta.NewRat[uint32](50, 1)
	if err != nil {
		t.Fatalf("focal rat: %v", err)
	}

	info := &EXIFInfo{}
	tags := []imagemeta.TagInfo{
		{Tag: "Model", Value: "  Camera X  "},
		{Tag: "ISOSpeedRatings", Value: uint16(800)},
		{Tag: "FNumber", Value: aperture},
		{Tag: "ExposureTime", Value: shutter},
		{Tag: "FocalLength", Value: focal},
		{Tag: "ImageWidth", Value: uint32(4000)},
		{Tag: "PixelXDimension", Value: uint32(6000)},
		{Tag: "ImageLength", Value: uint32(3000)},
		{Tag: "PixelYDimension", Value: uint32(4000)},
		{Tag: "DateTime", Value: "2024:01:02 03:04:05"},
		{Tag: "DateTimeOriginal", Value: "2020:05:06 07:08:09"},
		{Tag: "Unknown", Value: "ignored"},
	}
	for _, tag := range tags {
		handleImagemetaTag(info, tag)
	}

	if info.Camera != "Camera X" || info.ISO != "800" {
		t.Fatalf("identity metadata = camera:%q ISO:%q", info.Camera, info.ISO)
	}
	if info.Aperture != "f/2.8" || info.Shutter != "1/250 s" || info.Focal != "50 mm" {
		t.Fatalf("exposure metadata = aperture:%q shutter:%q focal:%q", info.Aperture, info.Shutter, info.Focal)
	}
	if info.Width != 6000 || info.Height != 4000 {
		t.Fatalf("dimensions = %dx%d", info.Width, info.Height)
	}
	if info.Date != "2024-01-02 03:04:05" {
		t.Fatalf("first date must win, got %q", info.Date)
	}
}
