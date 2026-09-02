---
category: Fixed
---

- **DING failure handling and resource identity** — stop when robot credentials are missing or the selected robot is invalid, and preserve source message IDs separately from DING IDs. Recall accepts opaque server-returned DING IDs without guessing resource type from their prefixes; callers check identity provenance in the receipt.
- **Whiteboard verification and recovery** — validate connector payloads locally, normalize numeric coordinate comparisons, return compact successful update receipts, and preserve committed-write evidence on readback failure without recommending duplicate append operations.
- **DING and Whiteboard guidance** — align mono/multi references, clarify product ownership, and reduce redundant discovery and readback without dropping business information.
