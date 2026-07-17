---
name: dingtalk-skill
description: DWS 技能市场搜索、下载、安装与内置技能部署。Use when 用户说搜索技能/找技能/安装技能/技能市场/企业技能库/安装 DWS multi 或 mono skill。注意：这是元 skill，只管理 dws 平台上的技能资源。Distinct from dws-shared(钉钉产品路由入口)、其他 dingtalk-* 产品 skill(执行具体业务能力)、本地 Codex skill 开发。命令前缀：dws skill。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# DWS 技能管理 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件

> **`use_skill(dws-shared)`** — 认证、全局参数、错误码与安全规则。执行网络请求前先读；本地 `skill setup` 可直接使用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 详细命令：[skill.md](references/skill.md)。

## 意图表

| 用户说 | 命令 |
|---|---|
| "搜索技能 / 找技能" | `dws skill search --query "<关键词>" [--source DingtalkMarket\|OrgInternal]` |
| "下载技能包" | `dws skill get --skill-id <skillId>` |
| "安装市场技能" | `dws skill install <skillId> <target>` |
| "安装 DWS mono/multi skills" | `dws skill setup --mode <mono\|multi> --target <target> --yes` |

## 约束

- `skillId` 必须来自 `skill search` 返回，不能用名称代替。
- `skill install` 的 `skillId` 与 `target` 是位置参数，不是 `--skill-id` flag。
- `skill setup --mode multi` 可用 `--skill/-s` 只装指定产品，或用 `--exclude/-x` 排除产品，两者不能同时使用。
- 搜索结果中的 `securityStatus` 需要如实展示；状态异常时不要把安装描述为已通过安全检测。
- 开源 CLI 不提供技能发布/上传命令；发布需求应转到对应的技能市场发布流程。

## 兼容提示

- `dws skill find` → `dws skill search --query <关键词>`
- `dws skill add` → `dws skill install <skillId> <target>`
