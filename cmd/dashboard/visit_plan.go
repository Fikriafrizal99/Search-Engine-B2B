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

type visitPlanFormData struct {
	Location string
	PlanDate string
	Target   int
}

type visitPlanPageData struct {
	Plan                 prospectstore.VisitPlan
	RoadMetrics          prospectstore.VisitPlanRoadMetrics
	TotalDistanceKM      float64
	TotalDurationSeconds float64
	RoutingLabel         string
}

var visitPlanFormTmpl = template.Must(template.New("visit-plan-form").Parse(visitPlanFormHTML))
var visitPlanTmpl = template.Must(template.New("visit-plan").Funcs(template.FuncMap{
	"wa":            waNumber,
	"durationLabel": durationLabel,
}).Parse(visitPlanHTML))

func registerVisitPlanRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("GET /visit-plans/new", a.handleVisitPlanForm)
	mux.HandleFunc("POST /visit-plans", a.handleCreateVisitPlan)
	mux.HandleFunc("GET /visit-plan/{id}", a.handleVisitPlan)
}

func (a *app) handleVisitPlanForm(w http.ResponseWriter, r *http.Request) {
	location := strings.TrimSpace(r.URL.Query().Get("location"))
	tomorrow := time.Now().In(jakartaLocation).AddDate(0, 0, 1).Format("2006-01-02")
	if err := visitPlanFormTmpl.Execute(w, visitPlanFormData{Location: location, PlanDate: tomorrow, Target: 25}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleCreateVisitPlan(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form tidak valid", http.StatusBadRequest)
		return
	}
	location := strings.TrimSpace(r.FormValue("location"))
	if location == "" {
		http.Error(w, "lokasi wajib dipilih", http.StatusBadRequest)
		return
	}
	planDate, err := time.ParseInLocation("2006-01-02", r.FormValue("plan_date"), jakartaLocation)
	if err != nil {
		http.Error(w, "tanggal rencana tidak valid", http.StatusBadRequest)
		return
	}
	startLat, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("start_lat")), 64)
	if err != nil {
		http.Error(w, "latitude titik mulai tidak valid", http.StatusBadRequest)
		return
	}
	startLon, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("start_lon")), 64)
	if err != nil {
		http.Error(w, "longitude titik mulai tidak valid", http.StatusBadRequest)
		return
	}
	target, _ := strconv.Atoi(r.FormValue("target"))
	if target <= 0 {
		target = 25
	}
	plan, err := a.store.CreateDailyVisitPlan(r.Context(), prospectstore.DailyVisitPlanInput{
		PlanDate:      planDate,
		LocationScope: location,
		StartLat:      startLat,
		StartLon:      startLon,
		TargetCount:   target,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/visit-plan/%d", plan.ID), http.StatusSeeOther)
}

func (a *app) handleVisitPlan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "visit plan id tidak valid", http.StatusBadRequest)
		return
	}
	plan, err := a.store.GetVisitPlan(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	metrics, err := a.store.GetVisitPlanRoadMetrics(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var totalDistance, totalDuration float64
	for _, item := range plan.Items {
		totalDistance += item.DistanceFromPreviousKM
		totalDuration += metrics.DurationSeconds[item.ID]
	}
	routingLabel := "Haversine fallback"
	if metrics.RoutingSource == "osrm" {
		routingLabel = "OSRM road routing"
	}
	data := visitPlanPageData{
		Plan:                 plan,
		RoadMetrics:          metrics,
		TotalDistanceKM:      totalDistance,
		TotalDurationSeconds: totalDuration,
		RoutingLabel:         routingLabel,
	}
	if err := visitPlanTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func durationLabel(seconds float64) string {
	if seconds <= 0 {
		return "-"
	}
	minutes := int(seconds/60 + 0.5)
	if minutes < 1 {
		minutes = 1
	}
	if minutes < 60 {
		return fmt.Sprintf("%d menit", minutes)
	}
	hours := minutes / 60
	remaining := minutes % 60
	if remaining == 0 {
		return fmt.Sprintf("%d jam", hours)
	}
	return fmt.Sprintf("%d jam %d menit", hours, remaining)
}

//go:embed visit_plan_form.html
var visitPlanFormHTML string

//go:embed visit_plan.html
var visitPlanHTML string
