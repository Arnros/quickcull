package hash

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/corona10/goimagehash"
)

// ComputePerceptualHash calculates the perceptual hash (pHash) of an image.
func ComputePerceptualHash(filePath string) (*goimagehash.ImageHash, error) {
	f, err := os.Open(filePath) // #nosec G304 -- path comes from the indexed media root.
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	return goimagehash.PerceptionHash(img)
}

// DistanceToSimilarity converts a pHash distance to a similarity percentage.
// pHash uses 64 bits, so the maximum distance is 64.
func DistanceToSimilarity(distance int) float64 {
	const maxDistance = 64.0
	if distance >= int(maxDistance) {
		return 0.0
	}
	return (1.0 - float64(distance)/maxDistance) * 100.0
}

// SimilarityToDistance converts a similarity percentage to a pHash distance.
func SimilarityToDistance(similarityPercent float64) int {
	const maxDistance = 64.0
	if similarityPercent <= 0 {
		return int(maxDistance)
	}
	if similarityPercent >= 100 {
		return 0
	}
	return int((1.0 - similarityPercent/100.0) * maxDistance)
}
