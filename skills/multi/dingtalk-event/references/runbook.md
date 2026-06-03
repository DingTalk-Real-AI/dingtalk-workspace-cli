# 端到端冒烟手册 (Runbook)

从零到看见第一条事件的全流程。

## 1. 钉钉开发者后台配置

1. 访问 https://open.dingtalk.com/app → 创建应用（或选择已有应用）
2. **应用能力** → 添加 **机器人**（如果做 IM 监听）
3. **权限管理** → 申请：
   - `im:message`（接收消息）
   - `im:message.group_at_msg`（群里被 @）
   - `im:message.p2p_msg`（私聊消息）
   - `im:message:send_as_bot`（发送消息，可选）
4. **事件订阅** → 选 "使用 **Stream 模式** 接收事件"（非 HTTP 推送）→ 勾选要订阅的事件类型，例如 `im.message.receive_v1`
5. 在应用基本信息页拿 **AppKey** (= ClientID) 和 **AppSecret** (= ClientSecret)

## 2. 本机配置凭证

### 方式 A：keychain（macOS/Linux desktop 推荐）

```bash
dws config init
# 按提示输入 AppKey 和 AppSecret，存入 OS keychain
```

### 方式 B：env var（CI/容器/SSH 推荐）

```bash
export DWS_CLIENT_ID=ding_abcdef123
export DWS_CLIENT_SECRET=YOUR_APP_SECRET
```

**注意**：两个 env var 必须**同时设置**才生效；只设一个会触发 stderr WARN 并回退到 keychain。

## 3. 第一条消息

终端 1：

```bash
dws event consume --event-types im.message.receive_v1 --compact --quiet
```

终端 2：用钉钉客户端给应用机器人发一条消息 "hello"

终端 1 应该看到一行：

```json
{"type":"im.message.receive_v1","event_id":"ev_xxx","message_id":"om_xxx","chat_id":"oc_xxx","content":"hello","sender_id":"ou_xxx","timestamp":1700000000123}
```

按 `Ctrl+C` 退出。

## 4. 查看 bus 状态

```bash
dws event status
```

输出（运行中）：

```
ClientID: ding_abcdef123
  Edition  : open
  Workdir  : /Users/me/.dws/events/open/e1092ee444e129c8...
  Bus      : running  pid=12345  uptime=15m20s
  Source   : state=connected  state_source=inferred  reconnects=0
  Consumers: 1 active
  Per-event-type counters (since bus start):
    im.message.receive_v1  received=3  dropped=0
  Consumers:
    PID    EVENT KEYS                RECEIVED  DROPPED
    12350  im.message.receive_v1     3         0
```

orphan 状态（bus 进程被 `kill -9` 后）：

```
Bus      : orphan  (last_pid=12345 not alive)
Action   : run `dws event consume` to force-restart, or rm -rf the workdir
```

## 5. 列出所有 consumer

```bash
# 当前 ClientID
dws event list

# 当前 edition 下所有 ClientID
dws event list --all

# 跨 edition（调试用）
dws event list --all-editions
```

## 6. 优雅停止 bus

```bash
dws event stop
# bus stopped
```

工作机制：读 `bus.lock` 内 PID → SIGTERM → 等进程退出（默认 5s timeout）。

## 7. 健康检查（CI/监控）

```bash
# orphan 时退出码 5（适合 cron / k8s liveness）
dws event status --fail-on-orphan
echo $?  # 0 if healthy, 5 if orphan present

# JSON 输出供脚本消费
dws event status -f json | jq '.[0].entry.state'
# "running" | "orphan" | "not_running"
```

## 8. systemd 部署

`/etc/systemd/system/dws-event.service`：

```ini
[Unit]
Description=DingTalk Event Consumer
After=network.target

[Service]
Type=simple
User=dws
Environment=DWS_CLIENT_ID=ding_abcdef123
Environment=DWS_CLIENT_SECRET=YOUR_SECRET
Environment=DWS_EVENT_BUS_IDLE_TIMEOUT=24h
ExecStart=/usr/local/bin/dws event consume \
  --foreground \
  --event-types im.message.receive_v1 \
  --compact --quiet \
  --output-dir /var/log/dws-events/
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now dws-event
sudo journalctl -u dws-event -f
```

## 9. 故障排查

| 现象 | 排查步骤 |
|------|---------|
| 跑 `consume` 立刻报 `app config missing` | 检查 `~/.dws/app.json` 是否存在；或设 env var |
| `consume` 跑着但收不到事件 | (1) 开放平台后台确认事件订阅模式是 **Stream 长连接** 不是 HTTP；(2) 确认对应事件类型已勾选；(3) `dws event status` 看 source state |
| `dws event status` 显示 orphan | bus 进程死了但 lock 没清。下次 `consume` 会自动清理；或 `dws event stop` 强制 |
| stderr 频繁 `WARN: event type backpressure` | drop rate 超阈值。consumer 处理太慢。增大 `DWS_EVENT_CONSUMER_BUFFER` 或加快消费 |
| `WARN: only one of DWS_CLIENT_ID/DWS_CLIENT_SECRET is set` | 两个 env var 必须同时设置；要么都设，要么都 unset |
| `dws event status --all-editions` 显示意外的 wukong/* 条目 | 历史遗留（之前用过 wukong overlay）。安全：`rm -rf <ConfigDir>/events/wukong/` |

## 10. 数据位置

| 路径 | 用途 |
|------|------|
| `~/.dws/events/<edition>/<clientIDHash>/bus.lock` | 单文件锁 + PID |
| `~/.dws/events/<edition>/<clientIDHash>/bus.meta` | 元数据（原 ClientID/edition/started_at）|
| `~/.dws/events/<edition>/<clientIDHash>/bus.sock` | Unix Socket（Linux/macOS）|
| `~/.dws/events/<edition>/<clientIDHash>/bus.log` | daemon slog 日志（含 drop WARN）|

Windows 用 `\\.\pipe\dws-event-<edition>-<clientIDHash>` 代替 sock。
