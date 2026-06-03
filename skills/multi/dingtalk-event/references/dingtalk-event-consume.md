# `dws event consume` 详细参考

订阅 DingTalk Stream 事件并将每条事件以 NDJSON 输出到 stdout。

## Synopsis

```
dws event consume [flags]
```

## Flags

| Flag | 默认 | 说明 |
|------|------|------|
| `--event-types <list>` | catch-all | 逗号分隔事件类型，下推到 bus；省略 = 接收 bus 上游所有事件 |
| `--filter <regex>` | — | 客户端正则过滤事件类型（下推到 bus）|
| `--compact` | false | 提示 bus 期望 compact 渲染（语义透传）|
| `-f, --format <format>` | `ndjson` | 输出格式：`ndjson` (默认) / `json` / `pretty` / `raw` / `compact`；`table/csv` fallback 到 ndjson |
| `--output-dir <dir>` | stdout | 每事件写一个文件 `{type}_{id}_{ts}.json`；与 stdout 互斥 |
| `--route <spec>` | — | `<regex>=dir:<path>`，可重复；未命中走 stdout/--output-dir |
| `--max-events <n>` | 0 | 收到 N 条后退出 (0 = 不限) |
| `--duration <duration>` | 不限 | Go duration (30s/5m)，事件流专用 |
| `--quiet` | false | 抑制 stderr 状态信息 |
| `--force` | false | 仅 `--foreground` 模式有意义；daemon 模式 → validation error |
| `--dry-run` | false | 仅打印解析后的配置，不连接 bus / 云端 |
| `--foreground` | false | 不 fork daemon，当前进程跑 bus (systemd/k8s 友好) |

## 凭证

- env var (覆盖式优先)：`DWS_CLIENT_ID` + `DWS_CLIENT_SECRET` 同时设置即生效
- keychain（默认）：`dws config init` 写入 OS keychain
- bot-only：不需要 `dws auth login`

## 输出契约

- 默认 `-f ndjson`：每事件一行 JSON。字段：`type/seq/event_id/event_born_time/event_corp_id/event_type/event_unified_app_id/data/headers/received_at_unix_ms`
- `-f compact`：经 processor 扁平化后的 map。常见字段（IM）：`type/event_id/timestamp/corp_id/app_id/message_id/chat_id/chat_type/message_type/content/sender_id`
- `-f raw`：仅 `event.Data`（SDK 原始 payload string），无 dws 封装
- `-f json/pretty`：每事件多行美化 JSON；**必须** 配 `--max-events` 或 `--duration`，否则 validation error
- stderr：默认输出连接 banner `connected bus pid=N source=... state=...`；`--quiet` 抑制

## Sink 选择

| Flag 组合 | 行为 |
|----------|------|
| (默认) | stdout NDJSON |
| `--output-dir <dir>` | 每事件写一个文件到该目录 |
| `--route '<regex>=dir:<path>'` | 匹配事件路由到该目录，未匹配走 stdout |
| `--route ... --output-dir <fallback>` | 匹配事件路由，未匹配走 fallback 目录 |

`--output-dir` / `--route` 与全局隐藏 `-o/--output`（单文件捕获）互斥；同时出现 → validation error。

## 退出码

| 场景 | 退出码 |
|------|--------|
| 优雅退出（ctx cancel / max-events / duration / bye）| 0 |
| 下游 stdout 管道关（SIGPIPE）| 0（不被视为错误）|
| validation error（flag 互斥、`--force` 不带 `--foreground` 等）| 5 |
| 凭证缺失 | 5 |
| bus 连接失败（discover deadline）| 5 |

## 信号

- `SIGINT` / `SIGTERM`：优雅退出，向 bus 发 Bye，返回 0
- 下游 EPIPE：检测到 stdout 关后立即退出 0

## 跨平台

- Unix：`<ConfigDir>/events/<edition>/<clientIDHash>/bus.sock`（权限 0600）
- Windows：Named Pipe `\\.\pipe\dws-event-<edition>-<clientIDHash>`（只允许当前用户）

## 示例

### 基础订阅（catch-all 到 stdout）

```bash
dws event consume
```

### Agent 友好（推荐）

```bash
dws event consume --event-types im.message.receive_v1 --compact --quiet
```

### 多类型订阅 + 过滤

```bash
dws event consume \
  --event-types im.message.receive_v1,im.message.at_v1,approval.instance.status_changed \
  --filter '^im\.' \
  --max-events 10
```

### 调试单条 SDK payload

```bash
dws event consume -f raw --max-events 1
```

### 持久化 + 路由

```bash
dws event consume \
  --route '^im\.=dir:./events/im/' \
  --route '^approval\.=dir:./events/approval/' \
  --output-dir ./events/other/
```

### 前台模式（systemd / k8s）

```bash
# Unit file 直接调用，不 fork daemon
ExecStart=/usr/local/bin/dws event consume --foreground --quiet
```

### 一次性运行（CI / 测试）

```bash
DWS_CLIENT_ID=$CI_DING_ID DWS_CLIENT_SECRET=$CI_DING_SECRET \
  dws event consume --max-events 1 --duration 30s --dry-run
```

## 错误消息和恢复指引

- `--force is only meaningful with --foreground (...) To restart the bus: dws event stop && dws event consume`
- `--format json requires --max-events or --duration (...) Use --format ndjson for unbounded streams.`
- `app config missing: run \`dws config init\` or set DWS_CLIENT_ID/DWS_CLIENT_SECRET env vars`
- `ClientSecret resolution failed (keychain unavailable?); try DWS_CLIENT_ID/DWS_CLIENT_SECRET env vars`
- `WARN: only one of DWS_CLIENT_ID/DWS_CLIENT_SECRET is set; env fallback disabled, using keychain/app config`（半套 env 警告）

## See Also

- [`dws event status`](runbook.md#status)
- [`dws event list`](runbook.md#list)
- [`dws event stop`](runbook.md#stop)
