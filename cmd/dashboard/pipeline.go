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

type pipelinePageData struct {
	Items  []prospectstore.PipelineItem
	Stats  prospectstore.PipelineStats
	Status string
}

type opportunityPageData struct {
	Prospect      prospectstore.Prospect
	Opportunity   prospectstore.Opportunity
	History       []prospectstore.OpportunityEvent
	Submission    prospectstore.Submission
	HasSubmission bool
}

var pipelineFuncs = template.FuncMap{
	"oppLabel":        opportunityStatusLabel,
	"submissionLabel": submissionStatusLabel,
	"contactTime":     contactTime,
	"inputTime":       opportunityInputTime,
	"money":           moneyIDR,
	"wa":              waNumber,
}

var pipelineTmpl = template.Must(template.New("pipeline").Funcs(pipelineFuncs).Parse(pipelineHTML))
var opportunityTmpl = template.Must(template.New("opportunity").Funcs(pipelineFuncs).Parse(opportunityHTML))

func registerPipelineRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("GET /pipeline", a.handlePipeline)
	mux.HandleFunc("GET /opportunity/{id}", a.handleOpportunity)
	mux.HandleFunc("POST /opportunity/{id}", a.handleOpportunityUpdate)
	mux.HandleFunc("POST /opportunity/{id}/submission", a.handleCreateSubmission)
	mux.HandleFunc("POST /submission/{id}", a.handleSubmissionUpdate)
}

func (a *app) handlePipeline(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	if status == "" {
		status = "all"
	}
	items, err := a.store.ListPipeline(r.Context(), status, time.Now(), 300)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	stats, err := a.store.PipelineStats(r.Context(), time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := pipelineTmpl.Execute(w, pipelinePageData{Items: items, Stats: stats, Status: status}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleOpportunity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid opportunity id", http.StatusBadRequest)
		return
	}
	o, err := a.store.GetOpportunity(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	record, err := a.store.Get(r.Context(), o.ProspectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	history, err := a.store.OpportunityHistory(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sub, hasSub, err := a.store.LatestSubmission(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := opportunityPageData{Prospect: record.Prospect, Opportunity: o, History: history, Submission: sub, HasSubmission: hasSub}
	if err := opportunityTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleOpportunityUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid opportunity id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	unitYear, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("unit_year")))
	var nextActionAt time.Time
	if raw := strings.TrimSpace(r.FormValue("next_action_at")); raw != "" {
		nextActionAt, err = time.ParseInLocation("2006-01-02T15:04", raw, jakartaLocation)
		if err != nil {
			http.Error(w, "format next action tidak valid", http.StatusBadRequest)
			return
		}
	}
	_, err = a.store.UpdateOpportunity(r.Context(), id, prospectstore.OpportunityInput{
		Status:           r.FormValue("status"),
		Product:          r.FormValue("product"),
		CustomerNeed:     r.FormValue("customer_need"),
		UnitType:         r.FormValue("unit_type"),
		UnitModel:        r.FormValue("unit_model"),
		UnitYear:         unitYear,
		Ownership:        r.FormValue("ownership"),
		PreferredContact: r.FormValue("preferred_contact"),
		NextAction:       r.FormValue("next_action"),
		NextActionAt:     nextActionAt,
		Owner:            r.FormValue("owner"),
		Notes:            r.FormValue("notes"),
		DocKTP:           r.FormValue("doc_ktp") == "1",
		DocSTNK:          r.FormValue("doc_stnk") == "1",
		DocBPKB:          r.FormValue("doc_bpkb") == "1",
		DocAdditional:    r.FormValue("doc_additional") == "1",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/opportunity/%d?saved=1", id), http.StatusSeeOther)
}

func (a *app) handleCreateSubmission(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid opportunity id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	_, err = a.store.CreateSubmission(r.Context(), id, prospectstore.SubmissionInput{
		Partner:     r.FormValue("partner"),
		Product:     r.FormValue("product"),
		ReferenceNo: r.FormValue("reference_no"),
		OutcomeNote: r.FormValue("outcome_note"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/opportunity/%d?submitted=1", id), http.StatusSeeOther)
}

func (a *app) handleSubmissionUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid submission id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	current, err := a.store.GetSubmission(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	amount, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("disbursed_amount")), 64)
	_, err = a.store.UpdateSubmission(r.Context(), id, prospectstore.SubmissionInput{
		Partner:         r.FormValue("partner"),
		Product:         r.FormValue("product"),
		ReferenceNo:     r.FormValue("reference_no"),
		Status:          r.FormValue("status"),
		OutcomeNote:     r.FormValue("outcome_note"),
		DisbursedAmount: amount,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/opportunity/%d?submission_saved=1", current.OpportunityID), http.StatusSeeOther)
}

func opportunityStatusLabel(v string) string {
	return labels(map[string]string{
		"qualifying": "Qualifying", "qualified": "Qualified", "docs_pending": "Docs Pending",
		"ready_to_submit": "Ready to Submit", "submitted": "Submitted", "not_qualified": "Not Qualified", "cancelled": "Cancelled",
	}, v, v)
}

func submissionStatusLabel(v string) string {
	return labels(map[string]string{
		"submitted": "Submitted", "processing": "Processing", "approved": "Approved",
		"rejected": "Rejected", "cancelled": "Cancelled", "disbursed": "Disbursed",
	}, v, v)
}

func opportunityInputTime(v string) string {
	if strings.TrimSpace(v) == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return ""
	}
	return t.In(jakartaLocation).Format("2006-01-02T15:04")
}

func moneyIDR(v float64) string {
	if v <= 0 {
		return "-"
	}
	raw := strconv.FormatInt(int64(v+0.5), 10)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + "." + raw[i:]
	}
	return "Rp" + raw
}

//go:embed pipeline.html
var pipelineHTML string

//go:embed opportunity.html
var opportunityHTML string
