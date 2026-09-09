package main

import (
	_ "embed"
	"html/template"
	"net/http"
)

type bukupayDatabaseData struct {
	Dashboard dashboardData
}

var bukupayDatabaseTmpl = template.Must(template.New("bukupay-database").Funcs(funcs).Parse(bukupayDatabaseHTML))

func (a *app) handleBukupayDatabase(w http.ResponseWriter, r *http.Request) {
	f := filterFromRequest(r)
	f.Limit = 500
	records, err := a.store.List(r.Context(), f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	st, err := a.store.Stats(r.Context(), f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := bukupayDatabaseData{Dashboard: dashboardData{Records: records, Stats: st, Filter: f, Collect: a.collectStatus()}}
	if err := bukupayDatabaseTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

//go:embed database_bukupay.html
var bukupayDatabaseHTML string
