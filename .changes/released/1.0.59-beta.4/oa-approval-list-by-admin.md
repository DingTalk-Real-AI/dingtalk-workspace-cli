---
category: Added
---

- **OA admin approval query** — `oa approval list-by-admin` queries approval instances of a template with admin scope, with simple flags and an advanced `--request` mode; `startTime`/`endTime` use `yyyy-MM-dd HH:mm:ss` strings per the 2026-08 MCP contract update (ISO-8601 flag inputs auto-convert), and pageSize/time format are validated client-side with localized errors.
