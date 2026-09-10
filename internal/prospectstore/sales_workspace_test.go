package prospectstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSalesWorkspaceExcludesCoverageOnlyStatuses(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "sales-workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	csvPath := filepath.Join(t.TempDir(), "merchants.csv")
	if err := writePlanningTestCSV(csvPath); err != nil {
		t.Fatal(err)
	}
	scope := "Sales Test Area, District, City, Province, Indonesia"
	if _, err := store.ImportCSV(ctx, csvPath, scope); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterScrape(ctx, scope); err != nil {
		t.Fatal(err)
	}
	records, err := store.List(ctx, Filter{Location: scope, Limit: 10})
	if err != nil || len(records) < 5 {
		t.Fatalf("records=%d err=%v", len(records), err)
	}

	statuses := []string{MerchantToVisit, MerchantVisited, MerchantOwnerNotFound, MerchantInterested, MerchantActive}
	for i, status := range statuses {
		merchant, err := store.EnsureMerchant(ctx, records[i].Prospect.ID, status)
		if err != nil {
			t.Fatal(err)
		}
		if merchant.Status != status {
			if _, err := store.UpdateMerchant(ctx, merchant.ID, MerchantInput{Status: status}); err != nil {
				t.Fatal(err)
			}
		}
	}

	now := time.Now()
	count, err := store.MerchantListCount(ctx, MerchantListFilter{Status: "all", Limit: 25}, now)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("default Sales Workspace count=%d want 2 (interested + active)", count)
	}

	items, err := store.FilterMerchantPipeline(ctx, MerchantListFilter{Status: "all", Limit: 25}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		switch item.Merchant.Status {
		case MerchantToVisit, MerchantVisited, MerchantOwnerNotFound:
			t.Fatalf("coverage-only status leaked into Sales Workspace: %s", item.Merchant.Status)
		}
	}

	explicitCoverage, err := store.MerchantListCount(ctx, MerchantListFilter{Status: MerchantToVisit, Limit: 25}, now)
	if err != nil || explicitCoverage != 1 {
		t.Fatalf("explicit coverage filter should remain available internally: count=%d err=%v", explicitCoverage, err)
	}
}
