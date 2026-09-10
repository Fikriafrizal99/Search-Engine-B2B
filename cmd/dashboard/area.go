package main

import (
	_ "embed"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type coverageResponse struct {
	Exists   bool                            `json:"exists"`
	Progress prospectstore.CoverageProgress `json:"progress"`
	Snapshot prospectstore.AreaSnapshot     `json:"snapshot"`
}

var areaTmpl = template.Must(template.Must(template.New("area").Funcs(phase2Funcs).Parse(areaHTML)).ParseFS(uiAssets, "ui/shared.html"))

func registerAreaRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("GET /areas", a.handleAreaPlanner)
	mux.HandleFunc("GET /api/coverage", a.handleCoverageSnapshot)
	mux.HandleFunc("GET /api/visit-report", a.handleDailyVisitReport)
	registerVisitPlanRoutes(mux, a)
	registerSalesDashboardRoutes(mux, a)
}

func (a *app) handleAreaPlanner(w http.ResponseWriter, r *http.Request) {
	areas, err := a.store.StoredAreas(r.Context())
	if err != nil {
		renderPhase2Error(w, http.StatusInternalServerError, "Area Planner", err)
		return
	}
	renderPhase2(w, areaTmpl, struct {
		Areas        []prospectstore.StoredArea
		InitialScope string
	}{areas, strings.TrimSpace(r.URL.Query().Get("location"))})
}

func (a *app) handleCoverageSnapshot(w http.ResponseWriter, r *http.Request) {
	scope := strings.TrimSpace(r.URL.Query().Get("location"))
	progress, exists, err := a.store.CoverageSnapshot(r.Context(), scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	snapshot, err := a.store.AreaSnapshot(r.Context(), scope, time.Now().In(jakartaLocation).AddDate(0, 0, 1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, coverageResponse{Exists: exists, Progress: progress, Snapshot: snapshot})
}

//go:embed area.html
var areaHTML string
