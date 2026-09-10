package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRouteVisitRejectsDownstreamSalesStage(t *testing.T) {
	_, mux, plan := setupP0RouteTest(t)
	prospectID := plan.Items[0].Prospect.ID
	form := url.Values{
		"channel": {"call"}, // crafted value must be ignored for a route visit
		"result":  {"installed"},
		"mode":    {"all"},
		"plan_id": {fmt.Sprint(plan.ID)},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/contact/%d/result", prospectID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("installed must be rejected as visit result: code=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Merchant Detail") {
		t.Fatalf("validation should explain where sales stage belongs: %s", w.Body.String())
	}
}

func TestVisitSessionSeparatesFieldResultsFromSalesStages(t *testing.T) {
	for _, want := range []string{
		`value="visited"`,
		`value="presented"`,
		`value="interested"`,
		`value="follow_up"`,
		`value="already_soundbox"`,
		`Sudah punya Soundbox sebelumnya`,
		`Registration / Installation / Installed / Active hanya diubah dari Sales Workspace`,
	} {
		if !strings.Contains(contactHTML, want) {
			t.Fatalf("visit session missing %q", want)
		}
	}
	for _, forbidden := range []string{`value="registered"`, `value="installed"`, `value="active"`} {
		if strings.Contains(contactHTML, forbidden) {
			t.Fatalf("downstream sales stage leaked into Visit Session: %s", forbidden)
		}
	}
}

func TestVisitSessionExposesLatestVisitCorrection(t *testing.T) {
	_, mux, plan := setupP0RouteTest(t)
	prospectID := plan.Items[0].Prospect.ID
	form := url.Values{"channel": {"visit"}, "result": {"visited"}, "mode": {"all"}, "plan_id": {fmt.Sprint(plan.ID)}}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/contact/%d/result", prospectID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("visit POST failed: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	contactURL := fmt.Sprintf("/contact?plan_id=%d&id=%d", plan.ID, prospectID)
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, contactURL, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("visit session: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Update Visit Merchant") || !strings.Contains(body, "Koreksi Visit Terakhir") || !strings.Contains(body, fmt.Sprintf("/contact/%d/visit/", prospectID)) || !strings.Contains(body, "/edit") {
		t.Fatalf("latest visit correction action missing from Visit Session: %s", body)
	}

	// Visit controls were intentionally moved out of Sales Workspace.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/merchant/prospect/%d", prospectID), nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("ensure merchant: %d %s", w.Code, w.Body.String())
	}
	merchantURL := w.Header().Get("Location")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, merchantURL, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("sales workspace: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "Koreksi Visit Terakhir") {
		t.Fatalf("visit correction action must stay in Visit Session, not Sales Workspace: %s", w.Body.String())
	}
}

func TestDatabasePolishMarkupAndResponsiveStyles(t *testing.T) {
	for _, want := range []string{
		`database-page`,
		`database-insights-grid`,
		`database-insight-card`,
		`database-insight-value`,
		`database-flow`,
		`databaseShortArea`,
	} {
		if !strings.Contains(bukupayDatabaseHTML, want) {
			t.Fatalf("database template missing %q", want)
		}
	}
	css, err := uiAssets.ReadFile("ui/layout-fixes.css")
	if err != nil {
		t.Fatal(err)
	}
	text := string(css)
	for _, want := range []string{
		`.database-page .database-insights-grid`,
		`.database-page .database-insight-card`,
		`.database-page .database-flow`,
		`font-size: 15px`,
		`@media (max-width: 640px)`,
		`grid-template-columns: 1fr`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("responsive typography/database CSS missing %q", want)
		}
	}
}
