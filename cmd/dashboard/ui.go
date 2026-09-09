package main

import (
	"embed"
	"net/http"
)

// uiAssets contains the reusable V2 tokens, components, icons and navigation.
// Subsequent pages can opt in without changing the existing page templates.
//
//go:embed ui/*.css ui/*.html ui/*.js
var uiAssets embed.FS

func registerUIAssets(mux *http.ServeMux) {
	registerPhase2Assets(mux)
	mux.HandleFunc("GET /assets/bukupay-ui-v2.css", func(w http.ResponseWriter, r *http.Request) {
		css, err := uiAssets.ReadFile("ui/bukupay-ui-v2.css")
		if err != nil {
			http.Error(w, "stylesheet unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(css)
	})
}
