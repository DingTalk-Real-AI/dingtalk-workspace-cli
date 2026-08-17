---
category: Fixed
---

- **Document write verification** (#960) — avoids false partial-success results when normalized Markdown, paginated blocks, inline images, or version reverts are confirmed by server readback. Document reverts and media inserts now require explicit readback evidence and report partial success when the server cannot prove the requested result.
