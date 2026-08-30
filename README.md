# Search Engine B2B

Standalone search engine and prospect database for **public B2B listings** from Google Maps, intentionally separated from the kost/boarding-house project.

## What is included

- Java + Sumatra geographic scope
- Cascading location selector: Province -> Regency/City -> District -> Village/Kelurahan
- Public region API with local cache
- 32 default business-category queries
- Google Maps collection through the upstream `gosom/google-maps-scraper` executable
- Filtering and deduplication
- SQLite prospect database (`data/prospects.db`)
- B2B dashboard
- Business scale, prospect priority, contact status, verification, and QC
- CSV and XLSX export
- Manual CSV import
- GitHub Actions CI

## B2B-only data model

The database stores business listing fields plus:

- Business Scale
- Priority
- Contact Status
- Detailed Business Type
- Operational Scale
- Products / Services
- Service Area
- Prospect Fit
- Verification Status
- Internal Notes
- QC Status / QC Note

There are **no kost-specific fields** in this repository.

## Setup

Requirements:

- Go 1.22+
- A built `gosom/google-maps-scraper` executable available locally

```bash
go mod tidy
make test
make build
```

Place or point to the upstream scraper binary, then run:

```bash
./bin/b2b-dashboard \
  -engine /path/to/google_maps_scraper \
  -collector ./bin/search-engine-b2b
```

Open:

```text
http://localhost:8080
```

## Direct collector example

```bash
./bin/search-engine-b2b \
  -engine /path/to/google_maps_scraper \
  -location "Sukamulya, Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia" \
  -- -c 2 -depth 5
```

The filtered CSV is written to `data/prospects.csv` and imported into `data/prospects.db` by default.

## Important boundary

This tool discovers and organizes **public business listings**. Priority and prospect-fit fields are for business workflow/contact management; they must not be used to infer that a named person or business is in financial distress.
