---
category: Added
---

- **Doc public-link and historical-version reads** — `dws doc read` forwards
  the reviewed `password` (internet-public documents with password protection)
  and `historyVersion` (read content as of a listed historical version; `0`
  denotes the document's initial version) parameters on the markdown, JSONML,
  and scope read paths via `--password` / `--version`; `dws doc +fetch` gains
  `--password` and `--version` with the same `historyVersion` forwarding, while
  `--revision` stays rejected with explicit guidance: revision is the document
  edit revision returned by JSONML reads for `+update --expected-revision`
  conditional writes, not a historical version number.
