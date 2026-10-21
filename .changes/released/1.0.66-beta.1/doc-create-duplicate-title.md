---
category: Fixed
---

- **Document creation title deduplication** — `dws doc +create` removes a leading Markdown H1 that matches the document name, including names inferred from Markdown, before chunking and readback verification. Different headings, indented code blocks, and JSONML content remain unchanged; inline document content preserves its surrounding whitespace.
