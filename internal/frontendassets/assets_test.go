package frontendassets

import (
	"testing"
)

func TestRawAssetsContainTrackedPlaceholder(t *testing.T) {
	_, err := rawAssets.Open("webdist/index.html")
	if err != nil {
		t.Fatalf("open embedded placeholder index: %v", err)
	}
}
