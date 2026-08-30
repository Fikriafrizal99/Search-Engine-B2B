package collectorconfig

import "testing"

func TestBuildQueriesForLocation(t *testing.T) {
	p := Preset{Name: "b2b", Keywords: []string{"bengkel motor", "toko bangunan"}, OutputFields: []string{"title"}}
	got, err := BuildQueriesForLocation(p, "Sukamulya, Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d queries", len(got))
	}
}
