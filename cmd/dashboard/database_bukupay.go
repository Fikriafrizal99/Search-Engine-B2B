package main

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/http"
)

type bukupayDatabaseData struct {
	Dashboard       dashboardData
	PhoneCoverage   float64
	WebsiteCoverage float64
}

type captureResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *captureResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *captureResponseWriter) WriteHeader(status int) { w.status = status }

func (w *captureResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

var bukupayDatabaseTmpl = template.Must(template.Must(template.New("bukupay-database").Funcs(funcs).Parse(bukupayDatabaseHTML)).ParseFS(uiAssets, "ui/shared.html"))

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
	if st.Total > 0 {
		data.PhoneCoverage = 100 * float64(st.WithPhone) / float64(st.Total)
		data.WebsiteCoverage = 100 * float64(st.WithWebsite) / float64(st.Total)
	}
	renderPhase2(w, bukupayDatabaseTmpl, data)
}

func (a *app) handleBukupayCollect(w http.ResponseWriter, r *http.Request) {
	capture := &captureResponseWriter{}
	a.handleCollect(capture, r)
	if capture.status >= 400 {
		for k, values := range capture.Header() {
			for _, value := range values {
				w.Header().Add(k, value)
			}
		}
		w.WriteHeader(capture.status)
		_, _ = w.Write(capture.body.Bytes())
		return
	}
	http.Redirect(w, r, "/database?collect=started", http.StatusSeeOther)
}

//go:embed database_bukupay.html
var bukupayDatabaseHTML string
