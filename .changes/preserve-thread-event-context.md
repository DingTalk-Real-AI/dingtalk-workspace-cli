---
category: Fixed
---

- **Thread event context** — 消息事件的扁平输出保留上游提供的 Thread、父会话和根消息标识；缺失字段保持省略，不推断消息归属。上游原始事件缺少标识时仍需服务端补齐。
