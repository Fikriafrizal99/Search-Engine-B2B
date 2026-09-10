package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

func setupP0RouteTest(t *testing.T) (*app, *http.ServeMux, prospectstore.VisitPlan) {
	t.Helper()
	dir := t.TempDir()
	s, err := prospectstore.Open(filepath.Join(dir, "p0.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	scope := "Tebet Timur, Tebet, Kota Administrasi Jakarta Selatan, Daerah Khusus Ibukota Jakarta, Indonesia"
	csvPath := filepath.Join(dir, "merchant.csv")
	csv := "title,category,address,phone,website,latitude,longitude,rating,review_count,maps_url\n" +
		"Warung P0 Satu,Warung,Jl. Tebet Raya 1,081111111111,,-6.230100,106.850100,4.6,120,https://maps.google.com/?q=-6.2301%2C106.8501\n" +
		"Warung P0 Dua,Warung,Jl. Tebet Raya 2,082222222222,,-6.230300,106.850300,4.5,80,https://maps.google.com/?q=-6.2303%2C106.8503\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportCSV(context.Background(), csvPath, scope); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterScrape(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	records, err := s.List(context.Background(), prospectstore.Filter{Location: scope, Limit: 10})
	if err != nil || len(records) != 2 {
		t.Fatalf("records=%d err=%v", len(records), err)
	}
	for _, record := range records {
		if _, err := s.EnsureMerchant(context.Background(), record.Prospect.ID, prospectstore.MerchantToVisit); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := s.CreateDailyVisitPlan(context.Background(), prospectstore.DailyVisitPlanInput{
		PlanDate:      time.Now().In(jakartaLocation),
		LocationScope: scope,
		StartLat:      -6.231,
		StartLon:      106.851,
		TargetCount:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Clean(filepath.Join(cwd, "../../config"))
	a := &app{store: s, dbPath: filepath.Join(dir, "p0.db"), configDir: configDir, collectorPath: "/bin/true", enginePath: "/bin/true"}
	mux := http.NewServeMux()
	registerContactRoutes(mux, a)
	return a, mux, plan
}

func TestRouteAwareVisitSessionAdvancesAndCompletes(t *testing.T) {
	_, mux, plan := setupP0RouteTest(t)
	if len(plan.Items) != 2 {
		t.Fatalf("route items=%d want 2", len(plan.Items))
	}
	first, second := plan.Items[0].Prospect.ID, plan.Items[1].Prospect.ID

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/contact?plan_id=%d", plan.ID), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("route contact GET: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Merchant 1 / 2") || !strings.Contains(body, fmt.Sprintf("name=\"plan_id\" value=\"%d\"", plan.ID)) {
		t.Fatalf("route context missing: %s", body)
	}
	if !strings.Contains(body, "Rute Aktif") {
		t.Fatalf("route active marker missing")
	}

	form := url.Values{"channel": {"visit"}, "result": {"visited"}, "mode": {"all"}, "plan_id": {fmt.Sprint(plan.ID)}}
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/contact/%d/result", first), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("first visit POST: %d %s", w.Code, w.Body.String())
	}
	wantNext := fmt.Sprintf("/contact?plan_id=%d&id=%d", plan.ID, second)
	if got := w.Header().Get("Location"); got != wantNext {
		t.Fatalf("first redirect=%q want %q", got, wantNext)
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, wantNext, nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Merchant 2 / 2") {
		t.Fatalf("second route page: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/contact/%d/result", second), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("last visit POST: %d %s", w.Code, w.Body.String())
	}
	wantDone := fmt.Sprintf("/visit-plan/%d?completed=1", plan.ID)
	if got := w.Header().Get("Location"); got != wantDone {
		t.Fatalf("last redirect=%q want %q", got, wantDone)
	}
}

func TestVisitSessionPrefersTodayRouteButExplicitQueueStillWorks(t *testing.T) {
	_, mux, plan := setupP0RouteTest(t)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contact", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), fmt.Sprintf("Rute Aktif · 1/%d", len(plan.Items))) {
		t.Fatalf("plain /contact did not prefer today's route: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contact?mode=all", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Action Queue") {
		t.Fatalf("explicit action queue unavailable: %d %s", w.Code, w.Body.String())
	}
}

func TestBukupayCollectReturnsJSONWithoutDatabaseRedirect(t *testing.T) {
	a, mux, _ := setupP0RouteTest(t)
	form := url.Values{
		"location":    {"Tebet Timur, Tebet, Jakarta Selatan, DKI Jakarta, Indonesia"},
		"query_mode":  {"defaults_only"},
		"depth":       {"1"},
		"concurrency": {"1"},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bukupay/collect", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("collect start: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), `"started":true`) {
		t.Fatalf("collect should stay in Area Planner, headers=%v body=%s", w.Header(), w.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for a.collectStatus().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}

func TestP0SalesStagesExcludeCoverageStatuses(t *testing.T) {
	stages, exceptions := merchantStages(prospectstore.BukupayPipelineStats{}, prospectstore.MerchantListFilter{Status: "all"})
	if len(stages) != 8 || stages[0].Status != "presented" || stages[len(stages)-1].Status != "active" {
		t.Fatalf("unexpected sales stages: %#v", stages)
	}
	for _, stage := range append(append([]pipelineStage{}, stages...), exceptions...) {
		if stage.Status == "to_visit" || stage.Status == "visited" || stage.Status == "owner_not_found" {
			t.Fatalf("coverage status leaked into sales UI: %s", stage.Status)
		}
	}
}

func TestAreaPlannerScriptPollsScraperWithoutNavigation(t *testing.T) {
	data, err := uiAssets.ReadFile("ui/area-planner.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	if !strings.Contains(script, "/api/collect/status") || !strings.Contains(script, "/collect/cancel") {
		t.Fatalf("area planner scraper polling/cancel missing")
	}
	if strings.Contains(script, "window.location.assign") {
		t.Fatalf("area planner still navigates away after scrape")
	}
}
