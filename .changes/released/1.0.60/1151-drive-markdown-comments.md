---
category: Added
---

- **Drive local-file comments** (#1151) — adds the complete global comment
  lifecycle for Drive files through the shared document comment service,
  including `create-v2`, `list-v2`, reply, update, delete, batch query,
  direct-reply listing, resolve, restore, and reaction replies. The existing
  `create` and `list` leaves retain their legacy behavior and output contract
  with deprecation guidance for an explicit migration.
- **Markdown comment reads** (#1151) — adds comment listing for native Markdown
  files with global, inline, resolution-status, and cursor filters, and exposes
  direct-reply listing across the shared Doc and Sheet comment lifecycle.
