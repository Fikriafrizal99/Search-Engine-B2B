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
