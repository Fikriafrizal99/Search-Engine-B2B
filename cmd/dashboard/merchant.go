package main

import (
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type merchantPipelinePageData struct {
	Items  []prospectstore.MerchantPipelineItem
	Stats  prospectstore.MerchantPipelineStats
	Status string
}

type merchantPageData struct {
	Prospect prospectstore.Prospect
	Merchant prospectstore.Merchant
	History  []prospectstore.MerchantEvent
}

var merchantFuncs = template.FuncMap{
	"merchantLabel": merchantStatusLabel,
	"contactTime":   contactTime,
	"inputTime":     merchantInputTime,
	"wa":            waNumber,
}

var merchantsTmpl = template.Must(template.New("merchants").Funcs(merchantFuncs).Parse(merchantsHTML))
var merchantTmpl = template.Must(template.New("merchant").Funcs(merchantFuncs).Parse(merchantHTML))

func registerMerchantRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("GET /merchants", a.handleMerchantPipeline)
	mux.HandleFunc("GET /merchant/{id}", a.handleMerchant)
	mux.HandleFunc("POST /merchant/{id}", a.handleMerchantUpdate)
	mux.HandleFunc("GET /merchant/prospect/{id}", a.handleEnsureMerchant)
	registerAreaRoutes(mux, a)
}

func (a *app) handleMerchantPipeline(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	if status == "" {
		status = "all"
	}
	items, err := a.store.ListMerchantPipeline(r.Context(), status, time.Now(), 500)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	stats, err := a.store.MerchantPipelineStats(r.Context(), time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := merchantsTmpl.Execute(w, merchantPipelinePageData{Items: items, Stats: stats, Status: status}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleEnsureMerchant(w http.ResponseWriter, r *http.Request) {
	prospectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || prospectID <= 0 {
		http.Error(w, "invalid prospect id", http.StatusBadRequest)
		return
	}
	m, err := a.store.EnsureMerchant(r.Context(), prospectID, prospectstore.MerchantToVisit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/merchant/%d", m.ID), http.StatusSeeOther)
}

func (a *app) handleMerchant(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid merchant id", http.StatusBadRequest)
		return
	}
	m, err := a.store.GetMerchant(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	record, err := a.store.Get(r.Context(), m.ProspectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	history, err := a.store.MerchantHistory(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := merchantTmpl.Execute(w, merchantPageData{Prospect: record.Prospect, Merchant: m, History: history}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleMerchantUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid merchant id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var nextActionAt time.Time
	if raw := strings.TrimSpace(r.FormValue("next_action_at")); raw != "" {
		nextActionAt, err = time.ParseInLocation("2006-01-02T15:04", raw, jakartaLocation)
		if err != nil {
			http.Error(w, "format next action tidak valid", http.StatusBadRequest)
			return
		}
	}
	_, err = a.store.UpdateMerchant(r.Context(), id, prospectstore.MerchantInput{
		Status:             r.FormValue("status"),
		MerchantType:       r.FormValue("merchant_type"),
		PICName:            r.FormValue("pic_name"),
		PICRole:            r.FormValue("pic_role"),
		HasQRIS:            r.FormValue("has_qris") == "1",
		QRISProvider:       r.FormValue("qris_provider"),
		HasSoundbox:        r.FormValue("has_soundbox") == "1",
		TrafficLevel:       r.FormValue("traffic_level"),
		TransactionLevel:   r.FormValue("transaction_level"),
		InterestLevel:      r.FormValue("interest_level"),
		RegistrationStatus: r.FormValue("registration_status"),
		InstallationStatus: r.FormValue("installation_status"),
		ActivationStatus:   r.FormValue("activation_status"),
		NextAction:         r.FormValue("next_action"),
		NextActionAt:       nextActionAt,
		Owner:              r.FormValue("owner"),
		Notes:              r.FormValue("notes"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/merchant/%d?saved=1", id), http.StatusSeeOther)
}

func merchantStatusLabel(v string) string {
	return labels(map[string]string{
		"to_visit": "To Visit", "visited": "Visited", "presented": "Presented", "interested": "Interested",
		"follow_up": "Follow Up", "registration": "Registration", "registered": "Registered",
		"installation": "Installation", "installed": "Installed", "active": "Active",
		"not_interested": "Tidak Tertarik", "owner_not_found": "Owner/PIC Tidak Ada",
		"already_soundbox": "Sudah Punya Soundbox", "closed": "Tutup", "invalid_lead": "Invalid Lead",
	}, v, v)
}

func merchantInputTime(v string) string {
	if strings.TrimSpace(v) == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return ""
	}
	return t.In(jakartaLocation).Format("2006-01-02T15:04")
}

//go:embed merchants.html
var merchantsHTML string

//go:embed merchant.html
var merchantHTML string
