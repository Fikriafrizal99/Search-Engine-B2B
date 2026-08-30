# Architecture

Search-Engine-B2B is intentionally separated from the kost/boarding-house search workflow.

Flow:

1. Region selector/API resolves Province -> Regency/City -> District -> Village/Kelurahan.
2. B2B preset supplies business-category keywords.
3. Query builder combines each keyword with the selected administrative scope.
4. The upstream `gosom/google-maps-scraper` executable performs Google Maps collection.
5. B2B post-processing requires public business contact fields, removes financing competitors, and deduplicates records.
6. Output is a B2B-only CSV. CRM/dashboard storage will live only in this repository.

The repository must not add kost-specific fields such as occupant segment, rent price, furnishing, or boarding-house facilities.
