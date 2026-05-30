package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist
var files embed.FS

func Handler() http.Handler {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}

	return spaHandler{
		files:      dist,
		fileServer: http.FileServer(http.FS(dist)),
	}
}

type spaHandler struct {
	files      fs.FS
	fileServer http.Handler
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if requestPath == "." || requestPath == "" {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	if exists(h.files, requestPath) || exists(h.files, path.Join(requestPath, "index.html")) {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	if path.Ext(requestPath) != "" {
		http.NotFound(w, r)
		return
	}

	indexRequest := r.Clone(r.Context())
	indexRequest.URL.Path = "/"
	h.fileServer.ServeHTTP(w, indexRequest)
}

func exists(files fs.FS, name string) bool {
	_, err := fs.Stat(files, name)
	return err == nil
}
