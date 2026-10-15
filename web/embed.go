package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var assets embed.FS

func Assets() fs.FS {
	f, e := fs.Sub(assets, "dist")
	if e != nil {
		panic(e)
	}
	return f
}
