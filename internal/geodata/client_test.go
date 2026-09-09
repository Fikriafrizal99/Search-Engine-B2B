package geodata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestProvincesFilteredAndCached(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `[{"id":"32","name":"JAWA BARAT"},{"id":"51","name":"BALI"}]`)
	}))
	defer srv.Close()
	c := NewWithBaseURL(filepath.Join(t.TempDir(), "cache"), srv.URL)
	got, err := c.Provinces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "32" {
		t.Fatalf("unexpected regions: %+v", got)
	}
	if _, err := c.Provinces(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("expected one upstream hit, got %d", hits)
	}
}

func TestV2WrappedResponsesAndDottedIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/regencies/31.json":
			fmt.Fprint(w, `{"data":[{"id":"31.74","name":"Kota Jakarta Selatan"}],"meta":{"level":2}}`)
		case "/districts/31.74.json":
			fmt.Fprint(w, `{"data":[{"id":"31.74.01","name":"Tebet"}],"meta":{"level":3}}`)
		case "/villages/31.74.01.json":
			fmt.Fprint(w, `{"data":[{"id":"31.74.01.1001","name":"Tebet Barat"}],"meta":{"level":4}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewWithBaseURL("", srv.URL)
	ctx := context.Background()

	regencies, err := c.Regencies(ctx, "31")
	if err != nil {
		t.Fatal(err)
	}
	if len(regencies) != 1 || regencies[0].ID != "31.74" {
		t.Fatalf("unexpected regencies: %+v", regencies)
	}

	districts, err := c.Districts(ctx, "31.74")
	if err != nil {
		t.Fatal(err)
	}
	if len(districts) != 1 || districts[0].ID != "31.74.01" {
		t.Fatalf("unexpected districts: %+v", districts)
	}

	villages, err := c.Villages(ctx, "31.74.01")
	if err != nil {
		t.Fatal(err)
	}
	if len(villages) != 1 || villages[0].ID != "31.74.01.1001" {
		t.Fatalf("unexpected villages: %+v", villages)
	}
}
