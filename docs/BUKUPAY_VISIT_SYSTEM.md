# Bukupay Merchant Coverage & Visit System

Dokumen ini adalah baseline kerja untuk branch `feat/bukupay-sales`.

## 1. Tujuan operasional

Aplikasi dipakai sales Bukupay untuk:

1. memilih satu desa/kelurahan;
2. scrape Google Maps satu kali untuk membangun master merchant desa tersebut;
3. menyimpan semua listing merchant yang valid tanpa business filtering agresif;
4. malam hari membuat rencana kunjungan untuk hari berikutnya;
5. target default 25 kunjungan/hari;
6. mengurutkan kunjungan otomatis berdasarkan jarak dari titik mulai lalu merchant terdekat berikutnya;
7. menyimpan hasil kunjungan, revisit, dan sales pipeline tanpa kehilangan data hasil scrape;
8. menyelesaikan coverage satu desa sebelum berpindah ke desa berikutnya.

## 2. Prinsip data

### Master merchant tidak dibuang

Data Google Maps adalah master discovery data. Re-scrape hanya melakukan upsert/enrichment.

Data yang tidak memiliki telepon, website, rating, atau review tetap boleh disimpan.

Yang boleh dieliminasi hanya masalah teknis seperti duplicate record. Record tanpa koordinat tetap boleh menjadi master merchant, tetapi tidak dapat masuk auto-route sampai memiliki koordinat yang valid.

### Discovery dan field sales terpisah

```text
prospects
  Google Maps / master merchant
        |
        +--> merchant_visit_state
        |      status canvassing terkini
        |
        +--> merchant_visit_history
        |      histori kunjungan immutable
        |
        +--> visit_plans / visit_plan_items
        |      rencana 25 merchant harian
        |
        +--> merchant_sales / merchant_events
               pipeline penjualan Bukupay
```

Data scrape boleh diperbarui. Data kunjungan dan data sales tidak boleh di-reset oleh re-scrape.

## 3. Pipeline 1 — Coverage / Canvassing

```text
DESA BELUM SCRAPE
      ↓
SCRAPED
      ↓
UNVISITED
      ↓
PLANNED
      ↓
VISITED
```

Status merchant canvassing canonical:

- `unvisited`
- `planned`
- `visited`
- `revisit_required`
- `excluded`

Status coverage desa:

- `scraped`
- `in_progress`
- `completed`

`completed` berarti tidak ada merchant yang masih `unvisited`, `planned`, atau `revisit_required`. Ini bukan berarti semua merchant berhasil menjadi Active.

## 4. Pipeline 2 — Sales Bukupay

Sesudah interaksi sales, proses bisnis dilanjutkan melalui pipeline sales:

```text
PRESENTED
   ↓
INTERESTED
   ↓
FOLLOW UP
   ↓
REGISTRATION
   ↓
REGISTERED
   ↓
INSTALLATION
   ↓
INSTALLED
   ↓
ACTIVE
```

Exception/terminal antara lain:

- `not_interested`
- `already_soundbox`
- `closed`
- `invalid_lead`

Merchant terminal tersebut tidak dipilih lagi untuk fresh canvassing normal, tetapi datanya tetap disimpan.

## 5. Model satu desa satu dataset kerja

Contoh:

```text
Desa Sirnagalih
Total scrape       186
Unvisited          111
Planned tomorrow    25
Visited             63
Revisit required    12
```

Tidak perlu scrape ulang setiap malam. Sistem memakai sisa master merchant desa yang sama sampai coverage selesai.

Re-scrape diperbolehkan bila ingin memperbarui Maps data atau menemukan merchant baru. Upsert tidak mereset field-sales state.

## 6. Daily Visit Plan

Input:

- tanggal rencana;
- desa/location scope;
- titik mulai (`start_lat`, `start_lon`);
- target, default `25`.

Candidate pool:

```text
UNVISITED
+
REVISIT_REQUIRED yang sudah jatuh tempo
```

Candidate harus memiliki koordinat valid agar dapat dirutekan otomatis.

