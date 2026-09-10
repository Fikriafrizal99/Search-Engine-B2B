# Bukupay UI/UX Execution Tasks

Baseline branch: `feat/bukupay-ui-v2-phase3`

Tujuan dokumen ini adalah menggabungkan audit UI/UX terbaru dengan keputusan workflow sebelumnya agar implementasi bisa dikerjakan satu per satu tanpa membuat halaman tumpang tindih.

## Prinsip utama

Setiap halaman hanya memiliki satu fungsi utama:

```text
Dashboard      = Monitor
Area Planner   = Plan
Route          = Review
Visit Session  = Execute
Merchant       = Follow-up / Sales
Database       = Inspect / Maintain
```

Workflow utama:

```text
Dashboard
   ↓
Area Planner
pilih kelurahan → scrape → coverage
   ↓
Buat Route
25 merchant
   ↓
Route Kunjungan
review urutan
   ↓
Visit Session
#1 → #2 → ... → #25
   ↓
Merchant Pipeline
follow-up → registration → installation → active

Database = master data di belakang seluruh proses
```

---

# P0 — WAJIB SEBELUM UI TAMBAHAN

## P0.1 — Visit Session harus route-aware

**Masalah sekarang**

Route sudah memiliki urutan benar, tetapi ketika masuk ke Visit Session konteks route hilang dan halaman kembali memakai Action Queue global.

**Tugas**

- [ ] Saat membuka Visit dari `visit_plan`, kirim `plan_id` dan `item_id/sequence` ke Visit Session.
- [ ] Visit Session membaca merchant dari `visit_plan_items`, bukan ContactNavigation global, jika ada route aktif.
- [ ] Tampilkan posisi `Merchant #x dari 25` berdasarkan sequence route.
- [ ] Previous / Next mengikuti urutan route yang tersimpan.
- [ ] Merchant yang sudah selesai tetap mempertahankan sequence asli.
- [ ] Merchant revisit tidak menyebabkan seluruh route dihitung ulang.
- [ ] Jika tidak ada route aktif, Action Queue lama tetap menjadi fallback.

**Acceptance criteria**

```text
Route #1 → Visit Session #1
Save
→ otomatis Visit Session #2
Save
→ #3
...
→ #25
```

Tidak boleh lompat ke merchant global di luar route.

**Files likely impacted**

- `cmd/dashboard/contact.go`
- `cmd/dashboard/contact.html`
- `cmd/dashboard/visit_plan.go`
- `cmd/dashboard/visit_plan.html`
- `internal/prospectstore/visit_plan*.go`

---

## P0.2 — Save hasil visit harus lanjut ke merchant berikutnya di route

**Masalah sekarang**

Setelah submit hasil, redirect kembali ke `/contact?mode=...`, sehingga route context hilang.

**Tugas**

- [ ] Preserve `plan_id` dan route item saat form submit.
- [ ] Setelah save, cari next unfinished item di route yang sama.
- [ ] Redirect ke next route item.
- [ ] Jika route selesai, tampilkan summary completion.
- [ ] Jika merchant hasilnya `owner_not_found` / `store_closed`, route item tetap dianggap sudah dieksekusi hari itu dan merchant masuk revisit queue sesuai policy.

**Acceptance criteria**

Tombol:

```text
Simpan & Lanjut Merchant #9
```

benar-benar membuka #9 dari route yang sama.

---

## P0.3 — Route Kunjungan hanya menjadi overview / controller

**Masalah sekarang**

Setiap stop punya terlalu banyak action: Maps, Visit, Detail, Call, WA. Ini membuat Route menjadi execution workspace kedua.

**Tugas**

- [ ] Pertahankan summary: merchant, selesai, sisa, revisit, jarak, durasi.
- [ ] Pertahankan sequence + distance per leg + route status.
- [ ] CTA utama hanya `Mulai / Lanjutkan Visit`.
- [ ] Action per stop dipangkas menjadi maksimal `Maps` + `Detail`.
- [ ] Hapus Call/WA sebagai action utama dari halaman route.
- [ ] Tombol Visit per stop jika dipertahankan hanya membuka Visit Session pada sequence tersebut, bukan workflow terpisah.
- [ ] Buat list route lebih compact pada mobile.

**Acceptance criteria**

Route dapat dipakai untuk melihat big picture, tetapi pencatatan hasil hanya dilakukan di Visit Session.

---

## P0.4 — Scrape harus tetap di Area Planner

**Masalah sekarang**

UI scraper sudah ada di Area Planner, tetapi setelah start backend masih redirect ke Database dan JS mengikuti redirect tersebut.

**Tugas**

