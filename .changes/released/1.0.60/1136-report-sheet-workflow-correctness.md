---
category: Changed
---

- **Report latest lookup** — scans bounded, strictly advancing outbox pages, reconciles duplicate IDs, and reads back the uniquely newest report instead of failing on the first continuation page.
- **Sheet create-with-data result** — returns the already probed `sheetId` at the top level while preserving legacy `.result.nodeId` and outer `requestId`, avoids repeating the sheet-list probe, keeps the main-compatible single readback check, and reports post-create partial/unknown state without unsafe whole-workflow retries.
- **Sheet workflow routing** — distinguishes local analysis from Excel-to-online import, exposes template discovery and apply routes, and preserves the full data-validation tri-state contract.
- **Received-report helper and routing** — restores same-profile sender resolution before inbox filtering, keeps Mono and Multi helpers identical, uses bounded complete pagination, renders epoch timestamps in the Shanghai timezone, fails closed instead of returning incomplete data, and keeps midnight query windows valid.
