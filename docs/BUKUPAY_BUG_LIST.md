# Bukupay Bug List

Status convention:

- `SOLVED` = code is fixed and confirmed in runtime or otherwise fully validated.
- `FIXED_CODE` = implementation/tests are present, but real-device/runtime confirmation is still required.
- `CLOSED` = investigated and not retained as an application logic bug.
- `OPEN` = not fixed yet.

| ID | Issue | Status | Notes |
|---|---|---|---|
| BUG #1 | Scraper status showed hardcoded 32 recommended queries | SOLVED | Query count now derives from the Bukupay preset dynamically. |
| BUG #2 | Area Irisan loaded entire neighboring cities/regencies | SOLVED | Border-area guardrail/whitelist applied while Semua Area remains broad. |
| BUG #3 | Dashboard and Merchant summary groups were left-heavy | SOLVED | Centered responsive card/stage layouts applied. |
| BUG #4 | Area Planner selector temporarily looked stuck after scrape | CLOSED | No persistent selector logic defect reproduced; high Chromium load was the likely runtime contributor. |
| BUG #5 | Google Maps scraper/Playwright Chromium stayed alive after `scrapemate exited` | FIXED_CODE | Collector detects completed scraper shutdown, gives a grace period, then terminates the process group and continues normalize/import. Needs one real scrape confirmation through `PHASE done`. |
| BUG #6 | `bukupay.db` could be created as `root:root` and become unwritable from host maintenance | SOLVED | Container runs with host-compatible UID/GID; runtime verified UID/GID 1000:1000 and SQLite write test passed. |
| BUG #7 | Generate Route failed with hundreds of eligible merchants | SOLVED | OSRM matrix now uses a distance shortlist instead of sending the entire 504+ candidate pool. Runtime route generation confirmed working. |
| BUG #8 | Dashboard → Buat Rute opened without Area Kerja | FIXED_CODE | Dashboard carries the selected/single area and route form supports area selection when multiple areas exist. Runtime confirmation still required. |
| BUG #9 | No safe way to correct an already saved visit | FIXED_CODE | Added latest-visit correction flow with audit table, stable Visit Count, Coverage/route state resync, and Sales Status resync. Runtime confirmation still required. |
| BUG #10 | Visit result list mixed field outcomes with downstream sales stages; “Soundbox terpasang” was ambiguous | FIXED_CODE | Visit Session now separates field outcomes from Registration/Installation/Installed/Active, clarifies “already had Soundbox”, and validates field results server-side. Runtime confirmation still required. |
| BUG #11 | Database insight cards and typography were inconsistent/small, especially on mobile | FIXED_CODE | Added consistent typography scale, equal Data Insights cards, compact area label, clearer metric hierarchy, workflow chips, responsive/mobile rules, and larger table/form/helper text. Runtime visual confirmation still required. |

## Current stabilization gate

Before marking the remaining items `SOLVED`:

1. Run one real scrape and confirm it reaches normalize, database import, and `PHASE done` without orphaned Chromium (`BUG #5`).
2. Open Dashboard → Buat Rute and confirm Area Kerja is usable directly (`BUG #8`).
3. Enter a visit, use **Koreksi Visit Terakhir**, and verify Visit Count stays unchanged while the corrected result and Sales Status update (`BUG #9`).
4. Confirm Visit Session no longer offers Bukupay `Installed`/`Active` as visit outcomes and “Sudah punya Soundbox sebelumnya” becomes a terminal exception rather than an Installed sale (`BUG #10`).
5. Check Database on desktop and mobile widths for card alignment, text hierarchy, table readability, and workflow wrapping (`BUG #11`).
