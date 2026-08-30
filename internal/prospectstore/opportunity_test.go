package prospectstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpportunitySubmissionWorkflow(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "prospects.csv")
	data := "title,address,phone,category\nToko Bangunan Jaya,Jl Raya,081234567890,Toko Bangunan\n"
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

	lead, err := s.NextContact(ctx, "new", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LogContact(ctx, ContactInput{ProspectID: lead.Record.Prospect.ID, Channel: "call", Result: "interested"}); err != nil {
		t.Fatal(err)
	}

	o, err := s.EnsureOpportunity(ctx, lead.Record.Prospect.ID, OpportunityQualifying)
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != OpportunityQualifying {
		t.Fatalf("unexpected opportunity: %+v", o)
	}

	next := time.Now().Add(2 * time.Hour)
	o, err = s.UpdateOpportunity(ctx, o.ID, OpportunityInput{
		Status:           OpportunityReadyToSubmit,
		Product:          "BPKB Mobil",
		CustomerNeed:     "Customer meminta informasi proses pembiayaan BPKB mobil",
		UnitType:         "mobil",
		UnitModel:        "Avanza",
		UnitYear:         2022,
		Ownership:        "sendiri",
		PreferredContact: "whatsapp",
		NextAction:       "Submit ke partner",
		NextActionAt:     next,
		Owner:            "Fikri",
		DocKTP:           true,
		DocSTNK:          true,
		DocBPKB:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != OpportunityReadyToSubmit || !o.DocKTP || !o.DocSTNK || !o.DocBPKB {
		t.Fatalf("unexpected updated opportunity: %+v", o)
	}

	exec, err := s.GetExecution(ctx, lead.Record.Prospect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != ExecutionQualified {
		t.Fatalf("expected qualified execution, got %+v", exec)
	}

	sub, err := s.CreateSubmission(ctx, o.ID, SubmissionInput{Partner: "Partner Finance", ReferenceNo: "REF-001"})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != SubmissionSubmitted || sub.Partner != "Partner Finance" {
		t.Fatalf("unexpected submission: %+v", sub)
	}

	sub, err = s.UpdateSubmission(ctx, sub.ID, SubmissionInput{
		Partner:         sub.Partner,
		Product:         sub.Product,
		ReferenceNo:     sub.ReferenceNo,
		Status:          SubmissionDisbursed,
		OutcomeNote:     "Disbursed by partner",
		DisbursedAmount: 25000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != SubmissionDisbursed || sub.DisbursedAt == "" || sub.DisbursedAmount != 25000000 {
		t.Fatalf("unexpected disbursed submission: %+v", sub)
	}

	exec, err = s.GetExecution(ctx, lead.Record.Prospect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != ExecutionDisbursed {
		t.Fatalf("expected disbursed execution, got %+v", exec)
	}

	stats, err := s.PipelineStats(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Disbursed != 1 {
		t.Fatalf("unexpected pipeline stats: %+v", stats)
	}
}

func TestEnsureOpportunityReusesOpenOpportunity(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "prospects.csv")
	if err := os.WriteFile(csvPath, []byte("title,address,phone\nBengkel Jaya,Jl Raya,081299999999\n"), 0o644); err != nil {
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
	first, err := s.EnsureOpportunity(ctx, lead.Record.Prospect.ID, OpportunityQualifying)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.EnsureOpportunity(ctx, lead.Record.Prospect.ID, OpportunityQualified)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same open opportunity, got %d and %d", first.ID, second.ID)
	}
}
