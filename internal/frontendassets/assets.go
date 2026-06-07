package frontendassets

import (
	"embed"
	"io/fs"
)

// rawAssets keeps the frontend bundle embedded under a stable tracked directory.
//
//go:embed all:webdist
var rawAssets embed.FS

var Assets fs.FS = resolveAssets()

func resolveAssets() fs.FS {
	if _, err := fs.Stat(rawAssets, "webdist/dist/index.html"); err == nil {
		return mustSub(rawAssets, "webdist/dist")
	}
	return mustSub(rawAssets, "webdist")
}

func mustSub(root fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(root, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
