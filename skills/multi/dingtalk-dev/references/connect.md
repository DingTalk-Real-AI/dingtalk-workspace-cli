# 本地建联（把机器人接到本地 agent）

> `dws dev connect` 是 dev 顶层命令，把一个现成机器人接到当前渠道的本地 agent CLI 做调试/值守——只建联、不建号。缺机器人时优先先创建应用并用 `robot config` 配置；若走无绑定的 `robot submit/result`，缺 `unifiedAppId` 时不能续写版本发布。

## 起连接

```bash
# 用现成机器人凭证起 Stream，接到当前渠道的本地 agent（前台运行，Ctrl-C 退出）
dws dev connect --channel auto --robot-client-id <clientId> --robot-client-secret <clientSecret>

# 用统一应用 ID，复用 credentials get 自动取凭证
dws dev connect --unified-app-id <unifiedAppId> --channel qoderwork

# 预览建联方案不实际起连接
dws dev connect --robot-client-id <clientId> --robot-client-secret <clientSecret> --dry-run --format json
```

正式 connect 是前台长驻进程：在对话里跑必须后台运行并告诉用户如何停止，或引导用户自己开终端跑。

`dev connect` 只做本地 Stream 调试/值守，不会创建版本、提交审批或发布应用。dry-run JSON 的 `invocation` 会声明 `scope=local_debug_only`、`doesNotPublish=true`、`completionState=LOCAL_DEBUG_ONLY`，真实前台/daemon 启动也会打印“本地调试，不代表线上发布完成”。完成态判定（建联成功不等于线上可用）以 [SKILL.md](../SKILL.md)「核心规则」为准。

| flag | 说明 |
|------|------|
| `--channel` | `auto`(默认,运行时信号自动识别) / openclaw / qoder / qoderwork / hermes / workbuddy / claudecode / codebuddy / codex / gemini / opencode |
| `--robot-client-id` / `--robot-client-secret` | 现成机器人凭证（clientId=AppKey, clientSecret=AppSecret）。命名带 `robot-` 前缀以避开全局 OAuth `--client-id` flag |
| `--unified-app-id` | 统一应用 ID，内部复用 `credentials get` 自动取凭证，替代手填 robot-client-id/secret。注意 clientSecret 仅建号时返回一次、未必可取，取不到时回退手填 |
| `--agent-memory` | 按会话续聊（默认开）：同一群/单聊共享 agent 会话，追问保留上下文。codex 走 app-server thread；opencode 走 `opencode serve` 的 HTTP session/message API；qoder/qoderwork 走常驻 `qodercli --input-format stream-json` 并传 `session_id`；claudecode/codebuddy/workbuddy 走 CLI `--session-id`/`--resume`。会话映射按机器人落盘，重启后可继续；gemini 保持无状态。`--agent-memory=false` 关闭 |
| `--agent-model` | 覆盖本地 agent 模型（如 claudecode 默认锁 haiku 求快，可改 `claude-sonnet-4-6` 换聪明）。env: `DWS_AGENT_MODEL` |
| `--agent-workdir` | agent 运行目录：放知识文件（如 CLAUDE.md）可给机器人企业上下文。默认空白临时目录（冷启动快 ~4s vs 大目录 ~29s，慢了会错过钉钉响应窗口）。env: `DWS_AGENT_WORKDIR` |
| `--reply-card` | 富回复（默认开）：🤔Thinking/🥳Done 表态永远生效；**卡片需配 `--card-template` 才启用**（同 hermes：没配模板=纯文字回复），失败自动回退文字；env `DWS_REPLY_CARD=0` 全关 |
| `--card-template` | AI 卡片模板 ID。**模板按应用授权**：去开发者后台→你的应用→AI 卡片设置注册/获取模板 ID，可去掉公共模板的第三方角标；默认用公共模板 best-effort。env `DWS_CARD_TEMPLATE` |
| `--allowed-groups` | 群白名单 openConversationId（逗号分隔），配置后只有名单内的群能触发机器人。env `DWS_ALLOWED_GROUPS` |
| `--allowed-users` | 用户白名单 staffId（逗号分隔），配置后只有名单内的用户能触发。env `DWS_ALLOWED_USERS` |
| `--knowledge-dir` | 答疑知识目录（.md/.txt）：每条消息本地检索 top-k 片段拼进 prompt，agent 仍在空目录跑、不拖慢回复。env `DWS_KNOWLEDGE_DIR` |
| `--user-rate-limit` | 单用户每分钟消息上限（防刷，每条消息都是一次 LLM 调用），0 关闭，默认 20。env `DWS_USER_RATE_LIMIT` |

## 建联前的依赖预检（agent 必做）

渠道背后是本地 agent CLI，用户可能没装。先 `--dry-run` 看出参 `cli` 字段再决定下一步：

