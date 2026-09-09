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

type phase3RoadRouter struct{}

func (phase3RoadRouter) Matrix(_ context.Context, points []prospectstore.RoutePoint) (prospectstore.RoadMatrix, error) {
	n := len(points)
	out := prospectstore.RoadMatrix{Distances: make([][]float64, n), Durations: make([][]float64, n)}
	for i := 0; i < n; i++ {
		out.Distances[i] = make([]float64, n)
		out.Durations[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			delta := i - j
			if delta < 0 {
				delta = -delta
			}
			out.Distances[i][j] = float64(delta) * 1200
			out.Durations[i][j] = float64(delta) * 300
		}
	}
	return out, nil
}

func TestPhase3OperationalPagesRender(t *testing.T) {
	dir := t.TempDir()
	s, err := prospectstore.Open(filepath.Join(dir, "phase3.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SetRoadRouter(phase3RoadRouter{})

	scope := "Tebet Timur, Tebet, Kota Administrasi Jakarta Selatan, Daerah Khusus Ibukota Jakarta, Indonesia"
	csvPath := filepath.Join(dir, "merchant.csv")
	csv := "title,category,address,phone,website,latitude,longitude,rating,review_count,maps_url\n" +
		"Warung Phase 3,Warung,Jl. Tebet Raya 1,081234567890,https://example.com,-6.230100,106.850100,4.6,120,https://maps.google.com/?q=-6.2301%2C106.8501\n"
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
	if err != nil || len(records) != 1 {
		t.Fatalf("records=%d err=%v", len(records), err)
	}
	merchant, err := s.EnsureMerchant(context.Background(), records[0].Prospect.ID, prospectstore.MerchantToVisit)
	if err != nil {
		t.Fatal(err)
	}

	a := &app{store: s}
	mux := http.NewServeMux()
	registerContactRoutes(mux, a)
	checks := []struct {
		path string
		want string
	}{
		{"/contact?mode=all", "Visit Session"},
		{"/database", "Database &amp; Scraper"},
		{"/visit-plans/new?location=" + url.QueryEscape(scope), "Buat Rute Kunjungan"},
		{fmt.Sprintf("/merchant/%d", merchant.ID), "Merchant Detail"},
		{"/assets/phase3.css", ".phase3"},
	}
	for _, tc := range checks {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("GET %s missing %q", tc.path, tc.want)
		}
		if strings.Contains(w.Body.String(), "ZgotmplZ") {
			t.Fatalf("GET %s contains unsafe template output", tc.path)
		}
	}

	plan, err := s.CreateDailyVisitPlan(context.Background(), prospectstore.DailyVisitPlanInput{
		PlanDate:      time.Now().In(jakartaLocation).AddDate(0, 0, 1),
		LocationScope: scope,
		StartLat:      -6.231,
		StartLon:      106.851,
		TargetCount:   25,
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/visit-plan/%d", plan.ID), nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Route Execution") || !strings.Contains(w.Body.String(), "OSRM road routing") {
		t.Fatalf("route page: %d %s", w.Code, w.Body.String())
	}
}
