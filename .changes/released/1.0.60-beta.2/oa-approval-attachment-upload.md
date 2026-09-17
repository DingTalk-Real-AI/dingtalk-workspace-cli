---
category: Added
---

- **OA approval attachment upload** — `dws oa approval attachment upload --file <path>` uploads a local file as an approval attachment in one command: it initializes the upload credential (MCP `oa/init_attachment_upload_info`), HTTP PUTs the file to OSS, then commits it (MCP `oa/commit_attachment_upload_info`). `--file-name` defaults to the file's base name and `--md5` is auto-computed when omitted.
