package prospectstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSalesExecutionWorkflow(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "prospects.csv")
	data := "place_id,data_id,title,category,address,phone,website,latitude,longitude,review_rating,review_count,link\n" +
		"p1,d1,Bengkel Maju,Bengkel motor,Jl Raya,08123456789,https://example.com,-6.8,107.1,4.7,120,https://maps.google.com/x\n"
	if err := os.WriteFile(csvPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "prospects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.ImportCSV(ctx, csvPath, "Cianjur, Jawa Barat, Indonesia"); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	lead, err := s.NextContact(ctx, "new", now)
	if err != nil {
		t.Fatal(err)
	}
	if lead.Record.Prospect.Title != "Bengkel Maju" || lead.Execution.Status != ExecutionNew {
		t.Fatalf("unexpected lead: %+v", lead)
	}

	if _, err := s.LogContact(ctx, ContactInput{
		ProspectID: lead.Record.Prospect.ID,
		Channel:    "call",
		Result:     "interested",
		Note:       "Minta informasi lanjutan",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ContactLead(ctx, lead.Record.Prospect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Execution.Status != ExecutionInterested || len(got.History) != 1 {
		t.Fatalf("unexpected execution: %+v history=%d", got.Execution, len(got.History))
	}

	if _, err := s.NextContact(ctx, "new", now); err != sql.ErrNoRows {
		t.Fatalf("expected no remaining new lead, got %v", err)
	}

	loc, _ := time.LoadLocation("Asia/Jakarta")
	stats, err := s.ExecutionStats(ctx, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	if stats.New != 0 || stats.Interested != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestRetryGetsDefaultFollowUp(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "prospects.csv")
	data := "title,address,phone\nToko Jaya,Jl Raya,081298765432\n"
	if err := os.WriteFile(csvPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "prospects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.ImportCSV(ctx, csvPath, "Bandung, Jawa Barat, Indonesia"); err != nil {
		t.Fatal(err)
	}
	lead, err := s.NextContact(ctx, "new", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.LogContact(ctx, ContactInput{ProspectID: lead.Record.Prospect.ID, Channel: "call", Result: "no_answer"})
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != ExecutionRetry || execution.NextFollowUpAt == "" {
		t.Fatalf("unexpected retry execution: %+v", execution)
	}
}