- [ ] `/bukupay/collect` tidak redirect ke `/database` untuk workflow Area Planner.
- [ ] Return response yang bisa dipakai Area Planner untuk mengetahui job berhasil dimulai.
- [ ] Area Planner tetap di halaman yang sama setelah Start Scrape.
- [ ] Poll `/api/collect/status` dari Area Planner.
- [ ] Tampilkan state: preparing, scraping, processing, importing, done, failed, cancelled.
- [ ] Tampilkan progress operasional yang ringkas.
- [ ] Detail log dibuat collapsible.
- [ ] Setelah done, refresh coverage dan snapshot area otomatis.
- [ ] CTA berubah menjadi `Buat Rute Besok` jika merchant sudah routable.
- [ ] Database tetap menyimpan data dan log di belakang layar.

**Acceptance criteria**

```text
Area Planner
→ Scrape Kelurahan
→ tetap di Area Planner
→ progress
→ selesai
→ coverage refresh
→ Buat Rute Besok
```

**Files likely impacted**

- `cmd/dashboard/database_bukupay.go`
- `cmd/dashboard/area.html`
- `cmd/dashboard/ui/area-planner.js`
- `cmd/dashboard/main.go`

---

## P0.5 — Pisahkan Coverage dan Sales secara visual

**Masalah sekarang**

Merchant Pipeline masih menampilkan `TO VISIT` dan `VISITED` bersama sales stages sehingga coverage dan sales terlihat seperti satu pipeline.

**Tugas**

- [ ] Merchant Pipeline utama mulai dari `PRESENTED` atau `INTERESTED` sesuai final rule backend.
- [ ] Coverage statuses ditampilkan terpisah: `UNVISITED`, `PLANNED`, `VISITED`, `REVISIT`.
- [ ] `Owner/PIC tidak ada` tampil sebagai revisit/coverage exception, bukan sales conversion stage.
- [ ] Keep exception sales: Not Interested, Already Soundbox, Closed, Invalid Lead.
- [ ] Merchant Detail tetap menampilkan Coverage Visit dan Sales Progress sebagai dua blok berbeda.

**Acceptance criteria**

User dapat menjawab dua pertanyaan berbeda tanpa bingung:

1. Sudah berapa merchant dikunjungi?
2. Dari yang sudah dikunjungi, berapa yang masuk sales pipeline?

---

## P0.6 — Konsistensi CTA dan navigation

**Tugas**

- [ ] Navbar tetap: Dashboard · Area Planner · Merchant · Visit Session · Database.
- [ ] Dashboard selalu menuju `/sales`.
- [ ] Database selalu menuju `/database`.
- [ ] CTA `Mulai Visit` mengutamakan route aktif hari ini bila tersedia.
- [ ] CTA `Buat Rute` hanya ada ketika area sudah memiliki candidate routable.
- [ ] Hindari CTA yang melakukan fungsi sama di tiga halaman berbeda.

**Acceptance criteria**

Satu action utama memiliki satu rumah:

```text
Scrape       → Area Planner
Generate     → Area Planner / Route Form
Execute      → Visit Session
Follow-up    → Merchant
Inspect data → Database
```

---

# P1 — SIMPLIFIKASI UI/UX SETELAH P0 STABIL

## P1.1 — Simplify Visit Session

Visit Session harus menjadi halaman paling sederhana karena dipakai saat field execution.

**Remove / collapse**

- [ ] Hapus Sales Pipeline lengkap dari tampilan utama Visit Session.
- [ ] Kurangi KPI global 7 kartu menjadi progress route + follow-up/revisit yang relevan.
- [ ] Session Summary yang redundant digabung ke header route progress.
- [ ] Activity History default collapsed / `Lihat riwayat`.

**Keep utama**

```text
Area / Route Hari Ini
7 / 25 selesai
Merchant #8
Maps / Call / WA
Catat hasil
Save & Next
```

---

## P1.2 — Simplify Merchant Detail

**Tugas**

- [ ] Kelompokkan field menjadi:
  - Informasi Merchant
  - QRIS & Soundbox
  - Sales Progress
  - Next Action
  - History
- [ ] Traffic / transaction / optional qualification masuk `Detail Tambahan` collapsible.
- [ ] Coverage status read-only di blok Coverage Visit.
- [ ] Jangan membuat user mengubah coverage state lewat dropdown sales.
- [ ] Merchant History dan Visit History tetap terpisah tetapi dapat dibuat collapsible di mobile.

---

## P1.3 — Simplify Dashboard

**Tugas**

