---
category: Added
---

- **Drive list type/time filtering** (#942) — `dws drive list` gains `--type
  file|folder`, `--start`, and `--end` for client-side filtering by node type
  and modification time on both the pan and workspace routes. Filtering runs
  a bounded full scan of the target directory (2000-entry cap, reported via
  `truncated=true`), composes with `--latest`/`--pattern`/`--depth`, and is
  mutually exclusive with `--versions`/`--cursor`/`--order-by`/`--order`/
  `--limit`. Time values accept relative forms (`24h`/`7d`/`2w`), RFC 3339,
  zone-less ISO 8601 (Asia/Shanghai), or a plain date.
