# VoIP 通话（个人语音视频通话）命令参考

命令前缀统一为 `dws voip`，属于 dingtalk-misc 长尾能力，不在 `dws --help` 展示；仍可直接执行 `dws voip --help` 和各叶子命令。所有调用追加 `--format json`。命令树已发布到 Agent Schema；参数、约束或确认语义不确定时，优先读取精确 compact leaf Schema（`dws schema "voip minutes auto-start" --compact`），仅在 Schema 不可用或与 Cobra 实际行为发生漂移时读取同一叶子的 `--help`。不要直接调用 MCP、HTTP API 或自行猜测工具名。

## 能力范围与前置条件

覆盖个人 VoIP 通话（1v1 音视频通话）设置：通话纪要自动开启开关。后端工具 `set_voip_minutes_auto_start` 由钉钉视频会议云端 MCP 服务承载。

前置条件：当前登录用户身份（目标用户由服务端按登录态确定，调用方不传 uid）。

Use when：用户要开启或关闭自己的 VoIP 通话纪要自动开启；配合 `dws event consume user_voip_call_receive_invite` 做通话链路验证时提前设置纪要自动开启。

Avoid when：

- 查询或导出听记内容（原文/摘要/关键词）走 `dws minutes`。
- 监听 VoIP 通话邀请事件走 `dws event`（参考 [dingtalk-event 的 VoIP 事件文档](../../dingtalk-event/references/event-voip.md)）。
- 视频会议预约/邀请入会/会中控制不在本能力范围（会中控制由会议控制插件提供）。

## 意图路由

| 用户意图 | 命令 |
|---|---|
| 开启个人 VoIP 通话纪要自动开启 | `dws voip minutes auto-start --enable` |
| 关闭个人 VoIP 通话纪要自动开启 | `dws voip minutes auto-start --enable=false` |

## 执行规则

1. `--enable` 必填且为布尔：开启传 `--enable`，关闭必须显式传 `--enable=false`（只写 flag 名等价 true）。
2. 本命令是写操作，`confirmation=user_required`：执行前先向用户确认目标状态（开启/关闭），确认后才加 `--yes`；先 `--dry-run` 预览请求。
3. 目标用户由服务端按当前登录身份注入（uid 侧服务端自动换算），不要询问或猜测 uid，也拒绝替他人设置的请求。
4. 返回 `success/errorCode/errorMsg`；`success=false` 时把 errorCode/errorMsg 原样呈现，不要自动重试（写操作幂等但重试无意义）。
5. 设置成功后如需行为验证，引导用户发起一通 VoIP 通话并观察纪要自动开启，或走听记产品查询结果，不要循环回读本命令（无查询工具）。

## 标准流程

```bash
# 1. 预览请求（不执行）
dws voip minutes auto-start --enable --dry-run --format json

# 2. 确认后执行
dws voip minutes auto-start --enable --yes --format json
```
