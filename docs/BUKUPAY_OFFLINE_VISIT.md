# Bukupay Offline Visit — Download / Upload

## Tujuan

Fitur ini adalah fallback lapangan ketika Bukupay tidak dapat diakses saat rute sedang berjalan. Sales dapat mengunduh rute ke XLSX sebelum berangkat, mengisi hasil kunjungan secara offline bila aplikasi bermasalah, lalu mengunggah file tersebut setelah sistem kembali normal. Tidak perlu memasukkan ulang visit satu per satu.

## Alur

1. Buat rute kunjungan harian seperti biasa.
2. Di halaman **Rute Kunjungan**, tombol **Download** dan **Upload** tersedia.
3. **Download** menghasilkan file `bukupay-visit-YYYY-MM-DD-rute-ID.xlsx` untuk rute tersebut.
4. Jika aplikasi tetap normal, file tidak perlu digunakan.
5. Jika aplikasi tidak dapat diakses di lapangan, isi hasil visit pada XLSX.
6. Setelah Bukupay kembali normal, buka rute yang sama dan tekan **Upload**.
7. Bukupay memvalidasi file dan memasukkan hanya baris yang memiliki **Hasil Visit**.
8. Visit masuk menggunakan **tanggal rute** dan **Jam Visit** dari file, bukan tanggal/jam upload.

## Kolom XLSX

Kolom yang terlihat:

| Kolom | Fungsi |
| --- | --- |
| Urutan | Urutan merchant pada rute |
| Merchant | Nama merchant |
| Kategori | Kategori Google Maps |
| Alamat | Alamat merchant |
| Telepon | Nomor telepon bila tersedia |
| Google Maps | URL lokasi merchant |
| Hasil Visit | Dropdown yang sama dengan Visit Session |
| Jam Visit | Jam kunjungan aktual, format `HH:MM` |
| Owner / PIC | Nama owner/PIC bila diketahui |
| Jadwal Follow-up | Digunakan terutama untuk hasil `Perlu follow-up` |
| Catatan | Catatan hasil kunjungan |

Kolom teknis seperti `plan_id`, `plan_item_id`, `prospect_id`, `plan_date`, `template_version`, dan `exported_at` disimpan dalam kolom tersembunyi. Kolom ini dipakai untuk memastikan file hanya dapat masuk ke rute dan merchant yang benar. Jangan menghapus atau mengubah kolom tersembunyi tersebut.

## Dropdown Hasil Visit

Pilihan pada XLSX sama dengan Visit Session:

- Kunjungan selesai — belum presentasi
- Sudah presentasi Bukupay
- Tertarik — lanjut proses
- Perlu follow-up
- Sudah punya Soundbox sebelumnya
- Tidak tertarik
- Owner / PIC tidak ada
- Toko tutup sementara

Dropdown merupakan data validation native XLSX sehingga dapat digunakan tanpa koneksi internet pada aplikasi spreadsheet yang mendukung validasi XLSX.

## Aturan Waktu

`plan_date` dari rute adalah tanggal visit yang otoritatif. Sales hanya mengisi **Jam Visit**. Contoh: rute tanggal 11 September, visit diisi `10:15`, lalu file baru di-upload tanggal 12 September. Sistem tetap menyimpan `visited_at` sebagai 11 September pukul 10:15 WIB.

Untuk `Perlu follow-up`, kolom **Jadwal Follow-up** wajib diisi. Format yang disarankan adalah `YYYY-MM-DD HH:MM`.

Untuk `Owner / PIC tidak ada`, bila jadwal tidak diisi sistem menerapkan kebijakan revisit minimal +3 hari dari waktu visit. Untuk `Toko tutup sementara`, sistem menerapkan minimal +7 hari.

## Sinkronisasi ke Sistem

Upload menggunakan efek bisnis yang sama dengan Visit Session:

- visit history bertambah;
- visit state dan jumlah visit diperbarui;
- route item berubah menjadi selesai atau revisit;
- waktu interaksi disimpan sesuai waktu visit offline;
- hasil yang masuk sales pipeline tetap mengikuti mapping Visit Session;
- `Tertarik — lanjut proses` masuk ke Sales Workspace sebagai `Interested`;
- hasil revisit mengikuti kebijakan +3/+7 hari;
- Report Harian akan membaca visit tersebut pada tanggal rute yang benar.

Tahap downstream `Registration → Registered → Installation → Installed → Active` tetap dikelola dari Sales Workspace dan tidak menjadi pilihan Hasil Visit.

## Anti Duplikasi dan Konflik

Upload bersifat aman untuk dicoba ulang. Bukupay hanya menerima row jika route item masih berstatus `planned`. Setelah satu row berhasil masuk, status route item berubah sehingga upload file yang sama untuk kedua kalinya akan melewati row tersebut sebagai **Sudah Tercatat**.

Jika sebagian merchant sudah dicatat melalui aplikasi sebelum outage, row tersebut juga dilewati saat file offline di-upload. Dengan demikian upload tidak membuat visit kedua untuk merchant yang sudah selesai pada rute yang sama.

Baris dengan **Hasil Visit** kosong selalu dilewati dan tidak mengubah data apa pun.

Sebelum data diubah, sistem memvalidasi seluruh metadata file terhadap rute tujuan. File dari rute lain atau metadata yang tidak cocok ditolak.

## Hasil Upload

Setelah upload, Bukupay menampilkan ringkasan:

- **Masuk** — visit baru yang berhasil disinkronkan;
- **Kosong** — row tanpa hasil visit;
- **Sudah Tercatat** — row yang sudah selesai / tidak perlu diimport ulang;
- **Gagal** — row yang perlu diperbaiki, misalnya jam visit kosong atau jadwal follow-up wajib belum diisi.

Maksimal beberapa error pertama ditampilkan agar mudah diperbaiki tanpa memenuhi layar.

## Catatan Operasional

Download bersifat opsional. Membuat rute tidak otomatis mengunduh file. Sebelum aktivitas lapangan, tekan **Download** bila ingin membawa fallback offline.

Gunakan file dari rute yang akan dikerjakan. Jangan menghapus kolom atau memindahkan data antar-row secara manual. Jika ingin mengurutkan/filter file, gunakan filter spreadsheet pada seluruh tabel agar metadata tersembunyi tetap mengikuti row merchant.

File upload maksimal 10 MB dan endpoint hanya ditujukan untuk workbook XLSX yang dibuat oleh Bukupay.
