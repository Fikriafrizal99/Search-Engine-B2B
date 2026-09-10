package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestVisitSessionDailyReportEndpointAndUI(t *testing.T) {
	_, mux, plan := setupP0RouteTest(t)
	prospectID := plan.Items[0].Prospect.ID
	form := url.Values{
		"channel": {"visit"},
		"result": {"interested"},
		"owner": {"Pak Dedi"},
		"note": {"Minta dilanjutkan registrasi"},
		"mode": {"all"},
		"plan_id": {fmt.Sprint(plan.ID)},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/contact/%d/result", prospectID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("visit POST: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/contact?plan_id=%d", plan.ID), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("visit session: %d %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"daily-visit-report-template", "Report Harian", "Copy Report", "/assets/daily-visit-report.js"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("Visit Session missing daily report UI %q", want)
		}
	}

	w = httptest.NewRecorder()
	query := url.Values{"date": {plan.PlanDate}, "location": {plan.LocationScope}}
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/visit-report?"+query.Encode(), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("daily report endpoint: %d %s", w.Code, w.Body.String())
	}
	var response dailyVisitReportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.TotalVisited != 1 || response.RouteTarget != 2 || response.RouteDone != 1 {
		t.Fatalf("unexpected report summary: %+v", response)
	}
	for _, want := range []string{"BUKUPAY — DAILY VISIT REPORT", "Tertarik", "Interested", "Pak Dedi", "Lanjut registrasi"} {
		if !strings.Contains(response.Text, want) {
			t.Fatalf("daily report text missing %q: %s", want, response.Text)
		}
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/daily-visit-report.js", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "/api/visit-report") {
		t.Fatalf("daily report asset: %d %s", w.Code, w.Body.String())
	}
}
