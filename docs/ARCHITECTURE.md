# Architecture

`Search-Engine-B2B` is a standalone B2B prospecting application. It does not share the kost database or kost enrichment schema.

Flow:

1. Cascading region selector resolves Province -> Regency/City -> District -> Village/Kelurahan through a public administrative-region API, cached locally.
2. The B2B preset combines the selected location with business-category queries.
3. The external `gosom/google-maps-scraper` executable collects public Google Maps listings.
4. The collector filters competitor/finance listings, requires public business phone data, and deduplicates results.
5. Clean results are imported into `data/prospects.db`.
6. The web dashboard manages B2B-only profile fields, contact status, QC, and exports CSV/XLSX.

No fields for boarding-house occupant gender, rent price, furnishing, or kost facilities exist in this database.
