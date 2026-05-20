package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var files embed.FS

func Handler() http.Handler {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}

	return http.FileServer(http.FS(dist))
}
