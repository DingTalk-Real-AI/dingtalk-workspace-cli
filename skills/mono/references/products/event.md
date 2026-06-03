# `dws event` — 事件订阅

通过 DingTalk Stream 长连接实时订阅事件，NDJSON 输出到 stdout，用于驱动事件触发的 Agent。

## 当用户说……

| 用户说 | 命令 |
|--------|------|
| "监听消息" / "实时接收消息" | `dws event consume --event-types im.message.receive_v1 --compact --quiet` |
| "审批触发时执行" | `dws event consume --event-types approval.instance.status_changed --compact --quiet` |
| "看 bus 状态" | `dws event status` |
| "列出消费者" | `dws event list --all` |
| "停止订阅" | `dws event stop` |

## 子命令

- `dws event consume [flags]` — 主命令，订阅 + 输出
- `dws event list [--all] [--all-editions]` — 列消费者
- `dws event status [--all] [--fail-on-orphan]` — bus 健康状态
- `dws event stop` — 优雅停 bus

## 凭证

bot-only，需要 ClientID + ClientSecret：
- env：`DWS_CLIENT_ID` + `DWS_CLIENT_SECRET`（成组）
- 或：`dws config init` 写入 keychain

## 输出格式

- `-f ndjson`（默认）：一行一对象
- `-f compact`：扁平化 + 解析嵌套（**Agent 强推荐**）
- `-f json/pretty`：美化 JSON（必须 `--max-events` 或 `--duration`）
- `-f raw`：SDK 原始 payload

## 关键 Flag

| Flag | 说明 |
|------|------|
| `--event-types <list>` | 限定事件类型，下推到 bus |
| `--filter <regex>` | 正则过滤事件类型 |
| `--max-events <n>` | 收 N 条后退出 |
| `--duration <duration>` | 运行时长上限 (30s/5m) |
| `--output-dir <dir>` | 每事件写一个文件 |
| `--route '<regex>=dir:<path>'` | 按正则路由事件到目录 |
| `--foreground` | 不 fork daemon（systemd/k8s 友好）|
| `--dry-run` | 仅打印配置 |

## 完整文档

详细见 multi 模式：`skills/multi/dingtalk-event/SKILL.md`
端到端冒烟手册：`skills/multi/dingtalk-event/references/runbook.md`
