package main

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type manualVisitForm struct {
	Title         string
	Category      string
	Phone         string
	Address       string
	LocationScope string
	MapsURL       string
}

type manualVisitPageData struct {
	Form  manualVisitForm
	Error string
}

var manualVisitTmpl = template.Must(template.Must(template.New("manual-visit").Parse(manualVisitHTML)).ParseFS(uiAssets, "ui/shared.html"))

func registerManualVisitRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("GET /contact/manual", a.handleManualVisitForm)
	mux.HandleFunc("POST /contact/manual", a.handleManualVisitCreate)
}

func (a *app) handleManualVisitForm(w http.ResponseWriter, r *http.Request) {
	renderManualVisit(w, http.StatusOK, manualVisitPageData{})
}

func (a *app) handleManualVisitCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderManualVisit(w, http.StatusBadRequest, manualVisitPageData{Error: "Form tidak dapat dibaca."})
		return
	}
	form := manualVisitForm{
		Title:         strings.TrimSpace(r.FormValue("title")),
		Category:      strings.TrimSpace(r.FormValue("category")),
		Phone:         strings.TrimSpace(r.FormValue("phone")),
		Address:       strings.TrimSpace(r.FormValue("address")),
		LocationScope: strings.TrimSpace(r.FormValue("location_scope")),
		MapsURL:       strings.TrimSpace(r.FormValue("maps_url")),
	}
	id, _, err := a.store.CreateManualProspect(r.Context(), prospectstore.ManualProspectInput{
		Title:         form.Title,
		Category:      form.Category,
		Phone:         form.Phone,
		Address:       form.Address,
		LocationScope: form.LocationScope,
		MapsURL:       form.MapsURL,
	})
	if err != nil {
		renderManualVisit(w, http.StatusBadRequest, manualVisitPageData{Form: form, Error: err.Error()})
		return
	}
	if strings.TrimSpace(r.FormValue("action")) == "list" {
		http.Redirect(w, r, "/database", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/contact?id="+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func renderManualVisit(w http.ResponseWriter, status int, data manualVisitPageData) {
	var b bytes.Buffer
	if err := manualVisitTmpl.Execute(&b, data); err != nil {
		renderPhase2Error(w, http.StatusInternalServerError, "Tambah Visit Manual", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(b.Bytes())
}

//go:embed manual_visit.html
var manualVisitHTML string
