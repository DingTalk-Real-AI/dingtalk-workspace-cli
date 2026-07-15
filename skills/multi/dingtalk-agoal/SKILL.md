---
name: dingtalk-agoal
description: 钉钉 Agoal 目标管理。Use when 用户说 目标管理/Agoal/战略解码/经营合约/计分卡/OKR/目标模板/周月报提交统计/跟催。Distinct from dingtalk-todo(待办任务)、dingtalk-report(日志)。命令前缀：dws agoal。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 Agoal Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。22 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。

> 命令参考：[agoal.md](references/agoal.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查战略解码 / OGSM" | `dws agoal strategy list --scope-type <DEPT|PERSONAL> --scope-id <ID>` |
| "看战略解码详情" | `dws agoal strategy detail --profile-id <PROFILE_ID>` |
| "更新战略解码" | 先 detail，基于完整数据修改后 `dws agoal strategy update ...` |
| "查经营合约 / KPI合约" | `dws agoal contract list/detail/fields` |
| "更新经营合约" | 先 detail，基于完整 dimensions 修改后 `dws agoal contract update ...` |
| "查计分卡" | `dws agoal scorecard detail --selected-time <ISO> --dept-id <DEPT_ID>` |
| "我的目标 / 个人目标" | `dws agoal user rules` → `dws agoal user objectives` |
| "周月报提交统计 / 跟催 / 迟交 / 未提交" | `dws agoal report list-statistics` → `dws agoal report submit-detail` |
| "目标模板" | `dws agoal obj-template list/create-or-update` |

## 写操作硬约束

- `strategy update` / `contract update` / `scorecard update` / `obj-template create-or-update` 都是覆盖式写入。
- 必须先读取现有详情或模板列表，在原数据基础上修改；禁止只传局部字段覆盖。
- 执行前必须向用户展示目标对象、修改字段和最终命令参数摘要，并等待确认。
