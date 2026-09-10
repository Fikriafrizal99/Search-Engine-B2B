package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFinalPolishStylesAreServed(t *testing.T) {
	mux := http.NewServeMux()
	registerUIAssets(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/bukupay-ui-v2.css", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("stylesheet returned %d", w.Code)
	}
	css := w.Body.String()
	for _, want := range []string{
		`Final Bukupay UI polish`,
		`"Segoe UI Variable"`,
		`.ui-body .p3-result-panel`,
		`.area-advanced`,
		`@media (max-width: 640px)`,
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("served stylesheet missing final polish marker %q", want)
		}
	}
}

func TestFinalVisitHierarchyKeepsFieldWorkFirst(t *testing.T) {
	resultAt := strings.Index(contactHTML, "Catat Hasil Visit")
	updateAt := strings.Index(contactHTML, "Update Visit")
	if resultAt < 0 || updateAt < 0 || resultAt >= updateAt {
		t.Fatalf("Visit Session hierarchy wrong: result=%d update=%d", resultAt, updateAt)
	}
	for _, unwanted := range []string{
		`/merchant/prospect/`,
		`FIELD EXECUTION`,
		`Pemisahan status:`,
		`Registration / Installation / Installed / Active`,
	} {
		if strings.Contains(contactHTML, unwanted) {
			t.Fatalf("Visit Session still exposes noisy/technical copy %q", unwanted)
		}
	}
}

func TestDailyReportLivesAfterVisitWorkflow(t *testing.T) {
	js, err := uiAssets.ReadFile("ui/daily-visit-report.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(js)
	if !strings.Contains(text, `main.append(fragment)`) {
		t.Fatal("daily report is not appended after the Visit Session workflow")
	}
	if strings.Contains(text, `insertAdjacentElement('afterend'`) {
		t.Fatal("daily report still jumps directly below the page heading")
	}
}

func TestRouteOverviewHidesRoutingInternals(t *testing.T) {
	for _, unwanted := range []string{
		`OSRM`,
		`Haversine`,
		`Routing`,
		`StartLat`,
		`StartLon`,
		`/merchant/prospect/`,
	} {
		if strings.Contains(visitPlanHTML, unwanted) {
			t.Fatalf("route overview exposes technical/mixed-workflow copy %q", unwanted)
		}
	}
	for _, want := range []string{"Urutan Kunjungan", "Titik awal rute", "Progress Rute"} {
		if !strings.Contains(visitPlanHTML, want) {
			t.Fatalf("route overview missing polished UI %q", want)
		}
	}
}

func TestDatabaseKeepsAdminDetailsQuiet(t *testing.T) {
	for _, unwanted := range []string{
		`Location Scope`,
		`database-flow`,
		`Peran Database`,
		`#{{.Prospect.ID}}`,
		`/merchant/prospect/`,
	} {
		if strings.Contains(bukupayDatabaseHTML, unwanted) {
			t.Fatalf("database exposes obsolete/technical UI %q", unwanted)
		}
	}
	if !strings.Contains(bukupayDatabaseHTML, "Detail Teknis") {
		t.Fatal("database must keep diagnostics available on demand")
	}
}
