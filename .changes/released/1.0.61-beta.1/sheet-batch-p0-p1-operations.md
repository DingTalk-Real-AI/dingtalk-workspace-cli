---
category: Added
---

- **Sheet batch operations** — expands `sheet batch-update` from 16 to 39 CLI operations, adds strict validation for the new P0/P1 inputs, preserves server-generated create IDs in `results[].data`, and JSON-encodes translated operations locally so nested number/boolean values survive the MCP transport.
- **Sheet batch dimension coordinates** — makes `delete-dimension` and `move-dimension` accept the same public coordinates as their standalone commands (1-based row numbers or column letters) and translates them locally to the batch API's 0-based indexes.
