---
category: Added
---

## 数字员工登录与本地生命周期整合

- 保留外部安全换票与 `manage login`，同时支持 `connect --profile-only`、本地 Agent 与 DSH 员工级生命周期。
- 共享安全登录内核，保持主管当前 Profile，保留员工上下文的 operator 身份解析。
- 登录命令对齐统一结果框架；外部换票失败时原样恢复新旧 Client Secret 存储槽。
