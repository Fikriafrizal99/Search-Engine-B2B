package prospectstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCorrectLatestVisitKeepsVisitCountAndFixesSalesStatus(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "prospects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	scope := "Tebet Timur, Tebet, Kota Administrasi Jakarta Selatan, Daerah Khusus Ibukota Jakarta, Indonesia"
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

	planDate := time.Date(2026, 9, 10, 0, 0, 0, 0, time.FixedZone("WIB", 7*60*60))
	plan, err := store.CreateDailyVisitPlan(ctx, DailyVisitPlanInput{
		PlanDate: planDate, LocationScope: scope, StartLat: -6.8200, StartLon: 107.1400, TargetCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	prospectID := plan.Items[0].Prospect.ID

	if _, err := store.LogContact(ctx, ContactInput{ProspectID: prospectID, Channel: "visit", Result: "installed", Owner: "Owner"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordVisitResult(ctx, VisitResultInput{ProspectID: prospectID, Channel: "visit", Result: "installed", PICName: "Owner", Note: "salah pilih"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TouchMerchantStatus(ctx, prospectID, MerchantInstalled, "Owner", "salah pilih", time.Time{}); err != nil {
		t.Fatal(err)
	}

	before, err := store.GetVisitState(ctx, prospectID)
	if err != nil {
		t.Fatal(err)
	}
	if before.VisitCount != 1 || before.LastResult != "installed" {
		t.Fatalf("unexpected before state: %+v", before)
	}
	visits, err := store.VisitHistory(ctx, prospectID, 10)
	if err != nil || len(visits) != 1 {
		t.Fatalf("visit history: %v %+v", err, visits)
	}

	after, err := store.CorrectLatestVisit(ctx, visits[0].ID, VisitResultInput{
		ProspectID: prospectID,
		Result:     "already_soundbox",
		PICName:    "Owner",
		Note:       "merchant sudah punya soundbox sebelum kunjungan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.VisitCount != 1 {
		t.Fatalf("correction must not increment visit count: %+v", after)
	}
	if after.VisitStatus != VisitVisited || after.LastResult != "already_soundbox" {
		t.Fatalf("unexpected corrected visit state: %+v", after)
	}

	merchant, ok, err := store.GetMerchantByProspect(ctx, prospectID)
	if err != nil || !ok {
		t.Fatalf("merchant: ok=%v err=%v", ok, err)
	}
	if merchant.Status != MerchantAlreadySoundbox || !merchant.HasSoundbox {
		t.Fatalf("wrong corrected merchant status: %+v", merchant)
	}

	var auditCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM visit_history_corrections WHERE visit_history_id=?`, visits[0].ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one correction audit, got %d", auditCount)
	}

	planAfter, err := store.GetVisitPlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(planAfter.Items) != 1 || planAfter.Items[0].Status != VisitVisited {
		t.Fatalf("unexpected route item after correction: %+v", planAfter.Items)
	}
}

func TestFieldVisitResultDoesNotIncludeSalesInstallationStages(t *testing.T) {
	for _, result := range []string{"registered", "installed", "active"} {
		if IsFieldVisitResult(result) {
			t.Fatalf("%q must stay in Sales Progress, not Visit Session", result)
		}
	}
	for _, result := range []string{"visited", "presented", "interested", "follow_up", "already_soundbox", "not_interested", "owner_not_found", "store_closed"} {
		if !IsFieldVisitResult(result) {
			t.Fatalf("%q must be allowed in Visit Session", result)
		}
	}
}
