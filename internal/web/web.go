// Package web serves the embedded UI (built from web/ into dist/).
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

const fallback = "200.html"

// Handler serves the UI mounted at prefix. Unknown paths get the SPA fallback page.
func Handler(prefix string) http.Handler {
	files, _ := fs.Sub(dist, "dist")
	return handler(prefix, files)
}

func handler(prefix string, files fs.FS) http.Handler {
	if _, err := fs.Stat(files, fallback); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "UI not built: run npm run build in web/", http.StatusServiceUnavailable)
		})
	}
	fileServer := http.FileServerFS(files)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(strings.TrimPrefix(r.URL.Path, prefix)), "/")
		if name != "" && name != "." {
			if st, err := fs.Stat(files, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/" + name
				fileServer.ServeHTTP(w, r2)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, files, fallback)
	})
}
