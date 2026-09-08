---
category: Fixed
---

- **AI 表格应用模式输入校验错误分类** (#1314) — 将 icon、background、config、layout 等字段的校验失败从 `internal` 错误和退出码 `5` 修正为 `validation` 错误和退出码 `3`；校验仍在 MCP 调用前完成。
