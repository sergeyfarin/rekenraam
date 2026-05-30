package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var files embed.FS

func Handler() http.Handler {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}

	return spaHandler{
		files: dist,
	}
}

type spaHandler struct {
	files fs.FS
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if requestPath == "." || requestPath == "" {
		h.serveFile(w, r, "index.html")
		return
	}

	if exists(h.files, requestPath) || exists(h.files, path.Join(requestPath, "index.html")) {
		if exists(h.files, requestPath) {
			h.serveFile(w, r, requestPath)
			return
		}

		h.serveFile(w, r, path.Join(requestPath, "index.html"))
		return
	}

	if path.Ext(requestPath) != "" {
		http.NotFound(w, r)
		return
	}

	h.serveFile(w, r, "index.html")
}

func exists(files fs.FS, name string) bool {
	_, err := fs.Stat(files, name)
	return err == nil
}

func (h spaHandler) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	http.ServeFileFS(w, r, h.files, name)
}
