package review

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	testCacheDir, err := os.MkdirTemp("", "quickcull-review-cache-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(testCacheDir)

	_ = os.Setenv("QUICKCULL_TEST_CACHE_DIR", testCacheDir)
	code := m.Run()
	os.Exit(code)
}
