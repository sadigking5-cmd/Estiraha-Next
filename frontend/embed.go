package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var rawDist embed.FS

// DistFS returns the embedded production Vite bundle rooted at frontend/dist.
func DistFS() (fs.FS, error) {
	return fs.Sub(rawDist, "dist")
}
