---
category: Added
---

- **Standalone whiteboards** — adds OpenNodes-based `whiteboard create-with-content` and extends
  the existing `whiteboard query` / `whiteboard update` entry points to operate
  on standalone boards when `--part-id` is omitted, while preserving the
  document-embedded flow when it is explicitly supplied. Standalone reads
  decode the service `resultJson`, and writes enforce revision and stable
  request-ID guards with compatible receipt validation and same-type read-back.
