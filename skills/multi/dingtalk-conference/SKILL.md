---
name: dingtalk-conference
description: 钉钉视频会议：发起/预约/邀请入会/会中控制。Use when 用户说 预约会议/开个会/发起会议/邀请入会/一键呼叫/开关麦/静音/开关摄像头/共享屏幕/录制/字幕/听记/视图/结束会议/视频会议。Distinct from dingtalk-calendar(日历日程含参与者/会议室/会议室预订)。命令前缀：dws conference。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉视频会议 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[conference.md](references/conference.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "开个会 / 发起会议"（无明确时间） | `dws conference start [--title "<主题>"]` |
| "预约会议 / 约个会"（有明确时间） | `dws conference meeting reserve --title "<主题>" --start <ISO> --end <ISO>` |
| "邀请张三入会" | `dws contact user search --query "张三"` → `dws conference get-id` → `dws conference member invite --conference-id <id> --nicks "<昵称>" --open-dingtalk-ids "<openDingTalkId>"` |
| "一键呼叫 / 打开邀请面板" | `dws conference invite all` / `dws conference invite panel` |
| "静音 / 取消静音 / 全员静音" | `dws conference mic mute` / `unmute` / `mute-all` |
| "开摄像头 / 关摄像头" | `dws conference camera open` / `close` |
| "共享屏幕 / 停止共享" | `dws conference share start` / `stop` |
| "开始录制 / 停止录制 / 申请录制" | `dws conference record start` / `stop` / `request` |
| "开字幕 / 开听记 / 切视图" | `dws conference subtitle open` / `transcript open` / `view grid` |
| "结束会议 / 离开会议" | `dws conference end` / `leave`（敏感操作需先确认） |

## 关键区分

`conference` 负责视频会议本身（即时发起、预约、会中控制、邀请入会），不自动创建日历日程、不订会议室。完整日程（含参与者 / 会议室）→ 切到 `dingtalk-calendar`（`schedule-meeting` recipe）。

## 安全约束

- 结束会议、离开会议、停止录制、关闭听记等敏感操作必须先向用户确认。
- 需要钉钉桌面端运行的会中控制命令，失败时按错误信息汇报，不要改用浏览器或 HTTP API。
