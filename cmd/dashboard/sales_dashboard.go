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
	mux.HandleFunc("GET /sales", a.handleSalesDashboard)
	mux.HandleFunc("GET /database", a.handleBukupayDatabase)
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