Merchant yang sudah `active`, `installed`, `not_interested`, `already_soundbox`, `closed`, atau `invalid_lead` dikeluarkan dari fresh route.

### Algoritma P0

P0 menggunakan Haversine + nearest-neighbor:

```text
START
 ↓ merchant terdekat
#1
 ↓ merchant terdekat dari #1
#2
 ↓ merchant terdekat dari #2
...
#25
```

Jarak disimpan per leg sebagai `distance_from_previous_km`.

Rencana yang sudah dibuat disimpan ke database dan tidak dihitung ulang setiap halaman dibuka.

Road distance / travel duration menggunakan routing engine seperti OSRM adalah P1, bukan syarat P0.

## 7. Revisit rules P0

Hasil kunjungan:

### Owner/PIC tidak ada

```text
visit_status = revisit_required
revisit default = +3 hari
```

### Toko tutup sementara

```text
visit_status = revisit_required
revisit default = +7 hari
```

### Visit selesai / sudah diklasifikasikan

Hasil seperti `visited`, `presented`, `interested`, `follow_up`, `registered`, `installed`, `active`, `already_soundbox`, dan `not_interested` menandai coverage merchant sebagai `visited`.

Sales status tetap dikelola terpisah.

## 8. History

Setiap kunjungan membuat baris baru di `merchant_visit_history`.

Contoh:

```text
10 Sep — owner_not_found — owner sedang keluar
13 Sep — presented — presentasi ke owner
16 Sep — registered — registrasi selesai
```

History tidak ditimpa oleh kunjungan berikutnya.

## 9. Scraper policy

Preset Bukupay tidak boleh melakukan business filtering agresif.

P0 policy:

- title diperlukan sebagai identitas minimum;
- phone tidak wajib;
- website tidak wajib;
- rating tidak wajib;
- review tidak wajib;
- tidak ada exclude-title business rules;
- dedup tetap aktif;
- semua hasil yang lolos validasi teknis di-upsert ke master database.

## 10. P0 checklist

### Data foundation

- [x] Master merchant menggunakan `prospects` + upsert/dedup.
- [x] Re-scrape tidak menghapus sales data.
- [x] Scrape scope desa diregistrasikan di `coverage_areas`.
- [x] Semua merchant scope memiliki `merchant_visit_state`.
- [x] Visit status dipisahkan dari data master Google Maps.
- [x] Visit history disimpan terpisah.

### Planning

- [x] Rencana harian disimpan di `visit_plans`.
- [x] Detail urutan disimpan di `visit_plan_items`.
- [x] Target default 25 merchant.
- [x] Merchant diurutkan nearest-neighbor berdasarkan koordinat.
- [x] Jarak Haversine per leg disimpan.
- [x] Existing active/terminal merchant tidak masuk fresh canvassing.
- [x] Revisit due dapat masuk kembali ke candidate pool.

### Field execution

- [x] Owner tidak ada -> revisit +3 hari secara default.
- [x] Toko tutup -> revisit +7 hari secara default.
- [x] Kunjungan memperbarui state tanpa menghapus history.
- [x] Re-scrape tidak mereset visit state.

### Validation

- [x] Unit test untuk urutan jarak.
- [x] Unit test untuk revisit dan re-scrape preservation.
- [ ] GitHub Actions CI hijau untuk commit P0.

## 11. P1 setelah P0

Setelah backend P0 stabil:

1. UI pemilihan desa + progress coverage;
2. form titik mulai manual / browser GPS;
3. tombol `Buat Rute Besok`;
4. tampilan 25 merchant berurutan;
5. OSRM road distance + travel duration;
6. route adjustment / backup merchant;
7. dashboard Bukupay khusus sales harian.

## 12. P2

- cluster visualization/map;
- daily backup list;
- route optimization lebih kuat daripada greedy nearest-neighbor;
- analytics conversion per desa/kecamatan;
- reactivation/cooldown policy yang lebih granular;
- PWA/mobile field-sales experience.
