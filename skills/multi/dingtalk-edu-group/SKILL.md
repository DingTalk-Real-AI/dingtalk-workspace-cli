---
name: dingtalk-edu-group
description: 钉钉家校群（师生群）。Use when 用户说 班级群/师生群/家长群/查师生群成员/创建班级群/解散班级群。仅教育场景。Distinct from dingtalk-chat(企业群)。命令前缀：dws edu-group。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 家校群 Skill（教育版）

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[edu-group.md](references/edu-group.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查师生群信息 / 是否已建" | `dws edu-group student-group info --dept-id <id>` / `exists --dept-id <id>` |
| "师生群成员" | `dws edu-group student-group members --dept-id <id>` |
| "建班级师生群" | `dws edu-group student-group create --dept-id <id>` |
| "解散班级师生群 ⚠️" | `dws edu-group student-group disband --dept-id <id>`（需用户确认 `--yes`） |

## 危险操作

`student-group disband` 不可逆，必须先向用户确认再加 `--yes`。

## 跨产品协作

- 班级列表 → 切到 `dingtalk-edu-contact`（school class-list）
- 企业群 → 切到 `dingtalk-chat`
