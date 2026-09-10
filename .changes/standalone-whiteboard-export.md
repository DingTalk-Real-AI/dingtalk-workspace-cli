---
category: Added
---

- **Standalone whiteboard export** — adds `whiteboard export` and `whiteboard export-get`
  to download PNG/PDF exports. Both commands support request-only dry-run previews.
  Signed URL query strings do not affect filenames; downloads validate their file
  signatures before atomic publication and never overwrite existing files. Failed
  tasks retain recovery instructions with the job ID, format, and output directory.
  Downloads enforce HTTPS/443 public network targets on redirects and actual
  connections, with a 512 MiB limit for both declared and streamed response sizes.
