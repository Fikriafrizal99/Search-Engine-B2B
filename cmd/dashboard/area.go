package main

import (
	_ "embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type coverageResponse struct {
	Exists   bool                           `json:"exists"`
	Progress prospectstore.CoverageProgress `json:"progress"`
}

var areaTmpl = template.Must(template.New("area").Parse(areaHTML))

func registerAreaRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("GET /areas", a.handleAreaPlanner)
	mux.HandleFunc("GET /api/coverage", a.handleCoverageSnapshot)
	registerVisitPlanRoutes(mux, a)
}

func (a *app) handleAreaPlanner(w http.ResponseWriter, r *http.Request) {
	if err := areaTmpl.Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleCoverageSnapshot(w http.ResponseWriter, r *http.Request) {
	scope := strings.TrimSpace(r.URL.Query().Get("location"))
	progress, exists, err := a.store.CoverageSnapshot(r.Context(), scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, coverageResponse{Exists: exists, Progress: progress})
}

//go:embed area.html
var areaHTML string
