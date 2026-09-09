# Architecture — Bukupay Merchant Hunter

Branch `feat/bukupay-sales` adalah aplikasi merchant acquisition Bukupay yang terpisah dari workflow BPKB.

Dokumen detail coverage dan visit planning ada di [`docs/BUKUPAY_VISIT_SYSTEM.md`](BUKUPAY_VISIT_SYSTEM.md).

## Core flow

```text
Region selector (Province -> Regency/City -> District -> Village)
        ↓
Google Maps scrape per desa
        ↓
Technical dedup / normalization
        ↓
Master Merchant Database (`prospects`)
        ↓
Coverage / Visit Planning
        ↓
25 merchant per hari berdasarkan jarak
        ↓
Visit history
        ↓
Sales pipeline Bukupay
        ↓
Registration -> Installation -> Active
```

## Data ownership

```text
prospects
  public Google Maps/master discovery data
        |
        +-> coverage_areas
        |     progress satu location scope/desa
        |
        +-> merchant_visit_state
        |     canonical canvassing state
        |
        +-> merchant_visit_history
        |     immutable field visit history
        |
        +-> visit_plans / visit_plan_items
        |     persisted daily route
        |
        +-> lead_execution / contact_events
        |     communication/follow-up execution
        |
        +-> merchant_sales / merchant_events
              Bukupay sales facts and conversion pipeline
```

## Important rules

1. Scrape data is retained as master merchant data; no aggressive business filtering.
2. Duplicate records are deduplicated/upserted rather than re-inserted.
3. Phone, website, rating, and review are not mandatory.
4. Coordinates are required only for automatic route planning, not for retaining a merchant in the master database.
5. Re-scraping updates Maps data and `last_seen`; it must not reset visit or sales history.
6. Visit state and sales state are separate concerns.
7. P0 route ordering uses Haversine nearest-neighbor from the chosen start point.
8. Road routing/OSRM is an enhancement after the P0 data and planning foundation is stable.
