# Search Engine B2B

Mesin pencari prospek bisnis publik B2B berbasis Google Maps, **terpisah sepenuhnya dari mesin pencari kos**.

## Fokus

- Jawa + Sumatra
- Filter lokasi sampai Provinsi -> Kabupaten/Kota -> Kecamatan -> Desa/Kelurahan
- Database wilayah dari public wilayah API dengan cache lokal
- Query berdasarkan kategori usaha
- Hanya listing bisnis publik
- Filter nomor telepon bisnis publik
- Dedup place ID / data ID / telepon / koordinat
- Mengecualikan bank, leasing, finance, pinjaman, pegadaian, dan koperasi simpan pinjam

## Engine

Repository ini menyimpan logic B2B sendiri. Scraping Google Maps dijalankan oleh executable upstream `gosom/google-maps-scraper`, sehingga codebase B2B tidak tercampur dengan project kos.

Build collector:

```bash
go test ./...
go build -o bin/search-engine-b2b ./cmd/collector
```

Contoh pencarian satu desa:

```bash
./bin/search-engine-b2b \
  -engine /path/to/google_maps_scraper \
  -location "Sukamulya, Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia" \
  -output data/prospects.csv
```

Dengan 18 keyword default, satu desa menghasilkan 18 query yang spesifik ke lokasi tersebut.

## Output awal

`place_id, data_id, title, category, address, phone, website, latitude, longitude, review_rating, review_count, link`

Dashboard dan database prospek akan dikembangkan hanya di repository ini.
