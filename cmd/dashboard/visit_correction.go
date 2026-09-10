package main

import (
	_ "embed"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type visitCorrectionPageData struct {
	Merchant prospectstore.Merchant
	Prospect prospectstore.Prospect
	Visit    prospectstore.VisitHistoryEntry
}

var visitCorrectionTmpl = template.Must(template.Must(template.New("visit-correction").Funcs(merchantFuncs).Parse(visitCorrectionHTML)).ParseFS(uiAssets, "ui/shared.html"))

func (a *app) loadVisitCorrection(r *http.Request) (visitCorrectionPageData, error) {
	merchantID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || merchantID <= 0 {
		return visitCorrectionPageData{}, &visitCorrectionError{"invalid merchant id", http.StatusBadRequest}
	}
	visitID, err := strconv.ParseInt(r.PathValue("visitID"), 10, 64)
	if err != nil || visitID <= 0 {
		return visitCorrectionPageData{}, &visitCorrectionError{"invalid visit id", http.StatusBadRequest}
	}
	merchant, err := a.store.GetMerchant(r.Context(), merchantID)
	if err != nil {
		return visitCorrectionPageData{}, &visitCorrectionError{err.Error(), http.StatusNotFound}
	}
	prospect, err := a.store.Get(r.Context(), merchant.ProspectID)
	if err != nil {
		return visitCorrectionPageData{}, &visitCorrectionError{err.Error(), http.StatusNotFound}
	}
	visits, err := a.store.VisitHistory(r.Context(), merchant.ProspectID, 1)
	if err != nil {
		return visitCorrectionPageData{}, &visitCorrectionError{err.Error(), http.StatusInternalServerError}
	}
	if len(visits) == 0 || visits[0].ID != visitID {
		return visitCorrectionPageData{}, &visitCorrectionError{"hanya visit terbaru yang dapat dikoreksi", http.StatusBadRequest}
	}
	return visitCorrectionPageData{Merchant: merchant, Prospect: prospect.Prospect, Visit: visits[0]}, nil
}

type visitCorrectionError struct {
	message string
	status  int
}

func (e *visitCorrectionError) Error() string { return e.message }

func (a *app) handleVisitCorrectionForm(w http.ResponseWriter, r *http.Request) {
	data, err := a.loadVisitCorrection(r)
	if err != nil {
		if e, ok := err.(*visitCorrectionError); ok {
			http.Error(w, e.message, e.status)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderPhase2(w, visitCorrectionTmpl, data)
}

func (a *app) handleVisitCorrection(w http.ResponseWriter, r *http.Request) {
	data, err := a.loadVisitCorrection(r)
	if err != nil {
		if e, ok := err.(*visitCorrectionError); ok {
			http.Error(w, e.message, e.status)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form koreksi tidak valid", http.StatusBadRequest)
		return
	}
	result := strings.TrimSpace(strings.ToLower(r.FormValue("result")))
	if !prospectstore.IsFieldVisitResult(result) {
		http.Error(w, "hasil koreksi visit tidak valid", http.StatusBadRequest)
		return
	}
	var next time.Time
	if raw := strings.TrimSpace(r.FormValue("next_follow_up")); raw != "" {
		next, err = time.ParseInLocation("2006-01-02T15:04", raw, jakartaLocation)
		if err != nil {
			http.Error(w, "format next follow-up tidak valid", http.StatusBadRequest)
			return
		}
	}
	if _, err := a.store.CorrectLatestVisit(r.Context(), data.Visit.ID, prospectstore.VisitResultInput{
		ProspectID:   data.Prospect.ID,
		Result:       result,
		PICName:      strings.TrimSpace(r.FormValue("owner")),
		Note:         strings.TrimSpace(r.FormValue("note")),
		NextActionAt: next,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/merchant/"+strconv.FormatInt(data.Merchant.ID, 10)+"?visit_corrected=1", http.StatusSeeOther)
}

//go:embed visit_correction.html
var visitCorrectionHTML string
