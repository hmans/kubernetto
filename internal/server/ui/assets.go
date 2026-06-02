package ui

import (
	"embed"
	"net/http"
)

//go:embed assets
var assets embed.FS

func HandleAssets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.FileServer(http.FS(assets)).ServeHTTP(w, r)
}
