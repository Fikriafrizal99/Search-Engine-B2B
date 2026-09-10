package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/collectorconfig"
)

func TestCollectStatusUsesPresetKeywordCount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell collector fixture")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(cwd, "../../config")
	preset, err := collectorconfig.LoadPreset(filepath.Join(configDir, "presets", "bukupay-merchants.json"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	collector := filepath.Join(dir, "collector-fixture")
	if err := os.WriteFile(collector, []byte("#!/bin/sh\nsleep 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	a := &app{collectorPath: collector, dbPath: filepath.Join(dir, "bukupay.db"), configDir: configDir, enginePath: "unused"}
	form := url.Values{
		"location":    {"Tebet Timur, Tebet, Jakarta Selatan, DKI Jakarta, Indonesia"},
		"query_mode":  {"defaults_only"},
		"depth":       {"5"},
		"concurrency": {"2"},
	}
	req := httptest.NewRequest("POST", "/bukupay/collect", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.handleBukupayCollect(w, req)

	want := fmt.Sprintf("%d query rekomendasi", len(preset.Keywords))
	if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
		t.Fatalf("collect status must use preset count %q: code=%d body=%s", want, w.Code, w.Body.String())
	}
}

func TestStabilizationAssetsAreServed(t *testing.T) {
	mux := http.NewServeMux()
	registerUIAssets(mux)

	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/assets/bukupay-ui-v2.css", []string{"sales-bottom-simplified", "merchant-pipeline .merchant-stages"}},
		{"/assets/phase2.css", []string{"merchant-pipeline .merchant-exceptions", "justify-content: center"}},
		{"/assets/area-planner.js", []string{"Area Irisan guardrail", "KOTA DEPOK", "CINERE", "PONDOK AREN"}},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != 200 {
			t.Fatalf("%s returned %d", tc.path, w.Code)
		}
		for _, want := range tc.want {
			if !strings.Contains(w.Body.String(), want) {
				t.Errorf("%s missing %q", tc.path, want)
			}
		}
	}
}

func TestCollectorRecoveryPhaseLabels(t *testing.T) {
	if got := phaseFromLog("PHASE scraper-shutdown-recovery", "Menjalankan scraper"); got != "Menutup scraper" {
		t.Fatalf("shutdown recovery label = %q", got)
	}
	if got := phaseFromLog("PHASE scraper-shutdown-recovery\nPHASE normalize-dedup", "Menjalankan scraper"); got != "Memproses hasil" {
		t.Fatalf("normalize label = %q", got)
	}
}

func TestRouteFormAutoSelectsSoleAvailableArea(t *testing.T) {
	a, _, plan := setupP0RouteTest(t)
	w := httptest.NewRecorder()
	a.handleVisitPlanForm(w, httptest.NewRequest(http.MethodGet, "/visit-plans/new", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("route form: %d %s", w.Code, w.Body.String())
	}
	want := `name="location" value="` + plan.LocationScope + `"`
	if !strings.Contains(w.Body.String(), want) {
		t.Fatalf("route form did not auto-select sole area %q: %s", plan.LocationScope, w.Body.String())
	}
}

func TestDashboardRouteCreationCarriesSoleArea(t *testing.T) {
	a, _, plan := setupP0RouteTest(t)
	w := httptest.NewRecorder()
	a.handleSalesDashboard(w, httptest.NewRequest(http.MethodGet, "/sales", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard: %d %s", w.Code, w.Body.String())
	}
	want := "location=" + url.QueryEscape(plan.LocationScope)
	if !strings.Contains(w.Body.String(), want) {
		t.Fatalf("dashboard route creation link missing sole area %q", plan.LocationScope)
	}
}
