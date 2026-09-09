package prospectstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestMerchantFiltersAndAreaSnapshot(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "phase2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	csv := filepath.Join(t.TempDir(), "merchants.csv")
	if err := writePlanningTestCSV(csv); err != nil {
		t.Fatal(err)
	}
	scope := "Test Village, District, City, Province, Indonesia"
	if _, err := s.ImportCSV(ctx, csv, scope); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterScrape(ctx, scope); err != nil {
		t.Fatal(err)
	}
	loc := time.FixedZone("WIB", 7*60*60)
	now := time.Date(2026, 9, 10, 10, 0, 0, 0, loc)
	list, err := s.List(ctx, Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for i, record := range list {
		m, err := s.EnsureMerchant(ctx, record.Prospect.ID, "to_visit")
		if err != nil {
			t.Fatal(err)
		}
		status := "follow_up"
		due := now.Add(-time.Hour)
		if i == 1 {
			status = "active"
		}
		if i == 2 {
			due = now.Add(time.Hour)
		}
		if _, err := s.UpdateMerchant(ctx, m.ID, MerchantInput{Status: status, PICName: "PIC 100%", HasQRIS: i == 0, HasSoundbox: i == 1, NextActionAt: due}); err != nil {
			t.Fatal(err)
		}
	}
	f := MerchantListFilter{Location: scope, Due: true, Search: "100%", Limit: 2}
	count, err := s.MerchantListCount(ctx, f, now)
	if err != nil || count != 3 {
		t.Fatalf("due filter = %d, %v", count, err)
	}
	items, err := s.FilterMerchantPipeline(ctx, f, now)
	if err != nil || len(items) != 2 {
		t.Fatalf("filter: %d, %v", len(items), err)
	}
	f.Offset = 2
	second, err := s.FilterMerchantPipeline(ctx, f, now)
	if err != nil || len(second) != 1 || second[0].Merchant.ID == items[0].Merchant.ID {
		t.Fatalf("pagination: %+v %v", second, err)
	}
	f.Search = "%"
	count, err = s.MerchantListCount(ctx, f, now)
	if err != nil || count != 3 {
		t.Fatalf("literal search: %d %v", count, err)
	}
	f.Search = "no match"
	count, err = s.MerchantListCount(ctx, f, now)
	if err != nil || count != 0 {
		t.Fatalf("empty search: %d %v", count, err)
	}
	f.Search = ""
	f.Status = "active"
	count, err = s.MerchantListCount(ctx, f, now)
	if err != nil || count != 0 {
		t.Fatalf("terminal due: %d %v", count, err)
	}
	snapshot, err := s.AreaSnapshot(ctx, scope, now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.WithMaps != 5 || snapshot.WithPhone != 0 || snapshot.MissingCoordinates != 0 || snapshot.EligibleTomorrow != 4 || len(snapshot.Categories) != 1 || snapshot.Categories[0].Count != 5 {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	// Future revisits must not be counted as available for tomorrow's route.
	if _, err := s.RecordVisitResult(ctx, VisitResultInput{ProspectID: list[0].Prospect.ID, Channel: "visit", Result: "owner_not_found", OccurredAt: now}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.AreaSnapshot(ctx, scope, now.AddDate(0, 0, 1))
	if err != nil || snapshot.EligibleTomorrow != 3 {
		t.Fatalf("revisit eligibility: %+v %v", snapshot, err)
	}
	plan, err := s.CreateDailyVisitPlan(ctx, DailyVisitPlanInput{LocationScope: scope, PlanDate: now.AddDate(0, 0, 1), StartLat: -6.82, StartLon: 107.14, TargetCount: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 3 {
		t.Fatalf("route eligibility disagrees: %d", len(plan.Items))
	}
	snapshot, err = s.AreaSnapshot(ctx, scope, now.AddDate(0, 0, 1))
	if err != nil || snapshot.TomorrowPlanID != plan.ID || snapshot.EligibleTomorrow != 0 {
		t.Fatalf("existing route: %+v %v", snapshot, err)
	}
	areas, err := s.StoredAreas(ctx)
	if err != nil || len(areas) != 1 || areas[0].Status != CoverageInProgress || areas[0].Total != 5 {
		t.Fatalf("areas: %+v %v", areas, err)
	}
	history, err := s.VisitHistory(ctx, list[0].Prospect.ID, 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("history preserved: %+v %v", history, err)
	}
}
