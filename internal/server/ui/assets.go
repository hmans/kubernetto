package ui

import (
	"embed"
	"net/http"
)

//go:embed assets/app.css assets/app.js
var assets embed.FS

func HandleStyles(w http.ResponseWriter, _ *http.Request) {
	serveAsset(w, "assets/app.css", "text/css; charset=utf-8")
}

func HandleScript(w http.ResponseWriter, _ *http.Request) {
	serveAsset(w, "assets/app.js", "text/javascript; charset=utf-8")
}

func serveAsset(w http.ResponseWriter, name, contentType string) {
	body, err := assets.ReadFile(name)
	if err != nil {
		http.Error(w, "asset not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}
