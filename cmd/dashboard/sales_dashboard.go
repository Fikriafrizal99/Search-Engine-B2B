package main

import (
	_ "embed"
	"html/template"
	"net/http"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type salesDashboardData struct {
	Summary prospectstore.SalesDashboardSummary
	Today   string
}

var salesDashboardTmpl = template.Must(template.New("sales-dashboard").Parse(salesDashboardHTML))

func registerSalesDashboardRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("GET /sales", a.handleSalesDashboard)
}

func (a *app) handleSalesDashboard(w http.ResponseWriter, r *http.Request) {
	summary, err := a.store.SalesDashboardSummary(r.Context(), time.Now(), jakartaLocation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := salesDashboardData{Summary: summary, Today: time.Now().In(jakartaLocation).Format("02 Jan 2006")}
	if err := salesDashboardTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

//go:embed sales_dashboard.html
var salesDashboardHTML string
