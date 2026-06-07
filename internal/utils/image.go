package utils

import (
	"image"
	"image/color"
	"image/draw"
)

// GenerateErrorPlaceholder returns a simple 300x300 image with a solid color.
// Used when an image (like a RAW) cannot be decoded.
func GenerateErrorPlaceholder() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	bg := color.RGBA{40, 40, 40, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	
	// Add a border
	borderColor := color.RGBA{100, 100, 100, 255}
	for x := 0; x < 300; x++ {
		img.Set(x, 0, borderColor)
		img.Set(x, 299, borderColor)
	}
	for y := 0; y < 300; y++ {
		img.Set(0, y, borderColor)
		img.Set(299, y, borderColor)
	}
	
	return img
}

// OrientImage applies the necessary rotation/mirroring according to the EXIF Orientation tag.
// Reference: https://magnushoff.com/articles/jpeg-orientation/
func OrientImage(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2: // Mirror horizontal
		return flip(img, true)
	case 3: // Rotate 180
		return rotate180(img)
	case 4: // Mirror vertical
		return flip(img, false)
	case 5: // Mirror horizontal + rotate 270 CW
		return rotate270(flip(img, true))
	case 6: // Rotate 90 CW
		return rotate90(img)
	case 7: // Mirror horizontal + rotate 90 CW
		return rotate90(flip(img, true))
	case 8: // Rotate 270 CW
		return rotate270(img)
	default:
		return img
	}
}

// toNRGBA converts an image.Image to *image.NRGBA for direct pixel buffer access.
// If the image is already an *image.NRGBA, it is returned as is without copying.
func toNRGBA(img image.Image) *image.NRGBA {
	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba
	}
	bounds := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)
	return dst
}

// rotate90 performs a 90° clockwise rotation.
func rotate90(img image.Image) image.Image {
	src := toNRGBA(img)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, sh, sw))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			srcOff := y*src.Stride + x*4
			dstOff := x*dst.Stride + (sh-1-y)*4
			dst.Pix[dstOff], dst.Pix[dstOff+1], dst.Pix[dstOff+2], dst.Pix[dstOff+3] =
				src.Pix[srcOff], src.Pix[srcOff+1], src.Pix[srcOff+2], src.Pix[srcOff+3]
		}
	}
	return dst
}

// rotate180 performs a 180° rotation.
func rotate180(img image.Image) image.Image {
	src := toNRGBA(img)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			srcOff := y*src.Stride + x*4
			dstOff := (sh-1-y)*dst.Stride + (sw-1-x)*4
			dst.Pix[dstOff], dst.Pix[dstOff+1], dst.Pix[dstOff+2], dst.Pix[dstOff+3] =
				src.Pix[srcOff], src.Pix[srcOff+1], src.Pix[srcOff+2], src.Pix[srcOff+3]
		}
	}
	return dst
}

// rotate270 performs a 270° clockwise rotation (90° counter-clockwise).
func rotate270(img image.Image) image.Image {
	src := toNRGBA(img)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, sh, sw))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			srcOff := y*src.Stride + x*4
			dstOff := (sw-1-x)*dst.Stride + y*4
			dst.Pix[dstOff], dst.Pix[dstOff+1], dst.Pix[dstOff+2], dst.Pix[dstOff+3] =
				src.Pix[srcOff], src.Pix[srcOff+1], src.Pix[srcOff+2], src.Pix[srcOff+3]
		}
	}
	return dst
}

// flip performs a horizontal or vertical flip.
func flip(img image.Image, horizontal bool) image.Image {
	src := toNRGBA(img)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			srcOff := y*src.Stride + x*4
			var dstOff int
			if horizontal {
				dstOff = y*dst.Stride + (sw-1-x)*4
			} else {
				dstOff = (sh-1-y)*dst.Stride + x*4
			}
			dst.Pix[dstOff], dst.Pix[dstOff+1], dst.Pix[dstOff+2], dst.Pix[dstOff+3] =
				src.Pix[srcOff], src.Pix[srcOff+1], src.Pix[srcOff+2], src.Pix[srcOff+3]
		}
	}
	return dst
}

