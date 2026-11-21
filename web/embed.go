package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var archivos embed.FS

var Dist, _ = fs.Sub(archivos, "dist")
