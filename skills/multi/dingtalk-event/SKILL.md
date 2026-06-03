---
name: dingtalk-event
description: 钉钉事件订阅。Use when 用户提到 实时监听消息/订阅事件/事件流/event consume/接收新消息/聊天机器人监听/审批事件/通讯录变更/日历事件/构建事件驱动 Agent。命令前缀：dws event。
cli_version: ">=1.0.31"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉事件订阅 Skill

> 🧪 **EXPERIMENTAL** — DingTalk Stream 事件订阅 (v1)。提供 daemon + 多消费者本地 IPC，对齐 lark-cli 风格。

> **PREREQUISITE:** 用 `dws config init` 配置 ClientID/ClientSecret，或通过 env var `DWS_CLIENT_ID` + `DWS_CLIENT_SECRET`（成组覆盖）。bot-only：不需要 `auth login`。

> 命令参考：[dingtalk-event-consume.md](references/dingtalk-event-consume.md) · 端到端示例：[runbook.md](references/runbook.md)

## 意图表

| 用户说 | 命令 |
|--------|------|
| "监听新消息" | `dws event consume --event-types im.message.receive_v1 --compact --quiet` |
| "订阅审批状态变化" | `dws event consume --event-types approval.instance.status_changed` |
| "看 bus 状态" | `dws event status` |
| "列活跃消费者" | `dws event list --all` |
| "停 bus" | `dws event stop` |
| "调试 SDK 原始 payload" | `dws event consume -f raw --max-events 1` |
| "事件持久化到文件" | `dws event consume --output-dir ./events` |
| "按事件类型路由到不同目录" | `dws event consume --route '^im\.=dir:./im/' --route '^approval\.=dir:./approval/'` |

## 架构（一图）

```
钉钉 Stream Gateway (WebSocket)
       │  (一个 ClientID 一条云端连接)
       ▼
┌───────────────────────────────────────────────────┐
│  dws event _bus (per ClientID, auto-forked)        │
│   Source (SDK 长连接) → Dedup (event_id LRU)       │
│   → Hub (Hello pushdown 按订阅过滤)                │
│   → UDS (Unix) / Named Pipe (Windows)              │
└───────────────────────┬───────────────────────────┘
                        │ NDJSON frames
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
   dws event       dws event       dws event
   consume (A)     consume (B)     status/list
   im.* 订阅        approval.*     ad-hoc RPC
```

**关键不变量：**
1. **emit 非阻塞** — SDK 回调里立即 ACK；慢消费者只丢自己（drop-oldest）
2. **dedup 必须有** — Stream 重连会重投递在途事件
3. **单 ClientID 单 bus** — `<ConfigDir>/events/<edition>/<clientIDHash>/bus.lock` 强制
4. **上游永远全订阅** — `--event-types` 只影响 bus → consumer 这一段；开放平台后台未勾选的事件类型即使设置 `--event-types` 也收不到
5. **consumer EOF 自动清理** — SIGKILL/崩溃后 bus 通过 socket EOF 立即 unregister

## 子命令总览

| 子命令 | 用途 |
|--------|------|
| `dws event consume` | 订阅事件并 NDJSON 输出（主命令）|
| `dws event list` | 列出当前 / 全 edition 的消费者 |
| `dws event status` | 显示 bus 健康状态 + per-event-type 计数 |
| `dws event stop` | 优雅停 bus（SIGTERM）|
| `dws event _bus` | （隐藏）daemon 进程入口，consume 按需 fork |

## 凭证解析顺序

1. **env var 覆盖**（CI/容器友好）：`DWS_CLIENT_ID` + `DWS_CLIENT_SECRET` **同时设置** → 整组使用，跳过 keychain；任一缺失则不启用 env 通道（避免半套配置）
2. **keychain**：`dws config init` 配置后存储于系统 keychain
3. **明文 config**（不推荐）：app config 文件中 `clientSecret` 字段直接为 plain string

`status` / HelloAck 会显示 `credentials_source` (env / keychain / app_config / plain_config)，方便用户验证实际生效的来源。

## 4 个 env var 调参（不需要新增 flag）

| Env var | 默认 | 用途 |
|---------|------|------|
| `DWS_EVENT_BUS_IDLE_TIMEOUT` | `5m` | bus 在 0 consumer 后自停时间 |
| `DWS_EVENT_CONSUMER_BUFFER` | `100` | 每 consumer sendCh 容量 |
| `DWS_EVENT_DEDUP_LRU` | `8192` | event_id LRU 容量 |
| `DWS_EVENT_DROP_WARN_PCT` | `5` | 单事件类型 drop 比超阈值时写 bus.log WARN |

## 常见 Agent pipeline

### 收消息 → Claude 回复

```bash
DWS_CLIENT_ID=ding_xxx DWS_CLIENT_SECRET=*** \
  dws event consume \
    --event-types im.message.receive_v1 \
    --compact --quiet \
  | while IFS= read -r ev; do
      content=$(echo "$ev" | jq -r '.content // empty')
      msg_id=$(echo "$ev" | jq -r '.message_id // empty')
      [[ -z "$content" ]] && continue
      reply=$(claude -p "用中文简短回复：$content")
      dws chat message reply --message-id "$msg_id" --text "$reply"
    done
```

