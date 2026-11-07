# VoIP 个人通话邀请事件

先读事件产品入口 [SKILL.md](../SKILL.md) 的命令规则、生命周期和失败处理。本参考覆盖当前公开的 VoIP 个人事件 `user_voip_call_receive_invite`。

当前用户收到语音通话邀请时，使用 `dws event consume` 长连接实时监听，不轮询通话记录。

## Event catalog

| 事件码 | 订阅规则 | 接收语义 | 必填参数 |
|---|---|---|---|
| `user_voip_call_receive_invite` | `all` | 当前用户作为被叫收到语音通话邀请；服务端已将通话切换为会议模式 | 无 |

CLI 使用当前用户 OAuth 身份创建 `ruleType=all`、`filterRule={}` 的订阅。不要添加 `--user`、`--open-dingtalk-id`、`--group`、`--query` 或 `--filter-json`。

## Commands

查看目录和稳定扁平 Schema：

```bash
dws event list --category voip
dws event schema user_voip_call_receive_invite --flatten
```

开始监听：

```bash
dws event consume user_voip_call_receive_invite --flatten -f ndjson
```

测试时先由被叫用户登录并启动监听，等待 `[event] ready`；再由另一用户呼叫该被叫用户。需要有界取样时可加 `--max-events 1`。

## Output contract

`--flatten` 把服务端 camelCase payload 映射为稳定的 snake_case 顶层字段：

```json
{
  "type": "user_voip_call_receive_invite",
  "event_id": "...",
  "timestamp": 0,
  "subscribe_id": "...",
  "biz_id": "VOIP_<roomId>_<calleeUid>",
  "corp_id": "ding...",
  "org_id": 0,
  "target_uid": 0,
  "call_id": "...",
  "caller_uid": "0147333457361236773",
  "caller_corp_id": "ding...",
  "callee_uid": "digital-3559506650",
  "callee_corp_id": "ding...",
  "call_type": "conference",
  "room_id": "...",
  "create_time": 0,
  "event_time": 0
}
```

- `event_id` 是 transport 事件 ID；`biz_id` 是业务事件唯一 ID，同一事件重试时保持不变，业务去重优先使用 `biz_id`。
- `target_uid` 是订阅并接收邀请的用户；正常情况下与 `callee_uid` 指向同一被叫用户。
- `caller_uid` 与 `callee_uid` 按服务端协议以字符串输出，必须保留前导 `0`、连字符等原始内容，不要转换为数字。滚动发布期间 DWS 仍兼容旧的 Long payload，并统一投影为字符串。
- `--flatten` 默认不输出敏感入会码 `room_code`，避免终端记录、`tee` 文件或 Agent 上下文意外泄露。
- `create_time` 与 `event_time` 都是毫秒时间戳；前者是通话邀请创建时间，后者是业务事件时间。
- payload 缺失、body 缺失、`bizid` 为空或无法解析时，consume 会在 stderr 输出 warning，并只把不含业务 payload 的基础事件字段写到 stdout，避免敏感值从异常回退路径泄露。
- 不传 `--flatten` 时仍保持 transport envelope，但 DWS 会从 `.data` 中移除敏感 `roomCode`。只有服务端联调确需检查原始 payload 时，才显式添加 `--debug-raw-events`；不要持久化或传播入会码。
- VoIP 事件的 `-f raw` 同样必须和 `--debug-raw-events` 一起使用，避免绕过默认脱敏。

## Boundary

DWS CLI 负责声明 EventKey、创建个人订阅、消费 Stream、输出稳定字段和管理生命周期。VoIP 服务端负责识别被叫用户、切换会议模式并投递真实事件；如果订阅创建成功但呼叫后没有事件，应携带 trace 与服务端团队核对 Provider、事件路由和 MQ 投递，不通过反复创建订阅绕过重试保护。
