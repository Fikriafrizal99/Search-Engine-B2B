package prospectstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestImportListAndProfile(t *testing.T) {
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
	n, err := s.ImportCSV(ctx, csvPath, "Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia")
	if err != nil || n != 1 {
		t.Fatalf("import n=%d err=%v", n, err)
	}
	rows, err := s.List(ctx, Filter{Location: "Jawa Barat", HasPhone: true})
	if err != nil || len(rows) != 1 {
		t.Fatalf("list len=%d err=%v", len(rows), err)
	}
	p := rows[0].Profile
	p.BusinessScale = "mikro"
	p.Priority = "high"
	p.ContactStatus = "follow_up"
	p.BusinessType = "Bengkel motor"
	p.VerificationStatus = "verified"
	p.QCStatus = "valid"
	if err := s.UpdateProfile(ctx, rows[0].Prospect.ID, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, rows[0].Prospect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile.Priority != "high" || got.Profile.BusinessScale != "mikro" || got.Profile.QCStatus != "valid" {
		t.Fatalf("unexpected profile: %+v", got.Profile)
	}
	st, err := s.Stats(ctx, Filter{})
	if err != nil || st.Total != 1 || st.WithPhone != 1 || st.WithWebsite != 1 {
		t.Fatalf("stats=%+v err=%v", st, err)
	}
}
