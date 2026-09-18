---
category: Fixed
---

- **Chat conversation categories** — treat `im/list_conversations_by_category` as the declared single-response interface when its explicit conversation array contains no pagination signal, while continuing to fail closed on partial or non-resumable pagination metadata. `+category-list-conversations` and `+feed-group-query-item` now publish the resolved pagination mode and source-exhaustion facts instead of rejecting every live response that omits `hasMore`.
