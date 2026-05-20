---
name: dingtalk-edu-contact
description: 钉钉家校通讯录（教育版）。Use when 用户说 学校/学校组织架构/学段/班级列表/学校统计/家长身份/学生身份。仅教育场景。Distinct from dingtalk-contact(企业通讯录)。命令前缀：dws edu-contact。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 家校通讯录 Skill（教育版）

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[edu-contact.md](references/edu-contact.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "我在学校的身份" | `dws edu-contact school roles` |
| "学校组织架构" | `dws edu-contact school structure` |
| "学校学段 / 类型" | `dws edu-contact school periods` / `school type` |
| "学校所有班级" | `dws edu-contact school class-list` |
| "学校统计" | `dws edu-contact school stats [--statistics-type <t>]` |

## 跨产品协作

- 企业通讯录场景 → 切到 `dingtalk-contact`
