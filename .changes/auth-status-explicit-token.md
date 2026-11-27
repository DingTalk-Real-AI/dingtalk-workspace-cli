---
category: Fixed
---

- **临时凭证状态检查** — `dws --token "$TOKEN" auth status` 使用注入凭证进行只读身份验证，成功后返回已认证；区分认证拒绝、无法判断和未验证状态，支持 `--readonly`，不持久化或刷新临时凭证。
