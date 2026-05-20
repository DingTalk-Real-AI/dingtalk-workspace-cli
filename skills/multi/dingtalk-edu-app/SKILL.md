---
name: dingtalk-edu-app
description: 钉钉家校应用（教育版）。Use when 用户说 家校/家长/学生消息/师生任务/班级消息摘要。仅教育场景。Distinct from dingtalk-chat(企业群聊)、dingtalk-edu-group(班级师生群)。命令前缀：dws edu-app。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 家校应用 Skill（教育版）

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[edu-app.md](references/edu-app.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查班级消息摘要" | `dws edu-app message summary-list --class-id <id> --cid <id> --target-role guardian\|student --status 0\|1` |
| "查家校任务" | 见 [edu-app.md](references/edu-app.md) `task` 章节 |

## 跨产品协作

- 师生群本身 → 切到 `dingtalk-edu-group`
- 家校通讯录 → 切到 `dingtalk-edu-contact`
