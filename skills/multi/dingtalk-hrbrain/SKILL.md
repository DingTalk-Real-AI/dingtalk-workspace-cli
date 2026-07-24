---
name: dingtalk-hrbrain
description: 钉钉组织大脑 Hrbrain。Use when 用户说 人才池/储备干部池/员工档案/职业历程/绩效记录/员工标签/组织大脑/人才搜索。Distinct from dingtalk-contact(通讯录/组织架构)、dingtalk-attendance(考勤)。命令前缀：dws hrbrain。
cli_version: ">=1.0.54-0"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 Hrbrain（组织大脑）Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。

> 命令参考：[hrbrain.md](references/hrbrain.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "人才池列表 / 储备干部池" | `dws hrbrain talent-pool list` |
| "人才池详情" | `dws hrbrain talent-pool detail --pool-code <POOL_CODE>` |
| "人才池里有哪些人" | `dws hrbrain talent-pool employees --pool-code <POOL_CODE>` |
| "员工档案元数据 / 档案结构" | `dws hrbrain profile metadata --work-no <WORK_NO>` |
| "查员工档案数据" | 先 `profile metadata` 确认字段编码，再 `dws hrbrain profile query --work-no <WORK_NO> --data-queries '[...]'` |
| "员工标签" | `dws hrbrain profile labels --staff-ids <WORK_NO1,WORK_NO2>` |
| "职业历程 / 内部履历" | `dws hrbrain profile career --work-no <WORK_NO>` |
| "绩效记录" | `dws hrbrain profile performance --work-no <WORK_NO>` |
| "搜人 / 按条件找人（简单）" | `dws hrbrain search employees --keyword <关键词>` |
| "搜人（复杂组合条件）" | 先 `dws hrbrain search fields` 获取字段，再 `dws hrbrain search employees-structured --origin-json '{...}' --fields '[...]'` |

## 权限与约束

- `talent-pool list` 需要账号单独开通"人才池查看权限"，返回 `errorCode=2002` 时提示用户联系管理员开通，不要重试或换 profile。
- `--data-queries`、`--fields`、`--origin-json` 必须是合法 JSON 字符串；`--staff-ids`、`--labels`、`--order-by` 是逗号分隔字符串，不是 JSON。