```bash
dws dev connect --channel <ch> --robot-client-id x --robot-client-secret y --dry-run --format json
# 输出里的 cli 字段：
#   "cli": {"required":"Claude Code","installed":false,"autoInstall":true,"installHint":"npm i -g @anthropic-ai/claude-code"}
```

| cli 状态 | agent 应该做什么 |
|----------|----------------|
| `installed: true` | 直接建联 |
| `installed: false, autoInstall: true` | 告知用户缺哪个 CLI，说明启动建联时会自动 `npm` 安装（或先手动执行 installHint 里的命令再连）；`DWS_CONNECT_NO_INSTALL=1` 可禁自动安装 |
| `installed: false, autoInstall: false` | **不要直接起连接**——桌面 App 渠道（qoder/qoderwork/workbuddy）需要用户先安装对应 App（installHint 是下载地址），装好后 CLI 随 App 自带；openclaw/hermes 引导用户走官方 onboarding |

dry-run 出参的完整建联预检结构（channel/detectedBy/credentialSource/agent/cli/connect）见 SKILL.md「通用出参约定」。

必须检查 dry-run 顶层：

```json
{
  "invocation": {
    "completionState": "LOCAL_DEBUG_ONLY",
    "doesNotPublish": true,
    "scope": "local_debug_only",
    "terminal": false
  }
}
```

这几个字段表示：连接器可以起本地调试，但版本发布闭环仍由 `robot result` 的 blocking `nextSteps` 或后续 `version status` 决定（完整门禁规则见 [SKILL.md](../SKILL.md)「核心规则」）。

## Codex 渠道注意

```bash
dws dev connect --unified-app-id <unifiedAppId> --channel codex --format json
```

- `--channel codex` 只走 Codex app-server 的 thread/turn 协议，不再降级到 `codex exec`。
- `DWS_AGENT_CMD` 不覆盖 Codex 渠道；自研或未支持的 AI 工具请用 `--channel custom --agent-cmd "<命令>"`。
- 给 Codex 固定知识/项目上下文：使用 `--agent-workdir /path/to/repo`。

## 机制与环境覆盖

- **stream-bridge 渠道**：Go 原生进程内 Stream 转发器，订阅 `TOPIC_ROBOT`。Claude/CodeBuddy/WorkBuddy/custom 每条 @机器人消息起一个无头 CLI 实例 → stdout 回钉钉；Qoder/QoderWork 会在 connect 生命周期内复用一个常驻 `qodercli --print --output-format stream-json --input-format stream-json` 子进程。
- **会话记忆**：Codex 记录 `conversationId -> threadId`；opencode 记录 `conversationId -> sessionId` 并通过本地 `opencode serve` 的 HTTP API 续聊；Qoder/QoderWork 记录 `conversationId -> Qoder session_id` 并在常驻 stream-json 子进程里续聊；Claude/CodeBuddy/WorkBuddy 使用 `--session-id`/`--resume`。映射按机器人落盘，重启后可继续。
- **会话指令 `/new` vs `/clear`**（对齐各渠道真实能力）：`/new`（含 `/start`、`/reset`）开新会话——丢掉当前映射、下一条消息起新 session，**旧 session 保留**（opencode/Codex/Claude 侧仍可按 id 续）；`/clear` 则**真正清掉当前会话**——能删的渠道调 agent 原生删除原语（opencode `DELETE /session/:id`），不暴露删除接口的渠道（Codex/Qoder/Claude 系）降级为与 `/new` 相同的重置。两者都只回 ack、不消耗 agent turn。
- **官方渠道**（openclaw/hermes）：dws 不代建机器人，输出官方 onboarding 指引。
- 环境覆盖：`DWS_AGENT_CMD`(整条命令覆盖,覆盖时不再注入模型/会话参数) / `DWS_AGENT_MODEL` / `DWS_AGENT_WORKDIR` / `DWS_CONNECT_CMD` / `DWS_CONNECT_NO_INSTALL=1` / `DWS_AGENT_TIMEOUT_MS`。

## 错误处理

| 情况 | 处理 |
|------|------|
| 缺凭证 | 优先用明确 `unifiedAppId` 走 `credentials get`；若只有 `robot submit/result` 的一次性 clientId/clientSecret，按敏感信息使用，缺 `unifiedAppId` 时不能续写版本发布 |
| Codex app-server 调用失败 | 检查本机 `codex` 是否可执行、是否已登录，以及 `--agent-workdir` 指向的目录是否可用；Codex 渠道不会降级到 `codex exec` |
| 桌面 App 渠道 `installed:false, autoInstall:false` | 引导用户先装对应 App（installHint 是下载地址），不要直接起连接 |

## 发现命令

起连接前先查清楚再敲：

```
# 浏览 connect 的子命令与 flag
dws dev connect --help

# 查 connect 的必填参数、类型、默认值
dws dev <command-path> --help
```

按 `--help` 输出构造 flag；不要凭旧 schema 名称猜参数。