### 审批 → 写入飞书文档

```bash
dws event consume \
  --event-types approval.instance.status_changed \
  --compact --quiet \
  | while IFS= read -r ev; do
      title=$(echo "$ev" | jq -r '.title')
      status=$(echo "$ev" | jq -r '.to_status')
      lark-cli docs +update --doc "DOC_URL" --mode append \
        --markdown "- $(date '+%H:%M') 审批 [$title] → $status"
    done
```

### 事件持久化（审计/回放）

```bash
# 每事件落一个 JSON 文件，按事件类型分目录
dws event consume \
  --route '^im\.=dir:./events/im/' \
  --route '^approval\.=dir:./events/approval/' \
  --route '^contact\.=dir:./events/contact/' \
  --output-dir ./events/other/
```

## 关键陷阱速查

| 陷阱 | 症状 | 解决 |
|------|------|------|
| 开放平台后台没勾事件 | consume 跑着但收不到事件 | 钉钉开发者后台 → 应用 → 事件订阅 → 长连接 → 勾对应事件类型 |
| `--force` 不带 `--foreground` | validation error | daemon 模式不能 force（会导致多 bus 抢同 socket）；要重启 bus 用 `event stop && event consume` |
| `--format json` 无界流 | validation error | json 必须配 `--max-events` 或 `--duration`；流式用 `ndjson`（默认）|
| CI/容器没 keychain | "ClientSecret resolution failed" | 用 env var：`DWS_CLIENT_ID + DWS_CLIENT_SECRET` 成组设置 |
| orphan bus 残留 | 上次 `kill -9` 后 status 显示 orphan | 下次 `dws event consume` 会自动清理 stale lock；手动可 `dws event stop` 或 `rm -rf` workdir |
| 多副本（K8s）只 1 个能跑 | 单 ClientID 单 bus 设计 | 每副本用独立 ClientID；不要共享 |
| stdout 被污染 | NDJSON 流里混入 daemon 日志 | bus daemon stdio 已 detach；日志在 `<workDir>/bus.log`；如果污染必是 bug 请 report |

## 输出格式（事件流默认 ndjson）

| `-f` 值 | 输出 | 用途 |
|---------|------|------|
| `ndjson` (默认) | 一行一个对象 | 管道 / jq / Agent |
| `json` | 多行美化 JSON | 人工调试（要求 bounded：必须 `--max-events` 或 `--duration`）|
| `pretty` | 同 json | 同上 |
| `raw` | 仅 SDK 原始 payload | 调试 SDK |
| `compact` | 扁平化 + 解析嵌套 + 语义字段 | **Agent 强推荐**（IM/审批/通讯录/日历/考勤都有处理器）|
| `table` / `csv` | fallback 到 ndjson + stderr WARN | 事件流无意义 |

`-f compact` 已注册 8 个处理器：im.message.receive_v1 / im.message.read_v1 / approval.instance.status_changed / approval.task.created / contact.user.{created,updated,deleted}_v3 / cal.event.{created,updated,deleted}_v1 / attendance.check_v1。未注册类型走 generic processor（payload JSON 字段扁平化到顶层）。

## 与 lark-cli event 对齐和超越

| 维度 | lark-cli | dws event |
|------|---------|-----------|
| 用户命令 | `consume <EventKey>` + skill `+subscribe` 套娃 | **`consume [--event-types ...]` 一层搞定** |
| `--compact/--filter/--route` | 在 `+subscribe` skill 里 | **全部下放到 `consume`** |
| 隐藏 daemon | `_bus.go` 按需 fork ✅ | 同 |
| event_id 去重 | ✅ | 同 |
| 连接状态 | 日志正则反推 | **inferred state machine + state_source 字段** |
| `status` 视图 | per-consumer 计数 | + **per-event-type 总计 + drop-rate WARN** |
| Identity | `--as user/bot/auto` | **bot-only**（命令面更干净）|
| 前台模式 | 强 daemon | + **`--foreground`**（systemd/k8s 友好）|
| Hello 下推过滤 | EventKey + 基础 | **+ filter regex / event_types 下推到 bus，减少 IPC 量** |
| 跨平台 | Unix Socket + Windows Named Pipe ✅ | 同 |

## 实施细节

- v1 仅 `RegisterAllEventRouter`（generic events）；ChatBot @-消息和卡片回调 v2 加（需要同步 ACK 业务响应体）
- v1 stop 走 SIGTERM；Windows graceful 是 v2 todo
- catch-all 列表 v1 为空（= bus 上游全订阅）；待 P0 真测 DingTalk event_type 字符串后填具体清单
- workdir：`<ConfigDir>/events/<edition>/<clientIDHash>/`，clientIDHash = `sha256(clientID)[:16]`

## 参考

- 详细命令参考：[references/dingtalk-event-consume.md](references/dingtalk-event-consume.md)
- 端到端冒烟手册：[references/runbook.md](references/runbook.md)
- 实施计划：`plans/2026-05-28_event_capability_v1.plan.md`
- P0 SDK 验证记录：`plans/2026-05-28_event_capability_v1.p0-sdk-probe.md`
