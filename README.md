# Search Engine B2B

Standalone search engine and prospect database for **public B2B listings** from Google Maps, intentionally separated from the kost/boarding-house project.

## What is included

- Java + Sumatra geographic scope
- Cascading location selector: Province -> Regency/City -> District -> Village/Kelurahan
- Public region API with local cache
- **Free-form custom queries**: enter any business category, one or many at once
- 32 recommended business-category queries as an optional starter preset
- Custom-only mode or custom + default mode
- Google Maps collection through the upstream `gosom/google-maps-scraper` executable
- Filtering and deduplication
- SQLite prospect database (`data/prospects.db`)
- B2B dashboard
- **Contact Session / Action Queue** for sales execution
- Call and WhatsApp shortcuts with editable pre-filled messages
- Contact result logging and contact history
- Follow-up scheduling and retry queue
- CSV and XLSX export
- Manual CSV import
- GitHub Actions CI

## Query model

The 32 default queries are **not a limit**. They are only recommendations. From the dashboard you can type up to 100 custom business keywords per collection run, separated by newline, comma, or semicolon.

Examples:

```text
agen pupuk
toko alat berat
bakery
dealer mobil bekas
distributor minuman
```

The selected location is appended automatically. For example:

```text
agen pupuk + Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia
```

becomes a Google Maps search query for that exact administrative scope.

Two modes are available:

- **Custom only**: disable the default-query checkbox.
- **Custom + defaults**: keep the checkbox enabled and your custom queries are added to the 32 recommendations.

If the custom-query box is empty, the normal 32-query preset is used.

## Sales execution model

The search engine and the sales workflow are separated intentionally:

```text
Google Maps -> Prospect Database -> Contact Session -> Follow-up -> Interested -> Qualified
```

Open the Contact Session from the dashboard or directly at:

```text
http://localhost:8080/contact
```

The Action Queue supports:

- New leads with a public business phone number
- Follow-ups due today
- Overdue follow-ups
- Interested leads
- Qualified leads
- Cases already in process

For each lead the session shows the business profile and provides:

- `Call` using the device `tel:` handler
- `WhatsApp Intro` with a pre-filled introductory message
- `WA Follow-up` with a pre-filled follow-up message
- Result selection: no answer, busy, requested WhatsApp, WA sent, follow-up, interested, qualified, not interested, wrong number, or unreachable
- Optional PIC/owner
- Notes
- Next follow-up date/time in WIB
- Complete contact history

After saving a result, the session automatically advances to the next actionable lead. `No answer` and `Busy` are placed into retry with a default follow-up 24 hours later when no schedule is supplied. A manually selected `Follow-up` requires a scheduled date/time.

WhatsApp messages are **not sent automatically**. The system only opens WhatsApp with editable text already filled in; the operator reviews and presses Send.

## Data model

The database keeps public Maps listing data separate from execution data.

Prospect/listing data includes:

- Business name
- Category
- Address and selected location scope
- Public business phone
- Website
- Rating/review count
- Google Maps URL and IDs

Execution data includes:

- Status
- Last contact
- Last result
- Next follow-up
- Next action
- PIC/owner
- Contact history and notes

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

## Direct collector examples

Default preset:

```bash
./bin/search-engine-b2b \
  -engine /path/to/google_maps_scraper \
  -location "Sukamulya, Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia" \
  -- -c 2 -depth 5
```

Custom-only:

```bash
./bin/search-engine-b2b \
  -engine /path/to/google_maps_scraper \
  -location "Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia" \
  -keywords "agen pupuk; toko alat berat; bakery" \
  -- -c 2 -depth 5
```

Custom + default:

```bash
./bin/search-engine-b2b \
  -engine /path/to/google_maps_scraper \
  -location "Cianjur, Jawa Barat, Indonesia" \
  -keywords "agen pupuk; toko alat berat" \
  -include-defaults=true \
  -- -c 2 -depth 5
```

The filtered CSV is written to `data/prospects.csv` and imported into `data/prospects.db` by default.

## Important boundary

This tool discovers and organizes **public business listings** and manages operator-recorded sales activity. It does not infer that a named person or business is in financial distress. Qualification is based on information actually obtained during communication, not assumptions from Maps data.
