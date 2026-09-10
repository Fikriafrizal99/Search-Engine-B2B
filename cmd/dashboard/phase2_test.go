package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

func TestPhase2PagesAndActions(t *testing.T) {
	s, err := prospectstore.Open(filepath.Join(t.TempDir(), "phase2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := &app{store: s}
	mux := http.NewServeMux()
	registerContactRoutes(mux, a)
	for _, path := range []string{"/sales", "/areas", "/merchants", "/merchants?q=hello&due=1&status=follow_up&location=Test", "/assets/phase2.css", "/assets/scrape-mode.css", "/assets/area-planner.js"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if !strings.HasPrefix(path, "/assets/") {
			if !strings.Contains(w.Body.String(), "ui-topbar") || strings.Contains(w.Body.String(), "ZgotmplZ") {
				t.Fatalf("invalid V2 page: %s", path)
			}
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/areas", nil))
	for _, want := range []string{"Pengaturan Pencarian", "area-advanced", "value=\"auto\"", "value=\"manual\"", "manual-include-defaults", "scrape-query-mode", "scrape-runtime"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("Area Planner missing scrape mode control %q", want)
		}
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/coverage?location=Empty", nil))
	var coverage coverageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &coverage); err != nil || coverage.Exists || coverage.Snapshot.EligibleTomorrow != 0 {
		t.Fatalf("empty coverage: %s %v", w.Body.String(), err)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/merchants?status=invalid", nil))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "ui-card") {
		t.Fatalf("styled invalid filter: %d", w.Code)
	}
	// Validate the actual scrape endpoint and concurrency guard without launching
	// a paid/live scraping task or writing to the runtime's data directory.
	a.collect.Running = true
	form := url.Values{"location": {"Village, District, City, Province, Indonesia"}, "query_mode": {"defaults_only"}, "depth": {"5"}, "concurrency": {"2"}}
	req := httptest.NewRequest("POST", "/bukupay/collect", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "collector masih berjalan") {
		t.Fatalf("scrape guard: %d %s", w.Code, w.Body.String())
	}
	manual := url.Values{"location": {"Village, District, City, Province, Indonesia"}, "query_mode": {"custom"}, "keywords": {"warung madura\nagen gas"}, "depth": {"5"}, "concurrency": {"2"}}
	req = httptest.NewRequest("POST", "/bukupay/collect", strings.NewReader(manual.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "collector masih berjalan") {
		t.Fatalf("manual scrape mode not accepted before concurrency guard: %d %s", w.Code, w.Body.String())
	}
	manual.Del("keywords")
	req = httptest.NewRequest("POST", "/bukupay/collect", strings.NewReader(manual.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "isi minimal satu custom query") {
		t.Fatalf("empty manual scrape validation: %d %s", w.Code, w.Body.String())
	}
}

func TestAreaScrapeStartsCollector(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell collector fixture")
	}
	// runCollector uses a relative data directory: isolate the whole invocation.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	executable := filepath.Join(dir, "collector-fixture")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > collector-arguments.txt\n"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{collectorPath: executable, dbPath: filepath.Join(dir, "isolated.db"), configDir: filepath.Join(cwd, "../../config"), enginePath: "unused-test-engine"}
	form := url.Values{"location": {"Tebet Timur, Tebet, Jakarta Selatan, DKI Jakarta, Indonesia"}, "query_mode": {"defaults_only"}, "depth": {"5"}, "concurrency": {"2"}}
	request := httptest.NewRequest("POST", "/bukupay/collect", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.handleBukupayCollect(w, request)
	if w.Code != http.StatusOK || w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), `"started":true`) {
		t.Fatalf("scrape should start in-place: %d location=%q body=%s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for a.collectStatus().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if state := a.collectStatus(); state.Running || state.Phase != "Selesai" {
		t.Fatalf("collector did not complete: %+v", state)
	}
	args, err := os.ReadFile(filepath.Join(dir, "collector-arguments.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{form.Get("location"), "-include-defaults=true", "-depth\n5", "-c\n2", a.dbPath} {
		if !strings.Contains(string(args), want) {
			t.Errorf("collector arguments missing %q", want)
		}
	}
}
