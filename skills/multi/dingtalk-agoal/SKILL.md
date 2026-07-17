---
name: dingtalk-agoal
description: 钉钉 Agoal 目标管理与经营目标跟进。Use when 用户说目标管理/战略解码/经营合约/计分卡/用户目标/OKR拆解/更新目标/查看经营合约/目标进展/目标负责人/周月报统计/周报提交情况/跟催/迟交/未提交。Distinct from dingtalk-todo(个人任务待办)、dingtalk-report(日报周报内容)、dingtalk-oa(审批流)。命令前缀：dws agoal。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉目标管理 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[agoal.md](references/agoal.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查战略解码 / 战略列表" | `dws agoal strategy list` |
| "查战略详情" | `dws agoal strategy detail --profile-id <id>` |
| "更新战略解码" | 先 `strategy detail` 获取完整内容，再 `strategy update` 覆盖更新 |
| "查经营合约" | `dws agoal contract list` / `contract detail` |
| "更新经营合约" | 先 `contract detail` 获取完整内容，再 `contract update` 覆盖更新 |
| "查计分卡" | `dws agoal scorecard detail` / `scorecard entity-detail` |
| "更新计分卡" | 先查详情，再按 [agoal.md](references/agoal.md) 覆盖更新 |
| "查周月报统计/提交情况/跟催" | `dws agoal report list-statistics` / `report submit-detail` |

## 硬约束

- `strategy update`、`contract update`、`scorecard update` 都是覆盖式更新，必须先查询详情，在返回数据基础上修改后再提交。
- 所有命令带 `--format json`；涉及写操作时回查确认。
