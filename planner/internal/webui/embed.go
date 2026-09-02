// Package webui 는 Vite 빌드 결과를 바이너리에서 서빙한다.
// `all:` 이라 dist/.gitkeep 이 포함돼 빌드 전에도 go build 가 된다.
package webui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

func init() {
	// Go 기본 표에 없다. 없으면 text/plain 이 되어 iOS 가 매니페스트로 인정하지 않는다.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

//go:embed all:dist
var distFS embed.FS

// Handler 는 정적 파일을 주고 없는 경로는 index.html 로 떨어뜨린다. /api/ 라우트 뒤에 마운트해야 한다.
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

		// 확장자가 있는 경로는 없으면 404 (index.html 로 떨어뜨리면 아이콘 등이 200 text/html 이 된다).
		if p != "index.html" && path.Ext(p) != "" && path.Ext(p) != ".html" {
			http.NotFound(w, r)
			return
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
