---
category: Fixed
---

- `sheet info` 对空、`null`、非对象或无效 JSON 响应返回结构化 API 错误，避免 nil map 崩溃或把无效响应报告为成功；正常对象的坐标转换与 dry-run 预览行为保持不变。
