---
category: Added
---

- **AI Table server-side statistics** — adds `dws aitable record stats` for
  ungrouped record-set metrics through `query_records_stats`, plus `dws aitable
  record group-stats` for grouped, distinct, and advanced aggregation through
  `query_stats`; both commands validate their JSON aggregation contracts before
  dispatch.
