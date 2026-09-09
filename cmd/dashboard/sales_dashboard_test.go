package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

func TestSalesDashboardRender(t *testing.T) {
	dir := t.TempDir()
	store, err := prospectstore.Open(filepath.Join(dir, "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := &app{store: store}
	mux := http.NewServeMux()
	registerSalesDashboardRoutes(mux, a)
	registerVisitPlanRoutes(mux, a)
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	w := get("/sales")
	if w.Code != http.StatusOK {
		t.Fatalf("render: %d %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"Belum ada prioritas hari ini", "Belum ada aktivitas tercatat", "aria-current=\"page\"", "/assets/bukupay-ui-v2.css", "TO VISIT", "FOLLOW UP", "ACTIVE"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(w.Body.String(), "ZgotmplZ") {
		t.Fatal("invalid template URL")
	}
	css := get("/assets/bukupay-ui-v2.css")
	if css.Code != 200 || !strings.Contains(css.Header().Get("Content-Type"), "text/css") || !strings.Contains(css.Body.String(), "--ui-navy") {
		t.Fatal("shared stylesheet unavailable")
	}
	for _, date := range []string{"2026-09-10", "2026-09-11"} {
		form := get("/visit-plans/new?plan_date=" + date)
		if form.Code != 200 || !strings.Contains(form.Body.String(), `value="`+date+`"`) {
			t.Fatalf("route action date: %d %s", form.Code, form.Body.String())
		}
	}
	if get("/visit-plans/new?plan_date=invalid").Code != http.StatusBadRequest {
		t.Fatal("invalid route date accepted")
	}
	csv := filepath.Join(dir, "merchants.csv")
	if err := os.WriteFile(csv, []byte("place_id,title,category,address,phone,latitude,longitude,link\nfixture,Test <Merchant>,warung,Test address,08123456789,-6.2,106.8,https://maps.google.com/?cid=123\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := store.ImportCSV(ctx, csv, "Test Area"); err != nil {
		t.Fatal(err)
	}
	records, err := store.List(ctx, prospectstore.Filter{Limit: 10})
	if err != nil || len(records) != 1 {
		t.Fatalf("import: %v %v", records, err)
	}
	merchant, err := store.EnsureMerchant(ctx, records[0].Prospect.ID, "follow_up")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateMerchant(ctx, merchant.ID, prospectstore.MerchantInput{Status: "follow_up", NextActionAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	w = get("/sales?location=Test+Area")
	if w.Code != 200 {
		t.Fatalf("populated render: %d %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"Test &lt;Merchant&gt;", "Tindak lanjut jatuh tempo", "https://wa.me/628123456789", "tel:+628123456789", "Mulai Visit", "Test Area", "Pipeline"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("populated render missing %q", want)
		}
	}
	if strings.Contains(w.Body.String(), "ZgotmplZ") {
		t.Fatal("invalid populated template URL")
	}
}
