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

type visitCorrectionPageData struct {
	Merchant   prospectstore.Merchant
	Prospect   prospectstore.Prospect
	Visit      prospectstore.VisitHistoryEntry
	FormAction string
	ReturnURL  string
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
	return visitCorrectionPageData{
		Merchant: merchant,
		Prospect: prospect.Prospect,
		Visit: visits[0],
		FormAction: fmt.Sprintf("/merchant/%d/visit/%d/edit", merchant.ID, visitID),
		ReturnURL: fmt.Sprintf("/merchant/%d?visit_corrected=1", merchant.ID),
	}, nil
}

func (a *app) loadContactVisitCorrection(r *http.Request) (visitCorrectionPageData, error) {
	prospectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || prospectID <= 0 {
		return visitCorrectionPageData{}, &visitCorrectionError{"invalid prospect id", http.StatusBadRequest}
	}
	visitID, err := strconv.ParseInt(r.PathValue("visitID"), 10, 64)
	if err != nil || visitID <= 0 {
		return visitCorrectionPageData{}, &visitCorrectionError{"invalid visit id", http.StatusBadRequest}
	}
	record, err := a.store.Get(r.Context(), prospectID)
	if err != nil {
		return visitCorrectionPageData{}, &visitCorrectionError{err.Error(), http.StatusNotFound}
	}
	visits, err := a.store.VisitHistory(r.Context(), prospectID, 1)
	if err != nil {
		return visitCorrectionPageData{}, &visitCorrectionError{err.Error(), http.StatusInternalServerError}
	}
	if len(visits) == 0 || visits[0].ID != visitID {
		return visitCorrectionPageData{}, &visitCorrectionError{"hanya visit terbaru yang dapat dikoreksi", http.StatusBadRequest}
	}
	formAction := fmt.Sprintf("/contact/%d/visit/%d/edit", prospectID, visitID)
	returnURL := fmt.Sprintf("/contact?id=%d", prospectID)
	if planID, parseErr := parseOptionalPositiveInt64(r.URL.Query().Get("plan_id")); parseErr == nil && planID > 0 {
		formAction += "?plan_id=" + strconv.FormatInt(planID, 10)
		returnURL = fmt.Sprintf("/contact?plan_id=%d&id=%d", planID, prospectID)
	}
	return visitCorrectionPageData{Prospect: record.Prospect, Visit: visits[0], FormAction: formAction, ReturnURL: returnURL}, nil
}

type visitCorrectionError struct {
	message string
	status  int
}

func (e *visitCorrectionError) Error() string { return e.message }

func renderVisitCorrectionError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*visitCorrectionError); ok {
		http.Error(w, e.message, e.status)
		return true
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
	return true
}

func (a *app) handleVisitCorrectionForm(w http.ResponseWriter, r *http.Request) {
	data, err := a.loadVisitCorrection(r)
	if renderVisitCorrectionError(w, err) {
		return
	}
	renderPhase2(w, visitCorrectionTmpl, data)
}

func (a *app) handleContactVisitCorrectionForm(w http.ResponseWriter, r *http.Request) {
	data, err := a.loadContactVisitCorrection(r)
	if renderVisitCorrectionError(w, err) {
		return
	}
	renderPhase2(w, visitCorrectionTmpl, data)
}

func (a *app) applyVisitCorrection(w http.ResponseWriter, r *http.Request, data visitCorrectionPageData) bool {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form koreksi tidak valid", http.StatusBadRequest)
		return false
	}
	result := strings.TrimSpace(strings.ToLower(r.FormValue("result")))
	if !prospectstore.IsFieldVisitResult(result) {
		http.Error(w, "hasil koreksi visit tidak valid", http.StatusBadRequest)
		return false
	}
	var next time.Time
	if raw := strings.TrimSpace(r.FormValue("next_follow_up")); raw != "" {
		var err error
		next, err = time.ParseInLocation("2006-01-02T15:04", raw, jakartaLocation)
		if err != nil {
			http.Error(w, "format next follow-up tidak valid", http.StatusBadRequest)
			return false
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
		return false
	}
	return true
}

func (a *app) handleVisitCorrection(w http.ResponseWriter, r *http.Request) {
	data, err := a.loadVisitCorrection(r)
	if renderVisitCorrectionError(w, err) {
		return
	}
	if !a.applyVisitCorrection(w, r, data) {
		return
	}
	http.Redirect(w, r, data.ReturnURL, http.StatusSeeOther)
}

func (a *app) handleContactVisitCorrection(w http.ResponseWriter, r *http.Request) {
	data, err := a.loadContactVisitCorrection(r)
	if renderVisitCorrectionError(w, err) {
		return
	}
	if !a.applyVisitCorrection(w, r, data) {
		return
	}
	http.Redirect(w, r, data.ReturnURL, http.StatusSeeOther)
}

//go:embed visit_correction.html
var visitCorrectionHTML string
