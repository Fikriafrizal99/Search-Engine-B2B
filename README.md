# Bukupay Merchant Hunter

Field-sales prospecting dan merchant acquisition workspace untuk membantu penjualan **Soundbox QRIS Bukupay** ke warung, toko FMCG, F&B, dan merchant lokal lainnya.

Branch pengembangan: `feat/bukupay-sales`.

## Tujuan

Aplikasi membantu alur kerja sales dari discovery sampai merchant aktif:

```text
Google Maps -> Prospect -> Visit Session -> Follow Up -> Registration -> Installation -> Active
```

## Fitur utama

- Google Maps merchant discovery melalui `gosom/google-maps-scraper`
- Scope Jawa + Sumatra
- Filter lokasi berjenjang: Provinsi -> Kabupaten/Kota -> Kecamatan -> Desa/Kelurahan
- Custom query hingga 100 keyword per collect
- Preset `bukupay-merchants` untuk warung, FMCG, F&B, dan usaha lokal
- Merchant tanpa nomor telepon tetap disimpan untuk canvassing lapangan
- SQLite prospect database
- Visit / Prospecting Session
- Google Maps, Call, dan WhatsApp shortcut
- Activity history dan follow-up scheduling
- Merchant Pipeline
- Data QRIS dan Soundbox
- Registrasi, instalasi, dan aktivasi tracking
- CSV/XLSX export
- Docker runtime dan GitHub Actions CI

## Merchant pipeline

Status utama:

```text
TO VISIT
  -> VISITED
  -> PRESENTED
  -> INTERESTED
  -> REGISTRATION
  -> REGISTERED
  -> INSTALLATION
  -> INSTALLED
  -> ACTIVE
```

Status tambahan:

- Follow Up
- Owner/PIC tidak ada
- Tidak tertarik
- Sudah punya Soundbox
- Toko tutup
- Invalid lead

## Data merchant

Prospect dari listing publik:

- Nama usaha
- Kategori
- Alamat
- Lokasi administratif
- Nomor telepon publik jika tersedia
- Website
- Rating/review
- Google Maps URL
- Koordinat

Data hasil kunjungan/komunikasi:

- Jenis merchant
- Nama dan peran PIC/owner
- Sudah menggunakan QRIS atau belum
- Provider QRIS saat ini
- Sudah mempunyai Soundbox atau belum
- Traffic level
- Transaction level
- Interest level
- Status registrasi
- Status instalasi
- Status aktivasi
- Next action dan due date
- Catatan lapangan

Traffic, transaksi, QRIS, Soundbox, dan ketertarikan merchant dicatat dari observasi atau komunikasi nyata. Aplikasi tidak menebaknya dari Google Maps.

## Default prospect keywords

Preset `config/presets/bukupay-merchants.json` mencakup antara lain:

```text
warung
warung sembako
toko kelontong
toko sembako
grosir sembako
minimarket
rumah makan
warteg
restoran
cafe
coffee shop
bakery
apotek
laundry
barbershop
bengkel motor
counter pulsa
```

Custom query tetap dapat digunakan dari dashboard.

## Build

Requirement:

- Go 1.22+
- executable `google_maps_scraper`

```bash
go mod tidy
go test ./...
go build ./cmd/collector ./cmd/dashboard
```

Jalankan dashboard:

```bash
./bin/b2b-dashboard \
  -engine /path/to/google_maps_scraper \
  -collector ./bin/search-engine-b2b
```

Buka:

```text
http://localhost:8080
```

Halaman utama sales:

```text
/contact     Visit / Prospecting Session
/merchants   Merchant Pipeline
```

## Collector langsung

```bash
./bin/search-engine-b2b \
  -location "Cilaku, Kabupaten Cianjur, Jawa Barat, Indonesia" \
  -keywords "warung; warung sembako; rumah makan; cafe" \
  -include-defaults=true \
  -- -c 2 -depth 5
```

Data hasil discovery tetap disimpan di `data/prospects.db` sehingga engine pencarian dan sales workflow dapat dikembangkan secara terpisah.