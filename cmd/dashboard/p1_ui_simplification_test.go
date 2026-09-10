package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

func TestP1VisitSessionIsExecutionFocused(t *testing.T) {
	for _, want := range []string{"FIELD EXECUTION", "Catat Hasil", "Riwayat Interaksi", "Progress Rute", "Update Visit Merchant", "Koreksi Visit Terakhir", "Riwayat Visit"} {
		if !strings.Contains(contactHTML, want) {
			t.Fatalf("Visit Session missing %q", want)
		}
	}
	for _, unwanted := range []string{"<h2>Sales Pipeline</h2>", "Statistik Visit Session", "Session Summary"} {
		if strings.Contains(contactHTML, unwanted) {
			t.Fatalf("Visit Session still contains redundant section %q", unwanted)
		}
	}
}

func TestP1SalesWorkspaceKeepsOnlyPrimarySalesFields(t *testing.T) {
	for _, want := range []string{"Sales Workspace", "Progress Sales", "Status Sales", "Punya QRIS?", "Provider QRIS", "Punya Soundbox?", "Next Action", "Tanggal Follow-up", "Simpan Progress", "Sales History", "Visit Session"} {
		if !strings.Contains(merchantHTML, want) {
			t.Fatalf("Sales Workspace missing %q", want)
		}
	}
	for _, unwanted := range []string{"<h2>Informasi Merchant</h2>", "Traffic Merchant", "Level Transaksi", "Status Registrasi", "Status Instalasi", "Status Aktivasi", "Riwayat & Koreksi Visit", "Koreksi Visit Terakhir"} {
		if strings.Contains(merchantHTML, unwanted) {
			t.Fatalf("Sales Workspace still exposes secondary/visit field %q", unwanted)
		}
	}
	if !strings.Contains(merchantHTML, "sales-workspace-stepper") {
		t.Fatal("sales stage stepper missing")
	}
}

func TestP1DashboardRemovesQuickAccessAndSeparatesPriorities(t *testing.T) {
	if strings.Contains(salesDashboardHTML, "Akses Cepat") {
		t.Fatal("dashboard still duplicates navbar with Akses Cepat")
	}
	if !strings.Contains(salesDashboardHTML, "Prioritas di Luar Rute") {
		t.Fatal("dashboard priority scope is not explicit")
	}
}

func TestP1RouteUsesCompactStops(t *testing.T) {
	if !strings.Contains(visitPlanHTML, "p3-stop-compact") || !strings.Contains(visitPlanHTML, "p3-stop-address") {
		t.Fatal("route does not use compact stop layout")
	}
	for _, unwanted := range []string{"Call", "WhatsApp"} {
		if strings.Contains(visitPlanHTML, unwanted) {
			t.Fatalf("route overview still contains execution action %q", unwanted)
		}
	}
}

func TestP1AreaPlannerShowsSingleStateDrivenNextAction(t *testing.T) {
	if !strings.Contains(areaHTML, "Langkah Berikutnya") {
		t.Fatal("Area Planner next-action section missing")
	}
	data, err := uiAssets.ReadFile("ui/area-planner.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	if !strings.Contains(script, "replaceChildren(box)") {
		t.Fatal("Area Planner recommendation does not replace the previous action")
	}
	if strings.Contains(script, "Tinjau ${q.RevisitRequired}") || strings.Contains(script, "Area berikut dengan sisa coverage terbanyak") {
		t.Fatal("Area Planner still builds the old recommendation list")
	}
}

func TestP1DatabaseKeepsScrapeControlInAreaPlanner(t *testing.T) {
	for _, want := range []string{"Database Merchant", "Kelola Scrape Area", "Informasi saja. Start, cancel, dan progress lengkap ada di Area Planner."} {
		if !strings.Contains(bukupayDatabaseHTML, want) {
			t.Fatalf("Database cleanup missing %q", want)
		}
	}
	if strings.Contains(bukupayDatabaseHTML, `action="/bukupay/collect"`) {
		t.Fatal("Database duplicates scrape controls")
	}
}

func TestP1DashboardPrioritiesExcludeMerchantsAlreadyInTodayRoute(t *testing.T) {
	dir := t.TempDir()
	store, err := prospectstore.Open(filepath.Join(dir, "priorities.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	scope := "P1 Test Area, Kecamatan, Kota, Provinsi, Indonesia"
	csvPath := filepath.Join(dir, "merchants.csv")
	csv := "title,category,address,phone,latitude,longitude,maps_url\n" +
		"Merchant A,Warung,Jl A,0811,-6.2000,106.8000,https://maps.google.com/?q=-6.2%2C106.8\n" +
		"Merchant B,Warung,Jl B,0822,-6.2005,106.8005,https://maps.google.com/?q=-6.2005%2C106.8005\n" +
		"Merchant C,Warung,Jl C,0833,-6.2100,106.8100,https://maps.google.com/?q=-6.21%2C106.81\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := store.ImportCSV(ctx, csvPath, scope); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterScrape(ctx, scope); err != nil {
		t.Fatal(err)
	}
	records, err := store.List(ctx, prospectstore.Filter{Location: scope, Limit: 10})
	if err != nil || len(records) != 3 {
		t.Fatalf("records=%d err=%v", len(records), err)
	}
	due := time.Now().Add(-time.Hour)
	for _, record := range records {
		merchant, err := store.EnsureMerchant(ctx, record.Prospect.ID, prospectstore.MerchantFollowUp)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.UpdateMerchant(ctx, merchant.ID, prospectstore.MerchantInput{Status: prospectstore.MerchantFollowUp, NextAction: "Follow up", NextActionAt: due}); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := store.CreateDailyVisitPlan(ctx, prospectstore.DailyVisitPlanInput{
		PlanDate:      time.Now().In(jakartaLocation),
		LocationScope: scope,
		StartLat:      -6.1999,
		StartLon:      106.7999,
		TargetCount:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	planned := map[int64]bool{}
	for _, item := range plan.Items {
		planned[item.Prospect.ID] = true
	}
	summary, err := store.SalesDashboardSummary(ctx, time.Now(), jakartaLocation)
	if err != nil {
		t.Fatal(err)
	}
	for _, priority := range summary.Priorities {
		if planned[priority.Prospect.ID] {
			t.Fatalf("merchant %d is already in today's route but leaked into dashboard priority", priority.Prospect.ID)
		}
	}
}
