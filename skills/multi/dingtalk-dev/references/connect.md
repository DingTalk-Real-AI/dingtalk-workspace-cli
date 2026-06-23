# 本地建联（把机器人接到本地 agent）

> `dws dev connect` 是 dev 顶层命令，把一个现成机器人接到当前渠道的本地 agent CLI 做调试/值守——只建联、不建号。缺机器人先走 robot.md 的 `robot submit`/`result` 建号拿 clientId/clientSecret。

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

| flag | 说明 |
|------|------|
| `--channel` | `auto`(默认,运行时信号自动识别) / openclaw / qoder / qoderwork / hermes / workbuddy / claudecode / codebuddy / codex / gemini / opencode |
| `--robot-client-id` / `--robot-client-secret` | 现成机器人凭证（clientId=AppKey, clientSecret=AppSecret）。命名带 `robot-` 前缀以避开全局 OAuth `--client-id` flag |
| `--unified-app-id` | 统一应用 ID，内部复用 `credentials get` 自动取凭证，替代手填 robot-client-id/secret。注意 clientSecret 仅建号时返回一次、未必可取，取不到时回退手填 |
| `--agent-memory` | 按会话续聊（默认开）：同一群/单聊共享 agent 会话，追问保留上下文。仅 claudecode/codebuddy/workbuddy（CLI 有 `--session-id`/`--resume`）；qoder 系/codex/gemini/opencode 无寻址会话，自动保持无状态。`--agent-memory=false` 关闭 |
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

## Codex 渠道注意

```bash
# Codex 默认空白工作目录会触发 "Not inside a trusted directory and --skip-git-repo-check was not specified"
CODEX_BIN="$(command -v codex || echo /Applications/Codex.app/Contents/Resources/codex)"
DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check" \
  dws dev connect --unified-app-id <unifiedAppId> --channel codex --format json
```

- `--channel codex` 真实建联时优先用上面的 `DWS_AGENT_CMD` 形式，避免 Codex CLI 在默认临时目录内因非可信 Git 目录拒绝执行。
- 给 Codex 固定知识/项目上下文：`DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check -C /path/to/repo"`（路径不要含空格）。
- 设置 `DWS_AGENT_CMD` 后 DWS 不再自动拼接 `--agent-model`/`--agent-workdir`/会话参数，需要的要一起写进 `DWS_AGENT_CMD`。

## 机制与环境覆盖

- **stream-bridge 渠道**：Go 原生进程内 Stream 转发器，订阅 `TOPIC_ROBOT`，每条 @机器人消息起一个无头 CLI 实例 → stdout 回钉钉，可 7×24 无人值守。
- **会话记忆**：首条消息 `--session-id <uuid>` 建会话，后续 `--resume <uuid>` 续聊；会话状态在内存里，连接器重启后从新会话开始；某会话坏了会自愈（下条消息换新会话）。
- **官方渠道**（openclaw/hermes）：dws 不代建机器人，输出官方 onboarding 指引。
- 环境覆盖：`DWS_AGENT_CMD`(整条命令覆盖,覆盖时不再注入模型/会话参数) / `DWS_AGENT_MODEL` / `DWS_AGENT_WORKDIR` / `DWS_CONNECT_CMD` / `DWS_CONNECT_NO_INSTALL=1` / `DWS_AGENT_TIMEOUT_MS`。

## 错误处理

| 情况 | 处理 |
|------|------|
| 缺凭证 | 先用 `robot submit`/`robot result` 建号拿 clientId/clientSecret（见 robot.md） |
| Codex agent 返回 `Not inside a trusted directory and --skip-git-repo-check was not specified` | 用 `DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check"` 重启 `connect --channel codex` |
| 桌面 App 渠道 `installed:false, autoInstall:false` | 引导用户先装对应 App（installHint 是下载地址），不要直接起连接 |
