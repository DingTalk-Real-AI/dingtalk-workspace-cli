---
name: dingtalk-aisearch
description: AI 搜问 - 搜人首选入口（按姓名/部门/职位/职责/上下级/手机号/工号维度）。Use when 用户说 找同事/找人/谁负责XX/XX的负责人是谁/查上级/查下级/团队成员/XX工号是谁/XX手机号。Distinct from dingtalk-contact(精确按 userId 查详情)。命令前缀：dws aisearch。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 AI 搜问（搜人）Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[aisearch.md](references/aisearch.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "找张三 / 张三是谁" | `dws aisearch person --keyword "张三" --dimension name` |
| "谁负责 XX / XX 负责人是谁" | `dws aisearch person --keyword "<XX>" --dimension duty` |
| "张三的上级 / 下级" | `dws aisearch person --keyword "张三" --dimension supervisor`（或 `subordinate`） |
| "X 部门有哪些人" | `dws aisearch person --keyword "<部门>" --dimension department` |
| "工号 12345 是谁 / 138xxxx 手机号是谁" | `dws aisearch person --keyword "<工号>" --dimension jobNumber` / `dws aisearch person --keyword "<手机号>" --dimension phone` |

## 评测高频硬约束

- 搜索目标必须完整保真：姓名、工号、手机号、部门名按用户原文完整传入 `--keyword`，严禁自行截断、拆字、改昵称或扩展同音字。
- 首次未命中时最多换维度重试一次（如 name → department/jobNumber/phone），仍必须保留完整目标值；不要用半截姓名扩大搜索。
- 找到候选后，如用户要邮箱、部门、职位、主管等详情，必须切到 `dingtalk-contact` 执行 `contact user get --ids <userId> --format json` 补全。
- 多候选且无法唯一判断时输出候选并询问；不要默认取第一个，也不要编造未返回的人员信息。
- 所有 `dws aisearch` 命令加 `--format json`。

## 跨产品协作

- 拿到 userId 后查详情 / 部门 → 切到 `dingtalk-contact`
- 拿到 userId 发消息 → 切到 `dingtalk-chat`
- 拿到 userId 发 DING → 切到 `dingtalk-ding`
