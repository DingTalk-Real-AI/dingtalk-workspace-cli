---
category: Fixed
---

- Reject nonzero `--offset` with `aitable +workflow-list --all`, including status-filtered reads, before making any request. Full-list results now always scan from the beginning; ordinary offset pagination is unchanged.
