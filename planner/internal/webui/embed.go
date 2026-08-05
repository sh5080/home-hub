// Package webui serves the Vite-built SPA from the binary.
//
// The `all:` prefix includes dotfiles, so the committed dist/.gitkeep keeps
// `go build` working before `npm run build` has ever run. Vite writes into
// this directory (see web/vite.config.ts outDir) — go:embed cannot reach
// outside its own package directory.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves static assets, falling back to index.html for any path the
// bundle doesn't contain so client-side routes deep-link correctly. It must be
// mounted AFTER the /api/ routes; an unknown /api path must never get HTML.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // embed layout is fixed at compile time
	}
	files := http.FS(sub)
	fileServer := http.FileServer(files)

	index, indexErr := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}

		if p != "index.html" {
			if f, err := files.Open("/" + p); err == nil {
				f.Close()
				// Vite hashes everything under /assets; safe to cache forever.
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		if indexErr != nil {
			// dist is the placeholder — frontend not built into this binary.
			http.Error(w, "frontend not built (run make web)", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
