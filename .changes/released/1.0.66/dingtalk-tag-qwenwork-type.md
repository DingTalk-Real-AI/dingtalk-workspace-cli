---
category: Changed
---

- `dws dingtalk-tag manage create|list|save-draft --type` 使用 `qwenwork` / `local_agent`；Help、Schema、Skill 和示例同步。
- 创建仍须显式选择类型，响应模式默认值、人设保存、发布前更新人校验及本地 Agent 接入逻辑不变。
- 返回值消费者需要识别 `qwenwork`。在分发新 CLI 前，须独立核实公共服务支持新类型以及旧客户端所需的兼容行为；本说明不代表线上部署已完成。
- 更新不传类型保持原值，类型创建后不可切换。`connect --channel opencode` 是本地适配器名称，不属于本次改名。
- 先核实公共服务兼容性，并同步 MCP 工具类型枚举/描述，再使用新 CLI。旧脚本中的 `--type open_code` 需改为 `--type qwenwork`。
- 回滚 CLI 时需一并核对服务端返回契约；本变更不迁移员工数据。
- `manage set-visibility` 在预览和执行前校验 ALL/PARTIAL、PARTIAL 非空目标及 ALL 与指定目标互斥，非法范围不会调用 MCP；合法请求和旧参数别名兼容保持不变。
- `run run-status` 的 Help 与 Skill 按真实返回说明 `result` 为字符串状态文案，程序应使用 `execStatus` 判断执行状态。
- Skill 上传失败保留公开权限管理链接，不再整段误脱敏；其他 URL、签名地址与凭证仍不输出。Skill 更新仅返回裸 `NO_PERMISSION` 时补充条件式权限引导，保留错误码、Trace ID 和退出码，不覆盖服务端具体说明、不重试写操作。
