# Bukupay Sales Workspace

Sales Workspace is the post-visit workspace. Visit Session records field facts; Sales Workspace processes merchants that have entered the sales pipeline.

## Workflow

```text
Visit Session
  Presented   -> Sales: Presented
  Interested  -> Sales: Interested
  Follow Up   -> Sales: Follow Up

Sales Workspace
  Interested -> Registration -> Registered -> Installation -> Installed -> Active
```

Coverage-only states (`to_visit`, `visited`, `owner_not_found`) stay outside the default Sales Workspace list. Terminal sales exceptions such as `not_interested` and `already_soundbox` remain available through sales filters/audit.

## Primary Sales form

The main sales form deliberately exposes only:

- Status Sales
- Punya QRIS?
- Provider QRIS
- Punya Soundbox?
- Next Action
- Tanggal Follow-up
- Catatan

Secondary merchant fields remain preserved in storage but are not shown in the primary form. Visit history is collapsed by default and the latest visit can still be corrected through the existing correction flow.

## UX rules

- Mobile first; one-column form on phones.
- Merchant summary and communication shortcuts stay at the top.
- Six-step sales progress is visible: Interested, Registration, Registered, Installation, Installed, Active.
- Save action is prominent and sticky on small screens.
- Sales list uses cards rather than a wide table on mobile.
- No route/coverage execution controls are duplicated inside Sales Workspace.
