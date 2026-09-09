package prospectstore

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVisitPlanningFlow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "prospects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	scope := "SIRNAGALIH, CILAKU, KABUPATEN CIANJUR, JAWA BARAT, Indonesia"
	csvPath := filepath.Join(dir, "merchants.csv")
	if err := writePlanningTestCSV(csvPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ImportCSV(ctx, csvPath, scope); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterScrape(ctx, scope); err != nil {
		t.Fatal(err)
	}

	progress, err := store.CoverageProgress(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if progress.Total != 5 || progress.Unvisited != 5 || progress.Status != CoverageScraped {
		t.Fatalf("unexpected initial progress: %+v", progress)
	}

	wib := time.FixedZone("WIB", 7*60*60)
	planDate := time.Date(2026, 9, 10, 0, 0, 0, 0, wib)
	plan, err := store.CreateDailyVisitPlan(ctx, DailyVisitPlanInput{
		PlanDate: planDate, LocationScope: scope, StartLat: -6.8200, StartLon: 107.1400, TargetCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 3 {
		t.Fatalf("expected 3 plan items, got %d", len(plan.Items))
	}
	wantOrder := []string{"Merchant A", "Merchant B", "Merchant C"}
	for i, want := range wantOrder {
		if got := plan.Items[i].Prospect.Title; got != want {
			t.Fatalf("sequence %d: want %q got %q", i+1, want, got)
		}
		if plan.Items[i].Sequence != i+1 {
			t.Fatalf("unexpected sequence: %+v", plan.Items[i])
		}
	}

	visitAt := time.Date(2026, 9, 10, 10, 0, 0, 0, wib)
	state, err := store.RecordVisitResult(ctx, VisitResultInput{
		ProspectID: plan.Items[0].Prospect.ID,
		Channel:    "visit",
		Result:     "owner_not_found",
		PICName:    "Staff",
		Note:       "Owner sedang keluar",
		OccurredAt: visitAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.VisitStatus != VisitRevisitRequired || state.VisitCount != 1 {
		t.Fatalf("unexpected visit state: %+v", state)
	}
	wantRevisit := visitAt.Add(3 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if state.NextRevisitAt != wantRevisit {
		t.Fatalf("want revisit %s got %s", wantRevisit, state.NextRevisitAt)
	}
	history, err := store.VisitHistory(ctx, state.ProspectID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].VisitResult != "owner_not_found" {
		t.Fatalf("unexpected history: %+v", history)
	}

	// Re-scraping the same village enriches/upserts master data but must not reset visit state.
	if _, err := store.ImportCSV(ctx, csvPath, scope); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterScrape(ctx, scope); err != nil {
		t.Fatal(err)
	}
	stateAfterRescrape, err := store.GetVisitState(ctx, state.ProspectID)
	if err != nil {
		t.Fatal(err)
	}
	if stateAfterRescrape.VisitStatus != VisitRevisitRequired || stateAfterRescrape.VisitCount != 1 {
		t.Fatalf("rescrape reset field-sales state: %+v", stateAfterRescrape)
	}

	records, err := store.List(ctx, Filter{Location: scope, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, rec := range records {
		ids[rec.Prospect.Title] = rec.Prospect.ID
	}
	if _, err := store.EnsureMerchant(ctx, ids["Merchant D"], MerchantActive); err != nil {
		t.Fatal(err)
	}

	// On the next day, B/C remain reserved by the previous plan, A is not due yet,
	// and D is already active. Only E is eligible for fresh canvassing.
	nextPlan, err := store.CreateDailyVisitPlan(ctx, DailyVisitPlanInput{
		PlanDate: planDate.AddDate(0, 0, 1), LocationScope: scope, StartLat: -6.8200, StartLon: 107.1400, TargetCount: 25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nextPlan.Items) != 1 || nextPlan.Items[0].Prospect.Title != "Merchant E" {
		t.Fatalf("unexpected next plan: %+v", nextPlan.Items)
	}
}

func TestDistanceKM(t *testing.T) {
	d := DistanceKM(-6.8200, 107.1400, -6.8210, 107.1400)
	if d < 0.10 || d > 0.12 {
		t.Fatalf("unexpected distance %.6f km", d)
	}
}

func writePlanningTestCSV(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	rows := [][]string{
		{"place_id", "data_id", "title", "category", "address", "phone", "website", "latitude", "longitude", "review_rating", "review_count", "link"},
		{"a", "", "Merchant A", "warung", "A", "", "", "-6.8210", "107.1400", "4.5", "10", "https://maps.example/a"},
		{"b", "", "Merchant B", "warung", "B", "", "", "-6.8220", "107.1400", "4.2", "4", "https://maps.example/b"},
		{"c", "", "Merchant C", "warung", "C", "", "", "-6.8230", "107.1400", "0", "0", "https://maps.example/c"},
		{"d", "", "Merchant D", "warung", "D", "", "", "-6.8400", "107.1400", "0", "0", "https://maps.example/d"},
		{"e", "", "Merchant E", "warung", "E", "", "", "-6.8500", "107.1400", "0", "0", "https://maps.example/e"},
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
