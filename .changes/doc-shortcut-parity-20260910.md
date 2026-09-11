---
category: Added
---

- **Doc shortcut workflows** — add local and online document structure checks with `doc +script`, verified standalone media upload with `doc +media-upload`, optional title fallback, and ordered media attachments during document creation.
- **Doc reading and editing** — add direct-folder search filtering, readable date filters, chapter and regex block-context reads, document comments, and verified same-document block copying and range editing while preserving existing commands and aliases. Range edits remain sequential, not atomic.
- **Doc media controls** — add clipboard image input, attachment preview/summary selection, vertical cover positioning, persistent preview output, and explicit download overwrite; fix tag filtering and keep upload progress off JSON stdout.
- **Doc download confirmation** — use `doc +download-overwrite --source media|cover` for confirmed replacement of local files. Existing media, preview, and cover downloads retain their no-clobber behavior and safety contracts; preserve the preview output shorthand `-o`.
