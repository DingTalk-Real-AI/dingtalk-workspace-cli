# DingTalk Tag 数字员工索引

DingTalk Tag 数字员工的管理、执行查询和能力资源命令，命令前缀 `dws dingtalk-tag`。

| 主题 | 命令前缀 | 安全属性 | 详见 |
|---|---|---|---|
| 管理态：创建 / 详情 / 列表 / 临时 DWS 授权码 / 草稿覆写 / 发布 / 删除 | `dws dingtalk-tag manage` | 授权码高敏感；其余含高影响写与不可逆删除 | [`manage.md`](./manage.md) |
| 执行态：执行状态 / 执行 trace | `dws dingtalk-tag run` | 全部只读；trace 含完整对话内容 | [`run.md`](./run.md) |
| 能力资源：Skill / MCP 创建与查询 | `dws dingtalk-tag capability` | 创建为高影响写；资源不会自动关联数字员工 | [`capability.md`](./capability.md) |
| 本地接入：已有已发布员工落盘 Profile 或接入本地 Agent/DSH | `dws dingtalk-tag connect` | 受管换票与 Profile 落盘；可选本地 Agent 运行或 DSH 幂等注册 | [`manage-and-connect.md`](./manage-and-connect.md) |

## 意图路由

| 用户说 | 命令 |
|---|---|
| 创建 / 新建数字员工 | `dws dingtalk-tag manage create`（只建草稿，不发布） |
| 查数字员工详情 | `dws dingtalk-tag manage detail` |
| 数字员工列表 / 搜数字员工 | `dws dingtalk-tag manage list` |
| 获取数字员工临时 DWS 授权码 | `dws dingtalk-tag manage get-dws-auth-code` |
| 改人设 / 岗位 / 部门 / 头像 | 先 `detail` 再 `dws dingtalk-tag manage save-draft`（**全量覆写**） |
| 发布 / 上线数字员工 | `dws dingtalk-tag manage publish` |
| 删除数字员工 | `dws dingtalk-tag manage delete`（不可逆） |
| 这次执行成功了吗 / 跑完没 / 什么状态 | `dws dingtalk-tag run run-status` |
| 为什么这么回答 / 看提示词 / 看工具调用 / 完整链路 | `dws dingtalk-tag run trace` |
| 手上只有 dws 发消息返回的 openTaskId | 先换成 openMessageId，见 [`run.md`](./run.md) |
| 只把已有 local_agent 数字员工转换成本地 Profile | `dws dingtalk-tag connect --agent-uuid ... --profile-only`，见 [`manage-and-connect.md`](./manage-and-connect.md) |
| 管理普通 Agent 的连接进程 | `dws dingtalk-tag connect list/status/stop/restart`，见 [`manage-and-connect.md`](./manage-and-connect.md) |
| 把已有 local_agent 数字员工接入普通本地 Agent | `dws dingtalk-tag connect --agent-uuid ... --channel codex --daemon --alwayson`，见 [`manage-and-connect.md`](./manage-and-connect.md) |
| 把已有 local_agent 数字员工接入 DSH | `dws dingtalk-tag connect --agent-uuid ... --channel dsh`，见 [`manage-and-connect.md`](./manage-and-connect.md) |

## 全局约束

- identity（corpId / userId）由可信登录态注入，不对 CLI 暴露；不要尝试传 `--org-id` / `--user-id`。
- 固定调用 MCP product/server `deap-dev`，端点跟随当前 MCP 环境自动选择；`DINGTALK_DEAP_DEV_MCP_URL` 仅用于本地调试覆盖。
- 临时 DWS 授权码只在当前受控调用链内使用，不写入文档、日志、命令历史、缓存或代码库。
- 以 leaf Schema 的 `confirmation` 为准：当前 `manage save-draft/publish/delete` 与 `capability skill/mcp create` 需要先 `--dry-run`、获得确认后再加 `--yes`；`manage create` 当前为 `confirmation=not_required`，但它非幂等，失败时不要盲目重复创建。
- 创建/发布与 connect 是独立事务：可以只管理数字员工，也可以只用 `--profile-only` 落盘 Profile，或用 `--channel dsh` 接入 DSH。connect 不创建、不修改、不发布，也不自动重启 DSH。
