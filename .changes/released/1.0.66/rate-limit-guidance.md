---
category: Fixed
---

- **限流错误提示** — 当服务返回 `server_error_code=RATE_LIMIT_ERROR` 时，明确说明当前请求频率过高、本次请求被限流，并引导等待 10 秒后重试；其他错误码保持原有处理。
