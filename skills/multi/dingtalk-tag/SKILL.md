---
name: dingtalk-tag
description: 钉钉数字员工的自然语言创建、查询、修改、发布、删除、能力资源、执行状态、本地 Profile 落盘与 DSH 接入。Use when 用户说创建或管理数字员工、改人设/岗位/响应模式、发布或询问下线能力、查执行状态或 trace、管理数字员工 Skill/MCP、把已有数字员工转换为本地 Profile，或接入本地 DSH。命令前缀：dws dingtalk-tag。
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 数字员工 Skill

执行任何 `dws` 操作前，先完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。先用 [dingtalk-tag-index.md](references/dingtalk-tag-index.md) 路由；生命周期读取 [manage.md](references/manage.md)，自然语言创建/恢复/DSH 接入再读 [manage-and-connect.md](references/manage-and-connect.md)；能力资源读取 [capability.md](references/capability.md)；执行排障读取 [run.md](references/run.md)。

## 路由

| 用户意图 | 命令 |
|---|---|
| 创建草稿 / 创建并发布 / 查询 / 修改 / 上线 / 删除 | `dws dingtalk-tag manage ...` |
| 下线 | 当前版本无独立下线命令；明确说明限制，不得用 delete 冒充下线 |
| A2A 或其他仅登录场景 | `dws dingtalk-tag manage login --agent-uuid ...` |
| 只把已有、已发布的本地数字员工转换为本地 Profile | `dws dingtalk-tag connect --agent-uuid ... --profile-only` |
| 把已有、已发布的本地数字员工接入 DSH | `dws dingtalk-tag connect --agent-uuid ... --channel dsh` |
| 把数字员工接入当前本地 Agent | `dws dingtalk-tag connect --agent-uuid ... --channel auto --daemon --alwayson` |
| 查询、停止或重启数字员工本地连接 | `dws dingtalk-tag connect list/status/stop/restart` |
| 解除本机连接，但保留员工和 Profile | `dws dingtalk-tag connect unbind --agent-uuid ...` |
| 将已有连接换为其他本地 Agent 或 DSH | `dws dingtalk-tag connect rebind --agent-uuid ... --channel ...` |
| 创建或查询 Skill / MCP 资源 | `dws dingtalk-tag capability ...` |
| 查一次执行的状态或完整 trace | `dws dingtalk-tag run ...` |

## 自然语言编排硬约束

- `mainProgramType` 仅支持 `open_code`、`local_agent`，`a2a` 暂不支持。创建或更新没有特殊要求时默认不传，由 OpenAPI 按 `open_code` 处理；用户明确要求时可以显式传 `open_code`；只有明确接入本地 Agent/DSH 时才传 `--main-program-type local_agent`。
- 发布前一次性收集名称、描述、头像、部门、岗位、响应模式、Prompt 等缺失信息，避免边执行边追问。
- `save-draft` 是全量覆写。修改前必须读取完整 draft，并保留未修改的 Skill、MCP 和其它字段；`mainProgramType` 按上一条规则处理，已有 `local_agent` 需要保持本地模式时显式保留 `local_agent`。
- 同一自然语言请求里的连续写操作只做一次汇总确认；确认后才加 `--yes`。先用 `--dry-run --format json` 展示计划。
- 创建成功后若保存或发布失败，必须返回已创建的 `agentUuid` 和恢复命令；重试禁止再次执行 create。
- “创建并落盘 Profile”可顺序执行创建/发布与 `connect --profile-only`；“创建并接入 DSH”则使用 `connect --channel dsh`。创建/发布与 connect 是独立事务，connect 绝不创建、修改或发布数字员工。
- 用户可以只创建/管理数字员工、只把已有员工转换为本地 Profile，或继续接入 DSH；三种操作互不强绑定。
- 所有 ID 统一使用 `agentUuid` / `--agent-uuid`，不得猜测。
- 普通本地 Agent 接入使用 Event Consume，默认仅主管可触发；白名单中的用户必须先在员工身份下精确解析。支持 Codex、Qoder/QoderWork、Claude Code、CodeBuddy/WorkBuddy、Gemini、OpenCode 和 custom。OpenClaw/Hermes 暂未适配，不要回退到机器人创建流程。
- 自然语言“创建发布并接入本机”在发布后显式使用 `--daemon --alwayson`；命令行默认前台。DSH 不加这两个参数，由运行中的宿主员工级启动；宿主不可用时按 restartRequired 提示启动宿主。connect 不提供开机自启，也不能在电脑休眠期间处理消息。
- connect 失败后保留员工 ID 和已落盘 Profile，检查 `connect status` 再恢复；不要重复 create、不要清除事件重试预算、不要隐式覆盖另一个 Adapter 的绑定。
- “暂停”使用 stop（保留绑定）；“解绑”使用 unbind（保留 Profile）；“换成本地另一个 Agent”使用 rebind，先 dry-run 并汇总确认。换绑先确认旧实例释放，unknown 或超时不得手动删绑定来绕过。新绑定已提交但启动失败使用 restart；旧绑定仍在 rebinding/unbinding 时重试原操作。

## 安全

- `manage login` 在内部完成 AuthCode 换票、在线身份核验，并保存精确 `corpId:userId` Profile；不得输出或转存 AuthCode/Token。企业本地 Agent 接入只调用 `connect`；A2A 或其他需要登录数字员工 DWS 的场景调用 `manage login`。
- 删除不可逆；修改、发布、删除和 connect 按 Schema 的确认要求执行。
- Channel 的 `reply` / `operator-private` 只供已绑定的本地 Adapter/DSH 机器协议使用，必须指定员工 Profile，正文只能走受限 stdin；不要为普通用户消息直接调用。
