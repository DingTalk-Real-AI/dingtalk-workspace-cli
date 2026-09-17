---
category: Added
---

- **Chat emotion favorite local image** (#85955640) — `dws chat emotion favorite` now accepts `--file-path` for a local image (jpg/jpeg/png/gif/webp/bmp, up to 10MB) as an alternative to `--media-id`; the CLI validates the file locally, uploads it through `dingtalk-file/upload_media` (bizType=chat_emoticon), and reuses the existing favorite flow with the returned mediaId (mediaIdV1 preferred, falling back to mediaIdV2). `--media-id` behavior is unchanged.
