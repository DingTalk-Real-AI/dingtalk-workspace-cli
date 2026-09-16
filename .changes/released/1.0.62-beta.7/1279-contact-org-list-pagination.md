---
category: Fixed
---

- **Contact org list pagination contract** — `dws contact org invite-list` and
  `dws contact org apply-list` now declare the unified cursor Pagination contract
  (`cursor` parameter, `meta.pagination` metadata) and their result data schemas
  no longer leak `hasMore`/`nextCursor`. Runtime responses project the server-side
  cursor fields into `meta.pagination` so Agents can resume paging from the
  standard contract.