- [ ] Pertahankan KPI utama.
- [ ] Pertahankan Route Hari Ini / Besok.
- [ ] Pertahankan Coverage.
- [ ] Pertahankan Sales Pipeline.
- [ ] Hapus `Akses Cepat` jika navbar sudah mencukupi.
- [ ] `Prioritas Hari Ini` hanya untuk item di luar route aktif: follow-up/revisit/action due.
- [ ] Jangan menampilkan merchant route hari ini lagi sebagai priority card jika sudah ada route card.

---

## P1.4 — Compact Route untuk mobile

**Tugas**

- [ ] Kurangi tinggi kartu per merchant.
- [ ] Nama + sequence + jarak + status menjadi satu baris/card compact.
- [ ] Address dibuat 1–2 line ellipsis.
- [ ] Maps/Detail dibuat small secondary actions.
- [ ] Completed items visually subdued.
- [ ] `NEXT` item dibuat paling menonjol.

---

## P1.5 — Simplify Area Planner bottom section

**Tugas**

- [ ] Snapshot Area tetap dipertahankan.
- [ ] `Rekomendasi Berikutnya` hanya satu primary recommendation berdasarkan state.
- [ ] Hindari list rekomendasi panjang jika CTA sudah jelas.
- [ ] Jika belum scrape → `Scrape Kelurahan Ini`.
- [ ] Jika sudah scrape dan routable → `Buat Rute Besok`.
- [ ] Jika route besok sudah ada → `Lihat Rute Besok`.
- [ ] Jika coverage selesai → `Pilih Area Berikutnya`.

---

## P1.6 — Database cleanup

Database boleh lebih padat karena fungsinya memang inspection / maintenance.

**Tugas**

- [ ] Pertahankan search/filter/export/master table.
- [ ] Pertahankan scraper status sebagai informasi saja.
- [ ] CTA Visit dibuat secondary.
- [ ] CTA utama untuk scrape tetap `Kelola Scrape Area`.
- [ ] Jangan duplikasi kontrol scraper lengkap di Database.

---

# P2 — SETELAH WORKFLOW HARIAN NYAMAN

- [ ] PWA / installable mobile app.
- [ ] Sticky bottom action pada Visit Session mobile.
- [ ] Map visualization untuk route.
- [ ] Backup merchant list untuk route harian.
- [ ] Route reorder manual opsional.
- [ ] Analytics conversion per area.
- [ ] Area completion recommendations.
- [ ] Better route optimizer jika nearest-neighbor sudah tidak cukup.

---

# Urutan eksekusi yang disarankan

Kerjakan satu task = satu commit agar mudah rollback.

```text
1. P0.1 Route-aware Visit Session
2. P0.2 Save → Next route merchant
3. P0.3 Route overview cleanup
4. P0.4 Area Planner scrape stays in place
5. P0.5 Coverage vs Sales visual separation
6. P0.6 CTA/navigation consistency
7. Regression test P0
8. P1.1 Simplify Visit Session
9. P1.2 Simplify Merchant Detail
10. P1.3 Simplify Dashboard
11. P1.4 Compact mobile Route
12. P1.5 Simplify Area Planner recommendations
13. P1.6 Database cleanup
```

---

# Definition of Done untuk P0

P0 dianggap selesai jika skenario ini berjalan tanpa kebingungan:

```text
MALAM
Area Planner
→ pilih kelurahan
→ scrape
→ tetap di Area Planner
→ coverage muncul
→ generate 25 merchant
→ review Route

BESOK
Visit Session
→ #1 / 25
→ save hasil
→ #2 / 25
→ ...
→ #25 / 25
→ route selesai

FOLLOW-UP
Merchant Pipeline
→ interested
→ follow up
→ registration
→ installed
→ active

DATABASE
→ hanya dipakai jika perlu inspeksi / maintenance data
```

Selain itu:

- [ ] Tidak ada dua execution workspace.
- [ ] Route sequence tidak berubah setelah dibuat.
- [ ] Revisit tidak merusak urutan route hari berjalan.
- [ ] Sales dan coverage tidak terlihat sebagai satu status chain.
- [ ] Scraper tidak memaksa user masuk Database.
- [ ] Mobile Visit Session fokus pada merchant saat ini dan action berikutnya.
- [ ] Existing backend coverage / visit history / merchant history tetap terjaga.
- [ ] `go test ./...` hijau.
- [ ] `go build ./cmd/collector ./cmd/dashboard` hijau.

---

# Scope guard

Saat mengerjakan UI/UX ini:

- jangan mengubah algoritma discovery tanpa kebutuhan langsung;
- jangan menghapus master merchant;
- jangan mengubah route order yang sudah tersimpan;
- jangan menyatukan visit history dengan merchant sales history;
- jangan menambah fitur kosmetik sebelum P0 selesai;
- jangan merge ke branch utama sampai workflow P0 diuji end-to-end.
