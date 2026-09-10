package prospectstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyVisitReportUsesLatestVisitPerMerchantPerDay(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "daily-report.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	scope := "Tebet Timur, Tebet, Jakarta Selatan, DKI Jakarta, Indonesia"
	csvPath := filepath.Join(t.TempDir(), "merchant.csv")
	csv := "title,category,address,phone,latitude,longitude,maps_url\n" +
		"Warung Report,Warung,Jl Report,0811,-6.2000,106.8000,https://maps.google.com/?q=-6.2%2C106.8\n"
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
	records, err := store.List(ctx, Filter{Location: scope, Limit: 10})
	if err != nil || len(records) != 1 {
		t.Fatalf("records=%d err=%v", len(records), err)
	}

	loc := time.FixedZone("WIB", 7*60*60)
	day := time.Date(2026, 9, 10, 0, 0, 0, 0, loc)
	if _, err := store.RecordVisitResult(ctx, VisitResultInput{
		ProspectID: records[0].Prospect.ID,
		Channel: "visit",
		Result: "presented",
		PICName: "Pak Dedi",
		OccurredAt: day.Add(10 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordVisitResult(ctx, VisitResultInput{
		ProspectID: records[0].Prospect.ID,
		Channel: "visit",
		Result: "interested",
		PICName: "Pak Dedi",
		Note: "Minta dilanjutkan registrasi",
		OccurredAt: day.Add(11 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	report, err := store.DailyVisitReport(ctx, day, loc, scope)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalVisited != 1 || len(report.Items) != 1 {
		t.Fatalf("daily report must count one merchant once: total=%d items=%d", report.TotalVisited, len(report.Items))
	}
	if report.ResultCounts["interested"] != 1 || report.ResultCounts["presented"] != 0 {
		t.Fatalf("latest result not used: %#v", report.ResultCounts)
	}
	if report.Items[0].VisitResult != "interested" || report.Items[0].PICName != "Pak Dedi" {
		t.Fatalf("unexpected report item: %+v", report.Items[0])
	}
}
