package collectorconfig

import "testing"

func TestBuildQueriesForVillage(t *testing.T) {
	p := Preset{Name:"b2b", Keywords:[]string{"bengkel motor","toko bangunan"}, OutputFields:[]string{"title"}}
	q, err := BuildQueriesForLocation(p, "Sukamulya, Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia")
	if err != nil { t.Fatal(err) }
	if len(q) != 2 { t.Fatalf("got %d queries", len(q)) }
	if q[0] != "bengkel motor Sukamulya, Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia" { t.Fatalf("unexpected query: %s", q[0]) }
}
