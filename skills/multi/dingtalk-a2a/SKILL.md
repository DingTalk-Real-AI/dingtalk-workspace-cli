---
name: dingtalk-a2a
description: A2A（Agent-to-Agent）协议客户端。Use when 用户说 A2A/Agent协作/Agent发现/JSON-RPC over HTTP/SSE 流式 / Agent 间通信。Distinct from dingtalk-chat(钉钉群聊会话)。命令前缀：dws a2a。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# A2A Agent 协作 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[a2a.md](references/a2a.md)。协议：A2A v1.0（JSON-RPC over HTTP，流式 SSE）。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "列出所有 Agent" | `dws a2a agents list` |
| "查 Agent 详情" | `dws a2a agents info --id <agentId>` |
| "向 Agent 发消息（同步）" | `dws a2a send --agent <id> --message "<内容>"` |
| "向 Agent 发消息（流式 SSE）" | `dws a2a send --agent <id> --message "<内容>" --stream` |

## 环境

- 默认网关：`https://mcp-gw.dingtalk.com`
- 本地联调：`DWS_A2A_GATEWAY=http://127.0.0.1:18080`

## 跨产品协作

- 钉钉会话 / 群聊消息 → 切到 `dingtalk-chat`（不同场景）
