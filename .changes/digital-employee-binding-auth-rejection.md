---
category: Fixed
---

- **数字员工服务端绑定** — 绑定、解绑与换绑在发起请求前复用统一 OAuth Token 自动刷新，并在网关明确拒绝请求时写入 `rejected` 回执；只有结果无法确认的请求才保留 `pending`。
