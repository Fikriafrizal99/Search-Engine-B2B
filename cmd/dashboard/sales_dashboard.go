package main

import (
	_ "embed"
	"html/template"
	"net/http"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type salesDashboardData struct {
	Summary  prospectstore.SalesDashboardSummary
	Pipeline prospectstore.BukupayPipelineStats
	Today    string
}

var salesDashboardTmpl = template.Must(template.New("sales-dashboard").Parse(salesDashboardHTML))

func registerSalesDashboardRoutes(mux *http.ServeMux, a *app) {
	// The legacy application still registers GET / as a catch-all. On the
	// Bukupay branch, claim the exact root so normal navigation can never fall
	// back into the old B2B dashboard.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/database", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /sales", a.handleSalesDashboard)
	mux.HandleFunc("GET /database", a.handleBukupayDatabase)
	mux.HandleFunc("POST /bukupay/collect", a.handleBukupayCollect)
}

func (a *app) handleSalesDashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	summary, err := a.store.SalesDashboardSummary(r.Context(), now, jakartaLocation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pipeline, err := a.store.BukupayPipelineStats(r.Context(), now, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := salesDashboardData{
		Summary:  summary,
		Pipeline: pipeline,
		Today:    now.In(jakartaLocation).Format("02 Jan 2006"),
	}
	if err := salesDashboardTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

//go:embed sales_dashboard.html
var salesDashboardHTML string
