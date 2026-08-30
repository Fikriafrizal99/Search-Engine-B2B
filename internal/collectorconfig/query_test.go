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

func TestCustomKeywordsAreFreeFormAndDeduplicated(t *testing.T) {
	custom := ParseKeywords("agen pupuk\ntoko alat berat, bakery;AGEN PUPUK")
	if len(custom) != 3 {
		t.Fatalf("expected 3 unique custom keywords, got %d: %#v", len(custom), custom)
	}
	keywords := MergeKeywords([]string{"bengkel motor", "toko bangunan"}, custom, true)
	got, err := BuildQueriesForKeywordsLocation(keywords, "Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 queries, got %d", len(got))
	}
	if got[2] != "agen pupuk Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia" {
		t.Fatalf("unexpected custom query: %q", got[2])
	}
}
