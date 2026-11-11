---
category: Added
---

- **企业自建应用事件定义配置** — 新增 `dws dev event list/get/create/update/validate/lint/debug/publish/offline` 九条命令，覆盖事件定义的草稿创建与编辑、静态校验、本地 lint、端到端调试、直接启用与下线。定义只从 `--file` 读取并做本地预检：eventKey 必须符合 `app_user_{业务系统}_{对象}_{动作}`，`payloadSchema.type` 固定为 object，`category` 不属于本轮输入契约，保留键（Scope/状态/版本/修订号）一律拒绝，CLI 不改写开发者文件。写操作必须携带从 `get` 读回的真实 `configRevision`，409 修订冲突按不可重试的显式失败返回并给出双方修订号，不自动换号重放。`debug` 区分未就绪、未通过与通过三种结论并列出未完成阶段，超时不视为通过。九条命令走统一结果契约，分页事实缺失时失败而非返回半页结果；`dws dev app event`（回调订阅）与 `dws event`（个人事件监听）两条命令面保持不变。
