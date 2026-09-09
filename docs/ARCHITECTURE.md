# Architecture — Bukupay Merchant Hunter

`feat/bukupay-sales` memisahkan merchant acquisition Bukupay dari workflow lama.

## Flow

1. Region selector: Province -> Regency/City -> District -> Village/Kelurahan.
2. Preset `bukupay-merchants` atau custom keyword membentuk query Google Maps.
3. `gosom/google-maps-scraper` mengumpulkan listing bisnis publik.
4. Collector melakukan filtering dan deduplication. Nomor telepon tidak wajib karena merchant dapat diproses lewat canvassing lapangan.
5. Listing bersih diimpor ke `data/prospects.db`.
6. Visit / Prospecting Session mencatat aktivitas sales.
7. Merchant yang relevan masuk ke `merchant_sales` dan Merchant Pipeline.
8. Pipeline melacak presentasi, minat, registrasi, instalasi Soundbox, dan aktivasi merchant.

## Separation

Discovery data dan sales execution dipisahkan:

```text
prospects / prospect_profiles
        |
        +-> lead_execution / contact_events
        |
        +-> merchant_sales / merchant_events
```

`prospects` menyimpan listing publik. `merchant_sales` hanya menyimpan fakta hasil observasi atau komunikasi sales seperti QRIS, Soundbox, PIC, traffic, interest, serta status registrasi/instalasi/aktivasi.