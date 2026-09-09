# Bukupay Merchant Hunter

Field-sales prospecting dan merchant acquisition workspace untuk membantu penjualan **Soundbox QRIS Bukupay** ke warung, toko FMCG, F&B, dan merchant lokal lainnya.

Baseline Bukupay: `main-bukupay`.

> `main-bukupay` diperlakukan sebagai produk/deployment terpisah dari `main` lama. Jangan deploy Bukupay dengan melakukan `git switch main-bukupay` di folder runtime B2B lama karena folder `data/` dapat ikut terbaca oleh aplikasi yang berbeda.

## Tujuan

Aplikasi membantu alur kerja sales dari discovery sampai merchant aktif:

```text
Google Maps -> Master Merchant -> Area Coverage -> Daily Route -> Visit -> Follow Up -> Registration -> Installation -> Active
```

## Fitur utama

- Google Maps merchant discovery melalui `gosom/google-maps-scraper`
- Jakarta Selatan sebagai area kerja utama + area irisan
- Filter lokasi berjenjang: Provinsi -> Kabupaten/Kota -> Kecamatan -> Desa/Kelurahan
- Merchant tanpa nomor telepon tetap disimpan untuk canvassing lapangan
- SQLite master merchant database
- Area/Coverage Planner
- Daily Visit Plan default 25 merchant
- OSRM road-distance routing dengan fallback Haversine
- Visit / Prospecting Session
- Google Maps, Call, dan WhatsApp shortcut
- Activity history dan revisit scheduling
- Merchant Pipeline
- Data QRIS dan Soundbox
- Registrasi, instalasi, dan aktivasi tracking
- CSV/XLSX export
- Docker runtime dan GitHub Actions CI

## Runtime terisolasi

Deployment Bukupay menggunakan identitas sendiri:

```text
Compose project : bukupay-sales
Container       : bukupay-sales
Image           : bukupay-sales:local
App port        : 8082
Database        : ./data/prospects.db
OSRM container  : bukupay-osrm
OSRM port       : 5000
```

Rekomendasi struktur server:

```text
~/Search-Engine-B2B/   # runtime B2B lama, data lama tetap di sini
~/Bukupay-Sales/       # runtime khusus Bukupay
```

Clone Bukupay sebagai folder baru:

```bash
cd ~
git clone -b main-bukupay https://github.com/Fikriafrizal99/Search-Engine-B2B.git Bukupay-Sales
cd Bukupay-Sales
```

Siapkan OSRM sekali:

```bash
chmod +x scripts/setup-osrm-java.sh
./scripts/setup-osrm-java.sh
```

Jalankan Bukupay:

```bash
docker compose --profile routing up -d --build
```

Buka:

```text
http://SERVER:8082/sales
```

B2B lama dapat tetap menggunakan port `8081` dari folder/runtime lamanya.

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

## Build

Requirement lokal:

- Go 1.22+
- executable `google_maps_scraper`

```bash
go mod tidy
go test ./...
go build ./cmd/collector ./cmd/dashboard
```

## Halaman utama

```text
/sales        Bukupay Sales Dashboard
/areas        Area Planner
/visit-plan/* Daily Route
/contact      Visit / Prospecting Session
/merchants    Merchant Pipeline
/             Database / scraper legacy UI
```

Data Bukupay disimpan di `data/prospects.db` pada folder deployment Bukupay dan tidak boleh menggunakan folder `data/` milik runtime B2B lama.
