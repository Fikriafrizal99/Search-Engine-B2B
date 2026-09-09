# Bukupay UI V2 Visual References

Approved visual references for the Bukupay Sales UI V2 redesign.

Target desktop layout: match the approved reference structure and proportions as closely as practical. Use real application data and working actions; never hardcode the sample values shown in the mockups.

Expected local reference files:
- `docs/ui-reference/dashboard.jpg` -> `/sales`
- `docs/ui-reference/area-planner.jpg` -> `/areas`
- `docs/ui-reference/merchant-pipeline.jpg` -> `/merchants`
- `docs/ui-reference/visit-session.jpg` -> `/contact`
- `docs/ui-reference/database-scraper.jpg` -> `/database`

## Install local image pack

The high-quality image pack is distributed as `bukupay-ui-reference-high.zip`.

After downloading it to the machine that runs Codex CLI:

```bash
cd ~/Bukupay-Sales
git fetch origin
git switch feat/bukupay-ui-v2
git pull origin feat/bukupay-ui-v2
bash scripts/install-ui-reference.sh /path/to/bukupay-ui-reference-high.zip
```

Then verify:

```bash
ls -lh docs/ui-reference/*.jpg
```

Codex CLI should use these local files as the visual source of truth.

## Rules

1. Existing Go backend, SQLite data, OSRM routing, Google Maps scraper, geo APIs, coverage state, merchant history and pipeline must remain functional.
2. No dead buttons, fake KPIs, fake merchant rows, fake activity, or fake maps.
3. All mockup controls must be backed by existing logic or implemented properly.
4. Keep canonical pipeline: `TO VISIT -> VISITED -> PRESENTED -> INTERESTED -> FOLLOW UP -> REGISTRATION -> REGISTERED -> INSTALLATION -> INSTALLED -> ACTIVE`.
5. Coverage remains separate: `SCRAPE -> UNVISITED -> PLANNED -> VISITED/REVISIT -> COMPLETED`.
6. Do not merge automatically.
